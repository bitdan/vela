package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vela/internal/application"
)

func TestCorruptLayoutRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	corrupt := []byte("{broken")
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	layout, err := LoadLayout(path)
	if !errors.Is(err, ErrInvalidLayout) || layout != application.DefaultLayout() {
		t.Fatalf("fallback: %+v, %v", layout, err)
	}
	backup, err := BackupLayout(path)
	if err != nil {
		t.Fatal(err)
	}
	layout.Dark = false
	if err := SaveLayout(path, layout); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(backup)
	if err != nil || string(original) != string(corrupt) {
		t.Fatalf("corrupt layout backup: %q, %v", original, err)
	}
	recovered, err := LoadLayout(path)
	if err != nil || recovered != layout {
		t.Fatalf("recovered layout: %+v, %v", recovered, err)
	}
}

func TestWorkspaceSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	snapshot := application.WorkspaceSnapshot{ActiveRepository: "vela", Repositories: []application.RepositorySnapshot{
		{Name: "vela", ActiveTab: "code:working:main.go", Tabs: []application.TabSnapshot{
			{Source: "working", Path: "main.go", Code: true, Cursor: 11, ScrollY: 25},
		}},
	}}
	if err := SaveSession(path, snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil || !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("session: %+v, %v", got, err)
	}
}

func TestLegacyLayoutUsesVersionControlDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	if err := os.WriteFile(path, []byte(`{"Dark":false,"Top":65}`), 0600); err != nil {
		t.Fatal(err)
	}
	layout, err := LoadLayout(path)
	if err != nil {
		t.Fatal(err)
	}
	defaults := application.DefaultLayout()
	if layout.Dark || layout.Top != 65 || layout.VersionControl != defaults.VersionControl ||
		layout.Appearance != defaults.Appearance || layout.Editor != defaults.Editor {
		t.Fatalf("legacy layout defaults: %+v", layout)
	}
}

func TestLayoutRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "layout.json")
	layout, err := LoadLayout(path)
	if err != nil || layout != application.DefaultLayout() {
		t.Fatalf("defaults: %+v, %v", layout, err)
	}
	layout.Top = 65
	layout.Dark = false
	layout.VersionControl.Log.Hash = true
	layout.VersionControl.Colors.Added = "#12AB34"
	layout.VersionControl.Colors.MergedPropertyConflicts = "#AB1234"
	layout.VersionControl.Git.UpdateMethod = "Rebase"
	layout.Appearance.FollowSystem = true
	layout.Appearance.Scale = 115
	layout.Editor.Font = "Consolas"
	layout.Editor.TabSize = 2
	if err := SaveLayout(path, layout); err != nil {
		t.Fatal(err)
	}
	recovered, err := LoadLayout(path)
	if err != nil || recovered != layout {
		t.Fatalf("recovered %+v, %v", recovered, err)
	}
}
