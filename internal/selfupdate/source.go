// Package selfupdate finds sbnn's releases on GitHub, downloads the archive
// for the running platform, verifies it, and puts the binary inside it in
// place of the running executable.
//
// It replaces an executable, so it is deliberately narrow about what it
// talks to and what it accepts: HTTPS to GitHub and nothing else, a timeout
// and a size cap on every response, the archive checked against the
// release's own checksums.txt before a byte of it is unpacked, and only the
// one binary taken out of it.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/version"
)

// Repo is the GitHub repository sbnn's releases are published in.
const Repo = "tenntenn/sbnn"

// The places a release is read from. The API says which release is the
// latest; the archives and checksums.txt are downloaded from the release's
// download URLs, which GitHub answers with a redirect to its asset storage.
const (
	DefaultAPIBase      = "https://api.github.com"
	DefaultDownloadBase = "https://github.com"
)

// Size caps. A response larger than these is not what GitHub sends for sbnn,
// so it is refused rather than read to the end.
const (
	maxAPIResponse = 1 << 20
	maxChecksums   = 64 << 10
	// MaxArchive bounds a downloaded archive. The release archives are a few
	// megabytes; this leaves room for growth and none for abuse.
	MaxArchive = 64 << 20
	// MaxBinary bounds the binary unpacked from an archive.
	MaxBinary = 192 << 20
)

// Timeouts for one request, redirects and body included.
const (
	apiTimeout      = 15 * time.Second
	downloadTimeout = 5 * time.Minute
)

// maxRedirects is how many redirects one download may follow. GitHub sends
// one, from the release download URL to its asset storage.
const maxRedirects = 5

// githubHosts are the only hosts a request may go to, the first one or one a
// redirect leads to: the API, the release download URLs, and the two hosts
// GitHub serves release assets from.
var githubHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// allowGitHub is the default Source.Allow: HTTPS, on the default port, to one
// of githubHosts, with no credentials in the URL.
func allowGitHub(u *url.URL) bool {
	return u.Scheme == "https" && githubHosts[u.Hostname()] &&
		(u.Port() == "" || u.Port() == "443") && u.User == nil
}

// Source is where releases are read from.
type Source struct {
	// APIBase and DownloadBase are DefaultAPIBase and DefaultDownloadBase,
	// except in tests, which point them at an httptest server.
	APIBase      string
	DownloadBase string
	// Token is sent to the API, and only to the API, as a bearer token. It
	// only raises the rate limit: the releases of a public repository need
	// no authentication.
	Token string
	// Allow decides whether a request may be sent to a URL, both the first
	// one and every one a redirect leads to. Nil means allowGitHub; tests
	// replace it to reach their httptest server. Changing the bases alone
	// therefore reaches nothing but GitHub.
	Allow func(u *url.URL) bool
}

// NewSource returns the Source of sbnn's real releases.
func NewSource(token string) *Source {
	return &Source{APIBase: DefaultAPIBase, DownloadBase: DefaultDownloadBase, Token: token}
}

func (s *Source) allowed(u *url.URL) bool {
	if s.Allow != nil {
		return s.Allow(u)
	}
	return allowGitHub(u)
}

// apiHost is the host the token may be sent to.
func (s *Source) apiHost() string {
	u, err := url.Parse(s.APIBase)
	if err != nil {
		return ""
	}
	return u.Host
}

