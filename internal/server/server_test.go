package server_test

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ntk148v/gosfs/internal/config"
	"github.com/ntk148v/gosfs/internal/server"
)

func newTestServer(root string) *server.Server {
	return server.New(&config.Config{RootDir: root, MaxUploadSize: config.DefaultMaxUploadSize}, log.New(io.Discard, "", 0))
}

func TestServeListings(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	s := newTestServer(root)
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "a.txt") {
		t.Errorf("listing failed: %d %s", w.Code, w.Body.String())
	}
}

func TestServeFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	s := newTestServer(root)
	r := httptest.NewRequest("GET", "/a.txt", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Errorf("file serve failed: %d %q", w.Code, w.Body.String())
	}
}

func TestServeRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(root)
	r := httptest.NewRequest("GET", "/../go.mod", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code == 200 {
		t.Errorf("traversal GET should not return 200")
	}
}

func TestServeMissingDir(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(root)
	r := httptest.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("missing path should 404, got %d", w.Code)
	}
}

func TestUpload(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(root)
	body := new(bytes.Buffer)
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("files", "up.txt")
	fw.Write([]byte("data"))
	mw.Close()
	r := httptest.NewRequest("POST", "/upload", body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 302 {
		t.Fatalf("upload status %d", w.Code)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "up.txt")); string(b) != "data" {
		t.Errorf("upload content wrong: %q", b)
	}
}

func TestUploadRejectsTraversalName(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(root)
	body := new(bytes.Buffer)
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("files", "evil.txt")
	fw.Write([]byte("x"))
	// The traversal vector is the "path" form field (the destination subdir), which
	// is NOT sanitized by multipart the way fh.Filename is.
	_ = mw.WriteField("path", "../../esc")
	mw.Close()
	r := httptest.NewRequest("POST", "/upload", body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("traversal upload should 400, got %d", w.Code)
	}
	// Nothing written outside root via the escaping destination.
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "esc", "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("evil file was written outside root")
	}
}

func TestIndexViewQueryAndCookie(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644)
	s := newTestServer(root)

	r := httptest.NewRequest("GET", "/?sort=size&order=desc&hidden=1&bytes=exact", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("index status %d", w.Code)
	}
	var set *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "gosfs_view" {
			set = c
		}
	}
	if set == nil {
		t.Fatal("expected gosfs_view cookie to be set")
	}
	if !strings.Contains(set.Value, "sort=size") || !strings.Contains(set.Value, "order=desc") ||
		!strings.Contains(set.Value, "hidden=1") || !strings.Contains(set.Value, "bytes=exact") {
		t.Errorf("cookie value malformed: %q", set.Value)
	}
	// exact bytes should show "3" not "3 B"
	if strings.Contains(w.Body.String(), ">3 B<") {
		t.Errorf("exact bytes rendered human form: %s", w.Body.String())
	}
}
