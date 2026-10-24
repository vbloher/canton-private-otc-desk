package http

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

var ErrInvalidUpstream = errors.New("upstream URL must look like http://host:port")

const (
	// dialTimeout makes a service that is gone (not just refusing connections) fail fast with 502.
	dialTimeout = 5 * time.Second
	// responseHeaderTimeout stays below the HTTP server's write timeout, so a hung service is answered with
	// 502 instead of a connection the server cuts.
	responseHeaderTimeout = 15 * time.Second

	userHeaderPrefix = "X-User-"
	cookieHeader     = "Cookie"
	cacheControl     = "Cache-Control"

	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// sessionPolicy says whether a service is handed the caller's session credentials: the Cookie header and the
// Authorization header, which carries the same session token as a bearer token.
type sessionPolicy int

const (
	// dropSession withholds them: the service trusts the principal headers and has no use for a session token.
	dropSession sessionPolicy = iota
	// keepSession forwards them unchanged: identity owns the sessions and reads the token itself on /auth/logout.
	keepSession
)

// newTransport is shared by the three proxies; it pools connections per service and carries the trace context.
func newTransport() http.RoundTripper {
	return otelhttp.NewTransport(&http.Transport{
		DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	})
}

// newProxy forwards requests unchanged (path, query, body, headers) to the service at rawURL and returns
// its answer unchanged (status, headers including Set-Cookie, body).
// The exceptions: whatever the client claims about who it is gets dropped and the authenticated principal's
// headers are set instead; the API key is never forwarded; the session credentials only if sessions says so;
// and a service's own Cache-Control gives way to the gateway's (see noStore).
func newProxy(rawURL string, transport http.RoundTripper, sessions sessionPolicy) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse: %w", err)
	}

	if (target.Scheme != schemeHTTP && target.Scheme != schemeHTTPS) || target.Host == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidUpstream, rawURL)
	}

	return &httputil.ReverseProxy{
		// Rewrite, not Director: it runs after the hop-by-hop headers are removed, so a client cannot delete
		// the principal's headers by naming them in Connection.
		Rewrite:        func(pr *httputil.ProxyRequest) { rewrite(pr, target, sessions) },
		ModifyResponse: dropCachePolicy,
		Transport:      transport,
		ErrorHandler:   upstreamUnavailable,
	}, nil
}

func rewrite(pr *httputil.ProxyRequest, target *url.URL, sessions sessionPolicy) {
	pr.SetURL(target)

	stripCallerHeaders(pr.Out.Header)
	stripCredentials(pr.Out.Header, sessions)

	p, ok := principal.From(pr.In.Context())
	if ok {
		p.SetHeaders(pr.Out.Header)
	}
}

// stripCallerHeaders removes everything with which a client could pass itself off as somebody else:
// the three principal headers (two of them are not named X-User-*) and any other X-User-* header.
func stripCallerHeaders(h http.Header) {
	principal.StripHeaders(h)

	for name := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), userHeaderPrefix) {
			delete(h, name)
		}
	}
}

// stripCredentials keeps the caller's secrets with the gateway. It authenticates every request itself and tells
// the services who is calling, so no service needs the API key, and a service that is never handed a session
// token (as a cookie or as a bearer token) cannot leak one, however exposed or compromised it is.
func stripCredentials(h http.Header, sessions sessionPolicy) {
	h.Del(domain.APIKeyHeader)

	if sessions == dropSession {
		h.Del(cookieHeader)
		h.Del(authorizationHeader)
	}
}

// dropCachePolicy removes the caching headers a service set. The gateway marks every API response uncacheable
// itself, before it is written, and that must be the only word on the subject.
func dropCachePolicy(res *http.Response) error {
	res.Header.Del(cacheControl)

	return nil
}

func upstreamUnavailable(w http.ResponseWriter, r *http.Request, err error) {
	_ = render.Error(r.Context(), err, "upstream unavailable")

	render.JSON(w, render.Err{Error: "upstream unavailable"}, http.StatusBadGateway)
}
