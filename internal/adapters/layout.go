package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"vela/internal/application"
)

var ErrInvalidLayout = errors.New("invalid layout")

func LoadLayout(path string) (application.Layout, error) {
	defaults := application.DefaultLayout()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("read layout: %w", err)
	}
	layout := defaults
	if err := json.Unmarshal(data, &layout); err != nil {
		return defaults, fmt.Errorf("%w: %v", ErrInvalidLayout, err)
	}
	for _, split := range []float32{layout.Top, layout.Commit, layout.Branches, layout.Log, layout.Files} {
		if split <= 0 || split >= 100 {
			return defaults, fmt.Errorf("%w: split percentage %g", ErrInvalidLayout, split)
		}
	}
	if layout.Keymap != nil {
		bindings, err := application.NormalizeKeymap(layout.Keymap.Bindings)
		if err != nil {
			return defaults, fmt.Errorf("%w: shortcuts: %v", ErrInvalidLayout, err)
		}
		layout.Keymap = nil
		if len(bindings) > 0 {
			layout.Keymap = &application.KeymapSettings{Bindings: bindings}
		}
	}
	return layout, nil
}

// BackupLayout preserves an invalid configuration before saving its replacement.
func BackupLayout(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read layout for backup: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-invalid-*.json")
	if err != nil {
		return "", fmt.Errorf("create layout backup: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("write layout backup: %w", err)
	}
	return file.Name(), nil
}

func SaveLayout(path string, layout application.Layout) error {
	if layout.Keymap != nil {
		bindings, err := application.NormalizeKeymap(layout.Keymap.Bindings)
		if err != nil {
			return fmt.Errorf("save shortcuts: %w", err)
		}
		layout.Keymap = nil
		if len(bindings) > 0 {
			layout.Keymap = &application.KeymapSettings{Bindings: bindings}
		}
	}
	if err := saveJSON(path, layout); err != nil {
		return fmt.Errorf("save layout: %w", err)
	}
	return nil
}

func saveJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "layout-*.json")
	if err != nil {
		return fmt.Errorf("create layout file: %w", err)
	}
	defer os.Remove(tmp.Name())
	writeErr := json.NewEncoder(tmp).Encode(value)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return fmt.Errorf("write layout: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace layout: %w", err)
	}
	return nil
}
