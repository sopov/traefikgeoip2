# Review: sopov/traefikgeoip2, branch `prod` (7caefe7)

Review-only. No source file was modified. Scope: middleware.go, types.go, middleware_test.go, vendor/, .traefik.yml, go.mod/go.sum, Makefile, Dockerfile, readme.md, LICENSE, both GitHub workflows, .golangci.toml, and the relationship to `nuki-io/traefikgeoip2` (main, 3a31b28), `GiGInnovationLabs/traefikgeoip2` (latest tag v0.22.0, now published as `github.com/traefik-plugins/traefikgeoip2`) and the other branches of this repository (`main`, `fix-shared-db-reader`).

Evidence gathered during the review (all outside the repository, nothing written into it):

- `go vet ./...` and `go test ./...` on Go 1.24.7 (output verbatim in the appendix): vet clean, 3 of 8 tests fail because the City/Country databases are not in the checkout.
- `go test -race` on a scratch module that drives the plugin with 16 concurrent requests for one new IP: the race detector reports writes in `setGeoHeaders` racing with reads in `ServeHTTP` on the cached `*GeoIPResult`.
- The plugin loaded under Yaegi v0.16.1 (the version Traefik 3.7.x pins) with the same `interp.Options{GoPath}` + `stdlib.Symbols` setup Traefik uses, with the vendored dependencies: import, `CreateConfig`, `New` and `ServeHTTP` all work; 200 consecutive `New` calls on one path add 0.1 MB of heap and leave 2 goroutines; an uncached lookup costs about 207 µs interpreted versus 5.5 µs compiled (ASN test database); a cache hit costs about 35 µs interpreted.
- `yaegi test -v .` (the Makefile and CI target) fails with `unable to find source related to: "github.com/IncSW/geoip2"` even in a GOPATH layout.
- `go mod vendor` on a copy regenerates `vendor/github.com/IncSW/geoip2` from the pristine v0.1.1 module and the package then no longer compiles (`rec.Traits.IsAnycast undefined`).
- golangci-lint v1.64.8 with the repository's `.golangci.toml`: 9 configuration warnings, 38 issues across 9 linters.

Severity scale: blocker = must be fixed before this can be published or relied on; major = real production defect or risk; minor = defect with limited impact or a hardening item; note = observation, no action strictly required.

## Summary

