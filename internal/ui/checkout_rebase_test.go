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
