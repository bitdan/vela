package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	expandHistorical      commandID = "history.files-expand"
	collapseHistorical    commandID = "history.files-collapse"
	historicalDiffIcon    commandID = "history.files-diff"
	historicalRevertIcon  commandID = "history.files-revert"
	historicalHistoryIcon commandID = "history.files-log"
	historicalViewIcon    commandID = "history.files-view"
)

func (a *Workspace) historicalToolbar(c *ui.Context, p tokens, commit string, files []domain.File) {
	r := a.repo()
	file, _ := matchFile(files, a.Model.Active().Historical)
	target := commandTarget{source: commit, file: file}
	ui.Row(c).Height(headerHeight).Padding(0, gap).Gap(2).AlignItems(ui.Center).Children(func() {
		tool := func(icon, id commandID, label, tip string, target commandTarget) {
			reason := a.disabledFor(id, target)
			if reason != "" {
				tip += " — " + reason
			}
			commitIcon(c, icon, label).Tooltip(tip).Disabled(reason != "").OnClick(func() { a.run(id, target) })
		}
		tool(historicalDiffIcon, previewDiff, "Show Historical Diff", "Show Diff  Ctrl+D", target)
		tool(historicalRevertIcon, revertChanges, "Revert Selected Changes", "Revert Selected Changes", target)
		tool(historicalHistoryIcon, historyToRevision, "History Up to Here", "History Up to Here", target)
		commitIcon(c, historicalViewIcon, "Historical View Options").Tooltip("Historical View Options").Menu(func(m *ui.Menu) {
			m.Item("Group By").Disabled(true)
			for _, id := range []commandID{groupDirectory, groupRepository, groupModule} {
				item := m.Item(commandLabel(id)).ID(string(id)).Radio(r.historyGroup == id || (r.historyGroup == "" && id == groupDirectory))
				if item.Chosen() {
					r.historyGroup = id
				}
			}
			m.Separator()
			m.Item("Layout").Disabled(true)
			logCheckInverted(m, "Show Details", &r.historyHideDetails)
			if m.Item("Show Diff Preview").Checked(r.historyPreview).Chosen() {
				r.historyPreview = !r.historyPreview
				if r.historyPreview && target.file.Path != "" {
					a.pending = func() { a.run(previewDiff, target) }
				}
			}
		})
		ui.Box(c).Grow(1)
		tool(expandChanges, expandHistorical, "Expand Historical Files", "Expand All", commandTarget{})
		tool(collapseChanges, collapseHistorical, "Collapse Historical Files", "Collapse All", commandTarget{})
	})
	divider(c, p)
}
