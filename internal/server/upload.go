package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ntk148v/gosfs/internal/config"
	"github.com/ntk148v/gosfs/internal/fsutil"
)

// upload writes uploaded multipart files under the root directory, into the
// optional subdirectory named by the "path" form field.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(s.cfg.MaxUploadSize)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fhs := r.MultipartForm.File["files"]
	sub := r.FormValue("path")

	for _, fh := range fhs {
		if fh.Size > int64(s.cfg.MaxUploadSize) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		// Trusted destination: the subdirectory comes from a form field and the
		// filename from multipart — both must stay under root.
		target, err := fsutil.SafeJoin(s.cfg.RootDir, filepath.Join(sub, fh.Filename))
		if err != nil {
			http.Error(w, "invalid upload path: "+err.Error(), http.StatusBadRequest)
			return
		}
		target, err = s.resolveDup(target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		file, err := fh.Open()
		if err != nil {
			s.logger.Printf("Error retrieving the file: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.logger.Printf("Uploaded file: %+v, file size: %+v, MIME header: %+v\n",
			fh.Filename, fh.Size, fh.Header)

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			file.Close()
			s.logger.Printf("Error creating upload dir: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			file.Close()
			s.logger.Printf("Error creating a new file: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err = io.Copy(dst, file); err != nil {
			dst.Close()
			file.Close()
			s.logger.Printf("Error copying new file: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dst.Close()
		file.Close()
	}

	dest := sub
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// resolveDup returns the path to write an upload to, applying the configured
// duplicate policy when the target already exists.
func (s *Server) resolveDup(target string) (string, error) {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return target, nil
	}
	switch s.cfg.DupMode {
	case config.DupOverwrite:
		return target, nil
	case config.DupRename:
		ext := filepath.Ext(target)
		stem := strings.TrimSuffix(target, ext)
		for i := 1; ; i++ {
			candidate := stem + "-" + strconv.Itoa(i) + ext
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				return candidate, nil
			}
		}
	default: // DupError
		return "", fmt.Errorf("file already exists: %s", filepath.Base(target))
	}
}
