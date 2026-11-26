package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type remoteDialogState struct {
	open       bool
	repository int
	items      []domain.Remote
	selected   int
	table      ui.ListState
	editor     *remoteEditor
	version    string
}

type remoteEditor struct {
	index  int
	item   domain.Remote
	rename bool
}

var remoteMinusIcon = changeIcon("M2 8h12")
var remoteEditIcon = changeIcon("M2.5 13.5l.7-3.5 8.6-8.6 2.8 2.8L6 12.8zM10.5 2.8l2.8 2.8")

func (a *Workspace) openRemotes() {
	if a.remotes.open {
		return
	}
	a.remotes = remoteDialogState{open: true, repository: a.Model.Index, selected: -1,
		items: slices.Clone(a.Model.Active().Repository.Remotes), version: a.Model.Active().Repository.RemoteManagementVersion}
}

func (a *Workspace) remotesDialog(c *ui.Context, p tokens) {
	s := &a.remotes
	repo := &a.Model.Sessions[s.repository].Repository
	real := repo.Root != ""
	busy := real && (a.OperateRepository == nil || a.repositoryWrite != "" || a.repositoryRead != nil || repo.IndexVersion == "" || a.Model.Index != s.repository)
	width, height := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Size(min(width-32, 660), min(height-32, 390)).Background(p.panel).
			Border(1, p.theme.Border).Radius(6).Label("Git Remotes dialog").Column().Children(func() {
			heading(c, p, "Git Remotes", repo.Name)
			ui.Column(c).Grow(1).FillWidth().Padding(inset).Gap(gap).Children(func() {
				selection := func() bool { return s.selected >= 0 && s.selected < len(s.items) }
				edit := func() {
					if selection() {
						s.editor = &remoteEditor{index: s.selected, item: s.items[s.selected]}
					}
				}
				ui.Row(c).FillWidth().Gap(3).Children(func() {
					remoteTool(c, p, "Add remote", commitIcons[projectNewIcon], s.editor != nil || busy,
						func() { s.editor = &remoteEditor{index: -1} })
					remoteTool(c, p, "Remove remote", remoteMinusIcon, s.editor != nil || !selection() || busy, func() {
						if real {
							a.confirmRemote(removeRemote, s.items[s.selected].Name, s.items[s.selected])
							return
						}
						s.items = slices.Delete(s.items, s.selected, s.selected+1)
						s.selected = min(s.selected, len(s.items)-1)
					})
					remoteTool(c, p, "Edit remote", remoteEditIcon, s.editor != nil || !selection() || busy, edit)
					if real {
						ui.Button(c, "Rename…").Label("Rename remote").Disabled(s.editor != nil || !selection() || busy).OnClick(func() {
							s.editor = &remoteEditor{index: s.selected, item: s.items[s.selected], rename: true}
						})
					}
				})
				s.table.Selected = &s.selected
				s.table.Key = func(i int) any { return s.items[i].Name }
				s.table.Label = func(i int) string { return s.items[i].Name }
				table := ui.Table(c, &s.table, []ui.TableColumn{{Title: "Name", Width: 150}, {Title: "URL"}}, len(s.items), func(row, column int) {
					item := s.items[row]
					value := item.Name
					if column == 1 {
						value = item.URL
					}
					ui.Text(c, value).Height(rowHeight).SingleLine().Ellipsis("…").Tooltip(value)
				}).Grow(1).FillWidth().Border(1, p.theme.Border).Label("Remotes table")
				if table.Submitted() && s.editor == nil && !busy {
					edit()
				}
				if len(s.items) == 0 {
					table.Children(func() { ui.Text(c, "No remotes configured").Padding(inset).TextColor(p.theme.TextMuted) })
				}
				if s.editor != nil {
					a.remoteForm(c, p, real, busy)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Label("Cancel remotes").OnClick(func() { s.open = false })
				ui.PrimaryButton(c, "OK").Label("Apply remotes").Disabled(s.editor != nil).OnClick(func() {
					if repo.Root == "" {
						repo.Remotes = s.items
					}
					s.open = false
				})
			})
		})
	})
}

func remoteTool(c *ui.Context, p tokens, label string, icon *ui.SVG, disabled bool, click func()) {
	ui.ButtonBase(c.Key(label)).Size(commitToolSize, commitToolSize).Center().Label(label).Tooltip(label).Disabled(disabled).
		Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(p.theme.TextMuted) }).OnClick(click)
}

func (a *Workspace) remoteForm(c *ui.Context, p tokens, real, busy bool) {
	s := &a.remotes
	edit := s.editor
	ui.Column(c.Key("remote-editor")).FillWidth().Gap(gap).Children(func() {
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			ui.TextInput(c, &edit.item.Name).Width(150).Placeholder("Name").Label("Remote name").AutoFocus().Disabled(busy || real && edit.index >= 0 && !edit.rename)
			if !edit.rename {
				ui.TextInput(c, &edit.item.URL).Grow(1).Placeholder("HTTPS, SSH or absolute local path").Label("Remote URL").Disabled(busy)
			}
		})
		item := domain.Remote{Name: strings.TrimSpace(edit.item.Name), URL: strings.TrimSpace(edit.item.URL)}
		var reason string
		if !domain.ValidRemoteName(item.Name) {
			reason = "Enter a remote name using letters, digits, dots, hyphens or underscores."
		} else if edit.rename && strings.EqualFold(item.Name, s.items[edit.index].Name) {
			reason = "Enter a different remote name."
		} else if !edit.rename && !domain.ValidRemoteLocation(item.URL) {
			reason = "Enter a Git remote URL without embedded credentials or an absolute local path."
		} else if duplicate := slices.IndexFunc(s.items, func(remote domain.Remote) bool { return remote.Name == item.Name }); duplicate >= 0 && duplicate != edit.index {
			reason = "A remote with this name already exists."
		}
		if reason != "" && (edit.item.Name != "" || edit.item.URL != "") {
			ui.Text(c, reason).FontSize(p.textSize(smallText)).TextColor(p.theme.Danger)
		}
		ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() {
			ui.Button(c, "Cancel").Label("Cancel remote edit").OnClick(func() { s.editor = nil })
			ui.Button(c, "Save").Label("Save remote").Disabled(reason != "" || busy).OnClick(func() {
				if real {
					id, original := addRemote, ""
					if edit.index >= 0 {
						id, original = editRemote, s.items[edit.index].Name
					}
					if edit.rename {
						id = renameRemote
					}
					a.confirmRemote(id, original, item)
					return
				}
				if edit.index < 0 {
					s.selected = len(s.items)
					s.items = append(s.items, item)
				} else {
					s.items[edit.index] = item
				}
				s.editor = nil
			})
		})
	})
}
