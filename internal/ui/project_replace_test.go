package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestProjectReplaceNativePreviewAndApply(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = theme == "dark"
			a := New(application.New(), layout)
			a.Model.Active().Repository.Root = t.TempDir()
			a.Model.Active().Repository.Project = []domain.File{{Path: "中文 space.txt"}}
			a.SaveRepositoryFile = func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error)) {
			}
			a.openCode("project", domain.File{Path: "中文 space.txt", New: "needle\n", DiskVersion: "disk"})
			entry := a.activeDocument()
			a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
				result, err := application.SearchProject(ctx, input, func(context.Context, string) (string, error) { return "needle\n", nil })
				done(result, err)
			}
			var complete func(domain.Repository, application.ProjectReplaceResult, error, error)
			calls := 0
			a.ReplaceProjectFiles = func(_ context.Context, repo domain.Repository, input application.ProjectReplaceInput, done func(domain.Repository, application.ProjectReplaceResult, error, error)) {
				calls++
				if input.Replacement != "new" || len(input.Matches) != 1 || input.Matches[0].Path != "中文 space.txt" {
					t.Fatalf("wrong replacement: %+v", input)
				}
				complete = done
			}
			a.run(replaceInProject, commandTarget{})
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Find in Files query"); err != nil {
				t.Fatal(err)
			}
			tt.Type("needle")
			if err := tt.Click("Project replacement text"); err != nil {
				t.Fatal(err)
			}
			tt.Type("new")
			if err := tt.Click("Find Project Text"); err != nil {
				t.Fatal(err)
			}
			if len(a.projectSearch.result.Matches) != 1 {
				t.Fatal("preview missing")
			}
			if button, ok := tt.Access("Replace Previewed"); !ok || !button.Disabled {
				t.Fatal("unconfirmed replacement enabled")
			}
			if err := tt.Click("Replace all previewed matches in the saved files"); err != nil {
				t.Fatal(err)
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, theme+"-replace-preview.png"))
			}
			if err := tt.Click("Replace Previewed"); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || complete == nil || !entry.code.ReadOnly || a.projectSearch != nil {
				t.Fatal("write did not lock matching document")
			}
			if err := tt.Click("Source editor"); err != nil {
				t.Fatal(err)
			}
			tt.Type("blocked")
			if entry.code.Buffer.String() != "needle\n" {
				t.Fatal("keyboard input changed file during replacement")
			}
			// A newer buffer must survive the completion/refresh of an older write.
			entry.code.Buffer.Set("newer user input")
			complete(a.Model.Active().Repository, application.ProjectReplaceResult{Paths: []string{"中文 space.txt"}, Replacements: 1}, errors.New("later file failed"), nil)
			if entry.code.ReadOnly || entry.code.Buffer.String() != "newer user input" || !entry.code.Buffer.Modified() || !strings.Contains(a.status, "Stopped") {
				t.Fatalf("completion lost draft or partial result: readonly=%t text=%q modified=%t status=%q", entry.code.ReadOnly, entry.code.Buffer.String(), entry.code.Buffer.Modified(), a.status)
			}
		})
	}
}

func TestProjectReplaceRejectsIncompleteAndDirtyPreviews(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	a.SearchProjectFiles = func(context.Context, application.ProjectSearchInput, func(application.ProjectSearchResult, error)) {}
	a.ReplaceProjectFiles = func(context.Context, domain.Repository, application.ProjectReplaceInput, func(domain.Repository, application.ProjectReplaceResult, error, error)) {
		t.Fatal("invalid preview dispatched")
	}
	a.run(replaceInProject, commandTarget{})
	s := a.projectSearch
	s.confirmed = true
	s.result = application.ProjectSearchResult{Matches: []application.ProjectSearchMatch{{Path: "draft.txt"}}}
	for _, kind := range []string{"limited", "unreadable", "memory", "dirty"} {
		t.Run(kind, func(t *testing.T) {
			s.result.Limited = kind == "limited"
			s.result.Unreadable = 0
			if kind == "unreadable" {
				s.result.Unreadable = 1
			}
			s.result.Matches[0].InMemory = kind == "memory"
			if kind == "dirty" {
				a.openCode("project", domain.File{Path: "draft.txt", New: "saved"})
				a.activeDocument().code.Buffer.Set("unsaved")
			}
			if a.disabledFor(applyProjectReplace, commandTarget{}) == "" {
				t.Fatal("unsafe preview enabled")
			}
			a.run(applyProjectReplace, commandTarget{})
		})
	}
}
