package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidOrigin is returned for a CORS_ORIGINS entry that is not a scheme://host[:port] origin.
var ErrInvalidOrigin = errors.New("CORS origin must look like https://host[:port], without a path")

const (
	corsMethods = "GET, POST, PUT, DELETE"
	corsHeaders = "Authorization, Content-Type, X-API-Key"
	// corsMaxAge is how many seconds a browser may reuse a preflight answer.
	corsMaxAge = "600"
)

// cors lets the pages of a fixed set of other origins call the API from a browser, and nobody else.
//
// Credentials never take part: no Access-Control-Allow-Credentials, because such a page sends no cookies, it
// presents its session as a bearer token.
type cors struct {
	origins map[string]struct{} // normalised, see normalizeOrigin
}

// newCORS takes the configured origins. Blank entries are skipped (a trailing comma, an unset variable);
// any other that is not an origin is an error naming it. No origins means no CORS.
func newCORS(entries []string) (*cors, error) {
	origins := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		origin, err := normalizeOrigin(entry)
		if err != nil {
			return nil, err
		}

		origins[origin] = struct{}{}
	}

	return &cors{origins: origins}, nil
}

// middleware answers preflights and marks the responses to the allowed origins, before authentication and
// routing, so every response a browser is about to read has the headers, errors from the gateway and the
// services included. A request from any other origin (or from none) passes untouched: for it there is no CORS.
func (c *cors) middleware(next http.Handler) http.Handler {
	if len(c.origins) == 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		_, allowed := c.origins[origin]
		if !allowed {
			next.ServeHTTP(w, r)

			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Add("Vary", "Origin")

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", corsMethods)
			h.Set("Access-Control-Allow-Headers", corsHeaders)
			h.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// originPatterns are the allowed origins as WebSocket accept patterns. They are full origins, scheme included,
// so a socket gets the same exact rule as CORS (a bare host would also let http://host in).
func (c *cors) originPatterns() []string {
	patterns := make([]string, 0, len(c.origins))

	for origin := range c.origins {
		patterns = append(patterns, patternEscaper.Replace(origin))
	}

	slices.Sort(patterns)

	return patterns
}

// patternEscaper keeps the brackets of an IPv6 host literal: the patterns are matched with path.Match, where [
// opens a character class. Validation has already kept out the other characters it treats specially.
var patternEscaper = strings.NewReplacer("[", `\[`, "]", `\]`) //nolint:gochecknoglobals

// normalizeOrigin checks that raw is an origin, scheme://host[:port], and returns it the way a browser sends it in
// the Origin header: lower case, IPv6 in canonical form, without the port when it is the scheme's default.
func normalizeOrigin(raw string) (string, error) {
	u, err := parseOrigin(raw)
	if err != nil {
		return "", err
	}

	host, err := originHost(strings.ToLower(u.Hostname()))
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrInvalidOrigin, raw, err)
	}

	port, err := originPort(u.Scheme, u.Port())
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrInvalidOrigin, raw, err)
	}

	return u.Scheme + "://" + host + port, nil
}

// parseOrigin parses raw and checks what is left of an origin once the host and port are set aside.
func parseOrigin(raw string) (*url.URL, error) {
	if strings.ContainsAny(raw, "?#%") {
		return nil, fmt.Errorf("%w: %q has a query, fragment or escape", ErrInvalidOrigin, raw)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidOrigin, raw, err)
	}

	u.Scheme = strings.ToLower(u.Scheme)

	switch {
	case u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS:
		return nil, fmt.Errorf("%w: %q needs the scheme http or https", ErrInvalidOrigin, raw)
	case u.Opaque != "" || u.User != nil || u.Path != "":
		return nil, fmt.Errorf("%w: %q has user info or a path", ErrInvalidOrigin, raw)
	case strings.HasSuffix(u.Host, ":"):
		return nil, fmt.Errorf("%w: %q has an empty port", ErrInvalidOrigin, raw)
	}

	return u, nil
}

var errBadHost = errors.New("the host must be a DNS name or an IP address")

// originHost accepts a DNS name or an IP literal and returns it as a browser writes it.
func originHost(host string) (string, error) {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return dnsName(host)
	}

	switch {
	case addr.Zone() != "":
		return "", errBadHost
	case addr.Is6():
		return "[" + addr.String() + "]", nil
	default:
		return addr.String(), nil
	}
}

// dnsName accepts lower-case letters, digits, - and . (no wildcards, no underscores).
func dnsName(name string) (string, error) {
	if name == "" || name[0] == '.' || name[0] == '-' {
		return "", errBadHost
	}

	for i := range len(name) {
		switch c := name[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.':
		default:
			return "", errBadHost
		}
	}

	return name, nil
}

var errBadPort = errors.New("the port must be a number from 1 to 65535")

// originPort returns ":port", or "" when there is none or it is the default of the scheme (browsers leave it out).
func originPort(scheme, port string) (string, error) {
	if port == "" {
		return "", nil
	}

	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errBadPort
	}

	if (scheme == schemeHTTP && n == 80) || (scheme == schemeHTTPS && n == 443) {
		return "", nil
	}

	return ":" + strconv.Itoa(n), nil
}
