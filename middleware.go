// Package traefikgeoip2 is a Traefik plugin for Maxmind GeoIP2.
package traefikgeoip2

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/IncSW/geoip2"
)

// Config the plugin configuration.
type Config struct {
	DBPath string `json:"dbPath,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		DBPath: DefaultDBPath,
	}
}

// TraefikGeoIP2 a traefik geoip2 plugin.
type TraefikGeoIP2 struct {
	next   http.Handler
	lookup LookupGeoIP2
	name   string
}

// New created a new TraefikGeoIP2 plugin.
func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	if _, err := os.Stat(cfg.DBPath); err != nil {
		log.Printf("[geoip2] DB `%s' not found: %v", cfg.DBPath, err)
		return &TraefikGeoIP2{
			lookup: nil,
			next:   next,
			name:   name,
		}, nil
	}

	return &TraefikGeoIP2{
		lookup: sharedLookup(cfg.DBPath),
		next:   next,
		name:   name,
	}, nil
}

var (
	lookupsMu sync.Mutex
	lookups   = map[string]LookupGeoIP2{}
)

// sharedLookup loads each database once per process: Traefik calls New on every
// dynamic configuration reload, and re-reading a City database (100+ MB) each time leaks memory.
func sharedLookup(dbPath string) LookupGeoIP2 {
	lookupsMu.Lock()
	defer lookupsMu.Unlock()

	if lookup, ok := lookups[dbPath]; ok {
		return lookup
	}

	lookup := openLookup(dbPath)
	if lookup != nil {
		lookups[dbPath] = lookup
	}
	return lookup
}

func openLookup(dbPath string) LookupGeoIP2 {
	if strings.Contains(dbPath, "City") {
		rdr, err := geoip2.NewCityReaderFromFile(dbPath)
		if err != nil {
			log.Printf("[geoip2] DB `%s' not initialized: %v", dbPath, err)
			return nil
		}
		return CreateCityDBLookup(rdr)
	}

	if strings.Contains(dbPath, "Country") {
		rdr, err := geoip2.NewCountryReaderFromFile(dbPath)
		if err != nil {
			log.Printf("[geoip2] DB `%s' not initialized: %v", dbPath, err)
			return nil
		}
		return CreateCountryDBLookup(rdr)
	}

	return nil
}

func (mw *TraefikGeoIP2) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// log.Printf("[geoip2] remoteAddr: %v, xRealIp: %v", req.RemoteAddr, req.Header.Get(RealIPHeader))

	if mw.lookup == nil {
		req.Header.Set(CountryHeader, Unknown)
		req.Header.Set(CountryNameHeader, Unknown)
		req.Header.Set(RegionHeader, Unknown)
		req.Header.Set(CityHeader, Unknown)
		mw.next.ServeHTTP(rw, req)
		return
	}

	ipStr := req.Header.Get(RealIPHeader)
	if ipStr == "" {
		ipStr = req.RemoteAddr
		tmp, _, err := net.SplitHostPort(ipStr)
		if err == nil {
			ipStr = tmp
		}
	}

	res, err := mw.lookup(net.ParseIP(ipStr))
	if err != nil {
		log.Printf("[geoip2] Unable to find for `%s', %v", ipStr, err)
		res = &GeoIPResult{
			country:     Unknown,
			countryName: Unknown,
			region:      Unknown,
			city:        Unknown,
		}
	}

	req.Header.Set(CountryHeader, res.country)
	req.Header.Set(CountryNameHeader, res.countryName)
	req.Header.Set(RegionHeader, res.region)
	req.Header.Set(CityHeader, res.city)

	mw.next.ServeHTTP(rw, req)
}
