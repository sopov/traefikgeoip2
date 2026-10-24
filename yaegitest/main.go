// Command yaegitest loads the plugin the way Traefik 3 does (Yaegi interpreter,
// GOPATH layout, vendored dependencies, mapstructure-decoded configuration) and
// runs a few requests against the test databases. It is a separate module so
// that the interpreter never becomes a dependency of the plugin itself.
//
// Usage: go run . <plugin repository root>
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/mitchellh/mapstructure"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const importPath = "github.com/sopov/traefikgeoip2"

func main() {
	root := ".."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("OK: plugin loads and runs under Yaegi with vendored dependencies")
}

func run(root string) error {
	goPath, err := stage(root)
	if err != nil {
		return err
	}
	defer os.RemoveAll(goPath)

	i := interp.New(interp.Options{GoPath: goPath, Env: os.Environ(), Stdout: os.Stdout, Stderr: os.Stderr})
	if err := i.Use(stdlib.Symbols); err != nil {
		return err
	}
	if _, err := i.Eval(fmt.Sprintf(`import %q`, importPath)); err != nil {
		return fmt.Errorf("import plugin: %w", err)
	}
	basePkg := filepath.Base(importPath)
	fnNew, err := i.Eval(basePkg + ".New")
	if err != nil {
		return err
	}
	fnCreateConfig, err := i.Eval(basePkg + ".CreateConfig")
	if err != nil {
		return err
	}
	pluginDir := filepath.Join(goPath, "src", importPath)
	build := func(next http.Handler, config map[string]any, name string) (http.Handler, error) {
		cfg := fnCreateConfig.Call(nil)[0]
		dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{WeaklyTypedInput: true, Result: cfg.Interface()})
		if err != nil {
			return nil, err
		}
		if err := dec.Decode(config); err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
		res := fnNew.Call([]reflect.Value{reflect.ValueOf(context.Background()), reflect.ValueOf(next), cfg, reflect.ValueOf(name)})
		if e, _ := res[1].Interface().(error); e != nil {
			return nil, e
		}
		h, ok := res[0].Interface().(http.Handler)
		if !ok {
			return nil, fmt.Errorf("New returned %T", res[0].Interface())
		}
		return h, nil
	}

	var seen http.Header
	sink := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.Header.Clone() })
	asn, err := build(sink, map[string]any{
		"dbPath": filepath.Join(pluginDir, "testdata", "GeoLite2-ASN-Test.mmdb"), "logLevel": "warn", "cacheSize": "1024",
	}, "geoip-asn")
	if err != nil {
		return err
	}
	city, err := build(asn, map[string]any{
		"dbPath": filepath.Join(pluginDir, "testdata", "GeoIP2-City-Test.mmdb"), "ipHeader": "X-Forwarded-For", "cacheTTL": "5m",
	}, "geoip")
	if err != nil {
		return err
	}
	// Repeated New on the same path must reuse the loaded database.
	for n := 0; n < 100; n++ {
		if _, err := build(sink, map[string]any{"dbPath": filepath.Join(pluginDir, "testdata", "GeoIP2-City-Test.mmdb")}, "again"); err != nil {
			return err
		}
	}

	do := func(remote string, hdr map[string]string) http.Header {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.RemoteAddr = remote
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		city.ServeHTTP(httptest.NewRecorder(), req)
		return seen
	}
	expect := func(h http.Header, key, want string) error {
		if got := h.Get(key); got != want {
			return fmt.Errorf("%s = %q, want %q (headers %v)", key, got, want, h)
		}
		return nil
	}

	h := do("216.160.83.56:443", map[string]string{"X-GeoIP2-Country": "ZZ", "X-GeoIP2-ASN": "64496"})
	for _, c := range []struct{ k, v string }{
		{"X-GeoIP2-Country", "US"}, {"X-GeoIP2-Region", "WA"}, {"X-GeoIP2-City", "Milton"},
		{"X-GeoIP2-Timezone", "America/Los_Angeles"}, {"X-GeoIP2-Anycast", "false"}, {"X-GeoIP2-ASN", "209"}, {"X-GeoIP2-ASOrg", "XX"},
	} {
		if err := expect(h, c.k, c.v); err != nil {
			return fmt.Errorf("chained lookup: %w", err)
		}
	}
	h = do("10.0.0.1:1", map[string]string{"X-Forwarded-For": "[2001:218::1]:443, 10.0.0.2"})
	if err := expect(h, "X-GeoIP2-Country", "JP"); err != nil {
		return fmt.Errorf("forwarded header: %w", err)
	}
	h = do("[fe80::1]:1", nil)
	if err := expect(h, "X-GeoIP2-Country", "XX"); err != nil {
		return fmt.Errorf("special address: %w", err)
	}
	h = do("214.1.1.1:1", nil)
	if err := expect(h, "X-GeoIP2-Anycast", "true"); err != nil {
		return fmt.Errorf("anycast: %w", err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < 50; k++ {
				req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
				req.RemoteAddr = fmt.Sprintf("81.2.69.%d:1", 160+(g+k)%32)
				city.ServeHTTP(httptest.NewRecorder(), req)
			}
		}(g)
	}
	wg.Wait()
	return nil
}

// stage copies the plugin repository into a temporary GOPATH at its import path,
// which is how Traefik lays plugins out for the interpreter.
func stage(root string) (string, error) {
	goPath, err := os.MkdirTemp("", "yaegitest-gopath-")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(goPath, "src", importPath)
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".git" || rel == "yaegitest" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
	if err != nil {
		os.RemoveAll(goPath)
		return "", err
	}
	return goPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
