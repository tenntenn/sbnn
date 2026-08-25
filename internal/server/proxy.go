package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// moProxy publishes the mo server through a loopback-only origin owned by sbnn.
//
// mo answers every request with "frame-ancestors 'none'", which forbids any
// page from framing it. sbnn needs the preview next to the diff in the same
// window, so the proxy relaxes exactly that directive to sbnn's own origin and
// forwards everything else untouched.
type moProxy struct {
	ln      net.Listener
	baseURL string
	target  *url.URL
	srv     *http.Server
}

func newMoProxy(targetURL, allowedOrigin string) (*moProxy, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid mo URL %q: %w", targetURL, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cannot listen for the preview proxy: %w", err)
	}
	p := &moProxy{
		ln:      ln,
		target:  target,
		baseURL: "http://" + ln.Addr().String(),
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	// mo streams live reload events; do not buffer them.
	rp.FlushInterval = -1
	rp.ModifyResponse = func(resp *http.Response) error {
		relaxFrameAncestors(resp.Header, allowedOrigin, p.baseURL)
		return nil
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "cannot reach the mo server at "+target.String()+": "+err.Error(), http.StatusBadGateway)
	}

	p.srv = &http.Server{
		Handler:           rp,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return p, nil
}

func (p *moProxy) serve() {
	_ = p.srv.Serve(p.ln)
}

func (p *moProxy) close() {
	_ = p.srv.Close()
}

// rewrite maps a URL of the mo server onto the proxy origin so that it can be
// framed. URLs that do not belong to mo are returned unchanged.
func (p *moProxy) rewrite(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if !sameEndpoint(u, p.target) {
		return raw
	}
	proxyURL, err := url.Parse(p.baseURL)
	if err != nil {
		return raw
	}
	u.Scheme, u.Host = proxyURL.Scheme, proxyURL.Host
	return u.String()
}

// sameEndpoint reports whether two URLs point at the same host:port, treating
// localhost and the loopback addresses as equal. A URL that spells out no port
// is compared on its scheme's default port, so "http://localhost/x" and
// "http://127.0.0.1:80/x" are the same endpoint.
func sameEndpoint(a, b *url.URL) bool {
	// Spelled exactly alike, including a scheme we may know no default port
	// for. Anything else has to go through the port defaulting below, which
	// is what tells http://example.com and https://example.com apart.
	if a.Host == b.Host && strings.EqualFold(a.Scheme, b.Scheme) {
		return true
	}
	ah, ap := hostPort(a)
	bh, bp := hostPort(b)
	if ap == "" || bp == "" {
		// A scheme with no known default port (or none at all) gives us
		// nothing to compare, so do not claim the URL belongs to mo.
		return false
	}
	return ap == bp && ah == bh
}

// hostPort splits a URL into its normalised host and its port, filling in the
// scheme's default port when the URL does not carry one. The port is empty
// when the scheme has no default, which callers must treat as unknown.
func hostPort(u *url.URL) (host, port string) {
	port = u.Port()
	if port == "" {
		port = defaultPort(u.Scheme)
	}
	return normalizeHost(u.Hostname()), port
}

// defaultPort returns the port a scheme implies when a URL omits it.
func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}
	return ""
}

func normalizeHost(h string) string {
	switch strings.ToLower(h) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return "localhost"
	}
	return strings.ToLower(h)
}

// relaxFrameAncestors rewrites the frame-ancestors directive of a Content
// Security Policy so that the given origins may frame the response. Every
// other directive is left exactly as the upstream server sent it.
func relaxFrameAncestors(h http.Header, origins ...string) {
	// X-Frame-Options has no origin list worth keeping; the CSP below is
	// what actually governs framing for browsers that support it.
	h.Del("X-Frame-Options")

	policies := h.Values("Content-Security-Policy")
	if len(policies) == 0 {
		return
	}
	var allow strings.Builder
	allow.WriteString("frame-ancestors 'self'")
	for _, o := range origins {
		if o != "" {
			allow.WriteString(" " + o)
		}
	}
	allowed := allow.String()
	rewritten := make([]string, 0, len(policies))
	for _, policy := range policies {
		directives := strings.Split(policy, ";")
		out := make([]string, 0, len(directives))
		replaced := false
		for _, d := range directives {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(d)), "frame-ancestors") {
				out = append(out, " "+allowed)
				replaced = true
				continue
			}
			out = append(out, d)
		}
		if !replaced {
			out = append(out, " "+allowed)
		}
		rewritten = append(rewritten, strings.Join(out, ";"))
	}
	h.Del("Content-Security-Policy")
	for _, p := range rewritten {
		h.Add("Content-Security-Policy", p)
	}
}
