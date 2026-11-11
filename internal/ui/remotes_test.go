package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestGitRemotesAndProviderMenus(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	menu := func(path ...string) {
		t.Helper()
		click("Git menu")
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatal(err)
		}
	}
	fill := func(label, value string) {
		t.Helper()
		click(label)
		tt.Key(native.Cmd, native.KeyA)
		tt.Type(value)
	}
	click("Main menu")
	menu("Commit…")
	if a.repo().toolPage != "Commit" {
		t.Fatal("Commit menu did not open the Commit tool window")
	}
	menu("GitHub", "Copy Repository URL")
	if tt.Clipboard() != "https://github.com/egoist/mygo" {
		t.Fatal("GitHub menu did not copy the repository URL")
	}
	menu("GitLab", "Open Repository in Browser")
	if !slices.Equal(tt.OpenedURLs(), []string{"https://gitlab.com/gitlab-org/gitlab"}) {
		t.Fatalf("unexpected GitLab browser URL: %v", tt.OpenedURLs())
	}
	menu("Manage Remotes…")
	click("Add remote")
	fill("Remote name", "origin")
	fill("Remote URL", "https://gitee.com/openharmony/docs.git")
	if save, ok := tt.Access("Save remote"); !ok || !save.Disabled {
		t.Fatal("duplicate remote name was accepted")
	}
	fill("Remote name", "upstream")
	click("Save remote")
	if len(a.Model.Active().Repository.Remotes) != 3 {
		t.Fatal("draft leaked into the repository")
	}
	click("Apply remotes")
	if len(a.Model.Active().Repository.Remotes) != 4 {
		t.Fatal("remote was not added")
	}
	menu("Manage Remotes…")
	click("upstream")
	click("Edit remote")
	fill("Remote URL", "git@gitee.com:openharmony/docs.git")
	click("Save remote")
	click("Apply remotes")
	if remote := a.Model.Active().Repository.Remotes[3]; remote.URL != "git@gitee.com:openharmony/docs.git" {
		t.Fatalf("remote was not edited: %+v", remote)
	}
	menu("Gitee", "upstream", "Copy Repository URL")
	if tt.Clipboard() != "https://gitee.com/openharmony/docs" {
		t.Fatal("Gitee SSH address did not convert to the repository URL")
	}
	menu("Manage Remotes…")
	click("upstream")
	click("Remove remote")
	click("Cancel remotes")
	if len(a.Model.Active().Repository.Remotes) != 4 {
		t.Fatal("cancel did not discard the removal")
	}
	menu("Manage Remotes…")
	click("upstream")
	click("Remove remote")
	click("Apply remotes")
	if len(a.Model.Active().Repository.Remotes) != 3 || len(a.Model.Sessions[1].Repository.Remotes) != 1 {
		t.Fatal("removal targeted the wrong repository")
	}
}

func TestRemoteWebURLs(t *testing.T) {
	for _, tt := range []struct{ raw, provider, web string }{
		{"https://github.com/egoist/mygo.git", "GitHub", "https://github.com/egoist/mygo"},
		{"git@gitee.com:openharmony/docs.git", "Gitee", "https://gitee.com/openharmony/docs"},
		{"ssh://git@gitlab.com:22/team/subgroup/project.git", "GitLab", "https://gitlab.com/team/subgroup/project"},
		{"https://github.com.invalid/group/repo.git", "", ""},
		{"https://token@github.com/group/repo.git", "", ""},
		{"https://github.com/group/../repo.git", "", ""},
	} {
		if provider, web := domain.RemoteWebURL(tt.raw); provider != tt.provider || web != tt.web {
			t.Errorf("%s: got %q, %q", tt.raw, provider, web)
		}
	}
}
