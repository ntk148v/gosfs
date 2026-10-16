# gosfs Refactor + Lean Feature Set Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor gosfs from a single `main.go` into a multi-package Go project, upgrade Go to `1.24`, fix two path-traversal security holes, and add lean features (sorting, hidden toggle, human/exact bytes, delete, upload duplicate handling, archive download).

**Architecture:** Split into `cmd/gosfs` + `internal/{config,server,fsutil,web}`. A single `fsutil.SafeJoin` root-causes all path-traversal bugs. `http.ServeMux` method patterns (`POST /upload`) require Go ≥ 1.22 (we target 1.24). No new third-party deps — all stdlib (`embed`, `io/fs`, `tar`, `archive/zip`, `flag`, `html/template`).

**Tech Stack:** Go 1.24+, stdlib only.

**Spec:** `docs/superpowers/specs/2026-02-04-gosfs-refactor-design.md`

## Global Constraints

- Go module: `github.com/ntk148v/gosfs`, `go 1.24` floor (local toolchain 1.26).
- No new third-party dependencies. Functionality must use stdlib only.
- Every handler that touches the filesystem resolves paths through `fsutil.SafeJoin`; never `filepath.Join(root, untrusted)` directly.
- Keep the tool lean: no auth, TLS, WebDAV, pastebin, QR, themes, mkdir, checksums, compression.
- HTML is safe by construction: use `html/template` (auto-escaping), never `template.HTML` on user data.
- Commits are `feat:`/`fix:`/`refactor:` scoped; each task ends green (`go build ./... && go test ./...`).

## Review Focus

- Query paths containing `..` or percent-encoded `%2e%2e`, and upload filenames containing `/` or `..`: must never write or read outside the served root. (Task 1)
- A requested path that does not exist: serve should 404, not render an empty listing or panic. (Task 3)
- Duplicate-filename upload on `rename`: generated `name-{N}` must not collide with an existing file. (Task 5)
- Archive of a missing or empty directory: 404 for missing, valid empty tar/zip for empty. (Task 5)
- Sorting while `dirs_first` is on: dirs keep leading position across all sort keys, each group sorted consistently. (Task 4)

---

### Task 1: `internal/fsutil` — traversal guard + byte formatter

**Files:**
- Create: `internal/fsutil/fsutil.go`
- Test: `internal/fsutil/fsutil_test.go`

**Interfaces:**
- Consumes: nothing (first package).
- Produces:
  - `func SafeJoin(root, p string) (string, error)` — joins `p` under `root` and errors if the result escapes `root` (lexically). `p` may be an absolute or slash/backslash-separated URL path or upload filename.
  - `func FormatBytes(b int64) string` — human-readable size, 1000-based (`"500 B"`, `"1.5 KB"`, `"2.3 MB"`); decimal point for values ≥ 1000.

- [ ] **Step 1: Write the failing tests**

```go
func TestSafeJoinRejectsTraversal(t *testing.T) {
    root := t.TempDir()
    bad := []string{"../secret", "/../../etc/passwd", "a/../../b", "%2e%2e/x", "..%2fsecret", "foo/..", "/"}
    for _, p := range bad {
        if _, err := fsutil.SafeJoin(root, p); err == nil {
            t.Errorf("SafeJoin(%q) should error", p)
        }
    }
}
func TestSafeJoinAllowsNormal(t *testing.T) {
    root := t.TempDir()
    for _, p := range []string{"file.txt", "/file.txt", "/sub/dir/file.txt", "./x"} {
        got, err := fsutil.SafeJoin(root, p)
        if err != nil {
            t.Fatalf("SafeJoin(%q) errored: %v", p, err)
        }
        if !strings.HasPrefix(filepath.Clean(got), filepath.Clean(root)) {
            t.Errorf("SafeJoin(%q) escaped root: %q", p, got)
        }
    }
}
func TestFormatBytes(t *testing.T) {
    cases := []struct{ in int64; want string }{
        {0, "0 B"}, {500, "500 B"}, {1500, "1.5 KB"}, {2500000, "2.5 MB"},
    }
    for _, c := range cases {
        if got := fsutil.FormatBytes(c.in); got != c.want {
            t.Errorf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
        }
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/fsutil/`
Expected: FAIL — package `fsutil` does not exist / undefined.

