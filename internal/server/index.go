package server

import (
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/ntk148v/gosfs/internal/fsutil"
	"github.com/ntk148v/gosfs/internal/web"
)

// File is one entry in a directory listing, passed to the index template.
type File struct {
	Link    string
	Size    string // human-readable form; "-" for directories
	ModTime string
	Name    string
	IsDir   bool
	Bytes   int64 // raw size in bytes, for exact display

	// unexported sort key; the template only sees the formatted fields above
	mtime time.Time
}

// Dir is the data model rendered by the directory-listing template.
type Dir struct {
	DisplayPath string
	Files       []File
	Sort        string
	Order       string
	ShowHidden  bool
	ExactBytes  bool
}

// SortKey selects how listDir orders entries.
type SortKey string

const (
	SortName SortKey = "name"
	SortTime SortKey = "time"
	SortSize SortKey = "size"
)

// Order selects ascending or descending sort.
type Order string

const (
	Asc  Order = "asc"
	Desc Order = "desc"
)

// index serves the directory listing for the requested path, or the file
// itself when the path resolves to a regular file.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	target, err := fsutil.SafeJoin(s.cfg.RootDir, r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.logger.Printf("Error stat path %q: %v", r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}

	view := s.viewFrom(r)
	dir, err := s.listDir(target, view.sort, view.order, view.showHidden, s.cfg.DirsFirst)
	if err != nil {
		s.logger.Printf("Error listing files in directory %q: %v", target, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dir.DisplayPath = r.URL.Path
	dir.Sort = string(view.sort)
	dir.Order = string(view.order)
	dir.ShowHidden = view.showHidden
	dir.ExactBytes = view.exactBytes
	for i := range dir.Files {
		base := strings.TrimSuffix(dir.Files[i].Name, "/")
		dir.Files[i].Link = path.Join(r.URL.Path, base)
		if dir.Files[i].IsDir {
			dir.Files[i].Link += "/"
		}
	}

	s.setViewCookie(w, view)

	if err := web.Render(w, dir); err != nil {
		s.logger.Printf("Error rendering index page: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// view is the resolved listing preferences for one request.
type view struct {
	sort       SortKey
	order      Order
	showHidden bool
	exactBytes bool
}

// viewFrom resolves listing preferences from query params, falling back to the
// persisted gosfs_view cookie, then to defaults.
func (s *Server) viewFrom(r *http.Request) view {
	v := view{sort: SortName, order: Asc}
	if c, err := r.Cookie("gosfs_view"); err == nil {
		for _, part := range strings.Split(c.Value, ",") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "sort":
				v.sort = SortKey(kv[1])
			case "order":
				v.order = Order(kv[1])
			case "hidden":
				v.showHidden = kv[1] == "1"
			case "bytes":
				v.exactBytes = kv[1] == "exact"
			}
		}
	}
	if q := r.URL.Query(); len(q) > 0 {
		if s := q.Get("sort"); s != "" {
			v.sort = SortKey(s)
		}
		if o := q.Get("order"); o != "" {
			v.order = Order(o)
		}
		if h := q.Get("hidden"); h != "" {
			v.showHidden = h == "1"
		}
		if b := q.Get("bytes"); b != "" {
			v.exactBytes = b == "exact"
		}
	}
	return v
}

// setViewCookie persists the current view preferences so they carry across
// navigation.
func (s *Server) setViewCookie(w http.ResponseWriter, v view) {
	val := "sort=" + string(v.sort) +
		",order=" + string(v.order) +
		",hidden=" + bool01(v.showHidden) +
		",bytes=" + boolVal(v.exactBytes, "exact", "human")
	http.SetCookie(w, &http.Cookie{Name: "gosfs_view", Value: val, Path: "/"})
}

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func boolVal(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

// listDir builds the Dir model for root, filtered and ordered per the request.
func (s *Server) listDir(root string, sortKey SortKey, order Order, showHidden, dirsFirst bool) (Dir, error) {
	dir := Dir{Files: []File{}}

	entries, err := os.ReadDir(root)
	if err != nil {
		return dir, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		var f File
		f.Name = name
		f.IsDir = info.IsDir()
		f.Bytes = info.Size()
		f.mtime = info.ModTime()
		f.ModTime = f.mtime.Format("2006-01-02 15:04")
		if f.IsDir {
			f.Name += "/"
			f.Size = "-"
		} else {
			f.Size = fsutil.FormatBytes(f.Bytes)
		}
		dir.Files = append(dir.Files, f)
	}

	slices.SortFunc(dir.Files, func(a, b File) int {
		if dirsFirst && a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		var c int
		switch sortKey {
		case SortTime:
			c = a.mtime.Compare(b.mtime)
		case SortSize:
			c = cmpInt(a.Bytes, b.Bytes)
		default: // SortName
			c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if order == Desc {
			c = -c
		}
		return c
	})

	return dir, nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
