package traefikgeoip2_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IncSW/geoip2"
)

// The vendored github.com/IncSW/geoip2 carries patches/0001-incsw-geoip2-no-unsafe-is-anycast.patch.
// Re-vendoring without `make vendor` silently drops it; this test makes that visible.
func TestVendoredGeoIP2IsPatched(t *testing.T) {
	// Compile-time check: the field exists only in the patched copy.
	var traits geoip2.Traits
	if traits.IsAnycast {
		t.Fatal("zero value must be false")
	}

	dir := filepath.Join("vendor", "github.com", "IncSW", "geoip2")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("vendored geoip2 sources not found in %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	sawAnycast := false
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, file, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"unsafe"` {
				t.Errorf("%s imports unsafe; Traefik refuses to load the plugin without useUnsafe", file)
			}
		}
		if strings.Contains(string(src), `case "is_anycast":`) {
			sawAnycast = true
		}
	}
	if !sawAnycast {
		t.Error("vendored traits.go does not decode is_anycast; run `make vendor` to re-apply patches/")
	}
}
