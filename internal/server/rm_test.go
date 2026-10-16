package server

import (
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ntk148v/gosfs/internal/config"
)

func TestRm(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "del.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(root, "dir"), 0o755)
	os.WriteFile(filepath.Join(root, "dir", "f.txt"), []byte("y"), 0o644)
	s := New(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize}, log.New(os.Stderr, "", 0))

	r := httptest.NewRequest("POST", "/rm?path=/del.txt", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if _, err := os.Stat(filepath.Join(root, "del.txt")); !os.IsNotExist(err) {
		t.Error("file not removed")
	}

	r = httptest.NewRequest("POST", "/rm?path=/dir", nil)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if _, err := os.Stat(filepath.Join(root, "dir")); !os.IsNotExist(err) {
		t.Error("dir not removed")
	}
}

func TestRmRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	s := New(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize}, log.New(os.Stderr, "", 0))
	r := httptest.NewRequest("POST", "/rm?path=/../tmp/x", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code == 200 || w.Code == 302 {
		t.Errorf("rm traversal should 4xx, got %d", w.Code)
	}
}

func TestRmMissing(t *testing.T) {
	root := t.TempDir()
	s := New(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize}, log.New(os.Stderr, "", 0))
	r := httptest.NewRequest("POST", "/rm?path=/nope", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("rm of missing path should 404, got %d", w.Code)
	}
}
