package openapi

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sortMods(m []moduleInfo) {
	sort.Slice(m, func(i, j int) bool { return m[i].path < m[j].path })
}

// TestParseWorkspaceModules verifies that a go.work file's `use` directives are
// resolved to (directory, module path) pairs, covering both the block form and
// relative path resolution against the go.work directory.
func TestParseWorkspaceModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "svc-a", "go.mod"), "module example.com/a\n\ngo 1.25\n")
	writeFile(t, filepath.Join(root, "svc-b", "go.mod"), "module example.com/b\n\ngo 1.25\n")
	work := filepath.Join(root, "go.work")
	writeFile(t, work, "go 1.25\n\nuse (\n\t./svc-a\n\t./svc-b\n)\n")

	got := parseWorkspaceModules(work)
	want := []moduleInfo{
		{dir: filepath.ToSlash(filepath.Join(root, "svc-a")), path: "example.com/a"},
		{dir: filepath.ToSlash(filepath.Join(root, "svc-b")), path: "example.com/b"},
	}
	sortMods(got)
	sortMods(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorkspaceModules:\n got=%+v\nwant=%+v", got, want)
	}
}

// TestFindGoWork covers GOWORK=off, an explicit GOWORK path, and the upward
// directory search when GOWORK is unset.
func TestFindGoWork(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		if got := findGoWork(); got != "" {
			t.Errorf("GOWORK=off: got %q, want \"\"", got)
		}
	})

	t.Run("explicit", func(t *testing.T) {
		work := filepath.Join(t.TempDir(), "go.work")
		t.Setenv("GOWORK", work)
		if got := findGoWork(); got != work {
			t.Errorf("explicit GOWORK: got %q, want %q", got, work)
		}
	})

	t.Run("walk up", func(t *testing.T) {
		root := t.TempDir()
		work := filepath.Join(root, "go.work")
		writeFile(t, work, "go 1.25\n")
		sub := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOWORK", "")
		t.Chdir(sub)

		got, err := filepath.EvalSymlinks(findGoWork())
		if err != nil {
			t.Fatalf("eval found path: %v", err)
		}
		wantPath, _ := filepath.EvalSymlinks(work)
		if got != wantPath {
			t.Errorf("walk up: got %q, want %q", got, wantPath)
		}
	})
}

// TestModuleRelativePaths_Workspace verifies that path conversion picks the
// correct module when several are in scope, in both directions.
func TestModuleRelativePaths_Workspace(t *testing.T) {
	t.Cleanup(func() {
		modules = nil
		modulesOnce = sync.Once{}
	})

	// Inject a two-module workspace and mark discovery as already done so that
	// loadModules() preserves the injected set.
	modules = []moduleInfo{
		{dir: "/repo/svc-a", path: "example.com/a"},
		{dir: "/repo/svc-b", path: "example.com/b"},
	}
	modulesOnce = sync.Once{}
	modulesOnce.Do(func() {})

	cases := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"to: fs path in svc-b", toModuleRelativePath, "/repo/svc-b/handlers/x.go", "example.com/b/handlers/x.go"},
		{"to: fs path in svc-a", toModuleRelativePath, "/repo/svc-a/y.go", "example.com/a/y.go"},
		{"to: already module-relative", toModuleRelativePath, "example.com/b/z.go", "example.com/b/z.go"},
		{"to: outside any module", toModuleRelativePath, "/elsewhere/q.go", "/elsewhere/q.go"},
		{"from: module-relative svc-b", fromModuleRelativePath, "example.com/b/handlers/x.go", filepath.FromSlash("/repo/svc-b/handlers/x.go")},
		{"from: not module-relative", fromModuleRelativePath, "annotations_test.go", "annotations_test.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fn(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