- [ ] **Step 3: Implement `fsutil.go`**

`SafeJoin`: `filepath.Clean(filepath.Join(root, filepath.FromSlash(p)))` (root from `p` first), then require the cleaned result equals `root` or has `root` + separator as a prefix. Reject `p` whose cleaned form is `..`/`.` resolution of an absolute input is handled by `Join` collapsing it — if the result no longer has `root` prefix, error. Note missing-file resolves fine; traversal that cleans inside root is allowed.

Add the one-line limitation comment: `shortcut: lexical check only; a symlink inside root pointing outside is not blocked`.

`FormatBytes`: `unit = 1000`; below unit → `"%d B"`; else divide, pick `"kMGTPE"` index, `"%.1f %cB"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/fsutil/`
Expected: PASS.

- [ ] **Step 5: Run the full build (module bump agreed here)**

```bash
go mod init github.com/ntk148v/gosfs
sed -i 's/^go .*/go 1.24/' go.mod
go build ./...
```
Expected: module created with `go 1.24`, build ok. (The old `main.go` still compiles; `internal/fsutil` is new.)

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/fsutil
git commit -m "feat(fsutil): add path traversal guard and byte formatter"
```

---

### Task 2: `internal/config` — Config + flag parsing

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Config struct { RootDir string; BindAddr string; Port int; MaxUploadSize int64; ReadTimeout, WriteTimeout time.Duration; DirsFirst bool }`
  - `func Parse(args []string) (*Config, error)` — parses flags: `-root-dir` (default `/tmp/gosfs`), `-bind-addr` (`0.0.0.0`), `-port` (`2690`), `-max-size` (`16 << 20`), `-dirs-first` (bool, default false).
  - `func (c *Config) ListenAddr() string` — `fmt.Sprintf("%s:%d", c.BindAddr, c.Port)`.

- [ ] **Step 1: Write the failing test**

```go
func TestParseDefaults(t *testing.T) {
    c, err := config.Parse(nil)
    if err != nil { t.Fatal(err) }
    if c.RootDir != "/tmp/gosfs" || c.Port != 2690 || c.MaxUploadSize != 16<<20 {
        t.Errorf("unexpected defaults: %+v", c)
    }
}
func TestParseOverrides(t *testing.T) {
    c, err := config.Parse([]string{"-root-dir", "/srv", "-port", "8080", "-max-size", "1000", "-dirs-first"})
    if err != nil { t.Fatal(err) }
    if c.RootDir != "/srv" || c.Port != 8080 || c.MaxUploadSize != 1000 || !c.DirsFirst {
        t.Errorf("overrides not applied: %+v", c)
    }
}
func TestListenAddr(t *testing.T) {
    c := &config.Config{BindAddr: "127.0.0.1", Port: 9000}
    if got := c.ListenAddr(); got != "127.0.0.1:9000" {
        t.Errorf("ListenAddr = %q", got)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — package `config` undefined.

- [ ] **Step 3: Implement `config.go`**

Use `flag.NewFlagSet("gosfs", flag.ContinueOnError)` inside `Parse` so it is testable without exiting. Register the four existing flags plus `-dirs-first`, parse `args`, copy into `Config`, set `ReadTimeout`/`WriteTimeout` to the moved constants (`10 * time.Second` each).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): move flag parsing into config package"
```

---

### Task 3: `internal/web` + `internal/server` — migrate handlers, fix traversal

