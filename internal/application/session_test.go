package application

import "testing"

func TestRepositorySessionsAreIndependent(t *testing.T) {
	app := New()
	first := app.Active()
	first.Working = "cmd/vela/main.go"
	first.Staged = "README.md"
	first.Branch = "feature/native-ui"
	first.Commit = 3
	first.Historical = "go.mod"
	app.Switch(1)
	second := app.Active()
	if second == first || second.Working != "" || second.Branch == first.Branch {
		t.Fatal("repository selection leaked")
	}
	app.Switch(0)
	if app.Active() != first || first.Commit != 3 || first.Historical != "go.mod" || first.Staged != "README.md" {
		t.Fatal("returning to repository lost the selections")
	}
}
