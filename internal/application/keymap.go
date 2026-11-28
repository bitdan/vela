package application

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// KeymapSettings is optional in legacy layouts. Published bindings are immutable;
// settings edits clone them before assigning or disabling a stable command ID.
type KeymapSettings struct {
	Bindings      map[string]string
	LocalBindings map[KeymapScope]map[string]string `json:",omitempty"`
}

type KeymapScope string

const (
	KeymapGlobal     KeymapScope = ""
	KeymapEditor     KeymapScope = "editor"
	KeymapList       KeymapScope = "list"
	KeymapToolWindow KeymapScope = "tool-window"
)

func KeymapScopes() []KeymapScope {
	return []KeymapScope{KeymapGlobal, KeymapEditor, KeymapList, KeymapToolWindow}
}

type KeyBinding struct{ Command, Shortcut string }

func WorkspaceShortcuts() []KeyBinding {
	return []KeyBinding{
		{"workspace.toggle-git", "Alt+0"}, {"workspace.toggle-commit", "Alt+1"}, {"workspace.toggle-theme", "Ctrl+Shift+T"},
		{"workspace.settings", "Ctrl+Alt+S"}, {"workspace.search-actions", "Ctrl+Shift+P"}, {"workspace.show-commit", "Ctrl+K"},
		{"git.push", "Ctrl+Shift+K"}, {"git.update-project", "Ctrl+T"}, {"git.new-branch", "Ctrl+Alt+N"},
		{"workspace.go-to-file", "Ctrl+Shift+N"}, {"workspace.recent-files", "Ctrl+E"}, {"workspace.focus-project", "Alt+Home"}, {"editor.close-tab", "Ctrl+W"},
	}
}

type Shortcut struct {
	Ctrl, Alt, Shift bool
	Key              string
}

type KeySequence struct {
	First, Second Shortcut
}

func (s KeySequence) String() string {
	if s.Second.Key == "" {
		return s.First.String()
	}
	return s.First.String() + " " + s.Second.String()
}

func (s KeySequence) Chord() bool { return s.Second.Key != "" }

func (s Shortcut) String() string {
	if s.Key == "" {
		return ""
	}
	var parts []string
	if s.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if s.Alt {
		parts = append(parts, "Alt")
	}
	if s.Shift {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, s.Key), "+")
}

func ParseShortcut(value string) (Shortcut, error) {
	var shortcut Shortcut
	if strings.TrimSpace(value) == "" {
		return shortcut, nil
	}
	bad := errors.New("use Ctrl or Alt, optional Shift, and A–Z, 0–9, F1–F12, Home or End")
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(value)), "+")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "CTRL":
			if shortcut.Ctrl {
				return Shortcut{}, bad
			}
			shortcut.Ctrl = true
		case "ALT":
			if shortcut.Alt {
				return Shortcut{}, bad
			}
			shortcut.Alt = true
		case "SHIFT":
			if shortcut.Shift {
				return Shortcut{}, bad
			}
			shortcut.Shift = true
		default:
			if shortcut.Key != "" {
				return Shortcut{}, bad
			}
			switch {
			case len(part) == 1 && (part[0] >= 'A' && part[0] <= 'Z' || part[0] >= '0' && part[0] <= '9'):
				shortcut.Key = part
			case part == "HOME":
				shortcut.Key = "Home"
			case part == "END":
				shortcut.Key = "End"
			case strings.HasPrefix(part, "F"):
				n, err := strconv.Atoi(part[1:])
				if err != nil || n < 1 || n > 12 || part != "F"+strconv.Itoa(n) {
					return Shortcut{}, bad
				}
				shortcut.Key = part
			default:
				return Shortcut{}, bad
			}
		}
	}
	if shortcut.Key == "" || !shortcut.Ctrl && !shortcut.Alt {
		return Shortcut{}, bad
	}
	return shortcut, nil
}

func ParseKeySequence(value string) (KeySequence, error) {
	var fields []string
	var stroke strings.Builder
	for _, field := range strings.Fields(value) {
		if field == "+" {
			stroke.WriteString(field)
			continue
		}
		if stroke.Len() > 0 && !strings.HasSuffix(stroke.String(), "+") && !strings.HasPrefix(field, "+") {
			fields = append(fields, stroke.String())
			stroke.Reset()
		}
		stroke.WriteString(field)
	}
	if stroke.Len() > 0 {
		fields = append(fields, stroke.String())
	}
	if len(fields) == 0 {
		return KeySequence{}, nil
	}
	if len(fields) > 2 {
		return KeySequence{}, errors.New("use one or two shortcut strokes separated by a space")
	}
	first, err := ParseShortcut(fields[0])
	if err != nil {
		return KeySequence{}, err
	}
	sequence := KeySequence{First: first}
	if len(fields) == 2 {
		sequence.Second, err = ParseShortcut(fields[1])
		if err != nil {
			return KeySequence{}, err
		}
	}
	return sequence, nil
}

var reservedShortcuts = []string{
	"Ctrl+A", "Ctrl+C", "Ctrl+X", "Ctrl+V", "Ctrl+Z", "Ctrl+Y", "Ctrl+Shift+Z", "Ctrl+S", "Ctrl+F", "Ctrl+H",
	"Ctrl+J", "Ctrl+L", "Ctrl+U", "Ctrl+Shift+L", "Ctrl+Shift+A", "Ctrl+Alt+L", "Ctrl+G", "Ctrl+E", "Ctrl+K", "Ctrl+Shift+K",
	"Ctrl+Home", "Ctrl+End", "Ctrl+Shift+Home", "Ctrl+Shift+End", "Alt+Z", "Alt+F5", "Alt+Shift+F5",
	"Ctrl+Shift+F", "Ctrl+Shift+R", "Ctrl+Alt+A", "Ctrl+Alt+P", "Ctrl+Alt+M", "Ctrl+Alt+Z", "Ctrl+Alt+Shift+C", "Alt+F4", "Ctrl+F4",
	"Ctrl+D", "Ctrl+F5", "Ctrl+Shift+U", "Ctrl+Shift+H", "Alt+Shift+M", "Ctrl+Alt+N",
}

