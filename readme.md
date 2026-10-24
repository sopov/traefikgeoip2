# Traefik GeoIP2 plugin

A [Traefik](https://doc.traefik.io/traefik/) middleware plugin that resolves the
client address against a MaxMind GeoIP2 or GeoLite2 database (City, Country or
ASN) and passes the result downstream as request headers.

Module path: `github.com/sopov/traefikgeoip2`. Requires Traefik 3.

## Configuration

### Static configuration

```yaml
experimental:
  plugins:
    geoip2:
      moduleName: github.com/sopov/traefikgeoip2
      version: "<latest tag of this repository>"
```

### Dynamic configuration

One middleware instance serves one database file. Chain a City (or Country)
instance and an ASN instance on a router to get both header sets:

```yaml
http:
  routers:
    my-router:
      rule: Host(`demo.localhost`)
      service: service-foo
      entryPoints:
        - web
      middlewares:
        - geoip
        - geoip-asn

  services:
    service-foo:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:5000

  middlewares:
    geoip:
      plugin:
        geoip2:
          dbPath: /var/lib/geoip/GeoLite2-City.mmdb
    geoip-asn:
      plugin:
        geoip2:
          dbPath: /var/lib/geoip/GeoLite2-ASN.mmdb
          logLevel: WARN
```

### Options

| option      | default         | description |
|-------------|-----------------|-------------|
| `dbPath`    | `GeoLite2-Country.mmdb` | Path of the `.mmdb` file. The file name must contain `City`, `Country` or `ASN`; that word selects the reader and the headers the instance sets. Use an absolute path: a relative one is resolved against Traefik's working directory. |
| `logLevel`  | `ERROR`         | `ERROR`, `WARN`, `INFO` or `DEBUG`. Per instance. |
| `ipHeader`  | `X-Real-IP`     | Header the client address is read from; `RemoteAddr` is the fallback. `X-Forwarded-For` uses the first (client-most) element. Empty or `RemoteAddr` ignores headers and uses the TCP peer. |
| `cacheSize` | `65536`         | Maximum number of cached lookup results per database file. `0` disables the cache. |
| `cacheTTL`  | `30m`           | How long a cached result is served, as a Go duration. |

An unknown `logLevel`, a negative `cacheSize`, an unparsable `cacheTTL` or an
invalid `ipHeader` is a configuration error and the middleware fails to build.
A missing or unreadable database is not: see below.

The cache settings of the first instance that loads a database file apply to
that file; instances sharing a file share its reader and its cache.

### Trusting the client address

`X-Real-IP` is set by Traefik itself from the TCP peer unless the peer is listed
in the entry point's `forwardedHeaders.trustedIPs`, in which case the header the
upstream proxy sent is kept. Only choose `X-Forwarded-For` (or another header)
when every hop in front of Traefik is trusted to set it; the plugin takes the
first element and does not validate it further. Use `ipHeader: RemoteAddr` when
Traefik is the edge and no proxy sits in front of it.

Addresses that are never in a GeoIP database (private, loopback, link-local,
multicast, unspecified) are not looked up and yield `XX`.

## Headers

Every value is `XX` when the database has no data for the address, when the
address could not be parsed, or when the database is unavailable.

| database file name contains | headers |
|-----------------------------|---------|
| `City` | `X-GeoIP2-Country` (ISO 3166-1 code), `X-GeoIP2-Region` (most specific subdivision ISO code, e.g. `ENG`, `WA`), `X-GeoIP2-City` (English name), `X-GeoIP2-Timezone` (IANA), `X-GeoIP2-Latitude`, `X-GeoIP2-Longitude`, `X-GeoIP2-Anycast` (`true`/`false`) |
| `Country` | the same set; region, city, time zone and coordinates are `XX` |
| `ASN` | `X-GeoIP2-ASN` (number), `X-GeoIP2-ASOrg` (organization) |

An instance sets only the headers of its own kind, so chained instances do not
overwrite each other. Before the first instance in a chain sets anything it
removes every `X-GeoIP2-*` header the client sent, so backends never see
client-supplied values. Go canonicalizes header names on the wire
(`X-Geoip2-Country`); HTTP header names are case-insensitive.

`X-GeoIP2-Anycast` is `false` both when the network is not anycast and when the
lookup failed.

## Databases

- Supported files: GeoIP2 and GeoLite2 City and Country (`GeoIP2-City`,
  `GeoLite2-City`, `GeoIP2-Enterprise`, `GeoIP2-Country`, `GeoLite2-Country`)
  and GeoLite2 ASN (`GeoLite2-ASN`). DB-IP files are not supported.
- Each database file is read once per process and shared by every middleware
  instance that references the same path, across dynamic configuration reloads.
- **Updates are picked up automatically.** The file is re-checked on every
  configuration reload and, from requests, at most once per 60 seconds. When
  its size or modification time changed, the new file is opened, swapped in
  atomically with a fresh cache, and the previous one is released after 30
  seconds. Replace the file atomically (write to a temporary name, then rename),
  as `geoipupdate` does. A replacement that does not open keeps the previous
  database and is reported once.
- A missing or broken database leaves the instance running with `XX` values and
  one `ERROR` log line; the file is picked up once it appears or is fixed.

## Logging

Lines look like `geoip2 level=error name="geoip@file" msg="..."`. Error and
warning lines go to stderr, which Traefik records at its `ERROR` level; info and
debug lines go to stdout, which Traefik records at `DEBUG`. Nothing is logged
per request below `DEBUG`; lookups that find no data are `INFO`.

## Development

```sh
make lint      # golangci-lint v2 with .golangci.yml
make test      # go test -race against the databases in testdata/
make vendor    # go mod vendor + patches/ (see patches/README.md)
make yaegi     # load the plugin with the Yaegi interpreter like Traefik does
```

`vendor/github.com/IncSW/geoip2` is patched (no `unsafe`, `IsAnycast`); never
edit it by hand, change `patches/` and run `make vendor`. The tests use the
MaxMind test databases documented in `testdata/README.md`. The `Dockerfile`
builds a Traefik v3.7 image with this repository as a local plugin.

## Credits and licence

Apache License 2.0, see `LICENSE`. This repository is an independently
maintained derivative of `github.com/GiGInnovationLabs/traefikgeoip2`
(GiG Innovation Labs) by way of `github.com/nuki-io/traefikgeoip2` (nuki-io);
see `NOTICE` for origins and third-party components.