// client returns an HTTP client that gives up after timeout and refuses a
// redirect to anywhere Allow does not accept.
func (s *Source) client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if !s.allowed(req.URL) {
				return fmt.Errorf("refusing to follow a redirect to %s", redact(req.URL))
			}
			// net/http already drops Authorization on a redirect to
			// another domain; this also drops it on a redirect to another
			// host of the same one, because the token is for the API and
			// nothing else.
			if req.URL.Host != s.apiHost() {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

// redact prints a URL without its query, which is where asset storage puts
// its signatures, and without any credentials.
func redact(u *url.URL) string {
	c := *u
	c.RawQuery, c.User = "", nil
	return c.String()
}

// errNotFound is returned for a 404, which for a release means the tag or the
// asset does not exist.
var errNotFound = errors.New("not found")

// get fetches raw and returns its body, refusing anything larger than max.
func (s *Source) get(ctx context.Context, raw string, timeout time.Duration, max int64) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !s.allowed(u) {
		return nil, fmt.Errorf("refusing to download from %s", redact(u))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sbnn/"+version.Version)
	if u.Host == s.apiHost() {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if s.Token != "" {
			req.Header.Set("Authorization", "Bearer "+s.Token)
		}
	}
	resp, err := s.client(timeout).Do(req)
	if err != nil {
		// The URL in the error may be the one a redirect led to, signature
		// and all; it is printed, so it is printed without its query.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			if p, perr := url.Parse(ue.URL); perr == nil {
				ue.URL = redact(p)
			}
		}
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", redact(u), errNotFound)
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%s: %s (the GitHub API rate limit may be used up; setting GITHUB_TOKEN raises it)", redact(u), resp.Status)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s: %s", redact(u), resp.Status)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("%s: %d bytes is more than the %d allowed", redact(u), resp.ContentLength, max)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", redact(u), err)
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s: more than the %d bytes allowed", redact(u), max)
	}
	return b, nil
}

// Release is one published release of sbnn.
type Release struct {
	// Tag is the git tag, such as v1.2.3, which is also the release's
	// version.
	Tag string
}

// Latest returns the latest release: the newest one that is neither a draft
// nor a pre-release, which is what GitHub means by latest.
func (s *Source) Latest(ctx context.Context) (Release, error) {
	return s.release(ctx, s.APIBase+"/repos/"+Repo+"/releases/latest")
}

// Tagged returns the release of tag, which must be a version such as v1.2.3.
func (s *Source) Tagged(ctx context.Context, tag string) (Release, error) {
	if Canonical(tag) == "" || !strings.HasPrefix(tag, "v") {
		return Release{}, fmt.Errorf("%q is not a version", tag)
	}
	rel, err := s.release(ctx, s.APIBase+"/repos/"+Repo+"/releases/tags/"+url.PathEscape(tag))
	if errors.Is(err, errNotFound) {
		return Release{}, fmt.Errorf("there is no release %s", tag)
	}
	return rel, err
}

func (s *Source) release(ctx context.Context, raw string) (Release, error) {
	b, err := s.get(ctx, raw, apiTimeout, maxAPIResponse)
	if err != nil {
		return Release{}, err
	}
	var body struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return Release{}, fmt.Errorf("reading the release: %w", err)
	}
	// The tag goes into a download URL next, so it is held to the shape of
	// a version here rather than trusted as whatever the API said.
	if Canonical(body.TagName) == "" || !strings.HasPrefix(body.TagName, "v") {
		return Release{}, fmt.Errorf("the release is tagged %q, which is not a version", body.TagName)
	}
	if body.Draft {
		return Release{}, fmt.Errorf("release %s is a draft", body.TagName)
	}
	return Release{Tag: body.TagName}, nil
}

// assetURL is the download URL of one file attached to a release.
func (s *Source) assetURL(rel Release, name string) string {
	return s.DownloadBase + "/" + Repo + "/releases/download/" + url.PathEscape(rel.Tag) + "/" + url.PathEscape(name)
}

// Download fetches the archive called name from rel and returns it once it
// matches the SHA-256 that rel's checksums.txt records for it. An archive
// that does not match, or that checksums.txt does not mention, is never
// returned.
func (s *Source) Download(ctx context.Context, rel Release, name string) ([]byte, error) {
	sums, err := s.get(ctx, s.assetURL(rel, "checksums.txt"), apiTimeout, maxChecksums)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, fmt.Errorf("release %s has no checksums.txt, so its archives cannot be verified", rel.Tag)
		}
		return nil, fmt.Errorf("downloading checksums.txt: %w", err)
	}
	archive, err := s.get(ctx, s.assetURL(rel, name), downloadTimeout, MaxArchive)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, fmt.Errorf("release %s has no %s", rel.Tag, name)
		}
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	if err := VerifyChecksum(sums, name, archive); err != nil {
		return nil, err
	}
	return archive, nil
}
