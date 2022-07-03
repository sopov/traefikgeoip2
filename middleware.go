// Package traefikgeoip2 is a Traefik plugin for Maxmind GeoIP2.
package traefikgeoip2

import (
	"context"
	"github.com/IncSW/geoip2"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

var (
	logInfo = log.New(ioutil.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
	logWarn = log.New(ioutil.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
	logErr  = log.New(ioutil.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
)

// Config the plugin configuration.
type Config struct {
	DBPath   string `json:"dbPath,omitempty"`
	LogLevel string `yaml:"loglevel"`
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

	switch strings.ToUpper(cfg.LogLevel) {
	case "INFO":
		logInfo.SetOutput(os.Stdout)
		logWarn.SetOutput(os.Stderr)
		logInfo.SetOutput(os.Stdout)
	case "WARN":
		logWarn.SetOutput(os.Stderr)
		logInfo.SetOutput(os.Stdout)
	case "ERROR":
		logInfo.SetOutput(os.Stdout)
	}
	logErr.SetOutput(os.Stderr)

	if _, err := os.Stat(cfg.DBPath); err != nil {
		logErr.Printf("GeoIP DB `%s' not found: %v", cfg.DBPath, err)
		return &TraefikGeoIP2{
			lookup: nil,
			next:   next,
			name:   name,
		}, nil
	}

	var lookup LookupGeoIP2
	if strings.Contains(cfg.DBPath, "City") {
		rdr, err := geoip2.NewCityReaderFromFile(cfg.DBPath)
		if err != nil {
			logWarn.Printf("GeoIP DB `%s' not initialized: %v", cfg.DBPath, err)
		} else {
			lookup = CreateCityDBLookup(rdr)
		}
	}

	if strings.Contains(cfg.DBPath, "Country") {
		rdr, err := geoip2.NewCountryReaderFromFile(cfg.DBPath)
		if err != nil {
			logWarn.Printf("GeoIP DB `%s' not initialized: %v", cfg.DBPath, err)
		} else {
			lookup = CreateCountryDBLookup(rdr)
		}
	}

	return &TraefikGeoIP2{
		lookup: lookup,
		next:   next,
		name:   name,
	}, nil
}

func (mw *TraefikGeoIP2) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	if mw.lookup == nil {
		logWarn.Printf("Unable to lookup remoteAddr: %v, xRealIp: %v", req.RemoteAddr, req.Header.Get(RealIPHeader))
		req.Header.Set(CountryHeader, Unknown)
		req.Header.Set(RegionHeader, Unknown)
		req.Header.Set(CityHeader, Unknown)
		mw.next.ServeHTTP(rw, req)
		return
	}

	ipStr := req.Header.Get(RealIPHeader)
	if ipStr == "" {
		ipStr = req.RemoteAddr
		host, _, err := net.SplitHostPort(ipStr)
		if err == nil {
			ipStr = host
		}
	}

	res, err := mw.lookup(net.ParseIP(ipStr))
	if err != nil {
		logWarn.Printf("Unable to find GeoIP data for `%s', %v", ipStr, err)
		res = &GeoIPResult{
			country: Unknown,
			region:  Unknown,
			city:    Unknown,
		}
	}

	req.Header.Set(CountryHeader, res.country)
	req.Header.Set(RegionHeader, res.region)
	req.Header.Set(CityHeader, res.city)

	logInfo.Printf("remoteAddr: %v, xRealIp: %v, Country: %v, Region: %v, City: %v",
		req.RemoteAddr,
		req.Header.Get(RealIPHeader),
		res.country,
		res.region,
		res.city,
	)

	mw.next.ServeHTTP(rw, req)
}
