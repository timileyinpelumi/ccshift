package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tarGz(t *testing.T, name string, body []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2})
	tw.Write([]byte("hi"))
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipped(t *testing.T, name string, body []byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	w.Write(body)
	zw.Close()
	return buf.Bytes()
}

func server(t *testing.T, tag string, assets map[string][]byte, corrupt bool) *httptest.Server {
	var sums bytes.Buffer
	for name, b := range assets {
		sum := sha256.Sum256(b)
		if corrupt {
			sum[0] ^= 0xff
		}
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/"+tag+"/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if name == "checksums.txt" {
			w.Write(sums.Bytes())
			return
		}
		b, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.3.1", "0.3.0", true}, {"v0.10.0", "0.9.9", true}, {"v1.0.0", "0.99.0", true},
		{"v0.3.0", "0.3.0", false}, {"v0.2.9", "0.3.0", false}, {"v0.4.0", "dev", false}, {"garbage", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestLatestAndInstall(t *testing.T) {
	for _, sys := range []struct{ goos, asset, name string }{
		{"linux", "ccshift_linux_amd64.tar.gz", "ccshift"},
		{"darwin", "ccshift_darwin_amd64.tar.gz", "ccshift"},
		{"windows", "ccshift_windows_amd64.zip", "ccshift.exe"},
	} {
		body := []byte("new binary for " + sys.goos)
		archive := tarGz(t, sys.name, body)
		if sys.goos == "windows" {
			archive = zipped(t, sys.name, body)
		}
		s := server(t, "v0.4.0", map[string][]byte{sys.asset: archive}, false)
		r := Releases{Base: s.URL, OS: sys.goos, Arch: "amd64"}
		tag, err := r.Latest(context.Background())
		if err != nil || tag != "v0.4.0" {
			t.Fatalf("%s: Latest = %q, %v", sys.goos, tag, err)
		}
		exe := filepath.Join(t.TempDir(), sys.name)
		os.WriteFile(exe, []byte("old"), 0o755)
		if err := r.Install(context.Background(), tag, exe); err != nil {
			t.Fatalf("%s: Install: %v", sys.goos, err)
		}
		if b, _ := os.ReadFile(exe); string(b) != string(body) {
			t.Fatalf("%s: binary = %q", sys.goos, b)
		}
		if fi, _ := os.Stat(exe); sys.goos != "windows" && fi.Mode().Perm()&0o100 == 0 {
			t.Fatalf("%s: not executable: %v", sys.goos, fi.Mode())
		}
	}
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	s := server(t, "v0.4.0", map[string][]byte{"ccshift_linux_amd64.tar.gz": tarGz(t, "ccshift", []byte("evil"))}, true)
	r := Releases{Base: s.URL, OS: "linux", Arch: "amd64"}
	exe := filepath.Join(t.TempDir(), "ccshift")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := r.Install(context.Background(), "v0.4.0", exe); err == nil {
		t.Fatal("a checksum mismatch must fail")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("the old binary must be left alone")
	}
}
