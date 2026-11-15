package ui

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	openRevisionEntry    commandID = "history.open-tree-entry"
	restoreRevisionEntry commandID = "history.restore-tree-entry"
	revisionParent       commandID = "history.tree-parent"
	reloadRevisionTree   commandID = "history.reload-tree"
	closeRevisionTree    commandID = "history.close-tree"
)

type revisionBrowserDialog struct {
	open                               bool
	root, revision, directory, message string
	entries                            []domain.RevisionTreeEntry
	selected                           int
	list                               ui.ListState
	request                            *readRequest
}

func (a *Workspace) closeRevisionBrowser() {
	if s := a.revisionBrowser; s != nil {
		stopRead(&s.request)
		a.revisionBrowser = nil
	}
}

func (a *Workspace) openRevisionBrowser(revision string) {
	a.closeRevisionBrowser()
	s := &revisionBrowserDialog{open: true, root: a.Model.Active().Repository.Root, revision: revision, selected: -1}
	a.revisionBrowser = s
	a.loadRevisionDirectory(s, "")
}

func (a *Workspace) loadRevisionDirectory(s *revisionBrowserDialog, directory string) {
	stopRead(&s.request)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.directory, s.message, s.selected = directory, "", -1
	s.entries = nil
	a.ReadRepositoryRevisionTree(ctx, s.root, s.revision, directory, func(page domain.RevisionTree, err error) {
		if a.revisionBrowser != s || s.request != request || a.Model.Active().Repository.Root != s.root || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		if err != nil {
			s.message = "Could not browse revision · " + err.Error()
			return
		}
		if page.Revision != s.revision || page.Directory != s.directory || len(page.Entries) > 10000 {
			s.message = "Historical tree identity changed; reload the directory"
			return
		}
		for _, entry := range page.Entries {
			if path.Dir(entry.Path) != directory && !(directory == "" && path.Dir(entry.Path) == ".") || !application.ValidProjectSearchPath(entry.Path) {
				s.message = "Invalid historical path"
				return
			}
		}
		s.entries = page.Entries
	})
}

func (a *Workspace) revisionBrowserDisabled(id commandID) string {
	s := a.revisionBrowser
	if s == nil || s.root != a.Model.Active().Repository.Root {
		return "Open a historical repository"
	}
	if id == revisionParent {
		if s.directory == "" {
			return "Already at the historical root"
		}
		return ""
	}
	if id == reloadRevisionTree {
		return ""
	}
	if s.request != nil {
		return "Wait for the directory"
	}
	if s.selected < 0 || s.selected >= len(s.entries) {
		return "Select a historical file or directory"
	}
	entry := s.entries[s.selected]
	if id == openRevisionEntry && entry.Kind == "tree" {
		return ""
	}
	if !entry.Regular() {
		return "Links and submodules can be inspected by type but cannot be opened or restored"
	}
	if id == openRevisionEntry {
		if a.ReadRepositoryFile == nil {
			return "File reader unavailable"
		}
		return ""
	}
	return a.advancedHistoryDisabled(getRevision, commandTarget{source: "revision:" + s.revision, file: domain.File{Path: entry.Path}})
}

func (a *Workspace) runRevisionBrowser(id commandID) {
	s := a.revisionBrowser
	switch id {
	case revisionParent:
		directory := path.Dir(s.directory)
		if directory == "." {
			directory = ""
		}
		a.loadRevisionDirectory(s, directory)
	case reloadRevisionTree:
		a.loadRevisionDirectory(s, s.directory)
	default:
		entry := s.entries[s.selected]
		if id == openRevisionEntry && entry.Kind == "tree" {
			a.loadRevisionDirectory(s, entry.Path)
			return
		}
		target := commandTarget{source: "revision:" + s.revision, file: domain.File{Path: entry.Path}}
		next := getRevision
		if id == openRevisionEntry {
			next = openRevision
		}
		a.closeRevisionBrowser()
		a.run(next, target)
	}
}

func (a *Workspace) revisionBrowserView(c *ui.Context, p tokens) {
	s := a.revisionBrowser
	w, h := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 880)).Height(min(h-32, 620)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Historical Repository dialog").Column().Children(func() {
			heading(c, p, "Repository at Revision", "Read only")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.root).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, s.revision).FillWidth().SingleLine().TextColor(p.theme.TextMuted)
				ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Button(c, "Up").Label("Parent Historical Directory").Disabled(a.revisionBrowserDisabled(revisionParent) != "").OnClick(func() { a.run(revisionParent, commandTarget{}) })
					ui.Text(c, "/"+s.directory).Grow(1).SingleLine().Ellipsis("…")
					ui.Button(c, "Reload").Label("Reload Historical Directory").OnClick(func() { a.run(reloadRevisionTree, commandTarget{}) })
				})
				if s.request != nil {
					ui.Text(c, "Loading historical directory…")
				} else if s.message == "" && len(s.entries) == 0 {
					ui.Text(c, "This directory is empty")
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			s.list.Selected = &s.selected
			s.list.Key = func(i int) any { return s.entries[i].Path }
			s.list.Label = func(i int) string { return s.entries[i].Path }
			list := ui.List(c, &s.list, len(s.entries), func(i int) {
				entry := s.entries[i]
				ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Children(func() {
					name := path.Base(entry.Path)
					if entry.Kind == "tree" {
						name += "/"
					}
					ui.Text(c, name).Grow(1).SingleLine().Ellipsis("…")
					kind := entry.Kind + " · " + entry.Mode
					if entry.Regular() {
						kind = fmt.Sprintf("%d bytes", entry.Size)
					}
					ui.Text(c, kind).Width(160).SingleLine().TextColor(p.theme.TextMuted)
				})
			}).Grow(1).FillWidth().Label("Historical Repository entries")
			if list.Submitted() {
				a.pending = func() { a.run(openRevisionEntry, commandTarget{}) }
			}
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Text(c, fmt.Sprintf("%d entries", len(s.entries))).Grow(1)
				ui.Button(c, "Close").Label("Close Historical Repository").OnClick(func() { a.run(closeRevisionTree, commandTarget{}) })
				ui.Button(c, "Restore File…").Label("Restore Historical File").Disabled(a.revisionBrowserDisabled(restoreRevisionEntry) != "").OnClick(func() { a.run(restoreRevisionEntry, commandTarget{}) })
				ui.PrimaryButton(c, "Open").Label("Open Historical Entry").Disabled(a.revisionBrowserDisabled(openRevisionEntry) != "").OnClick(func() { a.run(openRevisionEntry, commandTarget{}) })
			})
		})
	})
	if !s.open {
		a.closeRevisionBrowser()
	}
}
