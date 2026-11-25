package ui

import (
	"context"
	native "github.com/egoist/mygo/ui"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"vela/internal/adapters"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestShelfGroupsNativeAndSession(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.Shelf = []domain.Shelf{{ID: "stable-one", OID: strings.Repeat("a", 40), Title: "First backup"}, {ID: "stable-two", OID: strings.Repeat("b", 40), Title: "Second backup"}}
			a.Model.Switch(a.installRepository(r))
			a.run(openShelf, commandTarget{})
			before := append([]domain.Shelf(nil), r.Shelf...)
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("New Shelf Group"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Shelf group name"); err != nil {
				t.Fatal(err)
			}
			tt.Type("Feature 中文")
			if err := tt.Click("Apply Shelf Group"); err != nil {
				t.Fatal(err)
			}
			id := a.repo().shelfGroups.Groups[0].ID
			if err := tt.RightClick("First backup"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Move to Group", "Feature 中文"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Shelf Group Filter"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Feature 中文"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !tt.HasText("First backup") || tt.HasText("Second backup") {
				t.Fatal("group filter failed")
			}
			exportSessionScreenshot(t, tt, "shelf-groups-"+theme)
			snap := a.SessionSnapshot()
			path := filepath.Join(t.TempDir(), "session.json")
			if err := adapters.SaveSession(path, snap); err != nil {
				t.Fatal(err)
			}
			saved, err := adapters.LoadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			var state *application.ShelfGrouping
			for _, repo := range saved.Repositories {
				if repo.Root == r.Root {
					state = repo.ShelfGroups
				}
			}
			if state == nil || state.Assignments["stable-one"] != id || state.Filter != id {
				t.Fatal("group persistence lost")
			}
			if err := tt.Click("Manage Shelf Group"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Rename Shelf Group…"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			_ = tt.Click("Shelf group name")
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type("Renamed")
			_ = tt.Click("Apply Shelf Group")
			tt.Frame()
			if a.repo().shelfGroups.Groups[0].Name != "Renamed" {
				t.Fatal("rename failed")
			}
			if err := tt.Click("Manage Shelf Group"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Delete Shelf Group…"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if err := tt.Click("Apply Shelf Group"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if len(a.repo().shelfGroups.Groups) != 0 || len(a.repo().shelfGroups.Assignments) != 0 {
				t.Fatal("delete did not preserve entries in default")
			}
			if !reflect.DeepEqual(before, a.Model.Active().Repository.Shelf) {
				t.Fatal("metadata edited backup objects")
			}
			b := New(application.NewWorkspace(), application.DefaultLayout())
			b.RestoreSession(saved)
			saved.Repositories[0].ShelfGroups = nil
			out := b.SessionSnapshot()
			if len(out.Repositories) == 0 {
				t.Fatal("unavailable roots lost")
			}
			var found bool
			for _, repo := range out.Repositories {
				if repo.Root == r.Root {
					found = repo.ShelfGroups != nil && repo.ShelfGroups.Assignments["stable-one"] == id
				}
			}
			if !found {
				t.Fatal("unavailable repository grouping lost")
			}
		})
	}
}

func TestShelfGroupsIdentityIsolationAndRestore(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.Shelf = []domain.Shelf{{ID: "shelf", OID: "original"}}
	a.Model.Switch(a.installRepository(r))
	index := a.Model.Index
	id, _ := a.repo().shelfGroups.Create("Saved")
	a.run(moveShelfGroup, commandTarget{shelfID: "shelf", shelfOID: "stale", source: id})
	if len(a.repo().shelfGroups.Assignments) != 0 {
		t.Fatal("stale backup identity moved")
	}
	a.run(moveShelfGroup, commandTarget{shelfID: "shelf", shelfOID: "original", source: id})
	a.Model.Active().Repository.Shelf = nil // Temporary disappearance must not prune assignments.
	snapshot := a.SessionSnapshot()
	a.selectRepository(0)
	if len(a.repo().shelfGroups.Groups) != 0 {
		t.Fatal("groups crossed roots")
	}
	a.selectRepository(index)
	if a.repo().shelfGroups.Assignments["shelf"] != id {
		t.Fatal("refresh lost unavailable assignment")
	}
	a.repo().shelfGroups.Groups[0].Name = "Changed"
	var saved application.RepositorySnapshot
	for _, v := range snapshot.Repositories {
		if v.Root == r.Root {
			saved = v
		}
	}
	if saved.ShelfGroups.Groups[0].Name != "Saved" {
		t.Fatal("snapshot shares state")
	}
	a.restoreRepositorySelections(saved)
	saved.ShelfGroups.Assignments["shelf"] = "bad"
	if a.repo().shelfGroups.Assignments["shelf"] != id {
		t.Fatal("restore shares assignments")
	}
}

func TestShelfGroupsRealBackupObjectsUnchanged(t *testing.T) {
	root := t.TempDir()
	g := gitcli.New()
	ctx := context.Background()
	if _, err := g.Initialize(ctx, root, "feature"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return string(out)
	}
	path := filepath.Join(root, "中文 file.txt")
	if err := os.WriteFile(path, []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	if err := os.WriteFile(path, []byte("saved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, err := g.ReadRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err = g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "backup", Paths: []string{"中文 file.txt"}}); err != nil {
		t.Fatal(err)
	}
	repo, err = g.ReadRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	refs, index, status := git("show-ref"), git("write-tree"), git("status", "--porcelain=v1", "-z")
	a := New(application.NewWorkspace(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		t.Fatal("group invoked Git write")
	}
	group, _ := a.repo().shelfGroups.Create("feature")
	entry := repo.Shelf[0]
	a.run(moveShelfGroup, commandTarget{shelfID: entry.ID, shelfOID: entry.OID, source: group})
	_ = a.repo().shelfGroups.Rename(group, "renamed")
	_ = a.repo().shelfGroups.Delete(group)
	if refs != git("show-ref") || index != git("write-tree") || status != git("status", "--porcelain=v1", "-z") {
		t.Fatal("metadata changed real Git state")
	}
	after, err := g.ReadRepository(ctx, root)
	if err != nil || !reflect.DeepEqual(after.Shelf, repo.Shelf) {
		t.Fatal("backup objects changed", err)
	}
}
