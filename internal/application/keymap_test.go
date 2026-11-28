package application

import "testing"

func TestKeySequenceParsingAndSerialization(t *testing.T) {
	for input, want := range map[string]string{
		" ctrl + alt + g ":            "Ctrl+Alt+G",
		"Ctrl+K Ctrl+C":               "Ctrl+K Ctrl+C",
		" ctrl + alt + j  ctrl + c ":  "Ctrl+Alt+J Ctrl+C",
		"Ctrl+Shift+F12 Ctrl+Alt+End": "Ctrl+Shift+F12 Ctrl+Alt+End",
	} {
		sequence, err := ParseKeySequence(input)
		if err != nil {
			t.Fatalf("ParseKeySequence(%q): %v", input, err)
		}
		if got := sequence.String(); got != want {
			t.Errorf("ParseKeySequence(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"Ctrl+A Ctrl+B Ctrl+C", "Ctrl+A B", "Ctrl+A Ctrl", "Ctrl+A  Ctrl+B  Ctrl+C"} {
		if _, err := ParseKeySequence(input); err == nil {
			t.Errorf("invalid sequence %q accepted", input)
		}
	}
}

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

func TestKeymapPrefixConflictsAndSharedChordPrefix(t *testing.T) {
	for _, bindings := range []map[string]string{
		{"workspace.toggle-git": "Ctrl+Alt+J", "workspace.toggle-commit": "Ctrl+Alt+J Ctrl+Alt+C"},
		{"workspace.toggle-git": "Ctrl+Alt+J Ctrl+Alt+C", "workspace.toggle-commit": "Ctrl+Alt+J"},
	} {
		if _, err := NormalizeKeymap(bindings); err == nil {
			t.Fatalf("prefix conflict accepted: %v", bindings)
		}
	}
	bindings := map[string]string{
		"workspace.toggle-git":    "Ctrl+Alt+J Ctrl+Alt+C",
		"workspace.toggle-commit": "Ctrl+Alt+J Ctrl+Alt+V",
	}
	if _, err := NormalizeKeymap(bindings); err != nil {
		t.Fatalf("independent chords with a shared prefix rejected: %v", err)
	}
	settings := &KeymapSettings{LocalBindings: map[KeymapScope]map[string]string{
		KeymapEditor: {"workspace.toggle-git": "Ctrl+Alt+J", "workspace.toggle-commit": "Ctrl+Alt+J Ctrl+Alt+C"},
	}}
	if _, err := NormalizeKeymapSettings(settings); err == nil {
		t.Fatal("local prefix conflict accepted")
	}
}

func TestScopedKeymapOverridesAndValidation(t *testing.T) {
	settings := &KeymapSettings{
		Bindings: map[string]string{"workspace.toggle-git": "Ctrl+Alt+G"},
		LocalBindings: map[KeymapScope]map[string]string{
			KeymapEditor:     {"workspace.toggle-theme": "Ctrl+Alt+G", "workspace.toggle-git": ""},
			KeymapList:       {"workspace.toggle-git": "Ctrl+Alt+J Ctrl+Alt+C"},
			KeymapToolWindow: {"workspace.toggle-git": "Alt+0"},
		},
	}
	normalized, err := NormalizeKeymapSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := normalized.LocalBindings[KeymapToolWindow]["workspace.toggle-git"]; got != "Alt+0" {
		t.Fatalf("default-equal local override was discarded: %q", got)
	}
	if got, _ := EffectiveShortcutForScope(normalized, KeymapGlobal, "workspace.toggle-theme"); got != "Ctrl+Shift+T" {
		t.Fatalf("global binding changed: %q", got)
	}
	if got, _ := EffectiveShortcutForScope(normalized, KeymapEditor, "workspace.toggle-theme"); got != "Ctrl+Alt+G" {
		t.Fatalf("local binding did not override another command's global binding: %q", got)
	}
	if got, _ := EffectiveShortcutForScope(normalized, KeymapEditor, "workspace.toggle-git"); got != "" {
		t.Fatalf("local disable did not suppress global binding: %q", got)
	}
	if got, _ := EffectiveShortcutForScope(normalized, KeymapList, "workspace.toggle-git"); got != "Ctrl+Alt+J Ctrl+Alt+C" {
		t.Fatalf("local chord = %q", got)
	}
	if got, _ := EffectiveShortcutForScope(normalized, KeymapToolWindow, "workspace.toggle-commit"); got != "Alt+1" {
		t.Fatalf("unoverridden local binding did not inherit global default: %q", got)
	}
	for _, invalid := range []*KeymapSettings{
		{LocalBindings: map[KeymapScope]map[string]string{KeymapGlobal: {"workspace.toggle-git": "Alt+0"}}},
		{LocalBindings: map[KeymapScope]map[string]string{"dialog": {"workspace.toggle-git": "Alt+0"}}},
		{LocalBindings: map[KeymapScope]map[string]string{KeymapEditor: {"workspace.toggle-git": "Ctrl+C"}}},
	} {
		if _, err := NormalizeKeymapSettings(invalid); err == nil {
			t.Fatalf("invalid scoped keymap accepted: %+v", invalid)
		}
	}
}
