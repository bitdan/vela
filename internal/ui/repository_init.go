package ui

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
)

type repositoryInitDialog struct {
	open      bool
	directory string
	branch    string
	picking   bool
	cancel    context.CancelFunc
	message   string
}

func (a *Workspace) openRepositoryInit() {
	if a.repositoryInit == nil {
		a.repositoryInit = &repositoryInitDialog{open: true, branch: "main"}
	}
}

func (a *Workspace) repositoryInitView(c *ui.Context, p tokens) {
	s := a.repositoryInit
	width, _ := c.Size()
	pending := s.cancel != nil
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(width-32, 560)).Background(p.panel).Border(1, p.theme.Border).
			Radius(6).Label("Initialize Git Repository dialog").Column().Children(func() {
			heading(c, p, "Initialize Git Repository", "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, "Creates a local Git repository in an existing folder.").TextColor(p.theme.TextMuted)
				ui.Text(c, "Repository directory")
				ui.Row(c).FillWidth().Gap(gap).Children(func() {
					ui.TextInput(c, &s.directory).Grow(1).AutoFocus().
						Label("Repository directory").Disabled(pending || s.picking)
					ui.Button(c, "Browse…").Disabled(a.PickRepositoryDirectory == nil || pending || s.picking).
						OnClick(func() {
							s.picking = true
							s.message = ""
							a.PickRepositoryDirectory(func(path string, err error) {
								if a.repositoryInit != s {
									return
								}
								s.picking = false
								if err != nil {
									s.message = err.Error()
								} else if path != "" {
									s.directory = path
								}
							})
						})
				})
				ui.Text(c, "Initial branch")
				ui.TextInput(c, &s.branch).FillWidth().Label("Initial branch").Disabled(pending)
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.cancel != nil {
						s.cancel()
					} else {
						s.open = false
					}
				})
				label := "Initialize"
				if pending {
					label = "Initializing…"
				}
				ui.PrimaryButton(c, label).Disabled(pending || s.picking || a.InitializeRepository == nil ||
					strings.TrimSpace(s.directory) == "" || strings.TrimSpace(s.branch) == "").OnClick(func() {
					a.initializeRepositoryFromDialog(s)
				})
			})
		})
	})
	if !s.open {
		if s.cancel != nil {
			s.cancel()
			s.open = true
		} else {
			a.repositoryInit = nil
		}
	}
}

func (a *Workspace) initializeRepositoryFromDialog(s *repositoryInitDialog) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.message = ""
	navigation := a.repositoryNavigation
	a.InitializeRepository(ctx, strings.TrimSpace(s.directory), strings.TrimSpace(s.branch), func(path string, err error) {
		cancelled := ctx.Err() != nil
		cancel()
		if a.repositoryInit != s {
			return
		}
		s.cancel = nil
		if err != nil {
			s.message = err.Error()
			if errors.Is(err, context.Canceled) {
				s.open = false
			}
			return
		}
		a.status = "Initialized Git repository · " + path
		s.open = false
		if !cancelled && navigation == a.repositoryNavigation && a.ReadRepository != nil {
			a.repositoryInit = nil
			a.loadRepository(path, false)
		}
	})
}