| id | severity | title | file:line |
|----|----------|-------|-----------|
| P1 | blocker | Module does not compile against its declared dependency; only the hand-patched vendor copy builds | go.mod:6, types.go:95, vendor/github.com/IncSW/geoip2/types.go:77 |
| T1 | blocker | Three tests fail in every clean checkout: the City/Country databases are not in the repository and `make prepare` points to a file that does not exist | middleware_test.go:38, Makefile:8 |
| CI1 | blocker | Both workflows use retired action versions (`actions/cache@v2`, `checkout@v2`, `setup-go@v2`) | .github/workflows/main.yml:29, .github/workflows/go-cross.yml:30 |
| CI2 | blocker | Both workflows run `make prepare`, which fails, and then the tests, which fail without the databases | .github/workflows/main.yml:70, .github/workflows/go-cross.yml:43 |
| C1 | major | Data race: `setGeoHeaders` writes into the shared cached `*GeoIPResult` while other requests read it | middleware.go:203-207, middleware.go:214-236 |
| C2 | major | A database file updated on disk is never reloaded; the registry serves the first-loaded copy until Traefik restarts | middleware.go:95-109 |
| IP1 | major | `X-Real-IP` is passed to `net.ParseIP` unprocessed: a port, bracket, zone or whitespace yields `XX` for every request | middleware.go:151-158, middleware.go:168 |
| IP2 | major | No `X-Forwarded-For` support and no configurable source header; behind any upstream proxy or CDN the plugin geolocates the proxy | middleware.go:151 |
| CA1 | major | Result cache has no size bound; growth is driven by the rate of distinct (including attacker-chosen) keys | middleware.go:177, types.go:22-23 |
| Y1 | major | The MaxMind decoder runs interpreted: ~40x slower than compiled; every cache miss pays it | vendor/github.com/IncSW/geoip2/common.go, types.go:84-138 |
| L1 | major | Log level is process-global, set by whichever instance ran `New` last, and can only be raised, never lowered | middleware.go:20-24, middleware.go:51-58 |
| L2 | major | Silent failure at the default level: unrecognised file names and corrupt databases are logged at WARN or not at all, and every request then gets `XX` | middleware.go:111-140, middleware.go:143-147 |
| T2 | major | Test suite misses the behaviours this branch added: shared registry, reload growth, cache, IPv6, header hygiene, concurrency | middleware_test.go |
| D1 | major | Region header carries the English subdivision name; both upstreams emit the ISO code since v0.1.0 | types.go:98 |
| P2 | major | The hand patch in vendor/ has no marker, no patch file and no test; `make vendor`, `make clean`, or any re-vendoring silently destroys it | Makefile:19-23, vendor/github.com/IncSW/geoip2/traits.go:57-61 |
| P3 | major | README does not match the code or the repository (module path, version, Traefik 2 options, example names, build steps, options) | readme.md:16-23, readme.md:39-48, readme.md:70-73 |
| LI1 | major | LICENSE attributes the work to Containous SAS / Traefik Labs; no NOTICE, no attribution to GiG Innovation Labs or nuki-io, no change notices | LICENSE:189-190, readme.md |
| CI3 | major | `main.yml` triggers on pushes to `master`, a branch that does not exist | .github/workflows/main.yml:3-9 |
| CI4 | major | Go 1.15 and golangci-lint v1.33.0 are pinned; the lint configuration uses removed linters and fails with current versions | .github/workflows/main.yml:17-18, .golangci.toml:31-45 |
| CI5 | major | Yaegi v0.9.20 is pinned and `yaegi test -v .` cannot resolve vendored imports in any version | .github/workflows/main.yml:19, Makefile:17 |
| C3 | minor | Registry is keyed by the raw path string and never released | middleware.go:99-107 |
| C4 | minor | A database that fails to open is re-read in full on every `New` call | middleware.go:103-108 |
| IP3 | minor | `X-Real-IP` is trusted unconditionally | middleware.go:151 |
| IP4 | minor | IPv6 forms are not normalised: zones fail, mapped and bracketed forms produce distinct cache keys | middleware.go:151-158 |
| IP5 | minor | Private, loopback and unspecified addresses are looked up and logged as failures | middleware.go:168-170 |
| IP6 | minor | Client-supplied `X-GeoIP2-*` headers of the other kind pass through to backends | middleware.go:201-247 |
| IP7 | minor | `kindOf` and `openLookup` disagree on paths containing more than one keyword; matching is case-sensitive | types.go:60-65, middleware.go:111-140 |
| IP8 | minor | `X-GeoIP2-Anycast` is `false` when the lookup failed; non-ASCII city names are written raw into header values | middleware.go:244, types.go:93 |
| CA2 | minor | Cache key is the raw, unnormalised header string; parse failures are cached | middleware.go:165, middleware.go:177 |
| CA3 | minor | Cache TTL and purge interval are hard-coded; purge holds the write lock over the whole map | types.go:22-23, vendor/github.com/patrickmn/go-cache/cache.go:931 |
| CA4 | minor | Unchecked type assertion on the cache value | middleware.go:166 |
| CA5 | note | One `GeoIPResult` with nine strings is cached per key regardless of database kind | types.go:68-78 |
| Y2 | minor | `log.Lshortfile` prints Yaegi's `restricted.go:51` for every line; timestamps duplicate Traefik's | middleware.go:21-23 |
| Y3 | note | The `unsafe` removal in vendor is what lets Traefik load the plugin; upgrading IncSW/geoip2 reintroduces `unsafe`, Go 1.25 and Go 1.22 syntax | vendor/github.com/IncSW/geoip2/common.go:367-369 |
| Y4 | note | go-cache's janitor goroutine and `runtime.SetFinalizer` work under Yaegi v0.16.1; the finalizer never fires by design | vendor/github.com/patrickmn/go-cache/cache.go:1113-1125 |
| L3 | minor | Under Traefik, plugin stderr is logged at ERROR and stdout at DEBUG, so WARN lines appear as errors and INFO lines are invisible | middleware.go:53-58 |
| L4 | minor | Per-request INFO line formats twelve fields and calls `time.Now` twice even when INFO is off | middleware.go:149, middleware.go:180-194 |
| L5 | minor | `fmt.Errorf("%w", err)` adds no context; `New` never returns an error | types.go:88, types.go:113, types.go:130, middleware.go:50-80 |
| CF1 | minor | `LogLevel` tagged `yaml:"loglevel"` while `DBPath` is tagged `json:"dbPath"`; values are not validated | middleware.go:28-29 |
| CF2 | minor | Relative default path; `.traefik.yml` testData names a file that is not in the repository | types.go:17, .traefik.yml:9-10 |
| CF3 | minor | Behavioural compatibility with upstream configs: same `dbPath`, different header values | types.go:98, types.go:27-48 |
| T3 | minor | Tests depend on live GeoLite2 data for a real IP | middleware_test.go:13-14, middleware_test.go:97-99 |
| T4 | minor | Tests share process-global state and only pass `-race` because they are sequential | middleware_test.go, middleware.go:88-91 |
| T5 | note | Test database provenance is not pinned; the current upstream file differs | middleware_test.go:147, testdata/GeoLite2-ASN-Test.mmdb |
| D2 | minor | Three divergent lines in one repository: `main` mirrors nuki-io, `prod` has an orphan root, `fix-shared-db-reader` has a test `prod` lacks | middleware.go:93-109 |
| D3 | minor | Features in the GiG successor (`traefik-plugins/traefikgeoip2` v0.22.0) and in IncSW/geoip2 v0.1.2-v0.1.4 that `prod` does not have | middleware.go, go.mod:6 |
| P4 | minor | Catalog readiness: no tag on `prod`; the only tag (v0.2.0) points at the old code; default-branch and topic requirements | .traefik.yml, go.mod:1 |
| P5 | minor | Dockerfile and Makefile are broken (Traefik 2.4.8 base, nonexistent `geoip.go`, nonexistent `.gz`) | Dockerfile:1-4, Makefile:7-8 |
| P6 | minor | `go 1.15` directive; raising it forces a re-vendor that collides with P1/P2 | go.mod:3, vendor/modules.txt |
| LI2 | minor | Modified MIT file carries no notice; test database licence not stated | vendor/github.com/IncSW/geoip2/traits.go, testdata/ |
| CI6 | minor | Workflow hygiene: wrong cache paths, no permissions block, no concurrency, no release workflow | .github/workflows/go-cross.yml:32-36 |

## 1. Concurrency and lifecycle

### C1 (major) Data race on the shared cached record

Location: middleware.go:203-207 (ASN branch), middleware.go:214-236 (geo branch), read side middleware.go:184-192 and middleware.go:209-210.

`ServeHTTP` stores the `*GeoIPResult` returned by the lookup in the cache (middleware.go:177) and passes the same pointer to `setGeoHeaders`, which fills empty fields in place (`record.asn = Unknown`, `record.city = Unknown`, ...). Every later request for the same key gets the same pointer out of the cache and reads those fields. Two requests for a new IP arriving together, or one request reading a cached entry while the first request is still filling it, are a write/read race on plain string fields.

Failure scenario: first request for `10.99.99.99` on an ASN instance (lookup fails, `asn` is empty) is in `setGeoHeaders` writing `record.asn`; a second request for the same address has just fetched the pointer from the cache and reads `record.asn`. `go test -race` flags this reliably with 16 goroutines. A torn string header read is a crash risk in compiled Go; under Yaegi the result is undefined behaviour inside the interpreter.

Fix: make cached records immutable. Fill the `Unknown` defaults inside the three `Create*DBLookup` closures (or in a `normalize()` called once before `cache.Set`) and have `setGeoHeaders` only read. Alternatively cache a value copy and apply defaults to a local copy.

### C2 (major) Updated database files are never reloaded

Location: middleware.go:95-109.

`sharedDB` keys the registry by path and returns the first reader forever. GeoLite2 databases are republished twice a week; the usual deployment (geoipupdate or a cron job) replaces the file in place. Traefik's dynamic reloads call `New` several times a day, but `New` will always get the stale reader. The old behaviour (a fresh read on every `New`) at least picked up new data at the cost of a leak; this branch trades the leak for permanent staleness.

