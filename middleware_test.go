package traefikgeoip2_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/sopov/traefikgeoip2"
)

const (
	ValidIP        = "188.193.88.199"
	ValidIPAndPort = "188.193.88.199:9999"
)

func TestGeoIPConfig(t *testing.T) {
	mwCfg := mw.CreateConfig()
	if mw.DefaultDBPath != mwCfg.DBPath {
		t.Fatalf("Incorrect path")
	}

	mwCfg.DBPath = "./non-existing"
	_, err := mw.New(context.TODO(), nil, mwCfg, "")
	if err != nil {
		t.Fatalf("Must not fail on missing DB")
	}

	mwCfg.DBPath = "Makefile"
	_, err = mw.New(context.TODO(), nil, mwCfg, "")
	if err != nil {
		t.Fatalf("Must not fail on invalid DB format")
	}
}

func TestGeoIPBasic(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./GeoLite2-City.mmdb"

	called := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) { called = true })

	instance, err := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2")
	if err != nil {
		t.Fatalf("Error creating %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

	instance.ServeHTTP(recorder, req)
	if recorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("Invalid return code")
	}
	if called != true {
		t.Fatalf("next handler was not called")
	}
}

func TestMissingGeoIPDB(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./missing"

	called := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) { called = true })

	instance, err := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2")
	if err != nil {
		t.Fatalf("Error creating %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

	instance.ServeHTTP(recorder, req)
	if recorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("Invalid return code")
	}
	if called != true {
		t.Fatalf("next handler was not called")
	}
	assertHeader(t, req, mw.CountryHeader, mw.Unknown)
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
}

func TestGeoIPFromRemoteAddr(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./GeoLite2-City.mmdb"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	instance, _ := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2")

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = ValidIPAndPort
	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.CountryHeader, "DE")
	assertHeader(t, req, mw.RegionHeader, "Bavaria")
	assertHeader(t, req, mw.CityHeader, "Munich")

	req = httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = "qwerty:9999"
	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.CountryHeader, mw.Unknown)
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
}

func TestGeoIPCountryDBFromRemoteAddr(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./GeoLite2-Country.mmdb"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	instance, _ := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2")

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = ValidIPAndPort
	instance.ServeHTTP(httptest.NewRecorder(), req)

	assertHeader(t, req, mw.CountryHeader, "DE")
	assertHeader(t, req, mw.RegionHeader, mw.Unknown)
	assertHeader(t, req, mw.CityHeader, mw.Unknown)
}

func TestGeoIPFromXRealIP(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./GeoLite2-City.mmdb"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	instance, _ := mw.New(context.Background(), next, mwCfg, "traefik-geoip2")

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = "1.1.1.1:9999"
	req.Header.Set("X-Real-Ip", ValidIP)

	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.CountryHeader, "DE")
	assertHeader(t, req, mw.RegionHeader, "Bavaria")
	assertHeader(t, req, mw.CityHeader, "Munich")
	assertHeader(t, req, mw.TimezoneHeader, "Europe/Berlin")
	assertHeader(t, req, mw.AnycastHeader, "false")
	if req.Header.Get(mw.LatitudeHeader) == mw.Unknown || req.Header.Get(mw.LongitudeHeader) == mw.Unknown {
		t.Fatalf("latitude/longitude not set: %s / %s", req.Header.Get(mw.LatitudeHeader), req.Header.Get(mw.LongitudeHeader))
	}
}

// ASN test database from github.com/maxmind/MaxMind-DB (test-data): 1.128.0.0/11 → AS1221 Telstra.
const asnTestDB = "./testdata/GeoLite2-ASN-Test.mmdb"

func TestGeoIPASNDBFromRemoteAddr(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = asnTestDB

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	instance, err := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2-asn")
	if err != nil {
		t.Fatalf("Error creating %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = "1.128.0.1:9999"
	req.Header.Set(mw.CountryHeader, "DE")
	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.ASNHeader, "1221")
	assertHeader(t, req, mw.ASOrgHeader, "Telstra Pty Ltd")
	// An ASN instance owns only its two headers: the chained geo instance's values survive.
	assertHeader(t, req, mw.CountryHeader, "DE")
	if req.Header.Get(mw.RegionHeader) != "" {
		t.Fatalf("ASN instance must not set geo headers, got region %q", req.Header.Get(mw.RegionHeader))
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.ASNHeader, mw.Unknown)
	assertHeader(t, req, mw.ASOrgHeader, mw.Unknown)
}

func TestMissingASNDBKeepsGeoHeaders(t *testing.T) {
	mwCfg := mw.CreateConfig()
	mwCfg.DBPath = "./testdata/GeoLite2-ASN-missing.mmdb"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	instance, err := mw.New(context.TODO(), next, mwCfg, "traefik-geoip2-asn")
	if err != nil {
		t.Fatalf("Error creating %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.RemoteAddr = ValidIPAndPort
	req.Header.Set(mw.CountryHeader, "DE")
	instance.ServeHTTP(httptest.NewRecorder(), req)
	assertHeader(t, req, mw.ASNHeader, mw.Unknown)
	assertHeader(t, req, mw.ASOrgHeader, mw.Unknown)
	assertHeader(t, req, mw.CountryHeader, "DE")
}

func assertHeader(t *testing.T, req *http.Request, key, expected string) {
	t.Helper()
	if req.Header.Get(key) != expected {
		t.Fatalf("invalid value of header [%s] != %s", key, req.Header.Get(key))
	}
}
