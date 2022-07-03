FROM traefik:2.4.8

COPY *.yml *.mmdb go.* ./geoip.go /plugins/go/src/github.com/sopov/traefikgeoip2/
COPY vendor/ /plugins/go/src/github.com/sopov/traefikgeoip2/vendor/