Failure scenario: geoipupdate writes a new `GeoLite2-City.mmdb` on Tuesday; the plugin keeps serving the file loaded at Traefik start until the next Traefik restart, which on a long-running instance may be months. Country reassignments and new ranges are missed.

Fix: store `os.FileInfo` (size and mtime) with the registry entry; in `sharedDB`, when the stat result differs, open the new file and replace the entry (and start a fresh cache) under `dbsMu`. The old reader is dropped and garbage-collected once in-flight requests finish. This piggybacks on the reloads that already happen several times a day. Optionally add a background refresh with a configurable interval for deployments without reloads.

### C3 (minor) Registry keyed by raw path string and never released

Location: middleware.go:99-107, middleware.go:61.

`./GeoLite2-City.mmdb`, `GeoLite2-City.mmdb` and a symlink to the same file load the database three times. Entries are never removed: a middleware instance deleted from the dynamic configuration keeps its 100+ MB resident for the process lifetime. The `New` signature gives no close hook, so a release policy would have to be a refcount with a grace period; the simpler improvement is key normalisation.

Fix: key the map by `filepath.Clean(filepath.Abs(path))` after `filepath.EvalSymlinks`. Document the retention policy in the README.

### C4 (minor) A database that fails to open is re-read in full on every New

Location: middleware.go:103-108.

A failed open is intentionally not cached. For a corrupt or misnamed 100+ MB file, every `New` call (one per router that references the middleware, times the number of middleware instances, on every reload) runs `ioutil.ReadFile` and the metadata parse again. With 17 routers that is 17 reads of the whole file per reload.

Fix: negative-cache the failure keyed by path plus size/mtime, and log once per distinct (path, mtime).

## 2. Client IP handling

### IP1 (major) X-Real-IP is parsed verbatim

Location: middleware.go:151-158, middleware.go:168.

Only `RemoteAddr` goes through `net.SplitHostPort`. `X-Real-IP` is handed directly to `net.ParseIP`. Observed under the Yaegi harness: `1.128.0.1:9999`, ` 1.128.0.1`, `1.128.0.1, 10.0.0.2`, `fe80::1%eth0` all yield `IP cannot be nil`, a WARN line, and `XX` headers, cached for 30 minutes under that exact string.

Failure scenario: an upstream proxy sets `X-Real-IP: 203.0.113.9:51234` (some load balancers do). Every request is `XX` and, with WARN enabled, every distinct value logs one line.

Fix: a `clientIP(req)` helper that trims whitespace, strips brackets and port (`net.SplitHostPort` or `netip.ParseAddrPort`), strips a zone, takes the first element of a comma list, and parses with `netip.ParseAddr`; use its canonical string as the cache key. Treat an unparsable value as "unknown" without caching it.

### IP2 (major) No X-Forwarded-For support, no configurable header

Location: middleware.go:151.

Traefik's own forwarded-headers handling only sets `X-Real-Ip` to the TCP peer when the peer is not in `forwardedHeaders.trustedIPs`. Behind a CDN or another load balancer the peer is the proxy, so unless the operator configures trusted IPs on the entry point and the proxy sets `X-Real-IP`, the plugin geolocates the proxy. The GiG successor added `preferXForwardedForHeader` for this reason; nuki-io main and this branch have nothing.

Fix: `ipHeader` (default `X-Real-IP`) or `preferXForwardedForHeader` plus a `trustedProxies`/`xffDepth` option to pick the right XFF element. Document that `X-Forwarded-For` must only be trusted when Traefik's entry point trusts the upstream.

### IP3 (minor) X-Real-IP trusted unconditionally

Location: middleware.go:151.

If `forwardedHeaders.insecure` is set, or trusted IPs include the client network, a client-chosen `X-Real-IP` controls the lookup, the log line and the cache key. Document the dependency on Traefik's forwarded-headers configuration, and consider an option to use `RemoteAddr` only.

### IP4 (minor) IPv6 forms are not normalised

Location: middleware.go:151-158.

`RemoteAddr` without a port (`2001:db8::1`) works only because `SplitHostPort` fails and the string is used unchanged. `::ffff:1.128.0.1`, `[::ffff:1.128.0.1]:80` and `1.128.0.1` all resolve correctly but to three different cache keys. `fe80::1%eth0` fails outright.

Fix: covered by the `clientIP` helper in IP1; key the cache by `netip.Addr.Unmap().String()`.

### IP5 (minor) Private and special addresses are looked up and logged as failures

Location: middleware.go:168-170.

RFC 1918, loopback, link-local and unspecified addresses hit the database, return `not found`, log at WARN (first time per 30 minutes per address) and cache a negative entry. On an internal entry point this is one WARN line per client every half hour and a cache full of private addresses.

Fix: short-circuit with `addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()` before the lookup; set the headers to `XX` (or a configurable value) without logging.

### IP6 (minor) Headers of the other kind are not cleared

Location: middleware.go:201-247.

Each instance sets only the headers of its own kind. The upstream code set every header it knew, which incidentally prevented clients from smuggling `X-GeoIP2-*` values to backends. Now a deployment with only a City instance passes a client-supplied `X-GeoIP2-ASN: 13335` through untouched, and an ASN-only deployment passes client-supplied `X-GeoIP2-Country`.

Failure scenario: a backend uses `X-GeoIP2-ASN` for rate limiting or fraud scoring while only the City middleware is configured; a client sets the header directly.

Fix: in the first instance of a chain delete every known `X-GeoIP2-*` header, or add a `clearHeaders: [list]` option and document that the chain should be `geoip` then `geoip-asn`. At minimum document the exposure.

### IP7 (minor) Kind detection by substring is fragile and case-sensitive

Location: types.go:60-65, middleware.go:111-140.

`kindOf` checks `ASN` first; `openLookup` checks `City`, then `Country`, then `ASN`, and returns at the first match. A path such as `/var/lib/ASN/GeoLite2-City.mmdb` gets a City reader and the ASN header set, so the instance emits `X-GeoIP2-ASN: XX` for every request and never the City data. `/geoip/City/GeoLite2-Country.mmdb` is opened with the City reader, fails the database-type check, returns `nil` and the instance serves `XX` silently. `geolite2-city.mmdb` (lower case) matches nothing and returns `nil` without a log line (middleware.go:139).

Fix: read the MMDB metadata once (`DatabaseType` is `GeoLite2-City`, `GeoIP2-Country`, `GeoLite2-ASN`, `GeoIP2-Enterprise`, ...) and pick the reader and kind from it; keep file-name matching only as a fallback; log unrecognised types at ERROR.

