package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
)

func TestProjectFilesSearchUnopenedAndSkipped(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{"中文 space name.go": "prefix\r\nNEEDLE needle\r\n", "binary": "needle\x00", "invalid": "needle\xff", "large": strings.Repeat("x", application.MaxProjectSearchFileBytes+1)} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	input := application.ProjectSearchInput{Root: root, Paths: []string{"中文 space name.go", "binary", "invalid", "large", "missing", "directory"}, Query: application.ProjectSearchQuery{Text: "needle"}}
	// Newlines cannot be represented in Windows filenames; use a portable name there.
	got, err := (ProjectFiles{}).SearchProject(context.Background(), input)
	if err != nil || len(got.Matches) != 2 || got.Scanned != 1 || got.Skipped != 4 || got.Unreadable != 1 {
		t.Fatalf("filesystem result: %+v %v", got, err)
	}
	m := got.Matches[0]
	if m.InMemory || m.Line != 2 || m.Start != 7 || m.Column != 1 {
		t.Fatalf("unopened match: %+v", m)
	}
	if _, err := (ProjectFiles{}).SearchProject(context.Background(), application.ProjectSearchInput{Root: filepath.Join(root, "missing"), Query: input.Query}); err == nil {
		t.Fatal("missing root accepted")
	}
}

func TestProjectFilesSearchScopeAndLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../secret", ".git/config", "nested/.GIT/config"} {
		_, err := (ProjectFiles{}).SearchProject(context.Background(), application.ProjectSearchInput{Root: root, Paths: []string{p}, Query: application.ProjectSearchQuery{Text: "needle"}})
		if err == nil {
			t.Fatalf("unsafe path accepted %q", p)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	got, err := (ProjectFiles{}).SearchProject(context.Background(), application.ProjectSearchInput{Root: root, Paths: []string{"link/secret"}, Query: application.ProjectSearchQuery{Text: "needle"}})
	if err != nil || len(got.Matches) != 0 || got.Skipped != 1 {
		t.Fatalf("linked directory searched: %+v %v", got, err)
	}
}
