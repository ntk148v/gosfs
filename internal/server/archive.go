package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/ntk148v/gosfs/internal/fsutil"
)

// ArchiveKind selects the archive format for download.
type ArchiveKind string

const (
	ArchiveTar ArchiveKind = "tar"
	ArchiveTgz ArchiveKind = "tgz"
	ArchiveZip ArchiveKind = "zip"
)

// archive streams a tar/tgz/zip download of a served directory.
func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "/"
	}
	target, err := fsutil.SafeJoin(s.cfg.RootDir, p)
	if err != nil {
		http.Error(w, "invalid path: "+err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(target)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	kind := ArchiveKind(r.URL.Query().Get("type"))
	switch kind {
	case ArchiveTar, ArchiveTgz, ArchiveZip:
	default:
		kind = ArchiveTar
	}

	name := path.Base(target)
	if name == "/" || name == "." {
		name = "archive"
	}
	var ext string
	switch kind {
	case ArchiveTgz:
		ext = ".tar.gz"
	case ArchiveZip:
		ext = ".zip"
	default:
		ext = ".tar"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s%s\"", name, ext))

	switch kind {
	case ArchiveZip:
		zw := zip.NewWriter(w)
		if err := s.writeZip(zw, target); err != nil {
			s.logger.Printf("zip archive error: %v", err)
		}
		zw.Close()
	case ArchiveTgz:
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		if err := s.writeTar(tw, target); err != nil {
			s.logger.Printf("tar archive error: %v", err)
		}
		tw.Close()
		gz.Close()
	default:
		tw := tar.NewWriter(w)
		if err := s.writeTar(tw, target); err != nil {
			s.logger.Printf("tar archive error: %v", err)
		}
		tw.Close()
	}
}

// writeTar writes all entries under root into tw, names relative to root.
func (s *Server) writeTar(tw *tar.Writer, root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, cErr := io.Copy(tw, f)
			f.Close()
			return cErr
		}
		return nil
	})
}

// writeZip writes all entries under root into zw, names relative to root.
func (s *Server) writeZip(zw *zip.Writer, root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		if d.IsDir() {
			return nil // zip stores directories implicitly
		}
		fw, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, cErr := io.Copy(fw, f)
		f.Close()
		return cErr
	})
}
