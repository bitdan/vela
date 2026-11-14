package ui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type repositoryCloneDialog struct {
	open                    bool
	source, target, message string
	cancel                  context.CancelFunc
	completed               bool
}

func (a *Workspace) repositoryCloneView(c *ui.Context, p tokens) {
	s := a.repositoryClone
	busy := s.cancel != nil
	width, _ := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(width-32, 640)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Clone Repository dialog").Column().Children(func() {
			heading(c, p, "Clone Git Repository", "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, "Clone into a new directory. Existing directories are kept. Submodules are not cloned recursively.").FillWidth().TextColor(p.theme.TextMuted)
				ui.Text(c, "Source URL or absolute local path")
				ui.TextInput(c, &s.source).Label("Clone source").FillWidth().AutoFocus().Disabled(busy || s.completed)
				ui.Text(c, "New absolute destination directory")
				ui.TextInput(c, &s.target).Label("Clone destination").FillWidth().Disabled(busy || s.completed)
				ui.Text(c, "On failure or cancellation, inspect the destination before retrying. Partial results are not automatically deleted.").FillWidth().TextColor(p.theme.TextMuted)
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.cancel != nil {
						s.cancel()
						s.message = "Cancellation requested · waiting for Git"
					} else {
						s.open = false
					}
				})
				ui.PrimaryButton(c, "Clone").Label("Clone Repository").Disabled(busy || s.completed || a.CloneRepository == nil || !domain.ValidRemoteLocation(strings.TrimSpace(s.source)) || !filepath.IsAbs(strings.TrimSpace(s.target))).OnClick(func() { a.cloneFromDialog(s) })
			})
		})
	})
	if !s.open {
		if s.cancel != nil {
			s.cancel()
			s.open = true
		} else {
			a.repositoryClone = nil
		}
	}
}

func (a *Workspace) cloneFromDialog(s *repositoryCloneDialog) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.message = cancel, ""
	navigation := a.repositoryNavigation
	a.beginRepositoryWrite(s.target, "Cloning repository…", cancel)
	a.CloneRepository(ctx, strings.TrimSpace(s.source), strings.TrimSpace(s.target), func(root string, err error) {
		wasCanceled := ctx.Err() != nil
		cancel()
		if a.repositoryClone != s {
			return
		}
		s.cancel = nil
		a.repositoryWrite, a.repositoryWriteCancel = "", nil
		a.repositoryCancelRequested = false
		if err != nil {
			s.message = "Clone did not complete · " + err.Error() + " · Inspect destination: " + s.target
			a.status = s.message
			return
		}
		s.completed, s.open = true, false
		a.status = "Cloned repository · " + root
		if wasCanceled {
			a.status = "Clone completed after cancellation · " + root
		}
		if !wasCanceled && navigation == a.repositoryNavigation && a.ReadRepository != nil {
			a.loadRepository(root, false)
		}
	})
}
