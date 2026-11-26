package ui

import (
	"context"
	"errors"
	native "github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"vela/internal/domain"
)

func TestLineHistoryNativeSelectionDiffAndIsolation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := inlineFixture(t)
			a.Layout.Dark = theme == "dark"
			var query domain.LineHistoryQuery
			a.ReadLineHistory = func(_ context.Context, _ domain.Repository, q domain.LineHistoryQuery, done func(domain.LineHistoryPage, error)) {
				query = q
				done(domain.LineHistoryPage{Entries: []domain.LineHistoryEntry{{Commit: domain.Commit{Hash: strings.Repeat("b", 40), Subject: "Changed selected line", Author: "Author"}, Hunks: []domain.LineHistoryHunk{{OldStart: 2, NewStart: 2, Before: "old\n", After: "second\n"}}}}}, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.Click("Source editor")
			tt.Key(native.Ctrl, native.KeyHome)
			tt.Key(0, native.KeyDown)
			tt.Key(native.Shift, native.KeyEnd)
			caret, ok := tt.TextCaret()
			if !ok {
				t.Fatal("missing native editor caret")
			}
			tt.RightClickAt(caret.X-8, caret.Y+caret.H/2)
			if err := tt.ChooseMenuItem("Git", "Show History for Selection…"); err != nil {
				t.Fatalf("%v; reason=%s selection=%+v", err, a.disabledFor(historySelection, commandTarget{}), a.activeDocument().code.Selection())
			}
			if a.lineHistory == nil || query.Start != 2 || query.End != 2 || a.lineHistory.diff == nil {
				t.Fatalf("bad native range %+v", query)
			}
			if !tt.HasText("Changed selected line") {
				t.Fatal("matching commit missing")
			}
			exportSessionScreenshot(t, tt, "line-history-"+theme)
			if err := tt.Click("Close Line History"); err != nil {
				t.Fatal(err)
			}
			if a.lineHistory != nil || len(a.Model.Active().Repository.History) != 2 {
				t.Fatal("global Log modified")
			}
		})
	}
}
func TestLineHistoryLateCallbacksAndDirtyRefusal(t *testing.T) {
	a := inlineFixture(t)
	var done func(domain.LineHistoryPage, error)
	var request context.Context
	a.ReadLineHistory = func(ctx context.Context, _ domain.Repository, _ domain.LineHistoryQuery, finish func(domain.LineHistoryPage, error)) {
		request, done = ctx, finish
	}
	a.activeDocument().code.Buffer.Set("dirty")
	if a.disabledFor(historySelection, commandTarget{}) == "" {
		t.Fatal("dirty source permitted")
	}
	a.activeDocument().code.Buffer.Undo()
	a.run(historySelection, commandTarget{})
	if done == nil {
		t.Fatal("query not started")
	}
	a.run(selectRepo, commandTarget{index: 0})
	done(domain.LineHistoryPage{Entries: []domain.LineHistoryEntry{{Commit: domain.Commit{Hash: strings.Repeat("b", 40)}}}}, nil)
	if request.Err() != context.Canceled || a.lineHistory != nil {
		t.Fatal("navigation accepted late result")
	}
}

func TestLineHistoryStatesAndVersionGuard(t *testing.T) {
	a := inlineFixture(t)
	var finish func(domain.LineHistoryPage, error)
	a.ReadLineHistory = func(_ context.Context, _ domain.Repository, _ domain.LineHistoryQuery, done func(domain.LineHistoryPage, error)) {
		finish = done
	}
	a.run(historySelection, commandTarget{})
	tt := native.NewTester(a.View, 1440, 900)
	if !tt.HasText("Loading line history…") {
		t.Fatal("missing loading state")
	}
	finish(domain.LineHistoryPage{}, errors.New("read failed"))
	tt.Frame()
	if !tt.HasText("Could not read line history · read failed") {
		t.Fatal("missing error")
	}
	a.run(closeLineHistory, commandTarget{})
	a.run(historySelection, commandTarget{})
	finish(domain.LineHistoryPage{}, nil)
	tt.Frame()
	if !tt.HasText("No matching line history") {
		t.Fatal("missing empty state")
	}
	a.run(closeLineHistory, commandTarget{})
	a.run(historySelection, commandTarget{})
	buffer := a.activeDocument().code.Buffer
	buffer.Set("edit")
	buffer.Undo()
	finish(domain.LineHistoryPage{Entries: []domain.LineHistoryEntry{{Commit: domain.Commit{Hash: strings.Repeat("b", 40)}, Hunks: []domain.LineHistoryHunk{{After: "first"}}}}}, nil)
	if len(a.lineHistory.entries) != 0 || !strings.Contains(a.lineHistory.problem, "changed") {
		t.Fatal("edit/undo accepted obsolete query")
	}
}
