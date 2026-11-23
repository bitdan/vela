package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func (a *Workspace) applyComparisonSideFilter(s *branchComparisonDialog, side *comparisonSide, index int, clear bool) {
	filter := side.draft
	if clear {
		filter = domain.ComparisonLogFilter{}
	}
	if err := application.ValidateComparisonLogFilter(filter); err != nil {
		side.filterMessage = "Filters not applied · " + err.Error()
		return
	}
	stopRead(&side.request)
	stopRead(&side.details)
	s.filters[index], side.comparison.Filter, side.draft = filter, filter, filter
	side.filterMessage, side.message, side.detailMessage, side.revision = "", "", "", ""
	side.commits, side.graph, side.files = nil, nil, nil
	side.chosen.Clear()
	side.groups, side.fileSources, side.restore = nil, nil, nil
	side.restoreFile, side.restoreSource = "", ""
	side.closedGroups = nil
	side.list, side.fileList = ui.ListState{}, ui.ListState{}
	side.selected, side.file, side.parent = -1, -1, 0
	side.more, side.filesReady = true, false
	a.loadComparisonSide(s, side)
}

func (a *Workspace) comparisonFilterControls(c *ui.Context, p tokens, side *comparisonSide, target commandTarget, label string) {
	ui.Column(c).FillWidth().Padding(gap).Gap(2).Children(func() {
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			ui.TextInput(c, &side.draft.Message).Label("Comparison message filter " + label).Placeholder("Commit message").Grow(1).Tooltip("Searches the complete fixed range; literal text or Go regular expression")
			ui.TextInput(c, &side.draft.Author).Label("Comparison author filter " + label).Placeholder("Author or email").Grow(1)
		})
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			ui.TextInput(c, &side.draft.Hash).Label("Comparison hash filter " + label).Placeholder("Hash (4+ hex)").Grow(1)
			ui.Checkbox(c, &side.draft.Regex, "Regex").Label("Comparison regex " + label)
			ui.Checkbox(c, &side.draft.CaseSensitive, "Match case").Label("Comparison match case " + label)
		})
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			ui.TextInput(c, &side.draft.Since).Label("Comparison start date " + label).Placeholder("From YYYY-MM-DD").Width(125).Tooltip("Inclusive committer date, UTC")
			ui.TextInput(c, &side.draft.Until).Label("Comparison end date " + label).Placeholder("To YYYY-MM-DD").Width(125).Tooltip("Inclusive committer date, UTC")
			ui.TextInput(c, &side.draft.Path).Label("Comparison path filter " + label).Placeholder("File or folder").Grow(1).Tooltip("Literal repository-relative file or directory; wildcards are not expanded")
			for _, button := range []struct {
				id    commandID
				label string
			}{{applyComparisonFilters, "Apply"}, {clearComparisonFilters, "Clear"}} {
				ui.Button(c, button.label).Label(commandLabel(button.id) + " " + label).Disabled(a.branchComparisonDisabled(button.id, target) != "").OnClick(func() { a.run(button.id, target) })
			}
		})
		if side.filterMessage != "" {
			ui.Text(c, side.filterMessage).FillWidth().TextColor(p.theme.Danger)
		} else if side.draft != side.comparison.Filter {
			ui.Text(c, "Filters changed · Apply to update results").FillWidth().TextColor(p.theme.TextMuted)
		}
	})
}
