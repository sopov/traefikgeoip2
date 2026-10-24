package traefikgeoip2

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/IncSW/geoip2"
)

// Unknown constant for undefined data.
const Unknown = "XX"

// DefaultDBPath default GeoIP2 database path.
const DefaultDBPath = "GeoLite2-Country.mmdb"

// DefaultLogLevel default Level of errors.
const DefaultLogLevel = "ERROR"

const DefaultCacheExpire = 30 * time.Minute
const DefaultCachePurge = 2 * time.Hour

// DefaultCacheExpire is Default purges Time

const (
	// RealIPHeader real ip header.
	RealIPHeader = "X-Real-IP"
	// CountryHeader country header name.
	CountryHeader = "X-GeoIP2-Country"
	// RegionHeader region header name.
	RegionHeader = "X-GeoIP2-Region"
	// CityHeader city header name.
	CityHeader = "X-GeoIP2-City"
	// TimezoneHeader timezone header name.
	TimezoneHeader = "X-GeoIP2-Timezone"
	// LatitudeHeader latitude header name.
	LatitudeHeader = "X-GeoIP2-Latitude"
	// LongitudeHeader longitude header name.
	LongitudeHeader = "X-GeoIP2-Longitude"
	// AnycastHeader anycast flag header name.
	AnycastHeader = "X-GeoIP2-Anycast"
	// ASNHeader autonomous system number header name (ASN database).
	ASNHeader = "X-GeoIP2-ASN"
	// ASOrgHeader autonomous system organization header name (ASN database).
	ASOrgHeader = "X-GeoIP2-ASOrg"
)

// dbKind tells which headers an instance owns: one instance serves one database,
// so a City/Country instance and an ASN instance chain without clobbering each other.
type dbKind int

const (
	kindGeo dbKind = iota
	kindASN
)

// kindOf picks the database kind by file name, the same way openLookup picks the reader.
func kindOf(dbPath string) dbKind {
	if strings.Contains(dbPath, "ASN") {
		return kindASN
	}
	return kindGeo
}

// GeoIPResult GeoIPResult.
type GeoIPResult struct {
	country   string
	region    string
	city      string
	timezone  string
	latitude  string
	longitude string
	anycast   bool
	asn       string
	asOrg     string
}

// LookupGeoIP2 LookupGeoIP2.
type LookupGeoIP2 func(ip net.IP) (*GeoIPResult, error)

// CreateCityDBLookup CreateCityDBLookup.
func CreateCityDBLookup(rdr *geoip2.CityReader) LookupGeoIP2 {
	return func(ip net.IP) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		retval := GeoIPResult{
			country:  rec.Country.ISOCode,
			region:   Unknown,
			city:     rec.City.Names["en"],
			timezone: rec.Location.TimeZone,
			anycast:  rec.Traits.IsAnycast,
		}
		if rec.Subdivisions != nil {
			retval.region = rec.Subdivisions[0].ISOCode
		}
		if rec.Location.Latitude != 0 || rec.Location.Longitude != 0 {
			retval.latitude = strconv.FormatFloat(rec.Location.Latitude, 'f', -1, 64)
			retval.longitude = strconv.FormatFloat(rec.Location.Longitude, 'f', -1, 64)
		}
		return &retval, nil
	}
}

// CreateCountryDBLookup CreateCountryDBLookup.
func CreateCountryDBLookup(rdr *geoip2.CountryReader) LookupGeoIP2 {
	return func(ip net.IP) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		retval := GeoIPResult{
			country: rec.Country.ISOCode,
			region:  Unknown,
			city:    Unknown,
			anycast: rec.Traits.IsAnycast,
		}
		return &retval, nil
	}
}

// CreateASNDBLookup CreateASNDBLookup. The record carries only the two ASN fields.
func CreateASNDBLookup(rdr *geoip2.ASNReader) LookupGeoIP2 {
	return func(ip net.IP) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		retval := GeoIPResult{asOrg: rec.AutonomousSystemOrganization}
		if rec.AutonomousSystemNumber != 0 {
			retval.asn = strconv.FormatUint(uint64(rec.AutonomousSystemNumber), 10)
		}
		return &retval, nil
	}
}
