package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	expandBranchFolders   commandID = "branches.expand-folders"
	collapseBranchFolders commandID = "branches.collapse-folders"
)

func (r *repoView) branchOutline() *folderOutline {
	if strings.TrimSpace(r.branchQuery) != "" {
		return &r.branchResults
	}
	return &r.branches
}

var deleteBranchIcon = changeIcon("M2.5 4h11M6 4V2h4v2m2 0-.6 10H4.6L4 4m3 2v6m2-6v6")

func (a *Workspace) branchToolbar(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	target := commandTarget{source: s.Branch}
	ui.Column(c).Width(32).FillHeight().Padding(4).Gap(2).Background(p.chrome).Children(func() {
		tool := func(id commandID, icon *ui.SVG) {
			label := commandLabel(id)
			reason := a.disabledFor(id, target)
			tip := label
			if reason != "" {
				tip += " — " + reason
			}
			ui.ButtonBase(c.Key(string(id))).Size(24, 24).Center().Label(label).Tooltip(tip).Disabled(reason != "").
				Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(p.theme.TextMuted) }).
				OnClick(func() { a.run(id, target) })
		}
		tool(newBranch, commitIcons[projectNewIcon])
		tool(deleteBranch, deleteBranchIcon)
		tool(branchDiff, commitIcons[previewDiff])
		commitIcon(c, logSearchIcon, "Search Branches").Tooltip("Search Branches").OnClick(func() { r.branchSearchFocus = true })
		commitIcon(c, logViewIcon, "Branch View Options").Tooltip("Branch View Options").Menu(func(m *ui.Menu) {
			a.menuGroup(m, commandTarget{}, expandBranchFolders, collapseBranchFolders, toggleBranches, toggleDetails)
		})
		tool(expandBranchFolders, commitIcons[expandChanges])
		tool(collapseBranchFolders, commitIcons[collapseChanges])
	})
}
