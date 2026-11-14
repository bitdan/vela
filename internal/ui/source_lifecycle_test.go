package ui

import (
	"context"
	"errors"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func sourceLifecycleFixture(t *testing.T, dark bool) (*Workspace, domain.Repository, domain.File) {
	t.Helper()
	layout := application.DefaultLayout()
	layout.Dark = dark
	a := New(application.New(), layout)
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	a.Model.Switch(a.installRepository(repo))
	a.SaveRepositoryFile = func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error)) {
	}
	file := domain.File{Path: "source.txt", New: "saved text\n", DiskVersion: "original"}
	a.openCode("project", file)
	return a, repo, file
}

func TestSourceCloseCancelDiscardAndReopen(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a, _, file := sourceLifecycleFixture(t, dark)
			entry := a.activeDocument()
			entry.code.Buffer.Set("unsaved text\n")
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			tt.Key(native.Ctrl, native.KeyF4)
			tt.Frame()
			if a.sourceDecision == nil {
				t.Fatal("dirty real file close did not ask for a decision")
			}
			exportSessionScreenshot(t, tt, "source-close-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Cancel")
			tt.Frame()
			if a.activeDocument() == nil || !a.activeDocument().modified() {
				t.Fatal("cancel closed or cleared dirty file")
			}
			a.run(closeTab, commandTarget{})
			tt.Frame()
			click("Discard and Close Source")
			tt.Frame()
			if a.activeDocument() != nil {
				t.Fatal("explicit editor discard did not close tab")
			}
			a.run(reopenTab, commandTarget{})
			if a.activeDocument().code.Buffer.String() != file.New || a.activeDocument().modified() {
				t.Fatal("discarded edits returned when reopening")
			}
		})
	}
}

