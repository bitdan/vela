package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	confirmSquash commandID = "git.confirm-squash"
	cancelSquash  commandID = "git.cancel-squash"
	refreshSquash commandID = "git.preview-squash"
)

type squashDialog struct {
	kind             domain.RepositoryOperationKind
	snapshot         domain.Repository
	selection        []string
	plan             domain.SquashPreview
	request          *readRequest
	cancel           context.CancelFunc
	message, problem string
	ack, busy        bool
	list             ui.ListState
}

func isSquashCommand(id commandID) bool {
	return rewriteEntryCommand(id) || id == confirmHistoryRewrite || id == cancelHistoryRewrite || id == refreshHistoryRewrite || id == squashCommit || id == confirmSquash || id == cancelSquash || id == refreshSquash
}

func (a *Workspace) squashBaseReason() string {
	r := a.Model.Active().Repository
	if r.Root == "" || a.OperateRepository == nil {
		return "Open a local repository with Squash available"
	}
	if r.Head == "" || r.CurrentBranch == "Detached HEAD" || r.ResolvingConflicts() || r.Bisect.Active() {
		return "Check out a local branch outside an active Git operation"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil {
		return "Wait for the repository operation"
	}
	if a.updateDirtyBuffer(r.Root) {
		return "Save or discard unsaved source documents before Squash"
	}
	for _, files := range [][]domain.File{r.Working, r.Staged} {
		for _, f := range files {
			if f.Status != "?" && f.Status != "!" {
				return "Squash requires clean tracked files and Index"
			}
		}
	}
	if err := a.Layout.VersionControl.Git.SafetyPolicy().CheckOperation(r, domain.RepositoryOperation{Kind: domain.SquashCommits}); err != nil {
		return err.Error()
	}
	return ""
}

func (a *Workspace) squashDisabled(id commandID, t commandTarget) string {
	if rewriteEntryCommand(id) {
		return a.rewriteEntryDisabled(id, t)
	}
	if id == cancelHistoryRewrite {
		id = cancelSquash
	}
	if id == confirmHistoryRewrite {
		id = confirmSquash
	}
	if id == refreshHistoryRewrite {
		id = refreshSquash
	}
	if id == cancelSquash {
		if a.squash == nil {
			return "No Squash dialog"
		}
		return ""
	}
	if reason := a.squashBaseReason(); reason != "" {
		return reason
	}
	if id == squashCommit {
		if a.PreviewRepositorySquash == nil {
			return "Squash preview is unavailable"
		}
		if a.squash != nil || a.repositoryOperation != nil || a.patchDialog != nil || a.patchImport != nil || a.commitConfirmation != nil {
			return "Close the current repository dialog"
		}
		ids := slices.Sorted(a.repo().logChosen.All())
		if t.source != "" && !a.repo().logChosen.Has(t.source) {
			return "Select the continuous commit range in Log first"
		}
		if err := application.ValidateSquashSelection(ids); err != nil {
			return err.Error()
		}
		for _, id := range ids {
			if !slices.ContainsFunc(a.Model.Active().Repository.History, func(c domain.Commit) bool { return c.Hash == id }) {
				return "Refresh and select loaded commits"
			}
		}
		return ""
	}
	s := a.squash
	if s == nil || s.busy || s.request != nil || s.snapshot.Root != a.Model.Active().Repository.Root {
		return "Wait for the Squash preview in the original repository"
	}
	if id == refreshSquash {
		return ""
	}
	r := a.Model.Active().Repository
	if r.Head != s.snapshot.Head || r.IndexVersion != s.snapshot.IndexVersion || r.WorktreeVersion != s.snapshot.WorktreeVersion {
		return "Repository changed; preview again"
	}
	if s.plan.Version == "" || !s.ack {
		return "Preview and confirm the rewritten range"
	}
	if err := application.ValidateSquashMessage(s.message); s.operationKind() != domain.FixupCommits && err != nil {
		return err.Error()
	}
	return ""
}

func (a *Workspace) runSquash(id commandID) {
	switch id {
	case squashCommit:
		a.squash = &squashDialog{selection: slices.Sorted(a.repo().logChosen.All())}
		a.previewSquash()
	case refreshSquash, refreshHistoryRewrite:
		a.previewSquash()
	case confirmSquash, confirmHistoryRewrite:
		a.applySquash()
	case cancelSquash, cancelHistoryRewrite:
		if s := a.squash; s != nil {
			if s.busy {
				if s.cancel != nil {
					s.cancel()
				}
				s.problem = "Cancellation requested; waiting for Git and refresh"
			} else {
				stopRead(&s.request)
				a.squash = nil
			}
		}
	}
}

func (a *Workspace) closeSquashPreview() {
	if s := a.squash; s != nil && !s.busy {
		stopRead(&s.request)
		a.squash = nil
	}
}

func (a *Workspace) previewSquash() {
	s := a.squash
	s.snapshot = a.Model.Active().Repository
	s.plan = domain.SquashPreview{}
	s.ack = false
	s.problem = ""
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	done := func(plan domain.SquashPreview, err error) {
		if a.squash != s || s.request != request || a.Model.Active().Repository.Root != s.snapshot.Root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		if err != nil {
			s.problem = "Could not preview " + s.title() + " · " + err.Error()
			return
		}
		actual, expected := slices.Clone(plan.Selected), slices.Clone(s.selection)
		slices.Sort(actual)
		slices.Sort(expected)
		if plan.Version == "" || !slices.Equal(actual, expected) || len(plan.Selected)+len(plan.Replay) > 1000 {
			s.problem = "Git returned an invalid commit-range identity"
			return
		}
		if err := application.ValidateSquashMessage(plan.Message); s.operationKind() == domain.SquashCommits && err != nil {
			s.problem = err.Error()
			return
		}
		s.plan = plan
		if s.message == "" || s.operationKind() == domain.FixupCommits {
			s.message = plan.Message
		}
	}
	if s.operationKind() == domain.SquashCommits {
		a.PreviewRepositorySquash(ctx, s.snapshot, slices.Clone(s.selection), done)
	} else {
		a.PreviewCommitRewrite(ctx, s.snapshot, s.operationKind(), slices.Clone(s.selection), done)
	}
}

func (a *Workspace) applySquash() {
	s := a.squash
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.cancel = cancel
	s.busy = true
	s.problem = ""
	restoreEditors := a.lockShelfEditors()
	op := domain.RepositoryOperation{Kind: domain.SquashCommits, SquashSelection: slices.Clone(s.selection), SquashVersion: s.plan.Version, ConfirmSquash: s.ack, Message: s.message}
	if s.operationKind() != domain.SquashCommits {
		op = domain.RepositoryOperation{Kind: s.operationKind(), RewriteSelection: slices.Clone(s.selection), RewriteVersion: s.plan.Version, ConfirmRewrite: s.ack, Message: s.message}
	}
	a.beginRepositoryWrite(s.snapshot.Root, s.title()+"…", cancel)
	a.OperateRepository(ctx, s.snapshot, op, func(updated domain.Repository, err, refreshErr error) {
		cancel()
		restoreEditors()
		s.cancel = nil
		s.busy = false
		message := s.title() + " completed"
		if err != nil {
			message = s.title() + " stopped · " + err.Error()
			s.problem = message
			s.plan.Version = ""
			s.ack = false
		}
		if updated.Rebase.Active() {
			message += " · Rebase is paused; resolve and use Continue or Abort"
		}
		a.finishRepositoryWrite(s.snapshot.Root, updated, refreshErr, message)
		a.refreshHistoricalWriteViews(s.snapshot.Root, op)
		if err == nil || updated.Rebase.Active() || a.Model.Active().Repository.Root != s.snapshot.Root {
			if a.squash == s {
				a.squash = nil
			}
		}
		if err == nil {
			for i, r := range a.Model.Sessions {
				if r.Repository.Root == s.snapshot.Root {
					a.repos[i].logChosen.Clear()
				}
			}
		}
	})
}

func (a *Workspace) squashView(c *ui.Context, p tokens) {
	s := a.squash
	open := true
	w, h := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 760)).MaxHeight(h-48).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("Squash dialog").Children(func() {
			heading(c, p, s.title(), s.snapshot.CurrentBranch)
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.snapshot.Root).FillWidth().SingleLine().Ellipsis("…")
				if s.request != nil {
					ui.Text(c, "Checking private commit range and remote-tracking refs…")
				}
				if s.plan.Version != "" {
					ui.Text(c, fmt.Sprintf("%d selected commits → 1 · %d later commits will be replayed", len(s.plan.Selected), len(s.plan.Replay))).FillWidth()
					s.list.Key = func(i int) any { return s.plan.Selected[i] }
					ui.List(c, &s.list, len(s.plan.Selected), func(i int) {
						oid := s.plan.Selected[i]
						label := oid
						if n := slices.IndexFunc(s.snapshot.History, func(c domain.Commit) bool { return c.Hash == oid }); n >= 0 {
							label += " · " + s.snapshot.History[n].Subject
						}
						ui.Text(c, label).FillWidth().SingleLine().Ellipsis("…").Tooltip(label)
					}).FillWidth().Height(min(float32(len(s.plan.Selected))*rowHeight, 5*rowHeight)).Label("Squash selected commit range")
					ui.Text(c, "Author kept from oldest selected commit: "+s.plan.Author).FillWidth()
				}
				messageLabel := "Squash commit message"
				if s.operationKind() != domain.SquashCommits {
					messageLabel = "History rewrite message"
				}
				ui.TextArea(c, &s.message).Label(messageLabel).Height(150).FillWidth().Disabled(s.busy || s.request != nil || s.operationKind() == domain.FixupCommits)
				ui.Text(c, s.title()+" keeps the final file tree. Git may stop during replay; use native Rebase Continue or Abort. Cancellation does not promise rollback.").FillWidth().TextColor(p.theme.TextMuted)
				ui.Checkbox(c, &s.ack, "Rewrite the selected range and its later commits").Disabled(s.busy || s.request != nil || s.plan.Version == "")
				if s.problem != "" {
					ui.Text(c, s.problem).FillWidth().TextColor(p.theme.Danger)
				}
			})
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Label("Cancel Squash").OnClick(func() { a.run(cancelSquash, commandTarget{}) })
				actions := []commandID{refreshSquash, confirmSquash}
				if s.operationKind() != domain.SquashCommits {
					actions = []commandID{refreshHistoryRewrite, confirmHistoryRewrite}
				}
				for _, id := range actions {
					reason := a.disabledFor(id, commandTarget{})
					label := commandLabel(id)
					if id == confirmSquash {
						label = "Run Squash Commits"
					}
					if id == confirmHistoryRewrite {
						label = "Run History Rewrite"
					}
					ui.Button(c, commandLabel(id)).Label(label).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(id, commandTarget{}) })
				}
			})
		})
	})
	if !open {
		a.run(cancelSquash, commandTarget{})
	}
}
