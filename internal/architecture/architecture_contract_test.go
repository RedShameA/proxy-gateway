package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainPackagesDoNotDependOnAdapters(t *testing.T) {
	root := repoRoot(t)
	domainDir := filepath.Join(root, "internal", "domain")
	if _, err := os.Stat(domainDir); os.IsNotExist(err) {
		t.Skip("internal/domain has not been introduced yet")
	}
	forbidden := []string{
		"database/sql",
		"net/http",
		"os",
		"path/filepath",
		"github.com/sagernet/sing",
		"github.com/sagernet/sing-box",
		"modernc.org/sqlite",
		"proxygateway/internal/app",
		"proxygateway/internal/application",
	}
	assertNoForbiddenImports(t, root, domainDir, forbidden)
}

func TestApplicationPackagesDoNotDependOnAdapters(t *testing.T) {
	root := repoRoot(t)
	applicationDir := filepath.Join(root, "internal", "application")
	forbidden := []string{
		"database/sql",
		"net/http",
		"os",
		"path/filepath",
		"github.com/sagernet/sing",
		"github.com/sagernet/sing-box",
		"modernc.org/sqlite",
		"proxygateway/internal/app",
	}
	assertNoForbiddenImports(t, root, applicationDir, forbidden)
}

func TestInfrastructurePackagesDoNotDependOnInterfacesOrApp(t *testing.T) {
	root := repoRoot(t)
	infrastructureDir := filepath.Join(root, "internal", "infrastructure")
	if _, err := os.Stat(infrastructureDir); os.IsNotExist(err) {
		t.Skip("internal/infrastructure has not been introduced yet")
	}
	forbidden := []string{
		"proxygateway/internal/app",
		"proxygateway/internal/interfaces",
	}
	assertNoForbiddenImports(t, root, infrastructureDir, forbidden)
}

func assertNoForbiddenImports(t *testing.T, root, dir string, forbidden []string) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			for _, item := range forbidden {
				if importPath == item || strings.HasPrefix(importPath, item+"/") {
					t.Errorf("%s imports forbidden adapter dependency %q", rel(root, path), importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func rel(root, path string) string {
	if out, err := filepath.Rel(root, path); err == nil {
		return out
	}
	return path
}