### IP8 (minor) Header value semantics

Location: middleware.go:244, types.go:93, types.go:98.

`X-GeoIP2-Anycast` is `false` both for "not anycast" and for "lookup failed", unlike every other header which uses `XX`. City and region names are written raw: `Zürich`, `Düsseldorf`, `São Paulo` put non-ASCII bytes into header values; HTTP/1.1 tolerates obs-text but some backends and proxies reject or mangle it.

Fix: emit `XX` for anycast on failure; offer ISO codes (see D1) or percent/ASCII-encode names, and document it.

## 3. Result cache

### CA1 (major) No size bound

Location: middleware.go:177, types.go:22-23, vendor/github.com/patrickmn/go-cache/cache.go:51.

`go-cache` is a map with a TTL and a periodic sweep. With `DefaultCacheExpire` 30 min and `DefaultCachePurge` 2 h, an entry lives between 30 minutes and 2.5 hours, so the live set is roughly the number of distinct keys seen in the last 2 to 2.5 hours. At the stated load (hundreds of thousands of distinct IPs per day) that is tens of thousands of entries, about 300-400 bytes each (map slot, key string, `Item`, `GeoIPResult` with nine string headers, string data), so tens of megabytes per database: tolerable. Nothing caps it: a scan from rotating IPv6 /64s, a botnet, or (with IP3) attacker-chosen `X-Real-IP` strings adds entries at line rate. Five million distinct keys in two hours is about 1.5 GB per database, inside the Traefik process.

Fix: replace go-cache with a bounded LRU (`hashicorp/golang-lru/v2` simplelru is small, dependency-free and interprets fine) sized by a `cacheSize` option, or keep go-cache but add a hard cap checked on `Set` (evict all or stop caching when `ItemCount()` exceeds the cap) and shorten the purge interval. Cache by `/24` and `/48` prefix if memory matters more than the rare prefix-boundary inaccuracy.

### CA2 (minor) Raw header string as key; failures cached

Location: middleware.go:165, middleware.go:177.

The key is whatever `clientIP` logic produced, unnormalised (see IP1/IP4). Parse failures (`qwerty`, empty `RemoteAddr`) are cached as `XX` records for 30 minutes. Negative caching of `not found` is useful; caching decode errors (a corrupt record for one range) hides the problem for 30 minutes per key.

Fix: key by canonical address; do not cache parse errors; cache `ErrNotFound` results but not other errors (or cache them briefly).

### CA3 (minor) Hard-coded TTL and purge; stop-the-world sweep

Location: types.go:22-25, vendor/github.com/patrickmn/go-cache/cache.go:931-945, cache.go:1076-1088.

TTL and purge are constants (the comment at types.go:25 is orphaned and describes nothing). `DeleteExpired` takes the exclusive lock and iterates the whole map; every `ServeHTTP` on that database blocks on `RLock` for the duration. Interpreted under Yaegi, a sweep over a million entries is a visible latency spike every two hours.

Fix: `cacheTTL` and `cacheSize` options with validation; an LRU removes the sweep entirely.

### CA4 (minor) Unchecked type assertion

Location: middleware.go:166.

`c.(*GeoIPResult)` panics if any other type is ever stored. Low risk today; use the two-value form and fall through to a lookup.

### CA5 (note) One record shape for all kinds

Location: types.go:68-78.

The ASN instance caches a `GeoIPResult` with seven unused string fields. A per-kind record (or two small structs) roughly halves per-entry memory and makes the "which fields are valid" question explicit.

## 4. Yaegi compatibility

### Y1 (major) Interpreted decoder cost

Location: vendor/github.com/IncSW/geoip2/common.go, vendor/github.com/IncSW/geoip2/reader.go:34-67, types.go:84-138.

Everything in vendor/ is interpreted. Measured with the harness: an uncached ASN lookup is ~207 µs interpreted versus 5.5 µs compiled; a cache hit is ~35 µs interpreted. City records are far larger (nested maps for names in several languages, subdivisions, location, postal, traits), so expect an uncached City lookup in the 0.5-2 ms range. The IncSW reader decodes every key of the record even though the plugin uses four of them. At hundreds of thousands of new IPs per day this is minutes of CPU per day, acceptable; under a distinct-IP flood every request pays it on the request path, before any rate limiter further down the chain.

Fix: keep the cache (it is the right call), make the City closure skip what it does not need if the reader is forked anyway (P1), put the plugin before any middleware that can reject requests cheaply, and evaluate Traefik 3's WASM plugin runtime (`runtime: wasm` in `.traefik.yml`) for native speed if the plugin keeps growing.

### Y2 (minor) `log.Lshortfile` and timestamps are wrong under the interpreter

Location: middleware.go:21-23.

Under Yaegi the caller recorded by `log` is Yaegi's own `restricted.go:51` for every line (observed in the harness), so the file:line suffix is noise. Traefik's zerolog adds its own timestamp and level, so `Ldate|Ltime` duplicates it.

Fix: `log.New(w, "geoip2: ", 0)` or a tiny leveled logger that writes `level=warn msg=...`.

### Y3 (note) The `unsafe` removal is load-bearing; upgrading IncSW/geoip2 reintroduces it

Location: vendor/github.com/IncSW/geoip2/common.go:367-369 (patched `b2s`), go.mod:6.

Pristine IncSW/geoip2 v0.1.1 through v0.1.4 implements `b2s` with `unsafe.Pointer`. Traefik refuses to load a plugin that imports `unsafe` unless `.traefik.yml` sets `useUnsafe: true` and the operator enables it in the static configuration (pkg/plugins/middlewareyaegi.go in Traefik 3.7). The vendored copy replaces it with `string(value)`; both upstreams carry the same patch. v0.1.4 also declares `go 1.25` and uses `for range n` (Go 1.22 syntax); Yaegi v0.16.1 targets Go 1.21/1.22, so an upgrade needs an interpreter test before it is believed.

Fix: see P1, publish a fork module that removes `unsafe` and adds `IsAnycast`, and test it under the pinned Yaegi version.

### Y4 (note) go-cache under Yaegi

Location: vendor/github.com/patrickmn/go-cache/cache.go:1113-1125.

