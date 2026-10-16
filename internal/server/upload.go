package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

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
