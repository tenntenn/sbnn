package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/selfupdate"
)

// releaseArchive builds the archive GoReleaser would publish for goos, with
// body as the sbnn binary.
func releaseArchive(t *testing.T, goos, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&buf)
		for name, content := range map[string]string{"README.md": "readme", "sbnn.exe": body} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte(content))
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{{"README.md", "readme"}, {"sbnn", body}} {
		tw.WriteHeader(&tar.Header{Name: f.name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(f.body))})
		tw.Write([]byte(f.body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeReleases serves one release of every platform the way GitHub does:
// the API answers with the tag, the download URLs with the files.
func fakeReleases(t *testing.T, tag string, tamper bool) *httptest.Server {
	t.Helper()
	files := map[string][]byte{}
	var sums strings.Builder
	for _, p := range [][2]string{{"linux", "amd64"}, {"windows", "amd64"}} {
		name, err := selfupdate.ArchiveName(p[0], p[1])
		if err != nil {
			t.Fatal(err)
		}
		files[name] = releaseArchive(t, p[0], "sbnn "+tag)
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
		if tamper {
			files[name] = releaseArchive(t, p[0], "something else")
		}
	}
	files["checksums.txt"] = []byte(sums.String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/"+selfupdate.Repo+"/releases/latest",
			r.URL.Path == "/repos/"+selfupdate.Repo+"/releases/tags/"+tag:
			fmt.Fprintf(w, `{"tag_name":%q}`, tag)
		case strings.HasPrefix(r.URL.Path, "/"+selfupdate.Repo+"/releases/download/"+tag+"/"):
			b, ok := files[strings.TrimPrefix(r.URL.Path, "/"+selfupdate.Repo+"/releases/download/"+tag+"/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeSource(srv *httptest.Server) *selfupdate.Source {
	host := strings.TrimPrefix(srv.URL, "http://")
	return &selfupdate.Source{
		APIBase:      srv.URL,
		DownloadBase: srv.URL,
		Allow:        func(u *url.URL) bool { return u.Host == host },
	}
}

// fakeExecutable is a stand-in for the running sbnn, in t.TempDir: the test
// binary itself is never what gets replaced.
func fakeExecutable(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "sbnn")
	if err := os.WriteFile(exe, []byte("sbnn old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestDoUpdate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		tag         string
		tamper      bool
		current     string
		goInstalled bool
		goos        string
		checkOnly   bool
		version     string
		servers     []serverListing
		wantErr     string
		wantOut     []string
		wantBinary  string // "" means the old binary must be untouched
	}{
		{
			name:       "installs a newer release",
			tag:        "v1.1.0",
			current:    "1.0.0",
			goos:       "linux",
			wantOut:    []string{"sbnn 1.0.0 -> v1.1.0"},
			wantBinary: "sbnn v1.1.0",
		},
		{
			name:       "installs a newer release on Windows",
			tag:        "v1.1.0",
			current:    "1.0.0",
			goos:       "windows",
			wantOut:    []string{"sbnn 1.0.0 -> v1.1.0"},
			wantBinary: "sbnn v1.1.0",
		},
		{
			name:    "a tampered archive replaces nothing",
			tag:     "v1.1.0",
			tamper:  true,
			current: "1.0.0",
			goos:    "linux",
			wantErr: "does not match its checksum",
		},
		{
			name:    "the same release does nothing",
			tag:     "v1.0.0",
			current: "1.0.0",
			goos:    "linux",
			wantOut: []string{"already installed"},
		},
		{
			name:    "never downgrades on its own",
			tag:     "v1.0.0",
			current: "1.1.0-rc.1",
			goos:    "linux",
			wantOut: []string{"newer than the latest release"},
		},
		{
			name:       "downgrades when --version names the release",
			tag:        "v1.0.0",
			current:    "1.1.0",
			goos:       "linux",
			version:    "1.0.0",
			wantOut:    []string{"sbnn 1.1.0 -> v1.0.0"},
			wantBinary: "sbnn v1.0.0",
		},
		{
			name:    "a release that does not exist",
			tag:     "v1.0.0",
			current: "1.1.0",
			goos:    "linux",
			version: "v0.9.0",
			wantErr: "no release v0.9.0",
		},
		{
			name:        "a go install build is told to use go install",
			tag:         "v1.1.0",
			current:     "1.0.0",
			goInstalled: true,
			goos:        "linux",
			wantErr:     goInstallHint,
		},
		{
			name:    "a source build is not replaced",
			tag:     "v1.1.0",
			current: "dev",
			goos:    "linux",
			wantErr: "source build",
		},
		{
			name:      "--check only reports",
			tag:       "v1.1.0",
			current:   "1.0.0",
			goos:      "linux",
			checkOnly: true,
			wantOut:   []string{"v1.1.0 is available: run sbnn update"},
		},
		{
			name:        "--check on a go install build names go install",
			tag:         "v1.1.0",
			current:     "1.0.0",
			goInstalled: true,
			goos:        "linux",
			checkOnly:   true,
			wantOut:     []string{"run " + goInstallHint},
		},
		{
			name:      "--check on a source build names the release",
			tag:       "v1.1.0",
			current:   "dev",
			goos:      "linux",
			checkOnly: true,
			wantOut:   []string{"source build; the latest release is v1.1.0"},
		},
		{
			name:    "a running server is told to restart",
			tag:     "v1.1.0",
			current: "1.0.0",
			goos:    "linux",
			servers: []serverListing{
				{Port: 6280, URL: "http://localhost:6280", Running: true, Version: "1.0.0"},
				{Port: 6281, URL: "http://localhost:6281", Running: true, Version: "1.1.0"},
			},
			wantOut:    []string{"http://localhost:6280", "sbnn --restart --port 6280"},
			wantBinary: "sbnn v1.1.0",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeReleases(t, tt.tag, tt.tamper)
			exe := fakeExecutable(t)
			var out bytes.Buffer
			err := doUpdate(t.Context(), &out, updateOptions{
				current:     tt.current,
				goInstalled: tt.goInstalled,
				goos:        tt.goos,
				goarch:      "amd64",
				checkOnly:   tt.checkOnly,
				version:     tt.version,
				source:      fakeSource(srv),
				executable:  func() (string, error) { return exe, nil },
				servers:     func(context.Context) []serverListing { return tt.servers },
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("doUpdate() = %v, want an error containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("doUpdate() = %v", err)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q does not contain %q", out.String(), want)
				}
			}
			if strings.Contains(out.String(), "6281") {
				t.Errorf("a server already on the new release was told to restart: %q", out.String())
			}
			b, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			want := tt.wantBinary
			if want == "" {
				want = "sbnn old"
			}
			if string(b) != want {
				t.Errorf("the executable holds %q, want %q", b, want)
			}
		})
	}
}

func TestUpdateCheckSkipped(t *testing.T) {
	newCmd := func(name string, flags map[string]string) *cobra.Command {
		c := &cobra.Command{Use: name}
		c.Flags().Bool("json", false, "")
		c.Flags().Bool("foreground", false, "")
		c.Flags().String("format", "prompt", "")
		for k, v := range flags {
			if err := c.Flags().Set(k, v); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for _, tt := range []struct {
		name    string
		cmd     *cobra.Command
		env     updateCheckEnv
		skipped bool
	}{
		{"a release at a terminal is checked", newCmd("sbnn", nil),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, false},
		{"SBNN_NO_UPDATE_CHECK", newCmd("sbnn", nil),
			updateCheckEnv{getenv: env(map[string]string{NoUpdateCheckEnv: "1"}), current: "1.0.0", stderrTerm: true}, true},
		{"a source build", newCmd("sbnn", nil),
			updateCheckEnv{getenv: env(nil), current: "dev", stderrTerm: true}, true},
		{"stderr is not a terminal", newCmd("sbnn", nil),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: false}, true},
		{"a hook's child", newCmd("comments", nil),
			updateCheckEnv{getenv: env(map[string]string{hookChildEnv: "http://localhost:6280"}), current: "1.0.0", stderrTerm: true}, true},
		{"--json", newCmd("comments", map[string]string{"json": "true"}),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, true},
		{"--format json", newCmd("comments", map[string]string{"format": "json"}),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, true},
		{"--format markdown is checked", newCmd("comments", map[string]string{"format": "markdown"}),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, false},
		{"the server", newCmd("sbnn", map[string]string{"foreground": "true"}),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, true},
		{"sbnn update itself", newCmd("update", nil),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, true},
		{"shell completion", newCmd(cobra.ShellCompRequestCmd, nil),
			updateCheckEnv{getenv: env(nil), current: "1.0.0", stderrTerm: true}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reason := updateCheckSkipped(tt.cmd, tt.env)
			if (reason != "") != tt.skipped {
				t.Errorf("updateCheckSkipped() = %q, want skipped %v", reason, tt.skipped)
			}
		})
	}
}

func TestPrintUpdateNotice(t *testing.T) {
	for _, tt := range []struct {
		name        string
		goInstalled bool
		want        string
	}{
		{"release binary", false, "run sbnn update to update"},
		{"go install build", true, "run " + goInstallHint + " to update"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "update-check.json")
			if err := selfupdate.SaveCache(path, selfupdate.Cache{CheckedAt: time.Now(), Latest: "v1.1.0"}); err != nil {
				t.Fatal(err)
			}
			c := &selfupdate.Check{
				Path:    path,
				Current: "1.0.0",
				Latest: func(context.Context) (string, error) {
					t.Error("a fresh cache asked GitHub")
					return "", nil
				},
			}
			c.Start(t.Context())
			var out bytes.Buffer
			printUpdateNotice(&out, c, tt.goInstalled)
			got := out.String()
			if strings.Count(got, "\n") != 1 || !strings.Contains(got, "v1.1.0 is available") || !strings.Contains(got, tt.want) {
				t.Errorf("notice = %q, want one line containing %q", got, tt.want)
			}
			out.Reset()
			printUpdateNotice(&out, c, tt.goInstalled)
			if out.Len() != 0 {
				t.Errorf("told twice: %q", out.String())
			}
		})
	}
	var out bytes.Buffer
	printUpdateNotice(&out, nil, false)
	if out.Len() != 0 {
		t.Errorf("a skipped check printed %q", out.String())
	}
}
