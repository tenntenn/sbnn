package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const moCSP = "default-src 'self'; script-src 'self' 'unsafe-eval'; connect-src 'self'; frame-ancestors 'none'"

func TestRelaxFrameAncestors(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Security-Policy", moCSP)
	h.Set("X-Frame-Options", "DENY")

	relaxFrameAncestors(h, "http://localhost:6280")

	got := h.Get("Content-Security-Policy")
	if strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("frame-ancestors was not relaxed: %q", got)
	}
	if !strings.Contains(got, "frame-ancestors 'self' http://localhost:6280") {
		t.Errorf("policy = %q", got)
	}
	// Everything else has to survive untouched.
	for _, directive := range []string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-eval'",
		"connect-src 'self'",
	} {
		if !strings.Contains(got, directive) {
			t.Errorf("policy lost %q: %q", directive, got)
		}
	}
	if h.Get("X-Frame-Options") != "" {
		t.Error("X-Frame-Options should be dropped")
	}
}

func TestRelaxFrameAncestorsAddsMissingDirective(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Security-Policy", "default-src 'self'")
	relaxFrameAncestors(h, "http://localhost:6280")
	if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'self' http://localhost:6280") {
		t.Errorf("policy = %q", h.Get("Content-Security-Policy"))
	}
}

func TestRelaxFrameAncestorsKeepsUnsetHeader(t *testing.T) {
	h := http.Header{}
	relaxFrameAncestors(h, "http://localhost:6280")
	if got := h.Get("Content-Security-Policy"); got != "" {
		t.Errorf("policy = %q, want no policy invented for a server that sends none", got)
	}
}

func TestMoProxyServesFramablePages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", moCSP)
		io.WriteString(w, "mo page for "+r.URL.String())
	}))
	defer upstream.Close()

	proxy, err := newMoProxy(upstream.URL, "http://localhost:6280")
	if err != nil {
		t.Fatal(err)
	}
	go proxy.serve()
	defer proxy.close()

	resp, err := http.Get(proxy.baseURL + "/sbnn-default?file=abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "mo page for /sbnn-default?file=abc") {
		t.Errorf("body = %q", body)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'self' http://localhost:6280") {
		t.Errorf("policy = %q", resp.Header.Get("Content-Security-Policy"))
	}
}

func TestMoProxyRewritesURLs(t *testing.T) {
	proxy, err := newMoProxy("http://localhost:6275", "http://localhost:6280")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()

	// mo may answer with either spelling of the loopback host.
	for _, in := range []string{
		"http://localhost:6275/sbnn-default?file=abc",
		"http://127.0.0.1:6275/sbnn-default?file=abc",
	} {
		got := proxy.rewrite(in)
		if !strings.HasPrefix(got, proxy.baseURL) || !strings.HasSuffix(got, "/sbnn-default?file=abc") {
			t.Errorf("rewrite(%q) = %q", in, got)
		}
	}
	// A URL of some other server is none of the proxy's business.
	other := "http://example.com/page"
	if got := proxy.rewrite(other); got != other {
		t.Errorf("rewrite(%q) = %q", other, got)
	}
}

func TestSameEndpoint(t *testing.T) {
	tests := map[string]struct {
		a, b string
		want bool
	}{
		"identical":                 {"http://localhost:6275/x", "http://localhost:6275", true},
		"loopback spellings":        {"http://127.0.0.1:6275/x", "http://localhost:6275", true},
		"ipv6 loopback":             {"http://[::1]:6275/x", "http://localhost:6275", true},
		"no port against http 80":   {"http://localhost/x", "http://localhost:80", true},
		"http 80 against no port":   {"http://127.0.0.1:80/x", "http://localhost", true},
		"both without a port":       {"http://localhost/x", "http://127.0.0.1", true},
		"no port against https 443": {"https://example.com/x", "https://example.com:443", true},
		"different port":            {"http://localhost/x", "http://localhost:6275", false},
		"different scheme default":  {"http://example.com/x", "https://example.com", false},
		"different host":            {"http://example.com/x", "http://localhost:80", false},
		"unknown scheme alike":      {"ftp://localhost/x", "ftp://localhost", true},
		"unknown scheme port":       {"ftp://localhost:21/x", "ftp://localhost", false},
		"relative url":              {"/sbnn-default?file=abc", "http://localhost:6275", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			a, err := url.Parse(tt.a)
			if err != nil {
				t.Fatal(err)
			}
			b, err := url.Parse(tt.b)
			if err != nil {
				t.Fatal(err)
			}
			if got := sameEndpoint(a, b); got != tt.want {
				t.Errorf("sameEndpoint(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMoProxyRewritesURLsWithoutAnExplicitPort(t *testing.T) {
	tests := map[string]struct {
		target string
		raw    string
	}{
		"mo reports no port":       {"http://localhost:80", "http://localhost/sbnn-default?file=abc"},
		"mo is configured no port": {"http://localhost", "http://127.0.0.1:80/sbnn-default?file=abc"},
		"both sides omit the port": {"http://localhost", "http://localhost/sbnn-default?file=abc"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			proxy, err := newMoProxy(tt.target, "http://localhost:6280")
			if err != nil {
				t.Fatal(err)
			}
			defer proxy.close()

			got := proxy.rewrite(tt.raw)
			if !strings.HasPrefix(got, proxy.baseURL) || !strings.HasSuffix(got, "/sbnn-default?file=abc") {
				t.Errorf("rewrite(%q) = %q, want it moved onto %s", tt.raw, got, proxy.baseURL)
			}
		})
	}
}
