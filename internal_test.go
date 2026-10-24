package traefikgeoip2

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCityDB = "testdata/GeoIP2-City-Test.mmdb"

func newTestInstance(t *testing.T, cfg *Config) (*TraefikGeoIP2, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	h, err := New(context.Background(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), cfg, t.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mw := h.(*TraefikGeoIP2) //nolint:errcheck // New returns this type
	errOut, out := &bytes.Buffer{}, &bytes.Buffer{}
	mw.log.errOut, mw.log.out = errOut, out
	return mw, errOut, out
}

func request(t *testing.T, h http.Handler, remoteAddr string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = remoteAddr
	h.ServeHTTP(httptest.NewRecorder(), req)
	return req
}

// tempDB copies the City test database to a temporary directory under a name that keeps its kind.
func tempDB(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(testCityDB)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "GeoIP2-City-Test.mmdb")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureRegistryLog(db *geoDB) (*bytes.Buffer, *bytes.Buffer) {
	errOut, out := &bytes.Buffer{}, &bytes.Buffer{}
	db.log.errOut, db.log.out = errOut, out
	return errOut, out
}

func TestRegistryIsSharedAcrossNewCalls(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = testCityDB
	first, _, _ := newTestInstance(t, cfg)
	second, _, _ := newTestInstance(t, cfg)
	if first.db != second.db {
		t.Fatal("instances for the same path must share one registry entry")
	}
	if first.db.current() != second.db.current() || first.db.current().lookup == nil {
		t.Fatal("instances for the same path must share one loaded state")
	}
	if first.log == second.log {
		t.Fatal("instances must not share a logger")
	}
}

func TestRepeatedNewDoesNotReload(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = tempDB(t)
	mw, _, _ := newTestInstance(t, cfg)
	state := mw.db.current()
	for i := 0; i < 50; i++ {
		again, _, _ := newTestInstance(t, cfg)
		if again.db.current() != state {
			t.Fatalf("New #%d reopened an unchanged database", i)
		}
	}
}

func TestReloadWhenFileChanges(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = tempDB(t)
	mw, _, _ := newTestInstance(t, cfg)
	errOut, out := captureRegistryLog(mw.db)
	old := mw.db.current()

	request(t, mw, "81.2.69.160:1")
	if old.cache.len() != 1 {
		t.Fatalf("cache len = %d, want 1", old.cache.len())
	}

	// Same content, new mtime: the file is reopened and the cache starts empty.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(cfg.DBPath, later, later); err != nil {
		t.Fatal(err)
	}
	saved := reloadGracePeriod
	reloadGracePeriod = 10 * time.Millisecond
	defer func() { reloadGracePeriod = saved }()

	mw.db.check(time.Now())
	cur := mw.db.current()
	if cur == old || cur.lookup == nil {
		t.Fatal("changed file must be reopened")
	}
	if cur.cache.len() != 0 {
		t.Fatalf("new state must start with an empty cache, got %d", cur.cache.len())
	}
	if !strings.Contains(out.String(), "database loaded") || errOut.Len() != 0 {
		t.Fatalf("unexpected log output: out=%q err=%q", out.String(), errOut.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for old.cache.len() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if old.cache.len() != 0 {
		t.Fatal("superseded cache must be released after the grace period")
	}
	req := request(t, mw, "81.2.69.160:1")
	if req.Header.Get(CountryHeader) != "GB" {
		t.Fatalf("lookup after reload = %q", req.Header.Get(CountryHeader))
	}
}

func TestCorruptReplacementKeepsOldDatabaseAndLogsOnce(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = tempDB(t)
	mw, _, _ := newTestInstance(t, cfg)
	errOut, _ := captureRegistryLog(mw.db)
	old := mw.db.current()

	if err := os.WriteFile(cfg.DBPath, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	mw.db.check(time.Now())
	mw.db.check(time.Now())
	if mw.db.current() != old {
		t.Fatal("a replacement that does not open must keep the previous database")
	}
	if n := strings.Count(errOut.String(), "level=error"); n != 1 {
		t.Fatalf("want exactly one error line for one broken file version, got %d: %s", n, errOut.String())
	}
	req := request(t, mw, "81.2.69.160:1")
	if req.Header.Get(CountryHeader) != "GB" {
		t.Fatal("requests must keep using the previous database")
	}
}

func TestMissingDatabaseAppearsLater(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "GeoIP2-Country-Test.mmdb")
	mw, _, _ := newTestInstance(t, cfg)
	errOut, _ := captureRegistryLog(mw.db)
	if mw.db.current().lookup != nil {
		t.Fatal("missing file must leave the database unavailable")
	}
	req := request(t, mw, "81.2.69.142:1")
	if req.Header.Get(CountryHeader) != Unknown {
		t.Fatal("unavailable database must yield Unknown")
	}
	// Repeated checks of the same missing file do not log again.
	mw.db.check(time.Now())
	if errOut.Len() != 0 {
		t.Fatalf("missing file must be reported once, at load: %s", errOut.String())
	}

	data, err := os.ReadFile("testdata/GeoIP2-Country-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.DBPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	mw.db.check(time.Now())
	req = request(t, mw, "81.2.69.142:1")
	if req.Header.Get(CountryHeader) != "GB" {
		t.Fatalf("database must be picked up once the file exists, got %q", req.Header.Get(CountryHeader))
	}
}

func TestMaybeCheckIsThrottled(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = tempDB(t)
	mw, _, _ := newTestInstance(t, cfg)
	now := time.Now()
	if mw.db.maybeCheck(now) {
		t.Fatal("New just checked the file; a request must not check again")
	}
	if !mw.db.maybeCheck(now.Add(reloadCheckInterval + time.Second)) {
		t.Fatal("a request after the interval must trigger a check")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mw.db.checkMu.Lock()
		busy := mw.db.checking
		mw.db.checkMu.Unlock()
		if !busy {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if mw.db.maybeCheck(now.Add(reloadCheckInterval + 2*time.Second)) {
		t.Fatal("a second request within the interval must not check")
	}
}

func TestNewRejectsBadConfiguration(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"log level": func(c *Config) { c.LogLevel = "VERBOSE" },
		"ttl":       func(c *Config) { c.CacheTTL = "soon" },
		"ttl zero":  func(c *Config) { c.CacheTTL = "0s" },
		"size":      func(c *Config) { c.CacheSize = -1 },
		"path":      func(c *Config) { c.DBPath = "" },
	} {
		cfg := CreateConfig()
		cfg.DBPath = testCityDB
		mutate(cfg)
		if _, err := New(context.Background(), nil, cfg, name); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := New(context.Background(), nil, nil, "nil"); err == nil {
		t.Error("nil config: want an error")
	}
}

func TestUnknownKindIsNonFatalAndLoggedOnce(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = "Makefile"
	mw, _, _ := newTestInstance(t, cfg)
	errOut, _ := captureRegistryLog(mw.db)
	mw.db.check(time.Now())
	again, _, _ := newTestInstance(t, cfg)
	if again.db != mw.db || errOut.Len() != 0 {
		t.Fatalf("an unusable file is reported once, at first load, not on every New: %s", errOut.String())
	}
	req := request(t, mw, "81.2.69.160:1")
	if req.Header.Get(CountryHeader) != Unknown {
		t.Fatal("unusable database must yield Unknown")
	}
}

func TestLogLevelIsPerInstance(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = testCityDB
	cfg.LogLevel = "DEBUG"
	verbose, _, verboseOut := newTestInstance(t, cfg)
	cfg.LogLevel = "ERROR"
	quiet, quietErr, quietOut := newTestInstance(t, cfg)

	request(t, verbose, "81.2.69.160:1")
	request(t, quiet, "81.2.69.160:1")
	if !strings.Contains(verboseOut.String(), "level=debug") {
		t.Fatalf("DEBUG instance must log requests, got %q", verboseOut.String())
	}
	if quietOut.Len() != 0 || quietErr.Len() != 0 {
		t.Fatalf("ERROR instance must stay silent, got out=%q err=%q", quietOut.String(), quietErr.String())
	}
}

func TestCachedResultsAreNeverMutated(t *testing.T) {
	cfg := CreateConfig()
	cfg.DBPath = testCityDB
	mw, _, _ := newTestInstance(t, cfg)
	// Exercise a fresh address from many goroutines: the first lookup publishes
	// a complete record, nobody writes to it afterwards (checked by -race).
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, ip := range []string{"81.2.69.160:1", "214.1.1.1:1", "10.9.9.9:1", "2001:218::1", "216.160.83.56:1"} {
				request(t, mw, ip)
			}
		}()
	}
	wg.Wait()
	if unknownResult.country != Unknown || unknownResult.asn != Unknown {
		t.Fatal("unknownResult was modified")
	}
}

func TestCacheBoundAndTTL(t *testing.T) {
	c := newResultCache(32, time.Minute) // 2 per shard
	now := time.Now()
	for i := 0; i < 1000; i++ {
		c.set(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}).String(), unknownResult, now)
	}
	if n := c.len(); n > 32 {
		t.Fatalf("cache holds %d entries, bound is 32", n)
	}
	c.set("k", unknownResult, now)
	if _, hit := c.get("k", now.Add(59*time.Second)); !hit {
		t.Fatal("entry must be served before the TTL")
	}
	if _, hit := c.get("k", now.Add(61*time.Second)); hit {
		t.Fatal("entry must expire after the TTL")
	}
	if newResultCache(0, time.Minute) != nil {
		t.Fatal("size 0 must disable the cache")
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newResultCache(cacheShardCount, time.Minute) // 1 per shard
	now := time.Now()
	// Find two keys in the same shard.
	first := "a"
	var second string
	for i := 0; second == "" && i < 10000; i++ {
		k := "k" + letters(i)
		if c.shard(k) == c.shard(first) {
			second = k
		}
	}
	c.set(first, unknownResult, now)
	c.set(second, unknownResult, now)
	if _, hit := c.get(first, now); hit {
		t.Fatal("oldest entry of a full shard must be evicted")
	}
	if _, hit := c.get(second, now); !hit {
		t.Fatal("newest entry must stay")
	}
}

func letters(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))
}

