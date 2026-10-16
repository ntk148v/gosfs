package server

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func namesOf(fs []File) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

func TestListDirSorting(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("bb"), 0o644)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "c.txt"), []byte("ccc"), 0o644)

	s := &Server{}
	d, _ := s.listDir(root, SortName, Asc, false, true)
	if got, want := namesOf(d.Files), []string{"sub/", "a.txt", "b.txt", "c.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sort name asc dirs-first = %v, want %v", got, want)
	}

	d, _ = s.listDir(root, SortSize, Desc, false, true)
	if got, want := namesOf(d.Files), []string{"sub/", "c.txt", "b.txt", "a.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sort size desc = %v, want %v", got, want)
	}
}

func TestListDirSortTime(t *testing.T) {
	root := t.TempDir()
	f1 := filepath.Join(root, "old.txt")
	os.WriteFile(f1, []byte("1"), 0o644)
	f2 := filepath.Join(root, "new.txt")
	os.WriteFile(f2, []byte("2"), 0o644)
	p1 := time.Now().Add(-2 * time.Hour)
	p2 := time.Now().Add(-time.Hour)
	os.Chtimes(f1, p1, p1)
	os.Chtimes(f2, p2, p2)

	s := &Server{}
	d, _ := s.listDir(root, SortTime, Desc, false, false)
	if got, want := namesOf(d.Files), []string{"new.txt", "old.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sort time desc = %v, want %v", got, want)
	}
}

func TestListDirHidden(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".hidden"), []byte("h"), 0o644)
	os.WriteFile(filepath.Join(root, "vis.txt"), []byte("v"), 0o644)

	s := &Server{}
	d, _ := s.listDir(root, SortName, Asc, false, false)
	for _, n := range namesOf(d.Files) {
		if n == ".hidden" {
			t.Errorf("hidden shown when showHidden=false: %v", namesOf(d.Files))
		}
	}

	d, _ = s.listDir(root, SortName, Asc, true, false)
	found := false
	for _, n := range namesOf(d.Files) {
		if n == ".hidden" {
			found = true
		}
	}
	if !found {
		t.Errorf("hidden missing when showHidden=true: %v", namesOf(d.Files))
	}
}