func TestSourceSaveCloseKeepsNewerEditsAndCanceledQueuedSuccess(t *testing.T) {
	a, repo, file := sourceLifecycleFixture(t, true)
	buffer := a.activeDocument().code.Buffer
	buffer.Set("first edit\n")
	var finish func(domain.Repository, domain.File, error, error)
	var request context.Context
	var submitted string
	a.SaveRepositoryFile = func(ctx context.Context, r domain.Repository, f domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		request, submitted, finish = ctx, text, done
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(closeTab, commandTarget{})
	tt.Frame()
	click("Save and Close Source")
	buffer.Set("newer typing\n")
	saved := file
	saved.New, saved.DiskVersion = submitted, "saved-1"
	finish(repo, saved, nil, nil)
	tt.Frame()
	if a.activeDocument() == nil || !buffer.Modified() || a.sourceDecision == nil {
		t.Fatal("save-close removed newer edits")
	}
	click("Save and Close Source")
	click("Cancel")
	saved.New, saved.DiskVersion = submitted, "saved-2"
	finish(repo, saved, nil, nil)
	tt.Frame()
	if request.Err() == nil || a.activeDocument() == nil || buffer.Modified() {
		t.Fatal("cancel allowed queued successful save to close tab or lost saved state")
	}
	buffer.Set("last edit\n")
	a.run(closeTab, commandTarget{})
	tt.Frame()
	click("Save and Close Source")
	saved.New, saved.DiskVersion = submitted, "saved-3"
	finish(repo, saved, nil, nil)
	tt.Frame()
	if a.activeDocument() != nil {
		t.Fatal("saved clean file did not close")
	}
}

func TestSourceReloadPreservesFailureAndNewerTyping(t *testing.T) {
	a, _, file := sourceLifecycleFixture(t, true)
	buffer := a.activeDocument().code.Buffer
	buffer.Set("unsaved\n")
	var finish func(domain.File, error)
	var request context.Context
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		request, finish = ctx, done
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(reloadProjectFile, commandTarget{tabID: a.activeDocument().id})
	tt.Frame()
	if finish != nil {
		t.Fatal("reload discarded before confirmation")
	}
	click("Reload Source File")
	finish(file, errors.New("disk unavailable"))
	tt.Frame()
	if buffer.String() != "unsaved\n" || !buffer.Modified() {
		t.Fatal("failed reload lost editing")
	}
	click("Reload Source File")
	buffer.Set("newer\n")
	fresh := file
	fresh.New, fresh.DiskVersion = "disk text\n", "fresh"
	finish(fresh, nil)
	tt.Frame()
	if buffer.String() != "newer\n" || !buffer.Modified() {
		t.Fatal("reload overwrote newer typing")
	}
	click("Reload Source File")
	finish(fresh, nil)
	tt.Frame()
	if buffer.String() != fresh.New || buffer.Modified() || a.activeDocument().file.DiskVersion != "fresh" {
		t.Fatal("confirmed reload did not install current disk version")
	}
	a.run(reloadProjectFile, commandTarget{tabID: a.activeDocument().id})
	a.run(selectRepo, commandTarget{index: 1})
	finish(file, nil)
	if request.Err() == nil || a.Model.Active().Repository.Name != "sandbox" {
		t.Fatal("reload read survived repository switch")
	}
}

func TestSourceSaveCloseFailureAndOriginalRepository(t *testing.T) {
	a, repo, file := sourceLifecycleFixture(t, true)
	buffer := a.activeDocument().code.Buffer
	buffer.Set("edited\n")
	var finish func(domain.Repository, domain.File, error, error)
	var submitted string
	a.SaveRepositoryFile = func(ctx context.Context, r domain.Repository, f domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		finish, submitted = done, text
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(closeTab, commandTarget{})
	tt.Frame()
	click("Save and Close Source")
	finish(repo, file, errors.New("external edit"), nil)
	tt.Frame()
	if a.activeDocument() == nil || !buffer.Modified() || a.sourceDecision == nil || a.sourceDecision.message == "" {
		t.Fatal("failed save closed or cleared the source")
	}
	click("Save and Close Source")
	a.run(selectRepo, commandTarget{index: 1})
	a.openCode("scratch", domain.File{Path: "unrelated.txt", New: "other view"})
	other := a.activeDocument().id
	saved := file
	saved.New, saved.DiskVersion = submitted, "saved"
	finish(repo, saved, nil, nil)
	tt.Frame()
	if a.Model.Active().Repository.Name != "sandbox" || a.activeDocument() == nil || a.activeDocument().id != other {
		t.Fatal("original save-close changed another repository")
	}
	for i, session := range a.Model.Sessions {
		if session.Repository.Root == repo.Root && len(a.repos[i].tabs) != 0 {
			t.Fatal("original saved tab remained open")
		}
	}
}

func TestSourceReloadUpdatesSharedSplitMetadata(t *testing.T) {
	a, _, _ := sourceLifecycleFixture(t, false)
	id := a.activeDocument().id
	a.splitTab(id, false, false)
	buffer := a.repo().split.code.Buffer
	buffer.Set("unsaved split\n")
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		f.New, f.DiskVersion = "fresh disk\n", "fresh"
		done(f, nil)
	}
	tt := native.NewTester(a.View, 1440, 900)
	a.run(reloadProjectFile, commandTarget{tabID: id})
	tt.Frame()
	exportSessionScreenshot(t, tt, "source-reload-light")
	if err := tt.Click("Reload Source File"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if buffer.String() != "fresh disk\n" || buffer.Modified() || a.repo().split.file.DiskVersion != "fresh" || a.activeDocument().file.DiskVersion != "fresh" || a.repo().split.code.Buffer != a.activeDocument().code.Buffer {
		t.Fatal("reload split buffer/version mismatch")
	}
}

func TestMockSourceReloadRemainsDisabled(t *testing.T) {
	for _, open := range []bool{false, true} {
		a := New(application.New(), application.DefaultLayout())
		a.ReadRepositoryFile = func(context.Context, string, string, domain.File, func(domain.File, error)) {
			t.Fatal("Mock reload reached real filesystem adapter")
		}
		if open {
			a.openCode("project", domain.File{Path: "mock.txt", New: "mock content"})
		}
		if a.disabledFor(reloadProjectFile, commandTarget{}) == "" {
			t.Fatal("Mock reload should remain disabled")
		}
		a.run(reloadProjectFile, commandTarget{})
		a.requestSourceReload(commandTarget{})
		if a.sourceDecision != nil {
			t.Fatal("Mock reload started a real file decision")
		}
	}
}
