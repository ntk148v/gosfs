package server

import (
	"net/http"
	"os"
	"path"

	"github.com/ntk148v/gosfs/internal/fsutil"
)

// rm deletes a file or directory (recursively) under the root, then redirects
// back to the parent listing. Destructive by design — no trash.
func (s *Server) rm(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "/"
	}
	target, err := fsutil.SafeJoin(s.cfg.RootDir, p)
	if err != nil {
		http.Error(w, "invalid path: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, path.Dir(p), http.StatusFound)
}
