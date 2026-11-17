package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

type commitChange struct {
	warnings       *application.CommitWarnings
	warningConsent string
	repo           domain.Repository
	message        string
	amend          bool
	push           bool
}

func (a *Workspace) commitForm(c *ui.Context, p tokens) {
	s := a.Model.Active()
	busy := a.repositoryWrite == s.Repository.Root
	divider(c, p)
	ui.Column(c).FillWidth().Padding(gap, inset).Gap(gap).Children(func() {
		ui.Checkbox(c, &s.Amend, "Amend last commit").
			Disabled(busy || s.Repository.Head == "" || s.Repository.CurrentBranch == "Detached HEAD").
			OnChange(func() {
				if s.Amend && strings.TrimSpace(s.Message) == "" {
					s.Message = s.Repository.HeadMessage
				}
			})
		ui.TextArea(c, &s.Message).Label("Commit message").Placeholder("Commit message").
			Height(66).FillWidth().Disabled(busy)
		ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
			label, access := "Commit", "Create Commit"
			if s.Amend {
				label, access = "Amend", "Amend Commit"
			}
			reason := a.disabledFor(commitRepository, commandTarget{})
			ui.PrimaryButton(c, label).Label(access).Disabled(reason != "").Tooltip(reason).
				OnClick(func() { a.run(commitRepository, commandTarget{}) })
			if len(s.Repository.Remotes) > 0 {
				a.button(c, commitAndPush, commandLabel(commitAndPush))
			}
			if len(s.Repository.Staged) > 0 {
				ui.Text(c, fileCount(len(s.Repository.Staged))+" staged").TextColor(p.theme.TextMuted)
			}
		})
	})
}

func (a *Workspace) commitDisabled() string {
	s := a.Model.Active()
	switch {
	case a.CommitRepository == nil:
		return "Commit is unavailable"
	case a.repositoryWrite != "" || a.repositoryRead != nil || a.indexConfirmation != nil || a.commitConfirmation != nil || a.repositoryOperation != nil:
		return "Wait for the current Git operation to finish"
	case s.Repository.IndexVersion == "":
		return "Refresh the repository before committing"
	case s.Repository.Bisect.Active():
		return "End Bisect before committing"
	case s.Repository.CurrentBranch == "Detached HEAD":
		return "Check out a local branch before committing"
	case s.Amend && s.Repository.Head == "":
		return "There is no commit to amend"
	case !s.Amend && len(s.Repository.Staged) == 0:
		return "No staged changes"
	case strings.TrimSpace(s.Message) == "":
		return "Enter a commit message"
	}
	if s.Amend {
		protected, err := domain.ProtectedBranch(a.Layout.VersionControl.Git.ProtectedBranches, s.Repository.CurrentBranch)
		if err != nil {
			return err.Error()
		}
		if protected {
			return "Amend is disabled on a protected branch"
		}
	}
	for _, file := range s.Repository.Working {
		if file.Status == "U" {
			return "Resolve the index conflicts before committing"
		}
	}
	if warning := a.Layout.VersionControl.Commit.Inspect(s.Message); warning != "" {
		return warning
	}
	return ""
}

func (a *Workspace) requestCommit(push bool) {
	s := a.Model.Active()
	change := &commitChange{repo: s.Repository, message: s.Message, amend: s.Amend, push: push}
	if change.amend {
		a.commitConfirmation = change
		return
	}
	a.applyCommit(change)
}

func (a *Workspace) confirmAmend(c *ui.Context) {
	change := a.commitConfirmation
	if change.warnings != nil {
		a.commitWarningDialog(c)
		return
	}
	message := fmt.Sprintf("Replace commit %s on %s in %s using the commit message and the current index (%s staged).",
		change.repo.Head[:min(8, len(change.repo.Head))], change.repo.CurrentBranch, change.repo.Name, fileCount(len(change.repo.Staged)))
	open := true
	result := ui.AlertDialog(c, &open, "Amend last commit?", message, "Cancel", "Replace Commit")
	if !open || result == 1 {
		a.commitConfirmation = nil
	}
	if result == 1 && a.repositoryWrite == "" && a.Model.Active().Repository.Root == change.repo.Root {
		a.applyCommit(change)
	}
}

func (a *Workspace) applyCommit(change *commitChange) {
	root := change.repo.Root
	progress, success, failure := "Creating commit…", "Created commit", "Commit failed"
	if change.amend {
		progress, success, failure = "Amending commit…", "Amended commit", "Amend failed"
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = application.WithCommitWarningConsent(ctx, change.warningConsent)
	showPushDialog := a.Layout.VersionControl.Git.ShowPushDialog
	a.beginRepositoryWrite(root, progress, cancel)
	a.CommitRepository(ctx, change.repo, change.message, change.amend, func(repo domain.Repository, err, refreshErr error) {
		wasCanceled := ctx.Err() != nil
		var warnings *application.CommitWarnings
		if errors.As(err, &warnings) {
			a.finishRepositoryWrite(root, repo, refreshErr, "Commit requires warning confirmation")
			if !wasCanceled && refreshErr == nil && a.Model.Active().Repository.Root == root {
				change.warnings = warnings
				a.commitConfirmation = change
			}
			return
		}
		message := success
		if err != nil {
			message = failure + " · " + err.Error()
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				message = "Git did not confirm the operation · " + err.Error()
			}
		} else {
			for i := range a.Model.Sessions {
				s := &a.Model.Sessions[i]
				if s.Repository.Root == root && s.Message == change.message {
					s.Message, s.Amend = "", false
					break
				}
			}
			message += " on " + change.repo.CurrentBranch
		}
		a.finishRepositoryWrite(root, repo, refreshErr, message)
		if change.push && !wasCanceled && err == nil && refreshErr == nil && a.Model.Active().Repository.Root == root && repo.CurrentBranch == change.repo.CurrentBranch &&
			a.disabledFor(push, commandTarget{}) == "" {
			a.openRepositoryOperation(push, commandTarget{})
			a.repositoryOperation.afterCommit = true
			upstream := repo.Upstreams[repo.CurrentBranch]
			op := a.repositoryOperation.op
			if !showPushDialog && upstream.Remote != "" && upstream.Remote != "." && upstream.Branch != "" && upstream == change.repo.Upstreams[change.repo.CurrentBranch] && repo.RemoteVersion != "" && repo.RemoteVersion == change.repo.RemoteVersion && op.Branch == repo.CurrentBranch && op.Remote == upstream.Remote && op.Target == upstream.Branch && !op.SetUpstream {
				a.applyRepositoryOperation(a.repositoryOperation)
			}
		}
	})
}
