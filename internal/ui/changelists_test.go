package ui

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
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
	policy := state.Policy()
	policy.EmptyInactive = "Remove"
	policy.KeepRemaining = false
	state.SetPolicy(policy)
	snapshot := a.SessionSnapshot()
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(snapshot)
	if b.Model.Active().Changelists.Owner(file.Path) != id || b.Model.Active().Changelists.Active != id || b.Model.Active().Changelists.Policy() != policy {
		t.Fatal("session lost lists")
	}
	snapshot.Repositories[0].Changelists.Lists[1].Paths[0] = "mutated"
	snapshot.Repositories[0].Changelists.Settings.KeepRemaining = true
	if a.Model.Active().Changelists.Owner(file.Path) != id || b.Model.Active().Changelists.Owner(file.Path) != id ||
		a.Model.Active().Changelists.Policy().KeepRemaining || b.Model.Active().Changelists.Policy().KeepRemaining {
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
	a.run(setActiveChangelist, commandTarget{source: id})
	policy := state.Policy()
	policy.EmptyInactive = "Remove"
	state.SetPolicy(policy)
	repo.Working = []domain.File{{Path: "b.go", OldPath: "a.go", Status: "R"}, {Path: "new.go"}}
	a.installRepository(repo)
	if a.Model.Active().Changelists.Owner("b.go") != id || a.Model.Active().Changelists.Owner("new.go") != id {
		t.Fatal("refresh lost membership")
	}
	repositoryIndex := a.Model.Index
	a.run(editChangelist, commandTarget{source: id})
	a.run(selectRepo, commandTarget{index: 0})
	if a.changelistDialog != nil || len(a.Model.Active().Changelists.Lists) != 1 || a.Model.Active().Changelists.Policy().EmptyInactive != "Show options" {
		t.Fatal("dialog/list leaked repositories")
	}
	a.run(selectRepo, commandTarget{index: repositoryIndex})
	if a.Model.Active().Changelists.Policy().EmptyInactive != "Remove" {
		t.Fatal("repository policy was not isolated")
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

func newChangelistCommitWorkspace(t *testing.T, emptyPolicy string, keepRemaining, named, partial bool, commitErr error) (*Workspace, string, application.ChangelistState) {
	t.Helper()
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = nil
	repo.Staged = []domain.File{{Path: "committed.go", Status: "M"}}
	if partial {
		repo.Working = []domain.File{{Path: "partial.go", Status: "M"}}
		repo.Staged = append(repo.Staged, domain.File{Path: "partial.go", Status: "M"})
	}
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(repo))
	state := &a.Model.Active().Changelists
	id := "default"
	if named {
		var err error
		id, err = state.Create("Committed feature")
		if err != nil {
			t.Fatal(err)
		}
		paths := []string{"committed.go"}
		if partial {
			paths = append(paths, "partial.go")
		}
		if err := state.Move(paths, id); err != nil {
			t.Fatal(err)
		}
	}
	policy := state.Policy()
	policy.EmptyInactive = emptyPolicy
	policy.KeepRemaining = keepRemaining
	state.SetPolicy(policy)
	before := application.CloneChangelists(*state)
	a.Model.Active().Message = "Commit changelist policy"
	a.CommitRepository = func(ctx context.Context, _ domain.Repository, _ string, _ bool, done func(domain.Repository, error, error)) {
		if commitErr != nil {
			done(repo, commitErr, nil)
			return
		}
		updated := repo
		updated.Head = "def"
		updated.IndexVersion = "next-index"
		updated.Staged = nil
		done(updated, nil, nil)
	}
	return a, id, before
}

func TestChangelistCommitEmptyPolicies(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		action     string
		named      bool
		commitErr  error
		wantLists  int
		wantPrompt bool
	}{
		{name: "silent remove", policy: "Remove", named: true, wantLists: 1},
		{name: "silent keep", policy: "Keep", named: true, wantLists: 2},
		{name: "confirm remove", policy: "Show options", action: "Remove Empty Changelists", named: true, wantLists: 1, wantPrompt: true},
		{name: "confirm keep", policy: "Show options", action: "Keep Empty Changelists", named: true, wantLists: 2, wantPrompt: true},
		{name: "default survives remove", policy: "Remove", wantLists: 1},
		{name: "failed commit unchanged", policy: "Remove", named: true, commitErr: errors.New("fixture commit failed"), wantLists: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, id, before := newChangelistCommitWorkspace(t, test.policy, true, test.named, false, test.commitErr)
			a.Model.Active().Changelists.Active = id
			if test.named {
				a.repo().changelistFilter = id
			}
			before.Active = id
			before.Normalize(a.Model.Active().Repository)
			tt := native.NewTester(a.View, 1440, 900)
			a.run(commitRepository, commandTarget{})
			if test.wantPrompt != (a.emptyChangelists != nil) {
				t.Fatalf("confirmation present = %v, want %v", a.emptyChangelists != nil, test.wantPrompt)
			}
			if test.commitErr != nil {
				if !reflect.DeepEqual(a.Model.Active().Changelists, before) {
					t.Fatalf("failed commit changed lists: got %+v want %+v", a.Model.Active().Changelists, before)
				}
				return
			}
			if test.action != "" {
				tt.Frame()
				if a.emptyChangelists == nil || !tt.HasText("Remove empty changelists?") {
					t.Fatalf("confirmation did not render: pending=%+v texts=%v", a.emptyChangelists, tt.Texts())
				}
				if err := tt.Click(test.action); err != nil {
					t.Fatal(err)
				}
			}
			state := a.Model.Active().Changelists
			if len(state.Lists) != test.wantLists || a.emptyChangelists != nil {
				t.Fatalf("post-commit lists: %+v prompt=%+v", state, a.emptyChangelists)
			}
			if test.named && test.wantLists == 1 && state.Active != "default" {
				t.Fatal("removing the active list did not activate Default")
			}
			if test.named && test.wantLists == 1 && a.repo().changelistFilter != "" {
				t.Fatal("removing a filtered list left the working view hidden")
			}
			if len(state.Lists) == 0 || state.Lists[0].ID != "default" {
				t.Fatal("default list was removed")
			}
		})
	}
}

