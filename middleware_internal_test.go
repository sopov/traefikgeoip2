package traefikgeoip2

import (
	"context"
	"reflect"
	"testing"
)

func TestSharedLookupReusesDatabase(t *testing.T) {
	cases := []struct{ name, dbPath string }{
		{"city", "./GeoLite2-City.mmdb"},
		{"country", "./GeoLite2-Country.mmdb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := CreateConfig()
			cfg.DBPath = tc.dbPath

			first, err := New(context.TODO(), nil, cfg, "a")
			if err != nil {
				t.Fatalf("first New: %v", err)
			}
			second, err := New(context.TODO(), nil, cfg, "b")
			if err != nil {
				t.Fatalf("second New: %v", err)
			}

			a := reflect.ValueOf(first.(*TraefikGeoIP2).lookup).Pointer()
			b := reflect.ValueOf(second.(*TraefikGeoIP2).lookup).Pointer()
			if a == 0 || a != b {
				t.Fatalf("lookup must be shared between instances: %#x != %#x", a, b)
			}
		})
	}
}

func TestSharedLookupDoesNotCacheFailures(t *testing.T) {
	lookupsMu.Lock()
	delete(lookups, "Makefile")
	lookupsMu.Unlock()

	if got := sharedLookup("Makefile"); got != nil {
		t.Fatalf("invalid DB must yield nil lookup")
	}
	lookupsMu.Lock()
	_, cached := lookups["Makefile"]
	lookupsMu.Unlock()
	if cached {
		t.Fatalf("failed load must not be cached")
	}
}
