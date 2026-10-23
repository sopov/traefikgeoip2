# Traefik GeoIP2 plugin

Plugin for getting information from MaxMind geo database and pass it to request headers.
Supports GeoIP2 and GeoLite2 databases.

## Usage

### Configuration

For each plugin, the Traefik static configuration must define the module name (as is usual for Go packages).

The following declaration (given here in YAML) defines an plugin:

```yaml
# Static configuration
pilot:
  token: xxxxx

experimental:
  plugins:
    geoip:
      moduleName: github.com/sopov/traefik-geoip2
      version: v0.1.0
```

Here is an example of a file provider dynamic configuration (given here in YAML), where the interesting part is the `http.middlewares` section:

```yaml
# Dynamic configuration

http:
  routers:
    my-router:
      rule: host(`demo.localhost`)
      service: service-foo
      entryPoints:
        - web
      middlewares:
        - geoip

  services:
    service-foo:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:5000

  middlewares:
    my-plugin:
      plugin:
        geoip:
          dbPath: ./GeoLite2-Country.mmdb
```

### Databases and headers

The database kind is taken from the file name. One middleware instance serves one database and sets only its own headers, so instances can be chained on a router (`geoip`, then `geoip-asn`):

| file name contains | headers                                                                                    |
| ------------------ | ------------------------------------------------------------------------------------------ |
| `City`             | `X-GeoIP2-Country`, `-Region`, `-City`, `-Timezone`, `-Latitude`, `-Longitude`, `-Anycast` |
| `Country`          | the same set; region, city and coordinates are `XX`                                        |
| `ASN`              | `X-GeoIP2-ASN` (number), `X-GeoIP2-ASOrg` (organization)                                   |

A missing or unreadable database leaves the instance running: it sets its headers to `XX` and never touches the headers of another instance. Each database is loaded once per process and shared across dynamic-configuration reloads.

## Development

To run linter and tests - execute

```sh
make prepare
make
```
