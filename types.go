package traefikgeoip2

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/IncSW/geoip2"
)

// Unknown is the header value used when a field is not available.
const Unknown = "XX"

// Defaults for the configuration; see Config.
const (
	DefaultDBPath    = "GeoLite2-Country.mmdb"
	DefaultLogLevel  = "ERROR"
	DefaultCacheSize = 65536
	DefaultCacheTTL  = "30m"
)

const (
	// RealIPHeader is the default header the client address is read from.
	RealIPHeader = "X-Real-IP"
	// CountryHeader country ISO code.
	CountryHeader = "X-GeoIP2-Country"
	// RegionHeader most specific subdivision ISO code.
	RegionHeader = "X-GeoIP2-Region"
	// CityHeader city name (English).
	CityHeader = "X-GeoIP2-City"
	// TimezoneHeader IANA time zone.
	TimezoneHeader = "X-GeoIP2-Timezone"
	// LatitudeHeader latitude.
	LatitudeHeader = "X-GeoIP2-Latitude"
	// LongitudeHeader longitude.
	LongitudeHeader = "X-GeoIP2-Longitude"
	// AnycastHeader anycast flag, "true" or "false".
	AnycastHeader = "X-GeoIP2-Anycast"
	// ASNHeader autonomous system number (ASN database).
	ASNHeader = "X-GeoIP2-ASN"
	// ASOrgHeader autonomous system organization (ASN database).
	ASOrgHeader = "X-GeoIP2-ASOrg"
)

// dbKind is derived from the file name and decides which reader opens the file
// and which headers an instance owns, so that a City/Country instance and an
// ASN instance can be chained without clobbering each other.
type dbKind int

const (
	kindUnknown dbKind = iota
	kindCity
	kindCountry
	kindASN
)

func kindOf(dbPath string) dbKind {
	switch {
	case strings.Contains(dbPath, "City"):
		return kindCity
	case strings.Contains(dbPath, "Country"):
		return kindCountry
	case strings.Contains(dbPath, "ASN"):
		return kindASN
	default:
		return kindUnknown
	}
}

// GeoIPResult is an immutable lookup result. Every string field is populated
// (Unknown when the database has no value) before a result is cached or shared
// between requests. Never modify a *GeoIPResult after it was returned.
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

// unknownResult is returned when no lookup could be made. It is shared and
// must never be modified.
var unknownResult = &GeoIPResult{
	country: Unknown, region: Unknown, city: Unknown, timezone: Unknown,
	latitude: Unknown, longitude: Unknown, asn: Unknown, asOrg: Unknown,
}

// fill replaces every empty string field with Unknown and returns r.
func (r *GeoIPResult) fill() *GeoIPResult {
	for _, f := range []*string{&r.country, &r.region, &r.city, &r.timezone, &r.latitude, &r.longitude, &r.asn, &r.asOrg} {
		if *f == "" {
			*f = Unknown
		}
	}
	return r
}

// LookupGeoIP2 resolves one address against one database.
type LookupGeoIP2 func(addr netip.Addr) (*GeoIPResult, error)

func toNetIP(addr netip.Addr) net.IP { return net.IP(addr.AsSlice()) }

// CreateCityDBLookup builds the lookup for a City (or Enterprise) database.
func CreateCityDBLookup(rdr *geoip2.CityReader) LookupGeoIP2 {
	return func(addr netip.Addr) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(toNetIP(addr))
		if err != nil {
			return nil, fmt.Errorf("city lookup: %w", err)
		}
		r := &GeoIPResult{
			country:  rec.Country.ISOCode,
			city:     rec.City.Names["en"],
			timezone: rec.Location.TimeZone,
			anycast:  rec.Traits.IsAnycast,
		}
		if len(rec.Subdivisions) > 0 {
			r.region = rec.Subdivisions[0].ISOCode
		}
		if rec.Location.Latitude != 0 || rec.Location.Longitude != 0 {
			r.latitude = strconv.FormatFloat(rec.Location.Latitude, 'f', -1, 64)
			r.longitude = strconv.FormatFloat(rec.Location.Longitude, 'f', -1, 64)
		}
		return r.fill(), nil
	}
}

// CreateCountryDBLookup builds the lookup for a Country database.
func CreateCountryDBLookup(rdr *geoip2.CountryReader) LookupGeoIP2 {
	return func(addr netip.Addr) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(toNetIP(addr))
		if err != nil {
			return nil, fmt.Errorf("country lookup: %w", err)
		}
		r := &GeoIPResult{country: rec.Country.ISOCode, anycast: rec.Traits.IsAnycast}
		return r.fill(), nil
	}
}

// CreateASNDBLookup builds the lookup for an ASN database; only the two ASN fields are set.
func CreateASNDBLookup(rdr *geoip2.ASNReader) LookupGeoIP2 {
	return func(addr netip.Addr) (*GeoIPResult, error) {
		rec, err := rdr.Lookup(toNetIP(addr))
		if err != nil {
			return nil, fmt.Errorf("asn lookup: %w", err)
		}
		r := &GeoIPResult{asOrg: rec.AutonomousSystemOrganization}
		if rec.AutonomousSystemNumber != 0 {
			r.asn = strconv.FormatUint(uint64(rec.AutonomousSystemNumber), 10)
		}
		return r.fill(), nil
	}
}
