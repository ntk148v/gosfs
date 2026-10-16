# gosfs refactor + lean feature set — design

Date: 2026-02-04
Status: Proposal

## Goal

Refactor gosfs from a single `main.go` into a multi-package Go project, upgrade Go
from 1.17 to a modern version (floor `go 1.24`, local toolchain 1.26), fix two path
traversal security holes, and add a small, cohesive set of features. Keep the tool
lean and idiomatic Go stdlib — no new third-party dependencies.

## Out of scope (deliberately)

Auth (basic/TLS), WebDAV, pastebin, QR codes, themes, `mkdir`, upload checksums,
response compression. These are deferred to keep gosfs a "simple file server" and to
keep the refactor's blast radius small. Name them here so nobody tries to sneak them in.

## Current state & bugs (to fix)

- Single `main.go`, Go 1.17, stdlib only, one embedded `index.html` template.
- **Bug A — path traversal on serve:** `filepath.Join(c.rootDir, r.URL.Path)` lets a
  `..` segment escape the served root. `http.ServeFile` alone rejects `..` in the
  *cleaned* path but the join happens before it, so a crafted URL can read files
  outside root.
- **Bug B — path traversal on upload:** `fh.Filename` is used raw in `os.OpenFile`,
  so `../../etc/passwd.xml` writes outside root.
- **Bug C — fragile upload destination:** the target subdirectory is reconstructed from
  the client-controlled `Referer`/`Origin` header. Must instead come from a trusted
  request form field / path.
- Template re-parsed on every request. `defer file.Close()` inside the upload loop.

## Features to add (lean set)

1. **Sorting** — by name / modified-time / size, ascending or descending; directories
   listed first. Driven by query params (`?sort=name&order=desc`), persisted to a
   server-side cookie so it survives navigation.
2. **Hidden files toggle** — `?hidden=1` shows dotfiles (default hidden).
3. **Byte display** — human-readable sizes (default) with exact bytes option.
4. **Delete** — remove a file or an empty/non-empty dir via a `POST` (or confirmed
   `GET`) action. Traversal-safe, only inside root.
5. **Upload duplicate handling** — `error` | `overwrite` | `rename-{N}` on existing
   filenames.
6. **Archive download** — serve a directory as `tar`, `tar.gz`, or `zip` via endpoints
   like `?archive=tar|tgz|zip`.

All new operations sanitize paths against traversal (fixes Bug A/B/C root cause: one
shared path-sanitizer used by every handler).

## Target layout

```
gosfs/
  go.mod                 module github.com/ntk148v/gosfs, go 1.24
  cmd/gosfs/main.go      flags, wiring, lifecycle (http server, graceful shutdown)
  internal/config/       Config struct + flag parsing + defaults
  internal/server/       controller, routes, ServeMux setup (Go 1.22+ method patterns)
  internal/handler/      index / upload / rm / archive handlers
  internal/fsutil/       shared path sanitizer + traversal guard (fixes A/B/C)
  internal/middleware/   tracing + logging
  internal/web/          embedded index.html + template
  README.md
```

Packages are created only where they earn their place; no empty shells.

## API surface

- `GET  /`                directory listing (sort/order/hidden/bytes query)
- `GET  /favicon.ico`     static (if present) or 204
- `POST /upload`          multipart upload; `path` form field is the target dir
- `POST /rm`              delete file/dir (traversal-safe)
- `GET  /archive?path=&type=tar|tgz|zip`  directory download
- `GET  /healthz`         healthcheck (kept)
- `GET  /<file>`          file serve via sanitizer + `http.ServeFile`

## Migration order (each step compiles + tests)

1. Upgrade `go.mod` to `go 1.24`; delete nothing that still works.
2. Extract `internal/fsutil` sanitizer + `internal/config` + `internal/server`
   skeleton, moving existing serve/upload/healthz into handlers verbatim (behavior
   preserved except the two traversal fixes).
3. Add `internal/web` embedded template; stop re-parsing per request.
4. Add listing features (sort/hidden/bytes), cookie persistence.
5. Add rm + archive + duplicate handling.
6. Write tests (unit for fsutil/config; handler tests with `httptest`).

## Risks / notes

- Moving `main.go` code into packages is a large diff; keep each step green.
- `http.ServeMux` method patterns (`POST /upload`) require Go ≥ 1.22 — fine with our floor.
- No new deps: tar/zip/embed/flag/io/fs are all stdlib.
