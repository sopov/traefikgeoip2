package traefikgeoip2

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// remoteAddrOnly is the ipHeader value that disables header lookup.
const remoteAddrOnly = "remoteaddr"

// normalizeIPHeader validates the ipHeader option. The empty string and
// "RemoteAddr" (any case) select the TCP peer address; anything else must be a
// header name.
func normalizeIPHeader(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, remoteAddrOnly) {
		return "", true
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		isToken := c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isToken {
			return "", false
		}
	}
	return http.CanonicalHeaderKey(v), true
}

// clientAddr returns the normalized client address. When header is set and
// present, its first comma-separated element is used (the client-most entry of
// X-Forwarded-For); otherwise RemoteAddr. The second result is false when no
// address could be parsed.
func clientAddr(req *http.Request, header string) (netip.Addr, bool) {
	raw := ""
	if header != "" {
		raw = req.Header.Get(header)
	}
	if raw == "" {
		raw = req.RemoteAddr
	}
	return parseClientAddr(raw)
}

// parseClientAddr accepts the forms seen in RemoteAddr and forwarding headers:
// "1.2.3.4", "1.2.3.4:443", "2001:db8::1", "[2001:db8::1]:443", "::ffff:1.2.3.4",
// "fe80::1%eth0", surrounding whitespace and a comma-separated list (first
// element wins). IPv4-mapped IPv6 addresses are unmapped and zones dropped, so
// one client always yields one cache key.
func parseClientAddr(raw string) (netip.Addr, bool) {
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(raw); err == nil {
		return ap.Addr().Unmap().WithZone(""), true
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

// isSpecial reports addresses that are never in a GeoIP database: private,
// loopback, link-local, multicast and unspecified. They get Unknown headers
// without a lookup, a log line or a cache entry.
func isSpecial(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified()
}

// pluginHeaders lists every header any instance kind may set.
var pluginHeaders = []string{
	CountryHeader, RegionHeader, CityHeader, TimezoneHeader, LatitudeHeader, LongitudeHeader, AnycastHeader,
	ASNHeader, ASOrgHeader,
}

// chainMarker marks a request whose client-supplied plugin headers were stripped.
type chainMarker struct{}

// stripClientHeaders removes every X-GeoIP2-* header the client may have sent.
// Only the first instance in a middleware chain strips, so the headers a
// chained instance set before this one survive; the request context carries
// the marker.
func stripClientHeaders(req *http.Request) *http.Request {
	if req.Context().Value(chainMarker{}) != nil {
		return req
	}
	for _, h := range pluginHeaders {
		req.Header.Del(h)
	}
	return req.WithContext(context.WithValue(req.Context(), chainMarker{}, true))
}
