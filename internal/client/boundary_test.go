package vergeio

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoHTTPOutsideClient fails when production code outside this package
// imports net/http or net/url. Tests may import net/http to stub httptest
// servers. Request URLs are built in this package.
func TestNoHTTPOutsideClient(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" || base == "tools" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "internal/client" || strings.HasPrefix(rel, "internal/client/") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Errorf("parse %s: %v", rel, parseErr)
			return nil
		}
		for _, imp := range file.Imports {
			quoted, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				t.Errorf("%s: import %s: %v", rel, imp.Path.Value, unquoteErr)
				continue
			}
			if quoted == "net/http" || quoted == "net/url" {
				t.Errorf("%s imports %s; only internal/client may import it", rel, quoted)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