**Files:**
- Create: `internal/web/web.go` (embedded template; parse once), `internal/web/index.html` (move existing file here)
- Create: `internal/server/server.go` (controller, routes, middleware, shutdown)
- Create: `internal/server/index.go`, `internal/server/upload.go`, `internal/server/healthz.go` (handlers), or keep handlers in `server.go` if they stay small — split only if the file grows.
- Create: `cmd/gosfs/main.go` (thin main: `config.Parse` → `server.New` → listen)
- Delete: `main.go`, `index.html` (top-level)
- Modify: `README.md` (build/run updated)
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `config.Config`, `fsutil.SafeJoin`, `fsutil.FormatBytes`.
- Produces:
  - `type Server struct { ... }`; `func New(cfg *config.Config, logger *log.Logger) *Server`
  - `func (s *Server) Handler() http.Handler` — `ServeMux` with `"/"` (GET), `"POST /upload"`, `"GET /healthz"`; wrapped by tracing + logging middleware (methods from `main.go`, moved verbatim).
  - `func (s *Server) ServeHTTP(w,r)` (convenience, delegates to Handler) — used in tests/handlers as `http.Handler`.
  - `func (s *Server) Shutdown(ctx context.Context) context.Context` — graceful shutdown logic moved from `main.go`.

- [ ] **Step 1: Write the failing handler tests**

```go
func TestServeListings(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
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
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    r := httptest.NewRequest("GET", "/a.txt", nil)
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code != 200 || w.Body.String() != "hello" {
        t.Errorf("file serve failed: %d %q", w.Code, w.Body.String())
    }
}
func TestServeRejectsTraversal(t *testing.T) {
    root := t.TempDir()
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    r := httptest.NewRequest("GET", "/../go.mod", nil) // or /%2e%2e/...
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code == 200 {
        t.Errorf("traversal GET should not return 200")
    }
}
func TestUpload(t *testing.T) {
    root := t.TempDir()
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    body := new(bytes.Buffer)
    mw := multipart.NewWriter(body)
    fw, _ := mw.CreateFormFile("files", "up.txt")
    fw.Write([]byte("data"))
    mw.Close()
    r := httptest.NewRequest("POST", "/upload", body)
    r.Header.Set("Content-Type", mw.FormDataContentType())
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code != 302 { t.Fatalf("upload status %d", w.Code) }
    if b, _ := os.ReadFile(filepath.Join(root, "up.txt")); string(b) != "data" {
        t.Errorf("upload content wrong: %q", b)
    }
}
func TestUploadRejectsTraversalName(t *testing.T) {
    root := t.TempDir()
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    body := new(bytes.Buffer)
    mw := multipart.NewWriter(body)
    fw, _ := mw.CreateFormFile("files", "../../evil.txt")
    fw.Write([]byte("x"))
    mw.Close()
    r := httptest.NewRequest("POST", "/upload", body)
    r.Header.Set("Content-Type", mw.FormDataContentType())
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code != 400 { t.Fatalf("traversal upload should 400, got %d", w.Code) }
    if _, err := os.Stat(filepath.Join(t.TempDir(), "..", "..", "evil.txt")); err == nil {
        t.Errorf("evil file was written outside root")
    }
    // Also confirm nothing written into a sibling dir is not our concern; assert no file at cwd
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/`
Expected: FAIL — packages `web`/`server` undefined, `main.go` no longer builds.

- [ ] **Step 3: Implement `internal/web/web.go` + move `index.html`**

```go
package web
import ("embed"; "html/template"; "io")
//go:embed index.html
var IndexHTML string
var tmpl = template.Must(template.New("index").Parse(IndexHTML))
func Render(w io.Writer, data any) error { return tmpl.Execute(w, data) }
```
Move the current `index.html` into `internal/web/index.html`. Keep `File`/`Dir` template structs in `server` (add `Size string`, `ModTime string`, `Name string`, `Link string` as today; extend in Tasks 4–5).

- [ ] **Step 4: Implement `cmd/gosfs/main.go`**

