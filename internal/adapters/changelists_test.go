package adapters

import (
	"path/filepath"
	"reflect"
	"testing"
	"vela/internal/application"
)

func TestChangelistsJSONRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	want := application.WorkspaceSnapshot{Repositories: []application.RepositorySnapshot{{Root: "repo", Changelists: &application.ChangelistState{Active: "feature", Lists: []application.Changelist{{ID: "default", Name: "Default"}, {ID: "feature", Name: "中文", Paths: []string{"a b.go"}}}}}}}
	if err := SaveSession(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("changelists JSON: %+v %v", got, err)
	}
}
