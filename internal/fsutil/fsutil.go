// Package fsutil provides filesystem helpers shared by the gosfs handlers.
package fsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeJoin joins the slash-separated path p under root and returns an absolute
// path that stays inside root. It errors if p would escape root lexically
// (via ".." or an absolute path). Percent-encoded input is NOT decoded here:
// callers must pass already-decoded paths (net/http decodes r.URL.Path, and
// r.FormValue decodes form fields) so ".." arrives as "..", not "%2e%2e".
//
// shortcut: lexical check only; a symlink inside root pointing outside is not
// blocked — resolve symlinks before trusting the result if that matters.
func SafeJoin(root, p string) (string, error) {
	root = filepath.Clean(root)
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(p)))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes root", p)
	}
	return target, nil
}

// FormatBytes renders a byte count in human units, 1000-based.
func FormatBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
