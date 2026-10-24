# Vendor patches

`make vendor` runs `go mod vendor` and then applies every patch in this
directory with `patch -p1`. Never edit files under `vendor/` by hand; change the
patch instead and re-run `make vendor`.

## 0001-incsw-geoip2-no-unsafe-is-anycast.patch

Applies to `github.com/IncSW/geoip2 v0.1.1` (MIT).

- `common.go`: `b2s` copies the bytes instead of casting through `unsafe.Pointer`.
  Traefik refuses to load a plugin that imports `unsafe` unless the plugin manifest
  and the static configuration both opt in (`useUnsafe`), so the plugin must not
  depend on it. Every upstream line of this plugin carries the same change.
- `traits.go`, `types.go`: decode the `is_anycast` trait into `Traits.IsAnycast`.
  MaxMind added the field to the City and Country databases in 2023; upstream added
  it in v0.1.3, a version that also requires a newer Go toolchain and still uses
  `unsafe`.

`vendor_patch_test.go` fails when the vendored copy does not carry this patch.