func TestParseClientAddr(t *testing.T) {
	for raw, want := range map[string]string{
		"1.2.3.4":                "1.2.3.4",
		"1.2.3.4:80":             "1.2.3.4",
		" 1.2.3.4 ":              "1.2.3.4",
		"1.2.3.4, 5.6.7.8":       "1.2.3.4",
		"2001:db8::1":            "2001:db8::1",
		"[2001:db8::1]":          "2001:db8::1",
		"[2001:db8::1]:443":      "2001:db8::1",
		"::ffff:1.2.3.4":         "1.2.3.4",
		"[::ffff:1.2.3.4]:443":   "1.2.3.4",
		"fe80::1%eth0":           "fe80::1",
		"[fe80::1%eth0]:1":       "fe80::1",
		"2001:DB8:0:0:0:0:0:1":   "2001:db8::1",
		"garbage":                "",
		"":                       "",
		",":                      "",
		"1.2.3.4:notaport":       "1.2.3.4", // lenient: a malformed port does not lose the address
		"300.1.1.1":              "",
		"2001:db8::1:443":        "2001:db8::1:443", // ambiguous without brackets: parsed as an address
		"unix:///var/run/x.sock": "",
	} {
		addr, ok := parseClientAddr(raw)
		if want == "" {
			if ok {
				t.Errorf("%q: want failure, got %v", raw, addr)
			}
			continue
		}
		if !ok || addr.String() != want {
			t.Errorf("%q: got %v/%v, want %s", raw, addr, ok, want)
		}
	}
}

