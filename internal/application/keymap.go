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
type KeymapSettings struct{ Bindings map[string]string }

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
		shortcut, err := ParseShortcut(overrides[id])
		if err != nil {
			return nil, err
		}
		value := shortcut.String()
		// Supported first strokes of locked MyGo v0.8.3 editor bindings, local
		// panel commands and OS keys. Existing defaults retain their old scopes.
		reserved := []string{
			"Ctrl+A", "Ctrl+C", "Ctrl+X", "Ctrl+V", "Ctrl+Z", "Ctrl+Y", "Ctrl+Shift+Z", "Ctrl+S", "Ctrl+F", "Ctrl+H",
			"Ctrl+J", "Ctrl+L", "Ctrl+U", "Ctrl+Shift+L", "Ctrl+Shift+A", "Ctrl+Alt+L", "Ctrl+G", "Ctrl+E", "Ctrl+K", "Ctrl+Shift+K",
			"Ctrl+Home", "Ctrl+End", "Ctrl+Shift+Home", "Ctrl+Shift+End", "Alt+Z", "Alt+F5", "Alt+Shift+F5",
			"Ctrl+Shift+F", "Ctrl+Shift+R", "Ctrl+Alt+A", "Ctrl+Alt+P", "Ctrl+Alt+M", "Ctrl+Alt+Z", "Ctrl+Alt+Shift+C", "Alt+F4", "Ctrl+F4",
			"Ctrl+D", "Ctrl+F5", "Ctrl+Shift+U", "Ctrl+Shift+H", "Alt+Shift+M", "Ctrl+Alt+N",
		}
		if slices.Contains(reserved, value) && value != defaults[n].Shortcut && !(id == "editor.close-tab" && value == "Ctrl+F4") {
			return nil, fmt.Errorf("%s is reserved for editing, panel navigation or the operating system", value)
		}
		if value != defaults[n].Shortcut {
			normalized[id] = value
		}
	}
	used := map[string]bool{}
	for _, binding := range defaults {
		value := binding.Shortcut
		if override, ok := normalized[binding.Command]; ok {
			value = override
		}
		if value == "" {
			continue
		}
		if used[value] {
			return nil, fmt.Errorf("%s is already assigned; change that action first", value)
		}
		used[value] = true
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func EffectiveShortcut(settings *KeymapSettings, id string) (string, bool) {
	for _, binding := range WorkspaceShortcuts() {
		if binding.Command != id {
			continue
		}
		if settings != nil {
			if value, ok := settings.Bindings[id]; ok {
				shortcut, err := ParseShortcut(value)
				if err == nil {
					return shortcut.String(), true
				}
			}
		}
		return binding.Shortcut, true
	}
	return "", false
}
