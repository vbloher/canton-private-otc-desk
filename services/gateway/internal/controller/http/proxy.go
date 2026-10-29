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
	dialTimeout           = 5 * time.Second
	responseHeaderTimeout = 15 * time.Second

	userHeaderPrefix = "X-User-"
	cookieHeader     = "Cookie"
)

type cookiePolicy int

const (
	dropCookie cookiePolicy = iota
	keepCookie
)

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

func newProxy(rawURL string, transport http.RoundTripper, cookies cookiePolicy) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse: %w", err)
	}

	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidUpstream, rawURL)
	}

	return &httputil.ReverseProxy{
		Rewrite:      func(pr *httputil.ProxyRequest) { rewrite(pr, target, cookies) },
		Transport:    transport,
		ErrorHandler: upstreamUnavailable,
	}, nil
}

func rewrite(pr *httputil.ProxyRequest, target *url.URL, cookies cookiePolicy) {
	pr.SetURL(target)

	stripCallerHeaders(pr.Out.Header)
	stripCredentials(pr.Out.Header, cookies)

	p, ok := principal.From(pr.In.Context())
	if ok {
		p.SetHeaders(pr.Out.Header)
	}
}

func stripCallerHeaders(h http.Header) {
	principal.StripHeaders(h)

	for name := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), userHeaderPrefix) {
			delete(h, name)
		}
	}
}

func stripCredentials(h http.Header, cookies cookiePolicy) {
	h.Del(domain.APIKeyHeader)

	if cookies == dropCookie {
		h.Del(cookieHeader)
	}
}

func upstreamUnavailable(w http.ResponseWriter, r *http.Request, err error) {
	_ = render.Error(r.Context(), err, "upstream unavailable")

	render.JSON(w, render.Err{Error: "upstream unavailable"}, http.StatusBadGateway)
}
