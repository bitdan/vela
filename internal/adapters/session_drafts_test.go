package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
)

func TestSessionRejectsOversizedDraftWithoutReplacingPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	previous := []byte(`{"ActiveRepository":"keep"}`)
	if err := os.WriteFile(path, previous, 0600); err != nil {
		t.Fatal(err)
	}
	raw := `{"Repositories":[{"Root":"C:/repo","Tabs":[{"Source":"project","Path":"a.txt","Code":true,"Draft":{"Text":` + `"` + strings.Repeat("x", 2*1024*1024+1) + `","DiskVersion":"v1","LineEnding":"\n"}}]}]}`
	var snapshot application.WorkspaceSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := SaveSession(path, snapshot); err == nil {
		t.Fatal("oversized draft accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(previous) {
		t.Fatal("invalid save replaced prior session")
	}
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSession(path); err == nil {
		t.Fatal("oversized draft loaded")
	}
}
