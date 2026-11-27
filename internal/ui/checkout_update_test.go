package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestCheckoutUpdateNativeConfirmation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.Head, r.Working, r.IndexVersion = strings.Repeat("a", 40), nil, "index"
			r.Branches = []string{"main", "topic"}
			r.Refs = map[string]string{"refs/heads/main": r.Head, "refs/heads/topic": r.Head}
			r.Upstreams = map[string]domain.Upstream{"topic": {Remote: "origin", Branch: "review/topic"}}
			r.Remotes = []domain.Remote{{Name: "origin"}}
			a.Model.Switch(a.installRepository(r))
			calls := 0
			a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				if op.Kind != domain.CheckoutUpdate || op.Branch != "topic" || op.Remote != "origin" || op.Target != "review/topic" {
					t.Fatalf("wrong checkout/update: %+v", op)
				}
				done(r, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.RightClick("topic")
			if err := tt.ChooseMenuItem("Checkout and Update"); err != nil {
				t.Fatal(err)
			}
			if calls != 0 || !tt.HasText("Check out topic, then fetch origin/review/topic and fast-forward. If Update stops after checkout, topic stays checked out.") {
				t.Fatal("missing confirmation of checkout and upstream")
			}
			exportSessionScreenshot(t, tt, "checkout-update-"+theme)
			if err := tt.Click("Run Checkout and Update"); err != nil || calls != 1 {
				t.Fatalf("operation not dispatched: %v", err)
			}
		})
	}
}
