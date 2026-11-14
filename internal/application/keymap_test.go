package application

import "testing"

func TestNormalizeWorkspaceKeymap(t *testing.T) {
	got, err := NormalizeKeymap(map[string]string{"workspace.toggle-git": " ctrl + alt + g ", "workspace.toggle-commit": ""})
	if err != nil {
		t.Fatal(err)
	}
	if got["workspace.toggle-git"] != "Ctrl+Alt+G" || got["workspace.toggle-commit"] != "" {
		t.Fatalf("normalization changed intent %v", got)
	}
	for _, input := range []map[string]string{
		{"unknown": "Ctrl+Alt+G"},
		{"workspace.toggle-git": "G"},
		{"workspace.toggle-git": "Ctrl+Ctrl+G"},
		{"workspace.toggle-git": "Ctrl+S"},
		{"workspace.toggle-git": "Ctrl+J"},
		{"workspace.toggle-git": "Ctrl+Home"},
		{"workspace.toggle-git": "Alt+Z"},
		{"workspace.toggle-git": "Alt+F4"},
		{"workspace.toggle-git": "Ctrl+Shift+P"},
		{"workspace.toggle-git": "Ctrl+Alt+G", "workspace.toggle-commit": "Ctrl+Alt+G"},
	} {
		if _, err := NormalizeKeymap(input); err == nil {
			t.Fatalf("invalid/conflicting bindings accepted %v", input)
		}
	}
}

func TestKeymapCanMoveExistingBindingAndRestoreDefault(t *testing.T) {
	got, err := NormalizeKeymap(map[string]string{"workspace.toggle-git": "Alt+1", "workspace.toggle-commit": ""})
	if err != nil {
		t.Fatal(err)
	}
	if got["workspace.toggle-git"] != "Alt+1" {
		t.Fatal("disabled default still reserved")
	}
	if _, err = NormalizeKeymap(map[string]string{"editor.close-tab": "Ctrl+F4"}); err != nil {
		t.Fatal("legacy close alias could not be selected", err)
	}
	if _, err = ParseShortcut("Ctrl+Shift+F12"); err != nil {
		t.Fatal(err)
	}
}
