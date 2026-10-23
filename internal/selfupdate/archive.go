package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// ArchiveName is the name GoReleaser gives the archive for goos and goarch
// (see .goreleaser.yml): sbnn_Darwin_arm64.tar.gz, sbnn_Linux_x86_64.tar.gz,
// sbnn_Windows_x86_64.zip. A platform the releases do not cover is an error
// rather than a guess, so it fails before anything is downloaded.
func ArchiveName(goos, goarch string) (string, error) {
	var osName string
	switch goos {
	case "darwin", "linux", "windows":
		osName = strings.ToUpper(goos[:1]) + goos[1:]
	default:
		return "", fmt.Errorf("there is no release of sbnn for %s", goos)
	}
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("there is no release of sbnn for %s/%s", goos, goarch)
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "sbnn_" + osName + "_" + arch + ext, nil
}

// BinaryName is the name of the executable inside the archive for goos.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "sbnn.exe"
	}
	return "sbnn"
}

// VerifyChecksum checks data against the SHA-256 that sums, a checksums.txt
// in the format sha256sum writes, records for name. A name sums does not
// mention, or mentions twice with different digests, is refused as surely as
// a digest that does not match: an archive that cannot be verified is not
// installed.
func VerifyChecksum(sums []byte, name string, data []byte) error {
	var want []byte
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// sha256sum marks a file read in binary mode with a leading "*".
		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return fmt.Errorf("checksums.txt has a malformed entry for %s", name)
		}
		if want != nil && !bytes.Equal(want, digest) {
			return fmt.Errorf("checksums.txt lists %s twice with different digests", name)
		}
		want = digest
	}
	if want == nil {
		return fmt.Errorf("checksums.txt has no entry for %s, so it cannot be verified", name)
	}
	got := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return fmt.Errorf("%s does not match its checksum (sha256 %x, checksums.txt says %x); nothing was replaced", name, got, want)
	}
	return nil
}

// maxUnpacked bounds everything read out of a compressed archive, the
// entries that are skipped included, so that a small archive that inflates
// to an enormous one is cut off rather than decompressed to the end.
const maxUnpacked = 2 * MaxBinary

// Extract writes the entry called bin out of archive, a .tar.gz or a .zip
// as archiveName says, to w, refusing it when it is larger than max.
//
// Only bin is ever written, and only when it is a regular file at the top of
// the archive. An archive with an entry that climbs out of it (an absolute
// path, a "..") is refused as a whole, even when that entry is not the one
// wanted: the archive has already been verified against checksums.txt, so
// such an entry means the release itself is not what sbnn's releases look
// like.
func Extract(archive []byte, archiveName, bin string, w io.Writer, max int64) error {
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		return extractTarGz(archive, bin, w, max)
	case strings.HasSuffix(archiveName, ".zip"):
		return extractZip(archive, bin, w, max)
	default:
		return fmt.Errorf("%s is not an archive sbnn knows how to unpack", archiveName)
	}
}

// entryName checks the name of an archive entry and returns it cleaned.
func entryName(name string) (string, error) {
	if name == "" || strings.Contains(name, `\`) || path.IsAbs(name) ||
		(len(name) >= 2 && name[1] == ':') {
		return "", fmt.Errorf("archive entry %q is not a relative path", name)
	}
	for elem := range strings.SplitSeq(name, "/") {
		if elem == ".." {
			return "", fmt.Errorf("archive entry %q points outside the archive", name)
		}
	}
	return path.Clean(name), nil
}

var errNoBinary = errors.New("the archive has no sbnn binary in it")

func extractTarGz(archive []byte, bin string, w io.Writer, max int64) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("reading the archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(bufio.NewReader(io.LimitReader(gz, maxUnpacked)))
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading the archive: %w", err)
		}
		name, err := entryName(hdr.Name)
		if err != nil {
			return err
		}
		if name != bin {
			continue
		}
		if found {
			return fmt.Errorf("the archive holds %s twice", bin)
		}
		found = true
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("%s in the archive is not a regular file", bin)
		}
		if hdr.Size > max {
			return fmt.Errorf("%s in the archive is %d bytes, more than the %d allowed", bin, hdr.Size, max)
		}
		if err := copyCapped(w, tr, bin, max); err != nil {
			return err
		}
	}
	if !found {
		return errNoBinary
	}
	return nil
}

func extractZip(archive []byte, bin string, w io.Writer, max int64) error {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("reading the archive: %w", err)
	}
	var match *zip.File
	for _, f := range zr.File {
		name, err := entryName(f.Name)
		if err != nil {
			return err
		}
		if name != bin {
			continue
		}
		if match != nil {
			return fmt.Errorf("the archive holds %s twice", bin)
		}
		match = f
	}
	if match == nil {
		return errNoBinary
	}
	if !match.Mode().IsRegular() {
		return fmt.Errorf("%s in the archive is not a regular file", bin)
	}
	if match.UncompressedSize64 > uint64(max) {
		return fmt.Errorf("%s in the archive is %d bytes, more than the %d allowed", bin, match.UncompressedSize64, max)
	}
	rc, err := match.Open()
	if err != nil {
		return fmt.Errorf("reading %s from the archive: %w", bin, err)
	}
	defer rc.Close()
	return copyCapped(w, rc, bin, max)
}

// copyCapped copies r to w and fails once more than max bytes come out of
// it. The header's size is checked first, but it is the bytes that are
// written that count.
func copyCapped(w io.Writer, r io.Reader, bin string, max int64) error {
	n, err := io.Copy(w, io.LimitReader(r, max+1))
	if err != nil {
		return fmt.Errorf("unpacking %s: %w", bin, err)
	}
	if n > max {
		return fmt.Errorf("%s in the archive is more than the %d bytes allowed", bin, max)
	}
	return nil
}