`runtime.SetFinalizer`, the janitor goroutine, `encoding/gob`, `crypto/rand` and `math/rand` are all available in Yaegi's stdlib symbol table and the harness ran them without error. The finalizer never fires because the registry holds every cache forever, which is the intent. `sharded.go` is dead code but costs interpretation time at import.

## 5. Error handling and logging

### L1 (major) Process-global, monotonic log level

Location: middleware.go:20-24, middleware.go:51-58.

The three loggers are package globals. `New` only ever calls `SetOutput(os.Stdout/Stderr)`; nothing ever sets them back to `ioutil.Discard`. With several instances per database and 17 routers, the effective level is "the highest level any instance ever requested since process start". Changing `loglevel: INFO` back to `ERROR` in the dynamic configuration has no effect until a restart. Unknown values (`DEBUG`, `error ` with a space, `Error` is fine because of `ToUpper`) are silently treated as ERROR.

Fix: give each `TraefikGeoIP2` its own level and write through one logger; validate the value in `New` and return an error for unknown levels; or reset outputs on every `New` to the configured level (last writer wins, but at least reversible).

### L2 (major) Silent failure at the default level

Location: middleware.go:111-140, middleware.go:143-147, middleware.go:62.

- A path that matches none of `City`, `Country`, `ASN` (lower case, renamed file, a `GeoIP2-ISP.mmdb`) returns `nil` from `openLookup` with no log line at all.
- A corrupt or wrong-type database is logged at WARN (`not initialized`), which is discarded at the default `ERROR` level.
- From then on every request gets `XX` headers. With WARN enabled the plugin writes one `Unable to lookup remoteAddr` line per request, which is the opposite problem.
- A missing file is logged at ERROR on every `New` call, so a misconfigured path produces 17 x instances identical ERROR lines per reload.

Fix: log open failures of any kind at ERROR, once per (path, mtime); log the per-request degraded state once (sync.Once per instance) or at DEBUG; keep per-request diagnostics out of WARN.

### L3 (minor) stdout/stderr mapping under Traefik

Location: middleware.go:53-58.

Traefik builds the interpreter with `Stdout` bound to its logger at DEBUG and `Stderr` at ERROR (pkg/plugins/middlewareyaegi.go). So `logWarn` and `logErr` both surface as Traefik `ERROR` lines, and `logInfo` is only visible when Traefik itself runs at `DEBUG`, at which point it is one twelve-field line per request through zerolog.

Fix: one writer (stdout) with explicit `level=` tags, and drop the per-request line or sample it.

### L4 (minor) Per-request cost paid even when INFO is off

Location: middleware.go:149, middleware.go:180-194.

`time.Now()` twice and twelve argument evaluations per request happen before `log.Printf` discards the line. Interpreted, this is a measurable share of the 35 µs cache-hit path.

Fix: guard with `if mw.level >= info { ... }`.

### L5 (minor) Error wrapping and `New` never failing

Location: types.go:88, types.go:113, types.go:130, middleware.go:50-80.

`fmt.Errorf("%w", err)` adds nothing; wrap with context (`city lookup %s: %w`) or return `err`. `New` returns `nil` error for missing, corrupt and unrecognised databases (inherited from upstream "dont fail on missing db"), so the Traefik dashboard shows a healthy middleware that sets `XX` everywhere. A `failOnMissingDB` option would let operators choose.

## 6. Configuration surface

### CF1 (minor) Inconsistent struct tags, no validation

Location: middleware.go:27-30, .traefik.yml:9-10.

Traefik decodes plugin configuration with `mapstructure` (`WeaklyTypedInput`, case-insensitive field-name match), so neither `json:"dbPath"` nor `yaml:"loglevel"` is consulted and `logLevel`, `loglevel` and `LogLevel` all work by accident. Document `logLevel` with the same `json:"logLevel,omitempty"` style, validate values in `New`, and add it to `testData` so the catalog analysis exercises it.

### CF2 (minor) Relative default path and testData

Location: types.go:17, .traefik.yml:10.

`GeoLite2-Country.mmdb` resolves against Traefik's working directory, which in containers is `/`. `.traefik.yml` testData points at `GeoLite2-City.mmdb`, which is not in the repository, so the catalog's analysis exercises only the missing-database path. Point testData at a committed test database (the ASN one exists; add the MaxMind City test database too) so the real code path is exercised.

### CF3 (minor) Compatibility with upstream configurations

Location: types.go:27-48, types.go:98.

`dbPath` is the only upstream option and is unchanged, so existing dynamic configurations load. Header output differs: `X-GeoIP2-Region` is an English name here and an ISO code upstream (D1); `X-GeoIP2-CountryName` from nuki-io main is absent; `X-GeoIP2-IPAddress` from the GiG successor is absent; seven headers are new. Backends written against either upstream will see different values. Document a migration table; consider `regionFormat: iso|name`.

## 7. Tests

### T1 (blocker) The suite cannot pass in a clean checkout

Location: middleware_test.go:38, middleware_test.go:89, middleware_test.go:111, middleware_test.go:127, Makefile:8, .gitignore:4-6.

`TestGeoIPFromRemoteAddr`, `TestGeoIPCountryDBFromRemoteAddr` and `TestGeoIPFromXRealIP` need `./GeoLite2-City.mmdb` and `./GeoLite2-Country.mmdb`. Both are gitignored, `make prepare` runs `gunzip -kf GeoLite2-City.mmdb.gz` on a file that is not tracked, and nothing provides the Country database at all (upstream shipped both in `geolite2.tgz`). `TestGeoIPBasic` passes only because it asserts nothing about headers, so it silently tests the missing-database path. Result: `go test ./...` fails (appendix).

Fix: commit the MaxMind test databases (`GeoIP2-City-Test.mmdb`, `GeoIP2-Country-Test.mmdb` from github.com/maxmind/MaxMind-DB, about 20 KB each, with the source commit noted) and assert against their fixed contents; rename files so the `City`/`Country` detection still works.

### T2 (major) Coverage gaps

Location: middleware_test.go.

Not covered: `sharedDB` reuse across `New` calls (the `fix-shared-db-reader` branch has such a test; `prod` does not); heap growth across repeated `New`; the cache (hit, TTL, negative entries, key normalisation); concurrency with `-race`; IPv6 (`RemoteAddr` with and without brackets, mapped, zone); `X-Real-IP` with port or list; corrupt database beyond "New does not error" (what do requests return?); header hygiene (client-supplied `X-GeoIP2-*` on a single-kind chain); log level handling; kind/reader mismatch paths (IP7); City/Country success paths at all in a clean checkout (T1); anycast `true`.

