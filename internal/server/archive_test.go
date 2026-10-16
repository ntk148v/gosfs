package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ntk148v/gosfs/internal/config"
)

func newTestServerConfig(cfg *config.Config) *Server {
	return New(cfg, log.New(io.Discard, "", 0))
}

func TestResolveDupError(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("1"), 0o644)
	s := &Server{cfg: &config.Config{DupMode: config.DupError}}
	if _, err := s.resolveDup(filepath.Join(root, "a.txt")); err == nil {
		t.Error("error mode should reject existing target")
	}
	if got, err := s.resolveDup(filepath.Join(root, "fresh.txt")); err != nil || got != filepath.Join(root, "fresh.txt") {
		t.Errorf("error mode should allow new file, got %q, %v", got, err)
	}
}

func TestResolveDupOverwrite(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("1"), 0o644)
	s := &Server{cfg: &config.Config{DupMode: config.DupOverwrite}}
	got, err := s.resolveDup(filepath.Join(root, "a.txt"))
	if err != nil || got != filepath.Join(root, "a.txt") {
		t.Errorf("overwrite should return target, got %q, %v", got, err)
	}
}

func TestResolveDupRename(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("1"), 0o644)
	s := &Server{cfg: &config.Config{DupMode: config.DupRename}}
	got, err := s.resolveDup(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if w := filepath.Base(got); w != "a-1.txt" {
		t.Errorf("rename got %q, want a-1.txt", w)
	}
	os.WriteFile(filepath.Join(root, "a-1.txt"), []byte("x"), 0o644)
	got, _ = s.resolveDup(filepath.Join(root, "a.txt"))
	if filepath.Base(got) != "a-2.txt" {
		t.Errorf("second rename got %q", got)
	}
	// no-extension files: stem-{N}
	os.WriteFile(filepath.Join(root, "notes"), []byte("n"), 0o644)
	got, _ = s.resolveDup(filepath.Join(root, "notes"))
	if filepath.Base(got) != "notes-1" {
		t.Errorf("no-ext rename got %q", got)
	}
}

func TestArchiveTar(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	os.Mkdir(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("world"), 0o644)
	s := newTestServerConfig(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize})
	r := httptest.NewRequest("GET", "/archive?path=/&type=tar", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("archive status %d", w.Code)
	}
	tr := tar.NewReader(bytes.NewReader(w.Body.Bytes()))
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	if !slices.Contains(names, "a.txt") || !slices.Contains(names, "sub/b.txt") {
		t.Errorf("tar missing entries: %v", names)
	}
}

func TestArchiveZip(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "z.txt"), []byte("data"), 0o644)
	s := newTestServerConfig(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize})
	r := httptest.NewRequest("GET", "/archive?path=/&type=zip", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "z.txt" {
		t.Errorf("zip bad: %v", zr.File)
	}
}

func TestArchiveMissingDir(t *testing.T) {
	root := t.TempDir()
	s := newTestServerConfig(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize})
	r := httptest.NewRequest("GET", "/archive?path=/nope&type=tar", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("archive of missing dir should 404, got %d", w.Code)
	}
}
