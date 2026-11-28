package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
)

func TestLayoutKeymapRoundTripAndInvalidWritePreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{
		Bindings: map[string]string{"workspace.toggle-git": "ctrl+alt+g", "workspace.toggle-commit": ""},
		LocalBindings: map[application.KeymapScope]map[string]string{
			application.KeymapEditor: {"workspace.toggle-theme": "ctrl+alt+j ctrl+alt+c"},
		},
	}
	if err := SaveLayout(path, layout); err != nil {
		t.Fatal(err)
	}
	recovered, err := LoadLayout(path)
	if err != nil || recovered.Keymap == nil || recovered.Keymap.Bindings["workspace.toggle-git"] != "Ctrl+Alt+G" {
		t.Fatalf("keymap not normalized/persisted %+v %v", recovered.Keymap, err)
	}
	if got := recovered.Keymap.LocalBindings[application.KeymapEditor]["workspace.toggle-theme"]; got != "Ctrl+Alt+J Ctrl+Alt+C" {
		t.Fatalf("scoped chord not persisted: %q", got)
	}
	before, _ := os.ReadFile(path)
	layout.Keymap.Bindings["workspace.toggle-git"] = "Ctrl+S"
	if err := SaveLayout(path, layout); err == nil {
		t.Fatal("invalid protected binding saved")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("failed validation changed layout")
	}
}

func TestLegacyKeymapJSONLoadsAndSavesWithoutScopeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	data := `{"Top":58,"Commit":26,"Branches":22,"Log":64,"Files":54,"Keymap":{"Bindings":{"workspace.toggle-git":"ctrl+alt+g"}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLayout(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Keymap == nil || loaded.Keymap.Bindings["workspace.toggle-git"] != "Ctrl+Alt+G" || loaded.Keymap.LocalBindings != nil {
		t.Fatalf("legacy keymap changed during load: %+v", loaded.Keymap)
	}
	if err := SaveLayout(path, loaded); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "LocalBindings") {
		t.Fatalf("legacy keymap gained a scope field: %s", saved)
	}
}

func TestInvalidKeymapUsesExistingLayoutBackupPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	data := `{"Keymap":{"Bindings":{"workspace.toggle-git":"Ctrl+Shift+P"}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLayout(path)
	if !errors.Is(err, ErrInvalidLayout) || loaded != application.DefaultLayout() {
		t.Fatal("invalid/conflicting keymap did not use existing invalid-layout policy", err)
	}
	backup, err := BackupLayout(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(backup)
	if string(saved) != data {
		t.Fatal("invalid keymap configuration was not preserved")
	}
}
