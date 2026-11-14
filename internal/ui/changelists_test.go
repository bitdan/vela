package ui

import (
	native "github.com/egoist/mygo/ui"
	"testing"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestChangelistNativeLifecycleAndSession(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.run(newChangelist, commandTarget{})
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Changelist name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Feature 中文")
	if err := tt.Click("Apply Changelist"); err != nil {
		t.Fatal(err)
	}
	state := &a.Model.Active().Changelists
	if len(state.Lists) != 2 {
		t.Fatal("create did not add list")
	}
	id := state.Lists[1].ID
	file := a.Model.Active().Repository.Working[0]
	a.run(moveChangelist, commandTarget{source: "working", file: file})
	tt.Frame()
	if err := tt.Click("Changelist destination"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyEnd)
	tt.Key(0, native.KeyEnter)
	if err := tt.Click("Apply Changelist"); err != nil {
		t.Fatal(err)
	}
	if state.Owner(file.Path) != id {
		t.Fatal("file did not move")
	}
	a.run(setActiveChangelist, commandTarget{source: id})
	snapshot := a.SessionSnapshot()
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(snapshot)
	if b.Model.Active().Changelists.Owner(file.Path) != id || b.Model.Active().Changelists.Active != id {
		t.Fatal("session lost lists")
	}
	snapshot.Repositories[0].Changelists.Lists[1].Paths[0] = "mutated"
	if a.Model.Active().Changelists.Owner(file.Path) != id || b.Model.Active().Changelists.Owner(file.Path) != id {
		t.Fatal("session aliases changelist paths")
	}
	a.run(removeChangelist, commandTarget{source: id})
	tt.Frame()
	if err := tt.Click("Apply Changelist"); err != nil {
		t.Fatal(err)
	}
	if state.Owner(file.Path) != "default" {
		t.Fatal("remove lost file")
	}
}

func TestChangelistRefreshAndRepositoryIsolation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.Working = []domain.File{{Path: "a.go"}}
	a.Model.Switch(a.installRepository(repo))
	state := &a.Model.Active().Changelists
	id, err := state.Create("Feature")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Move([]string{"a.go"}, id); err != nil {
		t.Fatal(err)
	}
	repo.Working = []domain.File{{Path: "b.go", OldPath: "a.go", Status: "R"}}
	a.installRepository(repo)
	if a.Model.Active().Changelists.Owner("b.go") != id {
		t.Fatal("refresh lost membership")
	}
	a.run(editChangelist, commandTarget{source: id})
	a.run(selectRepo, commandTarget{index: 0})
	if a.changelistDialog != nil || len(a.Model.Active().Changelists.Lists) != 1 {
		t.Fatal("dialog/list leaked repositories")
	}
}

func TestChangelistFilterClearsHiddenSelection(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	files := a.Model.Active().Repository.Working
	a.repo().workingChosen.Add(files[0].Path)
	a.repo().workingChosen.Add(files[1].Path)
	a.run(filterChangelist, commandTarget{source: "default"})
	if a.repo().workingChosen.Has(files[0].Path) || a.repo().workingChosen.Has(files[1].Path) {
		t.Fatal("filter retained hidden selection")
	}
}

func TestChangelistEditTargetsClickedFilesList(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	state := &a.Model.Active().Changelists
	id, err := state.Create("Inactive feature")
	if err != nil {
		t.Fatal(err)
	}
	file := a.Model.Active().Repository.Working[0]
	if err := state.Move([]string{file.Path}, id); err != nil {
		t.Fatal(err)
	}
	a.run(editChangelist, commandTarget{source: "working", file: file})
	if a.changelistDialog == nil || a.changelistDialog.id != id {
		t.Fatal("Edit did not target clicked file's changelist")
	}
}
