// Package traefikgeoip2 is a Traefik middleware plugin that resolves the client
// address against MaxMind GeoIP2/GeoLite2 databases and sets request headers.
package traefikgeoip2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

// Config is the plugin configuration.
type Config struct {
	// DBPath is the path of a MaxMind .mmdb file. The file name must contain
	// City, Country or ASN, which selects the reader and the headers set.
	DBPath string `json:"dbPath,omitempty"`
	// LogLevel is ERROR (default), WARN, INFO or DEBUG.
	LogLevel string `json:"logLevel,omitempty"`
	// IPHeader is the request header the client address is read from, with
	// RemoteAddr as fallback. Default X-Real-IP; X-Forwarded-For uses the first
	// (client-most) element, which is only trustworthy when Traefik's entry point
	// trusts the upstream proxy. Empty or "RemoteAddr" uses the TCP peer only.
	IPHeader string `json:"ipHeader,omitempty"`
	// CacheSize is the maximum number of cached lookup results per database;
	// 0 disables the cache.
	CacheSize int `json:"cacheSize,omitempty"`
	// CacheTTL is how long a cached result is served, as a Go duration ("30m").
	CacheTTL string `json:"cacheTTL,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		DBPath:    DefaultDBPath,
		LogLevel:  DefaultLogLevel,
		IPHeader:  RealIPHeader,
		CacheSize: DefaultCacheSize,
		CacheTTL:  DefaultCacheTTL,
	}
}

// TraefikGeoIP2 is one middleware instance. Instances for the same database
// path share its reader and cache through the registry.
type TraefikGeoIP2 struct {
	next     http.Handler
	name     string
	kind     dbKind
	ipHeader string
	db       *geoDB
	log      *logger
}

// New creates a middleware instance. Configuration errors are returned;
// database problems are logged once and leave the instance running with
// Unknown values, so a missing or broken file never takes a router down.
func New(_ context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	if cfg == nil {
		return nil, errors.New("geoip2: configuration is nil")
	}
	if cfg.DBPath == "" {
		return nil, errors.New("geoip2: dbPath is required")
	}
	level, err := parseLogLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("geoip2: %w", err)
	}
	ipHeader, ok := normalizeIPHeader(cfg.IPHeader)
	if !ok {
		return nil, fmt.Errorf("geoip2: ipHeader %q is not a header name", cfg.IPHeader)
	}
	if cfg.CacheSize < 0 {
		return nil, fmt.Errorf("geoip2: cacheSize must not be negative, got %d", cfg.CacheSize)
	}
	ttlText := cfg.CacheTTL
	if ttlText == "" {
		ttlText = DefaultCacheTTL
	}
	ttl, err := time.ParseDuration(ttlText)
	if err != nil || ttl <= 0 {
		return nil, fmt.Errorf("geoip2: cacheTTL must be a positive duration such as \"30m\", got %q", cfg.CacheTTL)
	}

	db := sharedDB(cfg.DBPath, cacheSettings{size: cfg.CacheSize, ttl: ttl})
	return &TraefikGeoIP2{
		next:     next,
		name:     name,
		kind:     db.kind,
		ipHeader: ipHeader,
		db:       db,
		log:      newLogger(level, name),
	}, nil
}

func (mw *TraefikGeoIP2) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	now := time.Now()
	state := mw.db.current()
	mw.db.maybeCheck(now)

	req = stripClientHeaders(req)
	addr, ok := clientAddr(req, mw.ipHeader)
	rec := mw.resolve(state, addr, ok, now)
	mw.setHeaders(req.Header, rec)

	if mw.log.enabled(levelDebug) {
		mw.log.debugf("remote=%s addr=%v country=%s region=%s city=%s asn=%s duration=%s",
			req.RemoteAddr, addr, rec.country, rec.region, rec.city, rec.asn, time.Since(now))
	}
	mw.next.ServeHTTP(rw, req)
}

// resolve returns the immutable result for addr: from the cache, from the
// database, or unknownResult when no lookup is possible.
func (mw *TraefikGeoIP2) resolve(state *dbState, addr netip.Addr, ok bool, now time.Time) *GeoIPResult {
	if state.lookup == nil || !ok || isSpecial(addr) {
		return unknownResult
	}
	key := addr.String()
	if state.cache != nil {
		if rec, hit := state.cache.get(key, now); hit {
			return rec
		}
	}
	rec, err := state.lookup(addr)
	if err != nil {
		mw.log.infof("no data for %s: %v", key, err)
		rec = unknownResult
	}
	if state.cache != nil {
		state.cache.set(key, rec, now)
	}
	return rec
}

// setHeaders writes the headers this instance owns and leaves the other kind's
// headers untouched so that a City/Country instance and an ASN instance chain.
func (mw *TraefikGeoIP2) setHeaders(h http.Header, rec *GeoIPResult) {
	if mw.kind == kindASN {
		h.Set(ASNHeader, rec.asn)
		h.Set(ASOrgHeader, rec.asOrg)
		return
	}
	h.Set(CountryHeader, rec.country)
	h.Set(RegionHeader, rec.region)
	h.Set(CityHeader, rec.city)
	h.Set(TimezoneHeader, rec.timezone)
	h.Set(LatitudeHeader, rec.latitude)
	h.Set(LongitudeHeader, rec.longitude)
	h.Set(AnycastHeader, strconv.FormatBool(rec.anycast))
}
