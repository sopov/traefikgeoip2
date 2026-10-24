package traefikgeoip2_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/sopov/traefikgeoip2"
)

// Databases from github.com/maxmind/MaxMind-DB test-data; see testdata/README.md
// for the pinned commit and the expected values.
const (
	cityDB    = "./testdata/GeoIP2-City-Test.mmdb"
	countryDB = "./testdata/GeoIP2-Country-Test.mmdb"
	asnDB     = "./testdata/GeoLite2-ASN-Test.mmdb"
)

func newInstance(t *testing.T, dbPath string, called *bool) http.Handler {
	t.Helper()
	cfg := mw.CreateConfig()
	cfg.DBPath = dbPath
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if called != nil {
			*called = true
		}
	})
	instance, err := mw.New(context.Background(), next, cfg, "traefik-geoip2")
	if err != nil {
		t.Fatalf("New(%s): %v", dbPath, err)
	}
	return instance
}

func serve(t *testing.T, h http.Handler, remoteAddr string, headers map[string]string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK { //nolint:bodyclose // recorder result carries no body to close
		t.Fatalf("unexpected status %d", rec.Result().StatusCode)
	}
	return req
}

func TestGeoIPConfig(t *testing.T) {
	cfg := mw.CreateConfig()
	if cfg.DBPath != mw.DefaultDBPath {
		t.Fatalf("default dbPath %q, want %q", cfg.DBPath, mw.DefaultDBPath)
	}

	cfg.DBPath = "./non-existing"
	if _, err := mw.New(context.Background(), nil, cfg, ""); err != nil {
		t.Fatalf("must not fail on missing DB: %v", err)
	}

	cfg.DBPath = "Makefile"
	if _, err := mw.New(context.Background(), nil, cfg, ""); err != nil {
		t.Fatalf("must not fail on invalid DB format: %v", err)
	}
}

func TestMissingGeoIPDB(t *testing.T) {
	called := false
	instance := newInstance(t, "./missing-City.mmdb", &called)
	req := serve(t, instance, "81.2.69.160:9999", nil)
	if !called {
		t.Fatal("next handler was not called")
	}
	assertHeader(t, req, mw.CountryHeader, mw.Unknown)
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
	assertHeader(t, req, mw.TimezoneHeader, mw.Unknown)
	assertHeader(t, req, mw.LatitudeHeader, mw.Unknown)
	assertHeader(t, req, mw.LongitudeHeader, mw.Unknown)
}

func TestCityDBFromRemoteAddr(t *testing.T) {
	called := false
	instance := newInstance(t, cityDB, &called)

	req := serve(t, instance, "81.2.69.160:9999", nil)
	if !called {
		t.Fatal("next handler was not called")
	}
	assertHeader(t, req, mw.CountryHeader, "GB")
	assertHeader(t, req, mw.RegionHeader, "ENG")
	assertHeader(t, req, mw.CityHeader, "London")
	assertHeader(t, req, mw.TimezoneHeader, "Europe/London")
	assertHeader(t, req, mw.LatitudeHeader, "51.5142")
	assertHeader(t, req, mw.LongitudeHeader, "-0.0931")
	assertHeader(t, req, mw.AnycastHeader, "false")

	// Non-ASCII city name, RemoteAddr without a port.
	req = serve(t, instance, "89.160.20.112", nil)
	assertHeader(t, req, mw.CountryHeader, "SE")
	assertHeader(t, req, mw.RegionHeader, "E")
	assertHeader(t, req, mw.CityHeader, "Linköping")

	// Unparsable RemoteAddr.
	req = serve(t, instance, "qwerty:9999", nil)
	assertHeader(t, req, mw.CountryHeader, mw.Unknown)
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
}

func TestCityDBIPv6(t *testing.T) {
	instance := newInstance(t, cityDB, nil)

	req := serve(t, instance, "[2001:218::1]:443", nil)
	assertHeader(t, req, mw.CountryHeader, "JP")
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
	assertHeader(t, req, mw.TimezoneHeader, "Asia/Tokyo")

	req = serve(t, instance, "2001:218::1", nil)
	assertHeader(t, req, mw.CountryHeader, "JP")
}

