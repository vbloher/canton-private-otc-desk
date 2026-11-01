package desk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const sessionCookie = "otc_session"

type session struct {
	base  http.RoundTripper
	renew func(ctx context.Context) error

	mu    sync.Mutex
	token string

	renewing sync.Mutex
}

func (s *session) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.token
}

func (s *session) store(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.token = token
}

func (s *session) RoundTrip(req *http.Request) (*http.Response, error) {
	token := s.current()

	resp, err := s.send(req, token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized || strings.Contains(req.URL.Path, apiPrefix+"/auth/") {
		return resp, nil
	}

	retry, ok := rewind(req)
	if !ok || !s.renewed(req.Context(), token) {
		return resp, nil
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return s.send(retry, s.current())
}

func (s *session) renewed(ctx context.Context, stale string) bool {
	return s.renewFrom(ctx, stale) == nil
}

func (s *session) renewFrom(ctx context.Context, stale string) error {
	s.renewing.Lock()
	defer s.renewing.Unlock()

	if s.current() != stale {
		return nil
	}

	return s.renew(ctx)
}

func (s *session) send(req *http.Request, token string) (*http.Response, error) {
	out := req.Clone(req.Context())
	if token != "" {
		out.Header.Set("Cookie", sessionCookie+"="+token)
	}

	resp, err := s.base.RoundTrip(out)
	if err != nil {
		return nil, fmt.Errorf("base.RoundTrip: %w", err)
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == sessionCookie && cookie.Value != "" {
			s.store(cookie.Value)
		}
	}

	return resp, nil
}

func rewind(req *http.Request) (*http.Request, bool) {
	retry := req.Clone(req.Context())

	if req.Body == nil || req.Body == http.NoBody {
		return retry, true
	}

	if req.GetBody == nil {
		return nil, false
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}

	retry.Body = body

	return retry, true
}