Thin main: `cfg, _ := config.Parse(os.Args[1:])`; `os.MkdirAll(cfg.RootDir)`; `s := server.New(cfg, logger)`; build `http.Server{Addr: cfg.ListenAddr(), Handler: s.Handler()}`; `ctx := s.Shutdown(...)`; `ListenAndServe` in goroutine; block on `<-ctx.Done()`. Delete top-level `main.go` and `index.html`.

- [ ] **Step 5: Implement `internal/server` handlers**

Move `index` (now route `"/"`), `upload`, `healthz`, `logging`, `tracing`, `middlewares`, constants into `server`, replacing filesystem access with `fsutil.SafeJoin`. The upload handler:
- parse multipart with `s.cfg.MaxUploadSize`; require a **trusted** destination derived from `r.FormValue("path")` (relative subdir under root) **and** `fh.Filename`, **not** `Referer`/`Origin`. Both go through `fsutil.SafeJoin(s.cfg.RootDir, ...)`; on error → `400`.
- drop `defer file.Close()` in the loop: `file.Close()` immediately after `io.Copy`.
- redirect to the destination listing (`r.FormValue("path")` or `/`) instead of `r.Referer()`.

The `index` handler: `SafeJoin(root, r.URL.Path)`; if `os.Stat` says it is a file → `http.ServeFile`; else render listing.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go build ./... && go test ./...`
Expected: all pass; old top-level `main.go` gone.

- [ ] **Step 7: Update README build/run instructions to `go run ./cmd/gosfs`**

- [ ] **Step 8: Commit**

```bash
git add cmd internal/web internal/server README.md go.mod
git rm main.go index.html
git commit -m "refactor: migrate to multi-package layout; fix serve+upload traversal"
```

---

### Task 4: Listing features — sorting, hidden toggle, bytes display

**Files:**
- Modify: `internal/server/index.go`, `internal/server/list.go` (if split out), `internal/web/index.html`
- Test: add to `internal/server/list_test.go` (new) or extend `server_test.go`

**Interfaces:**
- Consumes: existing `server` methods.
- Produces:
  - `type SortKey string` with `const (SortName, SortTime, SortSize SortKey)`; `type Order string` with `const (Asc, Desc Order)`.
  - `func (s *Server) listDir(root string, sortKey SortKey, order Order, showHidden, dirsFirst bool) (Dir, error)` — returns entries; dirs always first when `dirsFirst`; each group sorted per `sortKey`/`order`.
  - `File` gains fields used by the template: `IsDir bool`, `Size string`, `ModTime string`, `Name string`, `Link string`, `SortSize int64` (unexported not usable in template — use an exported helper), and `Bytes int64` for exact display.

- [ ] **Step 1: Write the failing sorting tests**

```go
func TestListDirSorting(t *testing.T) {
    root := t.TempDir()
    // create: b.txt (small), a.txt (smaller-or-equal mtime control), sub/ (dir), c.txt
    os.WriteFile(filepath.Join(root, "b.txt"), []byte("bb"), 0o644)
    os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
    os.Mkdir(filepath.Join(root, "sub"), 0o755)
    os.WriteFile(filepath.Join(root, "c.txt"), []byte("ccc"), 0o644)
    // control mtimes: make sizes distinct regardless of time by sorting by name for a baseline
    s := &server.Server{}
    d, _ := s.listDir(root, server.SortName, server.Asc, false, true)
    got := namesOf(d.Files) // helper: collect Name
    want := []string{"sub/", "a.txt", "b.txt", "c.txt"} // dirs first, then alpha asc
    if !reflect.DeepEqual(got, want) {
        t.Errorf("sort name asc dirs-first = %v, want %v", got, want)
    }
    d, _ = s.listDir(root, server.SortSize, server.Desc, false, true)
    got = namesOf(d.Files) // first entry after dir? dirs first means sub first regardless
    // expect: sub/ then c.txt(3) b.txt(2) a.txt(1) by size desc
    if !reflect.DeepEqual(got, []string{"sub/", "c.txt", "b.txt", "a.txt"}) {
        t.Errorf("sort size desc = %v", got)
    }
}
func TestListDirSortTime(t *testing.T) {
    root := t.TempDir()
    // simpler: two files, touch mtimes deterministically
    f1 := filepath.Join(root, "old.txt"); os.WriteFile(f1, []byte("1"), 0o644)
    f2 := filepath.Join(root, "new.txt"); os.WriteFile(f2, []byte("2"), 0o644)
    p1 := time.Now().Add(-2 * time.Hour); p2 := time.Now().Add(-time.Hour)
    os.Chtimes(f1, p1, p1); os.Chtimes(f2, p2, p2)
    s := &server.Server{}
    d, _ := s.listDir(root, server.SortTime, server.Desc, false, false)
    got := namesOf(d.Files)
    if !reflect.DeepEqual(got, []string{"new.txt", "old.txt"}) {
        t.Errorf("sort time desc = %v", got)
    }
}
func TestListDirHidden(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, ".hidden"), []byte("h"), 0o644)
    os.WriteFile(filepath.Join(root, "vis.txt"), []byte("v"), 0o644)
    s := &server.Server{}
    d, _ := s.listDir(root, server.SortName, server.Asc, false, false)
    if namesOf(d.Files) (contains .hidden) { t.Error("hidden shown when showHidden=false") }
    d, _ = s.listDir(root, server.SortName, server.Asc, true, false)
    if !(!namesOf(d.Files) has .hidden) { t.Error("hidden missing when showHidden=true") }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestListDir`
Expected: FAIL — `listDir` has wrong signature / undefined.

- [ ] **Step 3: Implement `listDir` sorting + filtering in `server`**

Filter dotfiles unless `showHidden`. Materialize entries, split into dirs/files, sort each group with `slices.SortFunc` by the chosen key (name: `strings.ToLower` compare; time: `ModTime`; size: `Size`), order-aware; prepend dirs when `dirsFirst`. `Server` needs `dirsFirst` — read from `s.cfg.DirsFirst` (set at construction from config).

- [ ] **Step 4: Add query parsing + cookie persistence to the index handler**

`index` reads query: `sort` (`name|time|size`), `order` (`asc|desc`), `hidden` (`1`), `bytes` (`exact|human`). Defaults come from a `gosfs_view` cookie if present, falling back to `name`/`asc`. Set the cookie on every listing response so the choice persists across navigation.

- [ ] **Step 5: Update `index.html`**

Add: sort/order controls (links or a select that reloads with query), a "hidden" toggle, a suggestions of archive links (placeholder — real links in Task 5), and render `Size` via the chosen display form. Pass the current `sort`/`order`/`hidden`/`bytes` values into the template data (`Dir` gains exported fields for them) so controls reflect current state.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS. Manual smoke: `go run ./cmd/gosfs -root-dir /tmp/x` and load `/` — see sort control.

- [ ] **Step 7: Commit**

```bash
git add internal/server internal/web
git commit -m "feat(listing): sorting, hidden toggle, byte display with cookie persistence"
```

---

### Task 5: Archive download, delete, upload duplicate handling

**Files:**
- Create: `internal/server/archive.go` (tar/tgz/zip writers), `internal/server/rm.go`
- Modify: `internal/server/upload.go` (duplicate handling), `internal/server/server.go` (register `POST /rm`, `GET /archive`), `internal/web/index.html` (delete + archive links)
- Test: `internal/server/archive_test.go`, `internal/server/rm_test.go`, extend `upload_test.go`

**Interfaces:**
- Consumes: `fsutil.SafeJoin`.
- Produces:
  - `const DupError, DupOverwrite, DupRename ` values from `config` — add `type DupMode string` to `config` with `cfg.DupMode` (flag `-dup`, default `error`).
  - `func (s *Server) resolveDup(target string) (string, error)` — returns final path: `error`→err if exists; `overwrite`→target; `rename`→first `name-{N}.ext` not existing.
  - `type ArchiveKind string` with `const ArchiveTar, ArchiveTgz, ArchiveZip`.
  - `func (s *Server) archive(w, r)` handler for `GET /archive?path=..&type=tar|tgz|zip`.

- [ ] **Step 1: Write the failing tests**

```go
func TestResolveDupError(t *testing.T) { /* existing target: error mode -> err */ }
func TestResolveDupOverwrite(t *testing.T) { /* returns target */ }
func TestResolveDupRename(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, "a.txt"), []byte("1"), 0o644)
    s := &server.Server{}
    got, err := s.resolveDup(filepath.Join(root, "a.txt"))
    if err != nil { t.Fatal(err) }
    w := filepath.Base(got)
    if w != "a-1.txt" { t.Errorf("rename got %q, want a-1.txt", w) }
    // second rename: a-1.txt now exists -> a-2.txt
    os.WriteFile(filepath.Join(root, "a-1.txt"), []byte("x"), 0o644)
    got, _ = s.resolveDup(filepath.Join(root, "a.txt"))
    if filepath.Base(got) != "a-2.txt" { t.Errorf("second rename got %q", got) }
}
func TestArchiveTar(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
    os.Mkdir(filepath.Join(root, "sub"), 0o755)
    os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("world"), 0o644)
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    r := httptest.NewRequest("GET", "/archive?path=/&type=tar", nil)
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code != 200 { t.Fatalf("archive status %d", w.Code) }
    tr := tar.NewReader(bytes.NewReader(w.Body.Bytes()))
    names := []string{}
    for { h, err := tr.Next(); if err == io.EOF { break }; if err != nil { t.Fatal(err) }; names = append(names, h.Name) }
    if !slices.Contains(names, "a.txt") || !slices.Contains(names, "sub/b.txt") {
        t.Errorf("tar missing entries: %v", names)
    }
}
func TestArchiveZip(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, "z.txt"), []byte("data"), 0o644)
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    r := httptest.NewRequest("GET", "/archive?path=/&type=zip", nil)
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    zr, _ := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
    if len(zr.File) != 1 || zr.File[0].Name != "z.txt" { t.Errorf("zip bad: %v", zr.File) }
}
func TestRm(t *testing.T) {
    root := t.TempDir()
    os.WriteFile(filepath.Join(root, "del.txt"), []byte("x"), 0o644)
    os.Mkdir(filepath.Join(root, "dir"), 0o755)
    os.WriteFile(filepath.Join(root, "dir", "f.txt"), []byte("y"), 0o644)
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    req := httptest.NewRequest("POST", "/rm?path=/del.txt", nil)
    rw := httptest.NewRecorder()
    s.Handler().ServeHTTP(rw, req)
    if _, err := os.Stat(filepath.Join(root, "del.txt")); !os.IsNotExist(err) { t.Error("file not removed") }
    req = httptest.NewRequest("POST", "/rm?path=/dir", nil)
    rw = httptest.NewRecorder()
    s.Handler().ServeHTTP(rw, req)
    if _, err := os.Stat(filepath.Join(root, "dir")); !os.IsNotExist(err) { t.Error("dir not removed") }
}
func TestRmRejectsTraversal(t *testing.T) {
    root := t.TempDir()
    s := server.New(&config.Config{RootDir: root}, log.New(io.Discard, "", 0))
    r := httptest.NewRequest("POST", "/rm?path=/../tmp/x", nil)
    w := httptest.NewRecorder()
    s.Handler().ServeHTTP(w, r)
    if w.Code == 200 || w.Code == 302 { t.Errorf("rm traversal should 4xx, got %d", w.Code) }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestResolveDup|TestArchive|TestRm'`
Expected: FAIL — undefined methods/handlers.

- [ ] **Step 3: Add `DupMode` to config**

`config` gains `type DupMode string`, `const DupError="error", DupOverwrite="overwrite", DupRename="rename"`, and a `-dup` flag defaulting to `DupError`. `Parse` returns `cfg.DupMode`.

- [ ] **Step 4: Implement `resolveDup` in `upload.go`**

On existing target, switch on `s.cfg.DupMode`: `error` → return `fsutil`-sourced error → handler returns 400; `overwrite` → return target; `rename` → loop `i=1..` building `stem-{i}.ext` (or `stem-{i}` if no ext) until one does not exist, return it. (Mirrors miniserve's loop.)

- [ ] **Step 5: Wire `resolveDup` into the upload handler** — after `SafeJoin` the target, call `resolveDup`; write to the returned path. Keep the immediate `Close()` (no `defer` in loop).

- [ ] **Step 6: Implement `archive.go`**

Handler `GET /archive` with `path` (default `/`) and `type` (`tar`|`tgz`|`zip`, default `tar`). `SafeJoin` the path (404 if missing or not a dir). Set `Content-Disposition: attachment; filename="<dir>.tar[.gz|.zip]"`. Walk with `filepath.WalkDir`, writing each file's name relative to the archived dir into `tar`/`tar.gz` (`gzip.NewWriter`) or `zip` (`zip.NewWriter`). Set file mode + modtime in headers so timestamps survive.

- [ ] **Step 7: Implement `rm.go`**

Handler `POST /rm?path=<rel>`: `SafeJoin`; if not existing → 404; `os.RemoveAll` (handles file or dir recursively); redirect back to parent listing. Register `"POST /rm"` and `"GET /archive"` in `server.Handler()`.

- [ ] **Step 8: Update `index.html`**

- Add per-directory archize links (`tar`, `tgz`, `zip`) using the current path.
- Add a delete button per file/dir; wrap in a JS `confirm()` before `POST /rm?path=...`.

- [ ] **Step 9: Run all tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10: Manual smoke**

Run: `go run ./cmd/gosfs -root-dir /tmp/smoke` — upload a file, sort the listing, delete it, download an archive.

- [ ] **Step 11: Commit**

```bash
git add cmd internal/config internal/server internal/web README.md
git commit -m "feat: archive download, delete, and upload duplicate handling"
```

---

## Self-Review

**Spec coverage:** every spec feature has a task: traversal fix (T1+T3), sorting/hidden/bytes (T4), delete+duplicates+archive (T5), package split+Go upgrade (T1–T3). OOS items are excluded and named. ✓

**Step scan:** each step gives one signature/test/command with exact values. No step carries a full function body except where the algorithm isn't determined by the test (listDir sort compare, archive walk, resolveDup loop) — those are described, not transcribed. ✓

**Type consistency:** `fsutil.SafeJoin(root, p) (string, error)` used identically in T1/T3/T5; `config.Parse([]string) (*Config, error)` in T2/T3; `listDir(root, sortKey, order, showHidden, dirsFirst)` in T4 matches the `Server` receiver. `DupMode` added in T5 step 3 before T5 step 4 uses it. ✓

**Review Focus:** all five lines map to a task test: traversal (T1, T3 serve/upload/rm), missing path (T3 listing/file, T5 archive 404), rename collision (T5), empty/missing archive (T5), dirs-first stability across sort keys (T4). ✓

**Proportion:** plan ~1× the spec — signatures, test assertions, and the few algorithms worth naming. ✓

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-02-04-gosfs-refactor.md`. Please review the plan.**

For this plan I **recommend Native execution**: the five tasks form a linear dependency chain (fsutil → config → server → listing → ops) where each produces interfaces the next consumes, so one uninterrupted session keeps the type contracts coherent and is fastest; the risk of a bad design decision is low because every decision (signatures, algorithm choices) is pinned in the plan. A single end-of-branch review on the most capable model will catch integration issues.

Does the plan capture what you want, and which approach should we use — **Subagent-driven** or **Native**?