Fix: an internal test package (`package traefikgeoip2`) with reset hooks for the registry and loggers, table-driven IP tests, a `-race` concurrency test, and a reload test asserting a single reader per path.

### T3 (minor) Live-data dependency

Location: middleware_test.go:13-14, middleware_test.go:97-99, middleware_test.go:137-141.

`188.193.88.199` -> `DE/Bavaria/Munich/Europe/Berlin` is true for one GeoLite2 release and will drift. The MaxMind test databases fix this.

### T4 (minor) Shared global state

Location: middleware_test.go, middleware.go:20-24, middleware.go:88-91.

The registry and the three loggers persist across tests, so results depend on order (`TestGeoIPConfig` opens `Makefile` as a geo database before `TestGeoIPBasic`). `-race` passes only because tests are sequential. Add reset helpers and run with `t.Parallel()` and `-race` in CI.

### T5 (note) Test database provenance

Location: middleware_test.go:147, testdata/GeoLite2-ASN-Test.mmdb.

The committed file (12,653 bytes, sha256 `75901b98...`) differs from the current `main` of maxmind/MaxMind-DB (11,958 bytes). Record the source commit in the comment and in a `testdata/README`.

## 8. Divergence from upstream

Context established from git and the Go module proxy: GiG Innovation Labs is the original author (module path `github.com/GiGInnovationLabs/traefikgeoip2`); nuki-io is a fork that added `X-GeoIP2-CountryName` (8a94a1e, 2022-02-14) and removed a per-request log line (3a31b28, 2022-03-07); this repository's `main` is byte-identical to nuki-io `main`; `prod` starts from an orphan commit eb9de35 ("Removed history", 2022-07-03, tagged v0.2.0) whose code predates the upstream region fix; GiG continued independently to v0.22.0 (2024-03-06) under the path `github.com/traefik-plugins/traefikgeoip2`.

### D1 (major) Region header semantics lost

Location: types.go:98.

Upstream commit 6579e56 "fix: iso code for region" (2021-07-19, shipped in v0.1.0, present in nuki-io main, GiG v0.2.0 and the traefik-plugins successor) changed `X-GeoIP2-Region` from `Subdivisions[0].Names["en"]` to `Subdivisions[0].ISOCode`. `prod` emits the name (`Bavaria`), and its tests assert it. Users moving from any upstream build will see `Bavaria` where they had `BY`. If this is deliberate, document it and offer `regionFormat`; if not, restore the ISO code.

### D2 (minor) Three lines in one repository

Location: middleware.go:93-109, branch `fix-shared-db-reader` (c15c40e), branch `main` (3a31b28).

`main` tracks nuki-io and still has the per-`New` reader leak. `fix-shared-db-reader` fixes that leak on top of `main` with `sharedLookup` and ships `middleware_internal_test.go` (shared-reader and no-negative-caching tests) that `prod` lacks. `prod` has the shared registry plus cache, ASN, log levels and seven headers, but an unrelated root, no `CountryName`, no `[geoip2]` prefix and no tag. Three divergent heads make the catalog story (which tag, which branch) and attribution unclear.

Fix: choose `prod` as the line, port the internal tests from `fix-shared-db-reader`, decide on `CountryName`, retire or merge the other two branches, and tag.

### D3 (minor) Upstream features absent from `prod`

Location: middleware.go, go.mod:6.

From `traefik-plugins/traefikgeoip2` v0.22.0 (2024-03): `preferXForwardedForHeader` option (IP2), `X-GeoIP2-IPAddress` header, `go 1.19`, module path move, `ResetLookup` test hook, `[geoip2]` log prefix. Its single global `lookup` shared across different `dbPath` values is worse than this branch's per-path registry; do not copy that part. From IncSW/geoip2 v0.1.2-v0.1.4: native `IsAnycast`, DB-IP database types, `Network` (prefix) in results, `os.ReadFile`; still uses `unsafe` and now requires Go 1.25 (Y3).

## 9. Packaging for the Traefik plugin catalog

### P1 (blocker) The module only compiles with the hand-patched vendor directory

Location: go.mod:6, types.go:95, types.go:119, vendor/github.com/IncSW/geoip2/types.go:77, vendor/github.com/IncSW/geoip2/traits.go:57-61.

`go.mod` requires `github.com/IncSW/geoip2 v0.1.1`. That module has no `Traits.IsAnycast`; the field and the `is_anycast` case exist only in the vendored copy (added in commit 7caefe7). `go mod verify` passes because it checks the module cache, not vendor/. Verified on a copy: `go mod vendor` restores the pristine files (and the `unsafe` import) and `go build ./...` then fails with `rec.Traits.IsAnycast undefined`. Any consumer of the module path, any tool that re-vendors, and any build with `-mod=mod` is broken. Traefik itself loads from the repository archive with vendor/ intact, which is why it works in production today.

Fix: publish the modified reader as a real module (for example a fork `github.com/sopov/geoip2` at a tag that adds `IsAnycast` and removes `unsafe`), require it in go.mod, and run `go mod vendor` cleanly. Avoid `replace` directives: they are ignored by consumers and confuse the catalog. Until then, add a build-tag-free compile check in CI with `-mod=mod` so the breakage is visible.

### P2 (major) The vendor patch is unmarked and unprotected

Location: vendor/github.com/IncSW/geoip2/common.go:367-369, traits.go:57-61, types.go:77, Makefile:19-23, .github/workflows/main.yml:63-66.

No comment marks the patched lines, no `patches/` file reproduces them, and `make vendor` (`go mod vendor`) and `make clean` (`rm -rf ./vendor`) both destroy them. The CI step that would have detected drift (`git diff --exit-code ./vendor/`) is commented out.

Fix: until P1 is done, keep `patches/incsw-geoip2-anycast-nounsafe.patch`, apply it in `make vendor`, add a unit test that asserts `geoip2.Traits{}.IsAnycast` exists and a CI grep that vendor/ contains no `"unsafe"` import, and remove the `clean` target.

### P3 (major) README versus code and repository

Location: readme.md.

