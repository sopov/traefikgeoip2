# Development image: Traefik 3 with this repository mounted as a local plugin.
#
#   docker build -t traefik-geoip2-dev .
#   docker run --rm -p 80:80 -v /path/to/GeoLite2-City.mmdb:/db/GeoLite2-City.mmdb traefik-geoip2-dev \
#     --experimental.localPlugins.geoip2.moduleName=github.com/sopov/traefikgeoip2 \
#     --providers.file.filename=/dynamic.yml ...
#
# Local plugins are read from /plugins-local/src/<module path>; vendor/ must be present.
FROM traefik:v3.7

COPY . /plugins-local/src/github.com/sopov/traefikgeoip2/
