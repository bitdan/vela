package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vela/internal/application"
)

func TestLayoutKeymapRoundTripAndInvalidWritePreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{Bindings: map[string]string{"workspace.toggle-git": "ctrl+alt+g", "workspace.toggle-commit": ""}}
	if err := SaveLayout(path, layout); err != nil {
		t.Fatal(err)
	}
	recovered, err := LoadLayout(path)
	if err != nil || recovered.Keymap == nil || recovered.Keymap.Bindings["workspace.toggle-git"] != "Ctrl+Alt+G" {
		t.Fatalf("keymap not normalized/persisted %+v %v", recovered.Keymap, err)
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
