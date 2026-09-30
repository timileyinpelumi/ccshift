// Package update finds the latest ccshift release on GitHub and swaps it in for the running binary.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultBase = "https://github.com/timileyinpelumi/ccshift"

type Releases struct {
	Base string // the repository URL; tests point it at a local server
	OS   string
	Arch string
}

var client = &http.Client{
	Timeout: 60 * time.Second,
	// The latest release is found from where /releases/latest redirects, which needs no API token.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Latest returns the tag of the newest release, such as "v0.4.0".
func (r Releases) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, r.Base+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" || !strings.Contains(loc, "/releases/tag/") {
		return "", fmt.Errorf("could not find the latest release (HTTP %d)", resp.StatusCode)
	}
	return path.Base(loc), nil
}

func (r Releases) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	dl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := dl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func (r Releases) asset() (archive, binary string) {
	if r.OS == "windows" {
		return fmt.Sprintf("ccshift_windows_%s.zip", r.Arch), "ccshift.exe"
	}
	return fmt.Sprintf("ccshift_%s_%s.tar.gz", r.OS, r.Arch), "ccshift"
}

// Install downloads the release, checks it against the published checksum, and replaces exe.
// Nothing is changed if any step fails.
func (r Releases) Install(ctx context.Context, tag, exe string) error {
	archive, binary := r.asset()
	base := r.Base + "/releases/download/" + tag + "/"
	data, err := r.fetch(ctx, base+archive)
	if err != nil {
		return err
	}
	sums, err := r.fetch(ctx, base+"checksums.txt")
	if err != nil {
		return err
	}
	if err := verify(data, sums, archive); err != nil {
		return err
	}
	var bin []byte
	if strings.HasSuffix(archive, ".zip") {
		bin, err = fromZip(data, binary)
	} else {
		bin, err = fromTarGz(data, binary)
	}
	if err != nil {
		return err
	}
	return Replace(exe, bin)
}

func verify(data, sums []byte, name string) error {
	got := sha256.Sum256(data)
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			if f[0] != hex.EncodeToString(got[:]) {
				return errors.New("the download does not match its published checksum")
			}
			return nil
		}
	}
	return fmt.Errorf("checksums.txt has no entry for %s", name)
}

func fromTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s not found in the release archive", name)
		}
		if path.Base(h.Name) == name {
			return io.ReadAll(tr)
		}
	}
}

func fromZip(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if path.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in the release archive", name)
}

// Replace swaps in a new binary. Processes already running the old one carry on. Windows will not
// overwrite a running executable but will rename it, so the old one is moved aside first.
func Replace(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".ccshift-new-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	os.Remove(old) // fails on Windows while the old one runs; it is removed on the next update
	return nil
}

// Newer reports whether tag (such as "v0.4.0") is a later version than current (such as "0.3.0").
// A development build is never older than anything.
func Newer(tag, current string) bool {
	a, ok1 := parse(tag)
	b, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
