// Package traefikgeoip2 is a Traefik plugin for Maxmind GeoIP2.
package traefikgeoip2

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IncSW/geoip2"
	"github.com/patrickmn/go-cache"
)

var (
	logInfo = log.New(io.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
	logWarn = log.New(io.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
	logErr  = log.New(io.Discard, "geoip2-", log.Ldate|log.Ltime|log.Lshortfile)
)

// Config the plugin configuration.
type Config struct {
	DBPath   string `json:"dbPath,omitempty"`
	LogLevel string `yaml:"loglevel"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		LogLevel: DefaultLogLevel,
		DBPath:   DefaultDBPath,
	}
}

// TraefikGeoIP2 a traefik geoip2 plugin.
type TraefikGeoIP2 struct {
	next   http.Handler
	lookup LookupGeoIP2
	name   string
	cache  *cache.Cache
	kind   dbKind
}

// New created a new TraefikGeoIP2 plugin.
func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	switch strings.ToUpper(cfg.LogLevel) {
	case "INFO":
		logInfo.SetOutput(os.Stdout)
		logWarn.SetOutput(os.Stderr)
	case "WARN":
		logWarn.SetOutput(os.Stderr)
	}
	logErr.SetOutput(os.Stderr)

	kind := kindOf(cfg.DBPath)
	if _, err := os.Stat(cfg.DBPath); err != nil {
		logErr.Printf("GeoIP DB `%s' not found: %v", cfg.DBPath, err)
		return &TraefikGeoIP2{
			lookup: nil,
			next:   next,
			name:   name,
			cache:  nil,
			kind:   kind,
		}, nil
	}

	db := sharedDB(cfg.DBPath)
	return &TraefikGeoIP2{
		lookup: db.lookup,
		next:   next,
		name:   name,
		cache:  db.cache,
		kind:   kind,
	}, nil
}

// geoDB is one loaded database with its own result cache (records differ per database).
type geoDB struct {
	lookup LookupGeoIP2
	cache  *cache.Cache
}

var (
	dbsMu sync.Mutex
	dbs   = map[string]*geoDB{}
)

// sharedDB loads each database once per process: Traefik calls New on every
// dynamic configuration reload, and re-reading a City database (100+ MB) each time leaks memory.
func sharedDB(dbPath string) *geoDB {
	dbsMu.Lock()
	defer dbsMu.Unlock()

	if db, ok := dbs[dbPath]; ok {
		return db
	}

	db := &geoDB{lookup: openLookup(dbPath)}
	if db.lookup != nil {
		db.cache = cache.New(DefaultCacheExpire, DefaultCachePurge)
		dbs[dbPath] = db
	}
	return db
}

func openLookup(dbPath string) LookupGeoIP2 {
	if strings.Contains(dbPath, "City") {
		rdr, err := geoip2.NewCityReaderFromFile(dbPath)
		if err != nil {
			logWarn.Printf("GeoIP DB `%s' not initialized: %v", dbPath, err)
			return nil
		}
		return CreateCityDBLookup(rdr)
	}

	if strings.Contains(dbPath, "Country") {
		rdr, err := geoip2.NewCountryReaderFromFile(dbPath)
		if err != nil {
			logWarn.Printf("GeoIP DB `%s' not initialized: %v", dbPath, err)
			return nil
		}
		return CreateCountryDBLookup(rdr)
	}

	if strings.Contains(dbPath, "ASN") {
		rdr, err := geoip2.NewASNReaderFromFile(dbPath)
		if err != nil {
			logWarn.Printf("GeoIP DB `%s' not initialized: %v", dbPath, err)
			return nil
		}
		return CreateASNDBLookup(rdr)
	}

	return nil
}

func (mw *TraefikGeoIP2) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if mw.lookup == nil {
		logWarn.Printf("Unable to lookup remoteAddr: %v, xRealIp: %v", req.RemoteAddr, req.Header.Get(RealIPHeader))
		mw.next.ServeHTTP(rw, mw.setGeoHeaders(req, &GeoIPResult{}))
		return
	}

	var start = time.Now()

	ipStr := req.Header.Get(RealIPHeader)
	if ipStr == "" {
		ipStr = req.RemoteAddr
		host, _, err := net.SplitHostPort(ipStr)
		if err == nil {
			ipStr = host
		}
	}

	var (
		record *GeoIPResult
		err    error
	)

	if c, found := mw.cache.Get(ipStr); found {
		record = c.(*GeoIPResult)
	} else {
		record, err = mw.lookup(net.ParseIP(ipStr))
		if err != nil {
			logWarn.Printf("Unable to find GeoIP data for `%s', %v", ipStr, err)
			record = &GeoIPResult{
				country: Unknown,
				region:  Unknown,
				city:    Unknown,
			}
		}
		mw.cache.Set(ipStr, record, cache.DefaultExpiration)
	}

	duration := time.Since(start)
	logInfo.Printf("remoteAddr: %v, xRealIp: %v, Country: %v, Region: %v, City: %v, TZ: %v, Lat: %v, Lon: %v, Anycast: %v, ASN: %v, ASOrg: %v, duration: %d µs",
		req.RemoteAddr,
		req.Header.Get(RealIPHeader),
		record.country,
		record.region,
		record.city,
		record.timezone,
		record.latitude,
		record.longitude,
		record.anycast,
		record.asn,
		record.asOrg,
		duration.Microseconds(),
	)

	mw.next.ServeHTTP(rw, mw.setGeoHeaders(req, record))
}

// setGeoHeaders writes the headers this instance owns; an ASN instance leaves
// the geo headers of a chained City/Country instance untouched.
func (mw *TraefikGeoIP2) setGeoHeaders(req *http.Request, record *GeoIPResult) *http.Request {
	if mw.kind == kindASN {
		if record.asn == "" {
			record.asn = Unknown
		}
		if record.asOrg == "" {
			record.asOrg = Unknown
		}
		req.Header.Set(ASNHeader, record.asn)
		req.Header.Set(ASOrgHeader, record.asOrg)
		return req
	}

	if record.country == "" {
		record.country = Unknown
	}

	if record.region == "" {
		record.region = Unknown
	}

	if record.city == "" {
		record.city = Unknown
	}

	if record.timezone == "" {
		record.timezone = Unknown
	}

	if record.latitude == "" {
		record.latitude = Unknown
	}

	if record.longitude == "" {
		record.longitude = Unknown
	}

	req.Header.Set(CountryHeader, record.country)
	req.Header.Set(RegionHeader, record.region)
	req.Header.Set(CityHeader, record.city)
	req.Header.Set(TimezoneHeader, record.timezone)
	req.Header.Set(LatitudeHeader, record.latitude)
	req.Header.Set(LongitudeHeader, record.longitude)
	req.Header.Set(AnycastHeader, strconv.FormatBool(record.anycast))

	return req
}