- readme.md:22 `moduleName: github.com/sopov/traefik-geoip2`; go.mod:1 and .traefik.yml:7 say `github.com/sopov/traefikgeoip2`.
- readme.md:23 `version: v0.1.0`; the repository has one tag, `v0.2.0`, on the old root commit; `prod` HEAD is untagged.
- readme.md:16-17 `pilot: token` is a Traefik 2 option removed in Traefik 3; the production target is 3.7.
- readme.md:39 the router references middleware `geoip`, but readme.md:48 names the middleware `my-plugin` (and `geoip` is the plugin key). The example cannot work as written.
- readme.md:70-73 `make prepare` runs `gunzip` on a file that does not exist.
- `logLevel` (and its values), the result cache, its TTL, the retention of loaded databases, the exact `X-Real-IP`/`RemoteAddr` rule and the Yaegi/`useUnsafe` situation are undocumented.
- readme.md:58-62 lists header names as `X-GeoIP2-*`; Go canonicalises them to `X-Geoip2-*` on the wire. Harmless for HTTP consumers but worth one sentence because `golangci-lint`'s `canonicalheader` flags it and operators grep logs.
- readme.md:60-61 says `-Region` without saying it is a name, not an ISO code (D1).
- readme.md:4 "Supports GeoIP2 and GeoLite2 databases": the ASN reader accepts only `GeoLite2-ASN` database_type (there is no GeoIP2 ASN product; GeoIP2-ISP is not supported), City accepts `GeoIP2-Enterprise`, DB-IP files are rejected.
- readme.md:64 is accurate: a missing or unreadable database yields `XX` and leaves other instances' headers alone; each database is loaded once per process.
- Title "Traefik GeoIP2 plugin" and `.traefik.yml` displayName "Traefik GeoIP2" are fine but differ from the upstream catalog entry "MaxMind GeoIP2".

Fix: rewrite the configuration section for Traefik 3 (`experimental.plugins.geoip2.moduleName/version`), correct names, document every option and header with examples, add a migration note from upstream, add a Credits section (LI1).

### P4 (minor) Catalog readiness

Location: .traefik.yml, go.mod:1, tags.

The catalog (plugins.traefik.io) imports a public GitHub repository carrying the `traefik-plugin` topic, reads `.traefik.yml` from the default branch, and offers semver tags; Traefik downloads an archive of the tagged tree with vendor/ included. Today the only tag `v0.2.0` points at the pre-cache, pre-ASN code; the default branch is probably `main` (which mirrors nuki-io, with the GiG import path in its `.traefik.yml`). Publishing requires: `prod` (or its content) as default branch, a new tag above v0.2.0, `.traefik.yml` `import` equal to the module path (it is), testData that exercises a committed database (CF2), and the topic set. Consider `iconPath`/`bannerPath` for the listing.

### P5 (minor) Dockerfile and Makefile are stale

Location: Dockerfile:1-4, Makefile:7-8, Makefile:22-23.

`FROM traefik:2.4.8` (2021, Traefik 2), `COPY ... ./geoip.go` names a file that does not exist, `COPY *.mmdb` fails when no database is present, and the local-plugin path layout is Traefik 2's. `make prepare` is broken (T1). Either delete both or rewrite for Traefik 3 `--experimental.localPlugins` with a committed test database.

### P6 (minor) `go 1.15` directive

Location: go.mod:3, vendor/modules.txt.

`go 1.15` keeps the language at 1.15 (fine for Yaegi) but disables modern linters and signals an unmaintained module. Raising it to 1.17 or later changes the required `vendor/modules.txt` format (`## explicit; go 1.14` lines), which forces a `go mod vendor`, which destroys the patch (P1/P2). Sequence: P1 first, then raise the directive to the highest version Yaegi v0.16.1 supports (1.22) and re-vendor.

## 10. Licensing

### LI1 (major) Attribution and notices for a derived Apache-2.0 work

Location: LICENSE:189-190, readme.md, repository root (no NOTICE).

The LICENSE file is the Apache-2.0 text with the appendix filled in as "Copyright 2020 Containous SAS / Copyright 2020 Traefik Labs", inherited unchanged from Traefik's plugin template through GiG and nuki-io. Nothing in the repository names GiG Innovation Labs (original author), nuki-io (fork author) or the current maintainer, and there is no NOTICE file. Apache-2.0 section 4 requires a redistributor of a modified work to (a) ship the License, (b) mark modified files with prominent notices stating they were changed, (c) retain all copyright, patent, trademark and attribution notices from the Source form, and (d) carry forward a NOTICE file if the original had one (it did not, so a NOTICE is optional but is the normal place for provenance). Section 4 also allows adding your own copyright statement for your modifications.

Fix:
- Keep the LICENSE text; leave the Containous/Traefik Labs lines (they are the retained notice for the template) and add a line for the current maintainer's modifications.
- Add `NOTICE` stating: this product is derived from `github.com/GiGInnovationLabs/traefikgeoip2` (GiG Innovation Labs, Apache-2.0) via `github.com/nuki-io/traefikgeoip2`, originally based on the Traefik plugin demo (Containous SAS / Traefik Labs, Apache-2.0); list the modifications at a high level.
- Add a short header comment to middleware.go, types.go and middleware_test.go ("Modified from github.com/GiGInnovationLabs/traefikgeoip2; see NOTICE") to satisfy 4(b).
- Add a Credits/License section to the README.

### LI2 (minor) Third-party components

Location: vendor/github.com/IncSW/geoip2/LICENSE, vendor/github.com/patrickmn/go-cache/LICENSE, vendor/github.com/IncSW/geoip2/traits.go, testdata/.

Both vendored libraries are MIT and their LICENSE files are present in vendor/, which satisfies MIT's notice requirement for the archive Traefik downloads. The modified IncSW files carry no "modified" comment; MIT does not require one but a one-line marker avoids confusion and supports P2. The committed test database comes from maxmind/MaxMind-DB; state its licence (the MaxMind-DB repository publishes its test data under a Creative Commons licence; confirm the exact one at the pinned commit) in `testdata/README` and NOTICE.

## 11. CI

### CI1 (blocker) Retired action versions

Location: .github/workflows/main.yml:29,35,42; .github/workflows/go-cross.yml:20,26,30.

