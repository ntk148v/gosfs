package fsutil_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ntk148v/gosfs/internal/fsutil"
)

func TestSafeJoinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	bad := []string{
		"../secret",
		"/../../etc/passwd",
		"a/../../b",
	}
	for _, p := range bad {
		if _, err := fsutil.SafeJoin(root, p); err == nil {
			t.Errorf("SafeJoin(%q) should error", p)
		}
	}
}

func TestSafeJoinAllowsNormal(t *testing.T) {
	root := t.TempDir()
	// "/" is the root listing; "foo/.." cleans back to root harmlessly.
	for _, p := range []string{
		"file.txt",
		"/file.txt",
		"/sub/dir/file.txt",
		"./x",
		"foo/..",
		"/",
		"/etc/passwd", // leading slash maps under root: root/etc/passwd
	} {
		got, err := fsutil.SafeJoin(root, p)
		if err != nil {
			t.Fatalf("SafeJoin(%q) errored: %v", p, err)
		}
		got = filepath.Clean(got)
		rootClean := filepath.Clean(root)
		if !strings.HasPrefix(got, rootClean+string(filepath.Separator)) && got != rootClean {
			t.Errorf("SafeJoin(%q) escaped root: %q", p, got)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1500, "1.5 KB"},
		{2500000, "2.5 MB"},
	}
	for _, c := range cases {
		if got := fsutil.FormatBytes(c.in); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