func TestIsSpecial(t *testing.T) {
	for raw, want := range map[string]bool{
		"10.0.0.1": true, "172.16.0.1": true, "192.168.1.1": true, "127.0.0.1": true, "0.0.0.0": true,
		"169.254.1.1": true, "224.0.0.1": true, "::1": true, "::": true, "fe80::1": true, "fc00::1": true, "ff02::1": true,
		"8.8.8.8": false, "81.2.69.160": false, "2001:218::1": false, "100.64.0.1": false,
	} {
		if got := isSpecial(netip.MustParseAddr(raw)); got != want {
			t.Errorf("isSpecial(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestNormalizeIPHeader(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""}, {"remoteaddr", ""}, {"RemoteAddr", ""}, {" x-real-ip ", "X-Real-Ip"},
		{"X-Forwarded-For", "X-Forwarded-For"}, {"CF-Connecting-IP", "Cf-Connecting-Ip"},
	} {
		got, ok := normalizeIPHeader(tc.in)
		if !ok || got != tc.want {
			t.Errorf("normalizeIPHeader(%q) = %q/%v, want %q", tc.in, got, ok, tc.want)
		}
	}
	for _, bad := range []string{"has space", "x:y", "a\nb"} {
		if _, ok := normalizeIPHeader(bad); ok {
			t.Errorf("normalizeIPHeader(%q) must fail", bad)
		}
	}
}
