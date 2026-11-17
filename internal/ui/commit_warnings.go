package ui

import "github.com/egoist/mygo/ui"

const (
	confirmCommitWarnings commandID = "git.confirm-commit-warnings"
	cancelCommitWarnings  commandID = "git.cancel-commit-warnings"
)

func (a *Workspace) commitWarningDisabled(id commandID) string {
	change := a.commitConfirmation
	if change == nil || change.warnings == nil {
		return "No commit warning confirmation"
	}
	if id == cancelCommitWarnings {
		return ""
	}
	repo := a.Model.Active().Repository
	if a.repositoryWrite != "" || a.repositoryRead != nil || repo.Root != change.repo.Root || repo.Head != change.repo.Head || repo.IndexVersion != change.repo.IndexVersion || repo.CurrentBranch != change.repo.CurrentBranch {
		return "Repository changed; cancel and start Commit again"
	}
	return ""
}

func (a *Workspace) runCommitWarning(id commandID) {
	change := a.commitConfirmation
	a.commitConfirmation = nil
	if id == confirmCommitWarnings {
		change.warningConsent = change.warnings.Digest
		a.applyCommit(change)
	}
}

func (a *Workspace) commitWarningDialog(c *ui.Context) {
	change := a.commitConfirmation
	p := design(a.darkTheme())
	width, height := c.Size()
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(width-32, 720)).MaxHeight(height-48).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Commit Warnings dialog").Column().Children(func() {
			heading(c, p, "Review Commit Warnings", change.repo.Root+" · "+change.repo.CurrentBranch)
			ui.Scroll(c).FillWidth().MaxHeight(min(height-240, 300)).Children(func() {
				ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
					for _, message := range change.warnings.Messages {
						ui.Text(c, message).FillWidth()
					}
				})
			})
			ui.Text(c, "Continue accepts only these inspected staged contents and warnings. Git hooks, signing and safety checks still apply.").FillWidth().Padding(inset).TextColor(p.theme.TextMuted)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Label(commandLabel(cancelCommitWarnings)).OnClick(func() { a.run(cancelCommitWarnings, commandTarget{}) })
				ui.PrimaryButton(c, "Commit Anyway").Label(commandLabel(confirmCommitWarnings)).Disabled(a.disabledFor(confirmCommitWarnings, commandTarget{}) != "").OnClick(func() { a.run(confirmCommitWarnings, commandTarget{}) })
			})
		})
	})
	if !open {
		a.run(cancelCommitWarnings, commandTarget{})
	}
}
