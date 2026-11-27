package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"vela/internal/application"
)

func TestSessionShelfGroupsRejectInvalidAndLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	bad := application.WorkspaceSnapshot{Repositories: []application.RepositorySnapshot{{ShelfGroups: &application.ShelfGrouping{Assignments: map[string]string{"shelf": "missing"}}}}}
	if err := SaveSession(path, bad); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("invalid assignments saved", err)
	}
	if err := os.WriteFile(path, []byte(`{"Repositories":[{"ShelfGroups":{"Groups":[{"ID":"id","Name":"bad\nname"}]}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSession(path); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("invalid persisted groups loaded", err)
	}
	if err := os.WriteFile(path, []byte(`{"Repositories":[{"Name":"legacy"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil || got.Repositories[0].ShelfGroups != nil {
		t.Fatal("legacy altered", err)
	}
}
