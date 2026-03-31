package interest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExtractFS writes all .md files from an fs.FS to destDir. This is used to
// convert an embedded FS into a directory path that Catalog.Load can accept.
func ExtractFS(fsys fs.FS, destDir string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, filepath.Base(path))
		return os.WriteFile(dest, data, 0644)
	})
}