`actions/cache@v2` talks to the legacy cache backend that GitHub shut down in early 2025; the step fails outright. `actions/checkout@v2` and `actions/setup-go@v2` run on Node 12/16 runtimes that GitHub has retired; the runner now forces Node 20 and the job fails or is scheduled to fail. The hand-rolled GOPATH layout in main.yml (`working-directory`, `go env -w GOPATH`) exists only to serve the old Yaegi test and is unnecessary with modules.

Fix: `actions/checkout@v4`, `actions/setup-go@v5` with `go-version-file: go.mod` (or an explicit version) and `cache: true`; delete the `actions/cache` steps and the GOPATH plumbing.

### CI2 (blocker) `make prepare` and the tests fail

Location: .github/workflows/main.yml:70-71, .github/workflows/go-cross.yml:43-44.

Both jobs run `make prepare` (fails: no `.gz`) and then the test suite (fails: no databases, T1). Fix T1 and drop `make prepare`.

### CI3 (major) Wrong trigger branch

Location: .github/workflows/main.yml:3-9.

Pushes to `master` never happen (branches are `main`, `prod`, `fix-shared-db-reader`), so the main workflow runs only for pull requests and `v*` tags. `go-cross.yml` runs on every push. Set `branches: [main, prod]` (or whichever survives D2).

### CI4 (major) Stale toolchain pins and lint configuration

Location: .github/workflows/main.yml:17-18, .github/workflows/main.yml:51, .github/workflows/go-cross.yml:14, .golangci.toml.

- Go 1.15 (EOL 2021) is the primary version; the matrix also builds `1.x` (currently 1.25).
- golangci-lint v1.33.0 (2020) cannot analyse code compiled by a current toolchain; the install script is fetched from `master`, which now serves the v2 installer.
- `.golangci.toml` with golangci-lint v1.64.8 produces warnings for `govet.check-shadowing` (deprecated), `scopelint`, `interfacer`, `maligned`, `golint` (removed), `goerr113` (renamed `err113`), and then 38 issues: `depguard` denies every third-party import by default (4), `canonicalheader` (9), `revive` unused parameters and comment style (10), `exhaustruct` (6), `gochecknoglobals` (5), `forcetypeassert`, `varnamelen`, `lll`, `gofumpt`. With golangci-lint v2 the file is rejected outright (`version: "2"` and the new layout are required).
- `go mod tidy && git diff --exit-code go.mod` is fine with current Go (`go mod tidy -diff` is empty).

Fix: Go matrix `[1.22, stable]` or a single version from go.mod; `golangci/golangci-lint-action@v7` with a pinned current version; rewrite the config deliberately (enable a chosen set instead of `enable-all`, configure `depguard` or drop it, decide on `canonicalheader` given the README names) and run `golangci-lint migrate` for the v2 layout.

### CI5 (major) Yaegi step cannot work

Location: .github/workflows/main.yml:19, .github/workflows/main.yml:54, .github/workflows/main.yml:73-76, Makefile:16-17.

Yaegi v0.9.20 (2021) does not match the interpreter Traefik 3.7 ships (v0.16.1) and predates several Go releases. Independently of version, `yaegi test -v .` cannot resolve the vendored `github.com/IncSW/geoip2` even in a GOPATH layout (verified with v0.16.1), while the same code loads fine through `interp` with `GoPath` set the way Traefik does it.

Fix: replace the step with a Go test (build tag `yaegi`) that uses `github.com/traefik/yaegi/interp` pinned to Traefik 3.7's version, imports the plugin by path with the repository as `GoPath`, calls `CreateConfig`/`New`/`ServeHTTP` against the committed test databases and asserts headers; or start Traefik with `--experimental.localPlugins` in CI and curl it.

### CI6 (minor) Hygiene

Location: .github/workflows/go-cross.yml:32-36, both workflows.

Cache paths list macOS and Windows directories for an ubuntu-only matrix; no top-level `permissions: contents: read`; no `concurrency` group to cancel superseded runs; no `-race` run; no Dependabot/Renovate for actions; no release workflow that validates `.traefik.yml` and the tag format.

### What must change for a green run (consolidated)

1. Commit the MaxMind test databases and fix the test expectations (T1, T3); delete `make prepare`.
2. Publish or vendor-patch the geoip2 dependency so `go build ./...` works with and without `-mod=mod` (P1, P2).
3. Replace the three `v2` actions with `checkout@v4` and `setup-go@v5`; remove `actions/cache` and the GOPATH plumbing (CI1).
4. Fix the trigger branches (CI3).
5. Pick a Go version from go.mod or a `[1.22, stable]` matrix (CI4).
6. Replace golangci-lint v1.33.0 and rewrite `.golangci.toml` for the chosen version; fix or suppress the resulting findings (CI4).
7. Replace `yaegi test` with an interp-based Go test or a local-plugin Traefik run (CI5).
8. Add `go test -race ./...` and a `-mod=mod` compile check (T4, P1).

## Appendix: verbatim tool output

Go toolchain: `go version go1.24.7 linux/amd64`.

```
$ go vet ./...
exit status: 0

$ go test ./...
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./non-existing' not found: stat ./non-existing: no such file or directory
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./GeoLite2-City.mmdb' not found: stat ./GeoLite2-City.mmdb: no such file or directory
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./missing' not found: stat ./missing: no such file or directory
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./GeoLite2-City.mmdb' not found: stat ./GeoLite2-City.mmdb: no such file or directory
--- FAIL: TestGeoIPFromRemoteAddr (0.00s)
    middleware_test.go:97: invalid value of header [X-GeoIP2-Country] != XX
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./GeoLite2-Country.mmdb' not found: stat ./GeoLite2-Country.mmdb: no such file or directory
--- FAIL: TestGeoIPCountryDBFromRemoteAddr (0.00s)
    middleware_test.go:120: invalid value of header [X-GeoIP2-Country] != XX
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./GeoLite2-City.mmdb' not found: stat ./GeoLite2-City.mmdb: no such file or directory
--- FAIL: TestGeoIPFromXRealIP (0.00s)
    middleware_test.go:137: invalid value of header [X-GeoIP2-Country] != XX
geoip2-2026/10/23 23:42:25 middleware.go:62: GeoIP DB `./testdata/GeoLite2-ASN-missing.mmdb' not found: stat ./testdata/GeoLite2-ASN-missing.mmdb: no such file or directory
FAIL
FAIL	github.com/sopov/traefikgeoip2	0.005s
FAIL
exit status: 1
```