func TestChangelistCommitKeepsListEditsMadeDuringCommit(t *testing.T) {
	a, _, _ := newChangelistCommitWorkspace(t, "Remove", true, true, false, nil)
	commit := a.CommitRepository
	a.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
		// The user creates a list while Git is still committing.
		if _, err := a.Model.Active().Changelists.Create("Created during commit"); err != nil {
			t.Fatal(err)
		}
		commit(ctx, repo, message, amend, done)
	}
	a.run(commitRepository, commandTarget{})
	state := a.Model.Active().Changelists
	if !slices.ContainsFunc(state.Lists, func(list application.Changelist) bool { return list.Name == "Created during commit" }) {
		t.Fatalf("commit completion discarded a list created during the commit: %+v", state.Lists)
	}
	if slices.ContainsFunc(state.Lists, func(list application.Changelist) bool { return list.Name == "Committed feature" }) {
		t.Fatalf("empty committed list was not removed by the Remove policy: %+v", state.Lists)
	}
}

func TestChangelistCommitRemainingChangesPolicy(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(map[bool]string{true: "keep original", false: "move to active"}[keep], func(t *testing.T) {
			a, id, _ := newChangelistCommitWorkspace(t, "Keep", keep, true, true, nil)
			a.Model.Active().Changelists.Active = "default"
			a.run(commitRepository, commandTarget{})
			want := "default"
			if keep {
				want = id
			}
			if owner := a.Model.Active().Changelists.Owner("partial.go"); owner != want {
				t.Fatalf("remaining owner = %q, want %q", owner, want)
			}
		})
	}
}

func TestChangelistSettingsApplyPerRepository(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	other := a.Model.Sessions[1].Changelists.Policy()
	tt := native.NewTester(a.View, 1440, 900)
	for _, label := range []string{"Settings", "Settings Changelists", "Keep remaining changes in their original changelists", "Empty changelist policy"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	policy := a.Model.Active().Changelists.Policy()
	if policy.KeepRemaining || policy.EmptyInactive != "Remove" {
		t.Fatalf("settings did not apply: %+v", policy)
	}
	if got := a.Model.Sessions[1].Changelists.Policy(); got != other {
		t.Fatalf("settings leaked to another repository: %+v", got)
	}
}

func TestChangelistPolicyScreenshots(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a, _, _ := newChangelistCommitWorkspace(t, "Show options", true, true, false, nil)
			a.Layout.Dark = dark
			tt := native.NewTester(a.View, 1440, 900)
			for _, label := range []string{"Settings", "Settings Changelists"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			exportSessionScreenshot(t, tt, "changelist-policy-settings-"+map[bool]string{false: "light", true: "dark"}[dark])
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			a.run(commitRepository, commandTarget{})
			if a.emptyChangelists == nil {
				t.Fatalf("commit did not request confirmation: %s", a.status)
			}
			exportSessionScreenshot(t, tt, "changelist-policy-confirmation-"+map[bool]string{false: "light", true: "dark"}[dark])
			tt.Frame()
			if err := tt.Click("Keep Empty Changelists"); err != nil {
				t.Fatal(err)
			}
		})
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
