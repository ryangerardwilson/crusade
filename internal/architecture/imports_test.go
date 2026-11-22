// Package architecture holds the dependency rules of crusade as tests, so a
// change that breaks one fails go test.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/ryangerardwilson/crusade/"

// rule limits what one package may import from this module and outside the
// standard library.
type rule struct {
	dir     string
	allowed []string
}

var rules = []rule{
	// The domain imports nothing outside the standard library.
	{dir: "internal/model"},
	{dir: "internal/box"},
	{dir: "internal/lineedit"},
	{dir: "internal/sqlite"},
	{dir: "internal/config"},
	{dir: "internal/store", allowed: []string{"internal/model", "internal/box", "internal/sqlite", "internal/config"}},
	{dir: "internal/dofile", allowed: []string{"internal/model", "internal/store", "internal/config"}},
	{dir: "internal/boxyaml", allowed: []string{"internal/box", "gopkg.in/yaml.v3"}},
	{dir: "nlp/internal/state", allowed: []string{"internal/sqlite", "nlp/bundle", "nlp/internal/course"}},
	{dir: "nlp/internal/course", allowed: []string{"nlp/bundle"}},
}

// sqliteOwners are the only packages that reach a sqlite file.
var sqliteOwners = map[string]bool{"internal/sqlite": true}

func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func imports(t *testing.T, dir string) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			out[path] = true
		}
	}
	return out
}

func outsideStdlib(path string) bool {
	first := strings.SplitN(path, "/", 2)[0]
	return strings.Contains(first, ".")
}

func TestPackagesImportOnlyWhatTheirRuleAllows(t *testing.T) {
	base := root(t)
	for _, r := range rules {
		allowed := map[string]bool{}
		for _, path := range r.allowed {
			allowed[path] = true
		}
		for path := range imports(t, filepath.Join(base, r.dir)) {
			if !outsideStdlib(path) {
				continue
			}
			local := strings.TrimPrefix(path, module)
			if !allowed[local] {
				t.Errorf("%s imports %s", r.dir, path)
			}
		}
	}
}

func TestOnlyTheSqlitePackageRunsSqlite(t *testing.T) {
	base := root(t)
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			if entry != nil && entry.IsDir() && entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(base, filepath.Dir(path))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "sqlite3") && !sqliteOwners[filepath.ToSlash(rel)] {
			t.Errorf("%s runs sqlite3 outside internal/sqlite", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNLPDoesNotImportTheCrusadeScreensOrStore(t *testing.T) {
	base := root(t)
	err := filepath.WalkDir(filepath.Join(base, "nlp"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		for imported := range imports(t, path) {
			if imported == module+"internal/tui" || imported == module+"internal/store" {
				t.Errorf("%s imports %s", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOnlyTheStoresImportTheSqlitePackage(t *testing.T) {
	base := root(t)
	owners := map[string]bool{"internal/store": true, "nlp/internal/state": true, "internal/sqlite": true}
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		if entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(base, path)
		if imports(t, path)[module+"internal/sqlite"] && !owners[filepath.ToSlash(rel)] {
			t.Errorf("%s imports internal/sqlite", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
