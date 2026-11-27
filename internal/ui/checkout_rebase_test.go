package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestCheckoutRebaseNativeDirectionAndPause(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			repo := sessionRepository(t.TempDir())
			repo.Head, repo.CurrentBranch, repo.Working = strings.Repeat("a", 40), "base", nil
			repo.IndexVersion = "index"
			tip := strings.Repeat("b", 40)
			repo.Branches = []string{"base", "topic"}
			repo.Refs = map[string]string{"refs/heads/base": repo.Head, "refs/heads/topic": tip}
			a.Model.Switch(a.installRepository(repo))
			calls := 0
			a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				if op.Kind != domain.CheckoutRebase || !op.ConfirmRewrite || op.Source != "refs/heads/topic" || snapshot.Head != repo.Head {
					t.Fatalf("incorrect direction or confirmation: %+v", op)
				}
				paused := repo
				paused.CurrentBranch = "Detached HEAD"
				paused.Rebase = domain.RebaseState{Branch: "topic", Original: tip, Onto: repo.Head}
				done(paused, errors.New("fixture conflict"), nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.RightClick("topic")
			if err := tt.ChooseMenuItem("Checkout and Rebase onto 'base'"); err != nil {
				t.Fatal(err)
			}
			if calls != 0 || !tt.HasText("Check out topic and rebase it onto base ("+repo.Head+"). The base branch stays unchanged.") {
				t.Fatal("missing confirmation and fixed direction")
			}
			exportSessionScreenshot(t, tt, "checkout-rebase-"+theme)
			if err := tt.Click("Run Checkout and Rebase"); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !a.Model.Active().Repository.Rebase.Active() {
				t.Fatal("native recovery controls did not receive the paused state")
			}
		})
	}
}

func TestRemoteCheckoutRebaseConfirmation(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		for _, theme := range []string{"dark", "light"} {
			name := "create-" + theme
			if reuse {
				name = "reuse-" + theme
			}
			t.Run(name, func(t *testing.T) {
				a := New(application.New(), application.DefaultLayout())
				a.Layout.Dark = theme == "dark"
				repo := sessionRepository(t.TempDir())
				repo.Head, repo.CurrentBranch, repo.Working = strings.Repeat("a", 40), "base", nil
				repo.IndexVersion = "index"
				remoteTip := strings.Repeat("b", 40)
				repo.Branches = []string{"base", "origin/topic"}
				repo.Refs = map[string]string{"refs/heads/base": repo.Head, "refs/remotes/origin/topic": remoteTip}
				repo.RemoteBranches = map[string]bool{"origin/topic": true}
				repo.Remotes = []domain.Remote{{Name: "origin"}}
				action := "Create local branch topic with upstream origin/topic."
				if reuse {
					repo.Branches = []string{"base", "topic", "origin/topic"}
					repo.Refs["refs/heads/topic"] = remoteTip
					repo.Upstreams = map[string]domain.Upstream{"topic": {Remote: "origin", Branch: "topic"}}
					action = "Reuse local branch topic with upstream origin/topic."
				}
				a.Model.Switch(a.installRepository(repo))
				calls := 0
				a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
					calls++
					if op.Kind != domain.CheckoutRebase || !op.ConfirmRewrite || op.Source != "refs/remotes/origin/topic" || op.Branch != "topic" || op.Remote != "origin" || op.Target != "topic" || snapshot.Head != repo.Head {
						t.Fatalf("incorrect remote checkout/rebase: %+v", op)
					}
					done(repo, nil, nil)
				}
				tt := native.NewTester(a.View, 1440, 900)
				if err := tt.RightClick("origin/topic"); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem("Checkout and Rebase onto 'base'"); err != nil {
					t.Fatal(err)
				}
				direction := action + " Check out topic and replay commits from local branch topic onto base (" + repo.Head + "). The base branch and remote-tracking ref stay unchanged."
				if calls != 0 || !tt.HasText(direction) {
					t.Fatal("confirmation does not show the local branch, upstream and replay direction")
				}
				exportSessionScreenshot(t, tt, "remote-checkout-rebase-"+name)
				if err := tt.Click("Run Checkout and Rebase"); err != nil || calls != 1 {
					t.Fatalf("remote operation not dispatched: %v", err)
				}
			})
		}
	}
}

func TestRemoteCheckoutRebaseDisabledReasons(t *testing.T) {
	for _, reason := range []string{"upstream", "protected", "dirty"} {
		t.Run(reason, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			repo := sessionRepository(t.TempDir())
			repo.Head, repo.CurrentBranch, repo.IndexVersion = strings.Repeat("a", 40), "base", "index"
			repo.Branches = []string{"base", "topic", "origin/topic"}
			repo.Refs = map[string]string{"refs/heads/base": repo.Head, "refs/heads/topic": strings.Repeat("b", 40), "refs/remotes/origin/topic": strings.Repeat("b", 40)}
			repo.RemoteBranches = map[string]bool{"origin/topic": true}
			repo.Remotes = []domain.Remote{{Name: "origin"}}
			repo.Upstreams = map[string]domain.Upstream{"topic": {Remote: "origin", Branch: "topic"}}
			switch reason {
			case "upstream":
				repo.Upstreams["topic"] = domain.Upstream{Remote: "origin", Branch: "other"}
			case "protected":
				a.Layout.VersionControl.Git.ProtectedBranches = "topic"
			case "dirty":
				repo.Working = []domain.File{{Path: "dirty.txt", Status: "M"}}
			}
			a.Model.Switch(a.installRepository(repo))
			a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.RightClick("origin/topic"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Checkout and Rebase onto 'base'"); err == nil || !strings.Contains(err.Error(), "disabled") {
				t.Fatalf("unsafe remote menu item was not disabled: %v", err)
			}
			disabled := a.disabledFor(checkoutRebase, commandTarget{source: "origin/topic"})
			if disabled == "" || (reason == "upstream" && !strings.Contains(disabled, "tracking origin/topic")) || (reason == "protected" && !strings.Contains(disabled, "protected")) || (reason == "dirty" && !strings.Contains(disabled, "save or shelve")) {
				t.Fatalf("missing disabled reason %q: %q", reason, disabled)
			}
		})
	}
}

func TestCheckoutRebaseOperationDistinguishesLocalAndRemoteNames(t *testing.T) {
	repo := sessionRepository(t.TempDir())
	repo.Remotes = []domain.Remote{{Name: "origin"}}
	repo.Refs = map[string]string{
		"refs/heads/origin/topic":   strings.Repeat("a", 40),
		"refs/remotes/origin/topic": strings.Repeat("b", 40),
	}
	repo.RemoteBranches = map[string]bool{"refs/remotes/origin/topic": true}
	local := checkoutRebaseOperation(repo, "origin/topic")
	if local.Source != "refs/heads/origin/topic" || local.Branch != "" || local.Remote != "" {
		t.Fatalf("local branch was confused with its remote: %+v", local)
	}
	remote := checkoutRebaseOperation(repo, "refs/remotes/origin/topic")
	if remote.Source != "refs/remotes/origin/topic" || remote.Branch != "topic" || remote.Remote != "origin" || remote.Target != "topic" {
		t.Fatalf("disambiguated remote branch was not recognized: %+v", remote)
	}
}
