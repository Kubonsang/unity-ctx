package safety

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnlySafetyImportsFileidGraph enforces the seam rule: unity-ctx may
// touch the unity-fileid-graph kernel only through internal/safety.
func TestOnlySafetyImportsFileidGraph(t *testing.T) {
	root := filepath.Join("..", "..")
	selfDir := filepath.Join("internal", "safety")

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".worktrees", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if strings.HasPrefix(rel, selfDir+string(filepath.Separator)) {
			return nil
		}

		fset := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range parsed.Imports {
			if strings.Contains(imp.Path.Value, "unity-fileid-graph") {
				t.Errorf("%s imports unity-fileid-graph; only internal/safety may", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
