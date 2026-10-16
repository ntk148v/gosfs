package server

import (
	"net/http"
	"os"

	"github.com/ntk148v/gosfs/internal/fsutil"
	"github.com/ntk148v/gosfs/internal/web"
)

// index serves the directory listing for the requested path, or the file
// itself when the path resolves to a regular file.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		return
	}

	path, err := fsutil.SafeJoin(s.cfg.RootDir, r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(path)
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
		http.ServeFile(w, r, path)
		return
	}

	dir, err := s.listDir(path)
	if err != nil {
		s.logger.Printf("Error listing files in directory %q: %v", path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dir.DisplayPath = r.URL.Path

	if err := web.Render(w, dir); err != nil {
		s.logger.Printf("Error rendering index page: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// listDir builds the Dir model for root, keeping the entries unsorted here;
// Task 4 adds sorting, hidden filtering and byte display.
func (s *Server) listDir(root string) (Dir, error) {
	dir := Dir{Files: []File{}}

	entries, err := os.ReadDir(root)
	if err != nil {
		return dir, err
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		var f File
		f.ModTime = info.ModTime().Format("2006-01-02 15:04")
		if info.IsDir() {
			f.Name = entry.Name() + "/"
			f.Size = "-"
		} else {
			f.Name = entry.Name()
			f.Size = fsutil.FormatBytes(info.Size())
		}
		dir.Files = append(dir.Files, f)
	}
	return dir, nil
}
