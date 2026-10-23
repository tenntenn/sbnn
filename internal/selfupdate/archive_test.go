package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestArchiveName(t *testing.T) {
	for _, tt := range []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{"darwin", "arm64", "sbnn_Darwin_arm64.tar.gz", false},
		{"darwin", "amd64", "sbnn_Darwin_x86_64.tar.gz", false},
		{"linux", "amd64", "sbnn_Linux_x86_64.tar.gz", false},
		{"linux", "arm64", "sbnn_Linux_arm64.tar.gz", false},
		{"windows", "amd64", "sbnn_Windows_x86_64.zip", false},
		{"windows", "arm64", "sbnn_Windows_arm64.zip", false},
		{"linux", "386", "", true},
		{"freebsd", "amd64", "", true},
	} {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got, err := ArchiveName(tt.goos, tt.goarch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ArchiveName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ArchiveName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("windows"); got != "sbnn.exe" {
		t.Errorf("BinaryName(windows) = %q", got)
	}
	if got := BinaryName("linux"); got != "sbnn" {
		t.Errorf("BinaryName(linux) = %q", got)
	}
}

func sumLine(data []byte, name string) string {
	return fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name)
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("the archive")
	const name = "sbnn_Linux_x86_64.tar.gz"
	other := sumLine([]byte("another archive"), "sbnn_Darwin_arm64.tar.gz")
	for _, tt := range []struct {
		name    string
		sums    string
		wantErr string
	}{
		{"match", other + sumLine(data, name), ""},
		{"match in binary mode", strings.Replace(sumLine(data, name), "  ", " *", 1), ""},
		{"mismatch", sumLine([]byte("tampered"), name), "does not match"},
		{"missing", other, "no entry"},
		{"empty", "", "no entry"},
		{"malformed digest", "zz  " + name + "\n", "malformed"},
		{"short digest", "abcd  " + name + "\n", "malformed"},
		{"listed twice differently", sumLine(data, name) + sumLine([]byte("x"), name), "twice"},
		{"name is a suffix of another", sumLine(data, "x"+name), "no entry"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyChecksum([]byte(tt.sums), name, data)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("VerifyChecksum() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("VerifyChecksum() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

// tarEntry is one entry of a test archive.
type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
	// size overrides the size the header claims, when it is not zero.
	size int64
}

func makeTarGz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typeflag
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Typeflag: typ, Linkname: e.linkname, Mode: 0o755}
		if typ == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name string
	body string
	mode fs.FileMode
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if mode == 0 {
			mode = 0o755
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTarGz(t *testing.T) {
	const bin = "sbnn"
	for _, tt := range []struct {
		name    string
		entries []tarEntry
		max     int64
		want    string
		wantErr string
	}{
		{
			name: "the binary among the other files",
			entries: []tarEntry{
				{name: "README.md", body: "readme"},
				{name: "sbnn", body: "new binary"},
				{name: "LICENSE", body: "license"},
			},
			want: "new binary",
		},
		{
			name:    "a leading ./ is the same entry",
			entries: []tarEntry{{name: "./sbnn", body: "new binary"}},
			want:    "new binary",
		},
		{
			name:    "no binary",
			entries: []tarEntry{{name: "README.md", body: "readme"}},
			wantErr: "no sbnn binary",
		},
		{
			name:    "binary in a subdirectory is not the binary",
			entries: []tarEntry{{name: "bin/sbnn", body: "x"}},
			wantErr: "no sbnn binary",
		},
		{
			name: "traversal in another entry refuses the archive",
			entries: []tarEntry{
				{name: "sbnn", body: "new binary"},
				{name: "../../etc/evil", body: "x"},
			},
			wantErr: "outside the archive",
		},
		{
			name:    "traversal that cleans to the binary",
			entries: []tarEntry{{name: "x/../../sbnn", body: "x"}},
			wantErr: "outside the archive",
		},
		{
			name:    "absolute path",
			entries: []tarEntry{{name: "/usr/local/bin/sbnn", body: "x"}},
			wantErr: "not a relative path",
		},
		{
			name:    "backslash path",
			entries: []tarEntry{{name: `..\sbnn`, body: "x"}},
			wantErr: "not a relative path",
		},
		{
			name:    "symlink entry",
			entries: []tarEntry{{name: "sbnn", typeflag: tar.TypeSymlink, linkname: "/bin/sh"}},
			wantErr: "not a regular file",
		},
		{
			name:    "hard link entry",
			entries: []tarEntry{{name: "sbnn", typeflag: tar.TypeLink, linkname: "README.md"}},
			wantErr: "not a regular file",
		},
		{
			name:    "directory entry",
			entries: []tarEntry{{name: "sbnn", typeflag: tar.TypeDir}},
			wantErr: "not a regular file",
		},
		{
			name:    "oversize",
			entries: []tarEntry{{name: "sbnn", body: strings.Repeat("x", 11)}},
			max:     10,
			wantErr: "more than the 10",
		},
		{
			name: "twice",
			entries: []tarEntry{
				{name: "sbnn", body: "one"},
				{name: "./sbnn", body: "two"},
			},
			wantErr: "twice",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			max := tt.max
			if max == 0 {
				max = MaxBinary
			}
			var out bytes.Buffer
			err := Extract(makeTarGz(t, tt.entries...), "sbnn_Linux_x86_64.tar.gz", bin, &out, max)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Extract() = %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract() = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("extracted %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestExtractZip(t *testing.T) {
	const bin = "sbnn.exe"
	for _, tt := range []struct {
		name    string
		entries []zipEntry
		max     int64
		want    string
		wantErr string
	}{
		{
			name: "the binary among the other files",
			entries: []zipEntry{
				{name: "README.md", body: "readme"},
				{name: "sbnn.exe", body: "new binary"},
			},
			want: "new binary",
		},
		{
			name:    "no binary",
			entries: []zipEntry{{name: "sbnn", body: "the unix name"}},
			wantErr: "no sbnn binary",
		},
		{
			name: "traversal",
			entries: []zipEntry{
				{name: "sbnn.exe", body: "x"},
				{name: "../evil.exe", body: "x"},
			},
			wantErr: "outside the archive",
		},
		{
			name:    "drive letter",
			entries: []zipEntry{{name: "C:/sbnn.exe", body: "x"}},
			wantErr: "not a relative path",
		},
		{
			name:    "symlink entry",
			entries: []zipEntry{{name: "sbnn.exe", body: "C:/Windows/evil.exe", mode: fs.ModeSymlink | 0o777}},
			wantErr: "not a regular file",
		},
		{
			name:    "oversize",
			entries: []zipEntry{{name: "sbnn.exe", body: strings.Repeat("x", 11)}},
			max:     10,
			wantErr: "more than the 10",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			max := tt.max
			if max == 0 {
				max = MaxBinary
			}
			var out bytes.Buffer
			err := Extract(makeZip(t, tt.entries...), "sbnn_Windows_x86_64.zip", bin, &out, max)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Extract() = %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract() = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("extracted %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestExtractUnknownFormat(t *testing.T) {
	if err := Extract(nil, "sbnn.7z", "sbnn", &bytes.Buffer{}, MaxBinary); err == nil {
		t.Fatal("Extract() of an unknown format = nil, want an error")
	}
}