func TestCityDBAnycast(t *testing.T) {
	instance := newInstance(t, cityDB, nil)
	req := serve(t, instance, "214.1.1.1:1", nil)
	assertHeader(t, req, mw.AnycastHeader, "true")
	assertHeader(t, req, mw.CountryHeader, mw.Unknown)
}

func TestCountryDBFromRemoteAddr(t *testing.T) {
	instance := newInstance(t, countryDB, nil)

	req := serve(t, instance, "81.2.69.142:9999", nil)
	assertHeader(t, req, mw.CountryHeader, "GB")
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
	assertHeader(t, req, mw.TimezoneHeader, mw.Unknown)
	assertHeader(t, req, mw.LatitudeHeader, mw.Unknown)
	assertHeader(t, req, mw.LongitudeHeader, mw.Unknown)
	assertHeader(t, req, mw.AnycastHeader, "false")

	req = serve(t, instance, "[2001:218::1]:1", nil)
	assertHeader(t, req, mw.CountryHeader, "JP")

	req = serve(t, instance, "214.1.1.1:1", nil)
	assertHeader(t, req, mw.AnycastHeader, "true")
}

func TestCityDBFromXRealIP(t *testing.T) {
	instance := newInstance(t, cityDB, nil)
	req := serve(t, instance, "1.1.1.1:9999", map[string]string{"X-Real-Ip": "81.2.69.160"})
	assertHeader(t, req, mw.CountryHeader, "GB")
	assertHeader(t, req, mw.RegionHeader, "ENG")
	assertHeader(t, req, mw.CityHeader, "London")
}

func TestASNDBFromRemoteAddr(t *testing.T) {
	instance := newInstance(t, asnDB, nil)

	req := serve(t, instance, "1.128.0.1:9999", map[string]string{mw.CountryHeader: "DE"})
	assertHeader(t, req, mw.ASNHeader, "1221")
	assertHeader(t, req, mw.ASOrgHeader, "Telstra Pty Ltd")
	// An ASN instance owns only its two headers: a chained geo instance's values survive.
	assertHeader(t, req, mw.CountryHeader, "DE")
	if req.Header.Get(mw.RegionHeader) != "" {
		t.Fatalf("ASN instance must not set geo headers, got region %q", req.Header.Get(mw.RegionHeader))
	}

	// Number without organization.
	req = serve(t, instance, "216.160.83.56:1", nil)
	assertHeader(t, req, mw.ASNHeader, "209")
	assertHeader(t, req, mw.ASOrgHeader, mw.Unknown)

	// IPv6.
	req = serve(t, instance, "[2600:1000::1]:1", nil)
	assertHeader(t, req, mw.ASNHeader, "237")
	assertHeader(t, req, mw.ASOrgHeader, "Merit Network Inc.")

	// Not in the database.
	req = serve(t, instance, "10.0.0.1:9999", nil)
	assertHeader(t, req, mw.ASNHeader, mw.Unknown)
	assertHeader(t, req, mw.ASOrgHeader, mw.Unknown)
}

func TestMissingASNDBKeepsGeoHeaders(t *testing.T) {
	instance := newInstance(t, "./testdata/GeoLite2-ASN-missing.mmdb", nil)
	req := serve(t, instance, "1.128.0.1:9999", map[string]string{mw.CountryHeader: "DE"})
	assertHeader(t, req, mw.ASNHeader, mw.Unknown)
	assertHeader(t, req, mw.ASOrgHeader, mw.Unknown)
	assertHeader(t, req, mw.CountryHeader, "DE")
}

func assertHeader(t *testing.T, req *http.Request, key, expected string) {
	t.Helper()
	if got := req.Header.Get(key); got != expected {
		t.Fatalf("header %s = %q, want %q", key, got, expected)
	}
}