func normalizeShortcut(id, value string, defaults []KeyBinding, index int) (string, error) {
	sequence, err := ParseKeySequence(value)
	if err != nil {
		return "", err
	}
	normalized := sequence.String()
	first := sequence.First.String()
	if slices.Contains(reservedShortcuts, first) && first != defaults[index].Shortcut && !(id == "editor.close-tab" && first == "Ctrl+F4") {
		return "", fmt.Errorf("%s is reserved for editing, panel navigation or the operating system", first)
	}
	return normalized, nil
}

func validateBindingCollisions(bindings []KeyBinding) error {
	for i, binding := range bindings {
		if binding.Shortcut == "" {
			continue
		}
		for _, other := range bindings[i+1:] {
			if other.Shortcut == "" {
				continue
			}
			if binding.Shortcut == other.Shortcut {
				return fmt.Errorf("%s is already assigned; change that action first", binding.Shortcut)
			}
			left, _, leftChord := strings.Cut(binding.Shortcut, " ")
			right, _, rightChord := strings.Cut(other.Shortcut, " ")
			if (!leftChord && rightChord && left == right) || (!rightChord && leftChord && left == right) {
				return fmt.Errorf("%s conflicts with another shortcut prefix; change that action first", binding.Shortcut)
			}
		}
	}
	return nil
}

// NormalizeKeymap validates the effective map, including defaults of unchanged
// commands. Empty strings disable bindings; default-equal overrides are omitted.
func NormalizeKeymap(overrides map[string]string) (map[string]string, error) {
	defaults := WorkspaceShortcuts()
	normalized := make(map[string]string)
	for _, id := range slices.Sorted(maps.Keys(overrides)) {
		n := slices.IndexFunc(defaults, func(binding KeyBinding) bool { return binding.Command == id })
		if n < 0 {
			return nil, errors.New("unsupported workspace shortcut action")
		}
		value, err := normalizeShortcut(id, overrides[id], defaults, n)
		if err != nil {
			return nil, err
		}
		if value != defaults[n].Shortcut {
			normalized[id] = value
		}
	}
	effective := make([]KeyBinding, 0, len(defaults))
	for _, binding := range defaults {
		value := binding.Shortcut
		if override, ok := normalized[binding.Command]; ok {
			value = override
		}
		binding.Shortcut = value
		effective = append(effective, binding)
	}
	if err := validateBindingCollisions(effective); err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func NormalizeKeymapSettings(settings *KeymapSettings) (*KeymapSettings, error) {
	if settings == nil {
		return nil, nil
	}
	bindings, err := NormalizeKeymap(settings.Bindings)
	if err != nil {
		return nil, err
	}
	defaults := WorkspaceShortcuts()
	locals := make(map[KeymapScope]map[string]string)
	for _, scope := range slices.Sorted(maps.Keys(settings.LocalBindings)) {
		if scope == KeymapGlobal || !slices.Contains(KeymapScopes(), scope) {
			return nil, errors.New("unsupported shortcut scope")
		}
		overrides := settings.LocalBindings[scope]
		normalized := make(map[string]string)
		effective := make([]KeyBinding, 0, len(overrides))
		for _, id := range slices.Sorted(maps.Keys(overrides)) {
			n := slices.IndexFunc(defaults, func(binding KeyBinding) bool { return binding.Command == id })
			if n < 0 {
				return nil, errors.New("unsupported workspace shortcut action")
			}
			value, err := normalizeShortcut(id, overrides[id], defaults, n)
			if err != nil {
				return nil, err
			}
			normalized[id] = value
			effective = append(effective, KeyBinding{Command: id, Shortcut: value})
		}
		if err := validateBindingCollisions(effective); err != nil {
			return nil, fmt.Errorf("%s: %w", scope, err)
		}
		if len(normalized) > 0 {
			locals[scope] = normalized
		}
	}
	if len(bindings) == 0 && len(locals) == 0 {
		return nil, nil
	}
	if len(locals) == 0 {
		locals = nil
	}
	return &KeymapSettings{Bindings: bindings, LocalBindings: locals}, nil
}

func EffectiveShortcut(settings *KeymapSettings, id string) (string, bool) {
	for _, binding := range WorkspaceShortcuts() {
		if binding.Command != id {
			continue
		}
		if settings != nil {
			if value, ok := settings.Bindings[id]; ok {
				shortcut, err := ParseKeySequence(value)
				if err == nil {
					return shortcut.String(), true
				}
			}
		}
		return binding.Shortcut, true
	}
	return "", false
}

func EffectiveShortcutForScope(settings *KeymapSettings, scope KeymapScope, id string) (string, bool) {
	if scope != KeymapGlobal && settings != nil {
		if overrides, ok := settings.LocalBindings[scope]; ok {
			if value, overridden := overrides[id]; overridden {
				sequence, err := ParseKeySequence(value)
				if err == nil {
					return sequence.String(), true
				}
			}
		}
	}
	return EffectiveShortcut(settings, id)
}
