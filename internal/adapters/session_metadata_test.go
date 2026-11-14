package adapters

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vela/internal/application"
)

func TestSessionEditorMetadataRoundtripAndLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	want := application.WorkspaceSnapshot{Repositories: []application.RepositorySnapshot{{Name: "repo", Tabs: []application.TabSnapshot{{Source: "project", Path: "你好.go", Code: true, Bookmarks: []int{0, 7}, FileType: "Plain Text"}}}}}
	if err := SaveSession(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata roundtrip: %+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"Repositories":[{"Name":"old","Tabs":[{"Source":"project","Path":"old.go","Code":true}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := LoadSession(path)
	if err != nil || legacy.Repositories[0].Tabs[0].FileType != "" || len(legacy.Repositories[0].Tabs[0].Bookmarks) != 0 {
		t.Fatal("legacy session rejected or changed")
	}
}
