package architecture

import (
	"bytes"
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
		"proxygateway/internal/infrastructure",
		"proxygateway/internal/interfaces",
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
		"proxygateway/internal/infrastructure",
		"proxygateway/internal/interfaces",
	}
	assertNoForbiddenImports(t, root, applicationDir, forbidden)
}

func TestInterfacePackagesDoNotDependOnInfrastructureOrApp(t *testing.T) {
	root := repoRoot(t)
	interfacesDir := filepath.Join(root, "internal", "interfaces")
	if _, err := os.Stat(interfacesDir); os.IsNotExist(err) {
		t.Skip("internal/interfaces has not been introduced yet")
	}
	forbidden := []string{
		"database/sql",
		"github.com/sagernet/sing",
		"github.com/sagernet/sing-box",
		"modernc.org/sqlite",
		"proxygateway/internal/app",
		"proxygateway/internal/infrastructure",
	}
	assertNoForbiddenImports(t, root, interfacesDir, forbidden)
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

func TestAppDoesNotDependOnConcreteDatabaseImplementation(t *testing.T) {
	root := repoRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	forbidden := []string{
		"database/sql",
		"modernc.org/sqlite",
		"proxygateway/internal/infrastructure/sqlite",
	}
	assertNoForbiddenImports(t, root, appDir, forbidden)
	assertNoForbiddenText(t, root, appDir, []string{
		"store.DB",
		"store.WithTx",
	})
}

func TestAppDoesNotDependOnProtocolRuntimeImplementation(t *testing.T) {
	root := repoRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	forbidden := []string{
		"github.com/sagernet/sing",
		"github.com/sagernet/sing-box",
	}
	assertNoForbiddenImports(t, root, appDir, forbidden)
}

func TestAppProductionCodeDoesNotDependOnTesting(t *testing.T) {
	root := repoRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	assertNoForbiddenImports(t, root, appDir, []string{
		"testing",
	})
}

func TestAppStorageInfrastructureImportsStayInCompositionFiles(t *testing.T) {
	root := repoRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	allowed := map[string]bool{
		"gateway.go": true,
		"schema.go":  true,
		"types.go":   true,
	}
	assertImportOnlyInFiles(t, root, appDir, "proxygateway/internal/infrastructure/storage", allowed)
}

func TestAppAndInnerLayersDoNotContainDirectSQL(t *testing.T) {
	root := repoRoot(t)
	dirs := []string{
		filepath.Join(root, "internal", "app"),
		filepath.Join(root, "internal", "application"),
		filepath.Join(root, "internal", "interfaces"),
	}
	for _, dir := range dirs {
		assertNoForbiddenText(t, root, dir, []string{
			"database/sql",
			"*sql.",
			"SELECT ",
			"INSERT ",
			"UPDATE ",
			"DELETE ",
			"CREATE ",
			"ALTER ",
			"PRAGMA ",
			"INSERT OR ",
			"ON CONFLICT",
		})
	}
}

func TestAppDoesNotOwnHTTPAPIRoutesOrProxyProtocolDetails(t *testing.T) {
	root := repoRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	err := filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(`"/api/`)) {
			t.Errorf("%s contains API route text outside the HTTP interface layer", rel(root, path))
		}
		for _, marker := range []string{
			"Proxy-Authorization",
			"HTTP/1.1 200 Connection Established",
			"readSOCKS5",
			"writeSOCKS5",
		} {
			if bytes.Contains(body, []byte(marker)) {
				t.Errorf("%s contains proxy protocol adapter marker %q", rel(root, path), marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
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

func assertImportOnlyInFiles(t *testing.T, root, dir, importPrefix string, allowed map[string]bool) {
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
			if importPath == importPrefix || strings.HasPrefix(importPath, importPrefix+"/") {
				if !allowed[filepath.Base(path)] {
					t.Errorf("%s imports %q outside app composition files", rel(root, path), importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertNoForbiddenText(t *testing.T, root, dir string, forbidden []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, item := range forbidden {
			if bytes.Contains(body, []byte(item)) {
				t.Errorf("%s contains forbidden text %q", rel(root, path), item)
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
