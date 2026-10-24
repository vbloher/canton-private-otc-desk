package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_bearerToken(t *testing.T) {
	cases := []struct {
		name   string
		header []string // the values of the Authorization header
		want   string
	}{
		{name: "a token", header: []string{"Bearer abc"}, want: "abc"},
		{name: "a session token", header: []string{"Bearer Zm9v-YmFy_Zm9vYmFyZm9vYmFyZm9vYmFyZm9vYmE"}, want: "Zm9v-YmFy_Zm9vYmFyZm9vYmFyZm9vYmFyZm9vYmE"},
		{name: "lower case scheme", header: []string{"bearer abc"}, want: "abc"},
		{name: "upper case scheme", header: []string{"BEARER abc"}, want: "abc"},
		{name: "extra spaces around", header: []string{"  Bearer   abc  "}, want: "abc"},
		{name: "no header", header: nil, want: ""},
		{name: "empty header", header: []string{""}, want: ""},
		{name: "scheme only", header: []string{"Bearer"}, want: ""},
		{name: "scheme and spaces only", header: []string{"Bearer    "}, want: ""},
		{name: "another scheme", header: []string{"Basic dXNlcjpwYXNz"}, want: ""},
		{name: "token without a scheme", header: []string{"abc"}, want: ""},
		{name: "two tokens", header: []string{"Bearer abc def"}, want: ""},
		{name: "a tab inside the token", header: []string{"Bearer abc\tdef"}, want: ""},
		{name: "a tab instead of the space", header: []string{"Bearer\tabc"}, want: ""},
		{name: "scheme glued to the token", header: []string{"Bearerabc"}, want: ""},
		{name: "two headers are ambiguous", header: []string{"Bearer abc", "Bearer def"}, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			header := http.Header{}
			for _, value := range c.header {
				header.Add("Authorization", value)
			}

			require.Equal(t, c.want, bearerToken(header))
		})
	}
}

func Test_BearerMiddleware_PutsTheTokenIntoTheContext(t *testing.T) {
	var seen string

	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = bearerFrom(r.Context())
	})

	for header, want := range map[string]string{
		"Bearer the-token": "the-token",
		"Basic abc":        "",
		"Bearer":           "",
		"":                 "",
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}

		seen = "untouched"

		BearerMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, want, seen, "%q", header)
	}
}

func Test_BearerMiddleware_NeverRejects(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "garbage")

	rec := httptest.NewRecorder()
	BearerMiddleware(next).ServeHTTP(rec, req)

	require.True(t, called)
	require.Equal(t, http.StatusTeapot, rec.Code)
}
