package ui

import "github.com/egoist/mygo/ui"

const shortcutHelpPage = "Shortcuts"

type shortcutHelpItem struct {
	action, scope string
	keys          []string
}

var shortcutHelpGroups = []struct {
	title string
	items []shortcutHelpItem
}{
	{"General", []shortcutHelpItem{
		{"Search Everywhere", "Global", []string{"Double Shift"}},
		{"Go to File", "Global", []string{"Ctrl+Shift+N"}},
		{"Find in Files", "Global", []string{"Ctrl+Shift+F"}},
		{"Replace in Files", "Local Repository", []string{"Ctrl+Shift+R"}},
		{"Recent Files", "Global", []string{"Ctrl+E"}},
		{"Project Navigation", "Global", []string{"Alt+Home"}},
		{"Show All Commands", "Global", []string{"Ctrl+Shift+P"}},
		{"Open Settings", "Global", []string{"Ctrl+Alt+S"}},
		{"Toggle Git Tool Window", "Global", []string{"Alt+0"}},
		{"Toggle Left Tool Window", "Global", []string{"Alt+1"}},
		{"Switch Theme", "Global", []string{"Ctrl+Shift+T"}},
		{"Open Context Menu", "Controls with a Context Menu", []string{"Shift+F10"}},
		{"Move Focus", "Interface Controls", []string{"Tab", "Shift+Tab"}},
	}},
	{"Files and Diff", []shortcutHelpItem{
		{"Save Source or Merge Result", "Editable Editor", []string{"Ctrl+S"}},
		{"Close Current Tab or Diff Preview", "Editor", []string{"Ctrl+W", "Ctrl+F4"}},
		{"Close Diff Preview", "Diff Preview", []string{"Esc"}},
		{"Show Diff for Selected File", "File Lists", []string{"Ctrl+D"}},
		{"Open Selected Source File", "Project, Changes, and Historical Files", []string{"F4"}},
		{"Open File in Right Split", "Project Tree", []string{"Shift+Enter"}},
		{"Group by Directory", "Commit File Lists", []string{"Ctrl+Alt+P"}},
		{"Group by Module", "Commit File Lists", []string{"Ctrl+Alt+M"}},
	}},
	{"Git Log and Shelf", []shortcutHelpItem{
		{"Open Commit Tool Window", "Global", []string{"Ctrl+K"}},
		{"Push", "Local Repository", []string{"Ctrl+Shift+K"}},
		{"Update Project", "Local Repository", []string{"Ctrl+T"}},
		{"New Branch", "Local Repository", []string{"Ctrl+Alt+N"}},
		{"Refresh Commit Log", "Git Log", []string{"Ctrl+F5"}},
		{"Copy Revision Number", "Git Log", []string{"Ctrl+Alt+Shift+C"}},
		{"Go to Parent Commit", "Git Log", []string{"→"}},
		{"Go to Child Commit", "Git Log", []string{"←"}},
		{"Unshelve Selected Mock Changes", "Shelf", []string{"Ctrl+Shift+U"}},
		{"Rename Selected Shelf Entry", "Shelf", []string{"F2"}},
		{"Move Shelf Entry to Recently Deleted", "Shelf", []string{"Delete"}},
	}},
}

func shortcutHelp(c *ui.Context, p tokens) {
	ui.Text(c, "Windows Shortcuts").FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted)
	for _, group := range shortcutHelpGroups {
		ui.Text(c, group.title).Bold().Padding(gap, 0)
		ui.Column(c).FillWidth().Background(p.chrome).Radius(5).Clip().Children(func() {
			for i, item := range group.items {
				if i > 0 {
					divider(c, p)
				}
				ui.Row(c).FillWidth().Height(headerHeight).Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.action).SingleLine().Ellipsis("…")
					ui.Text(c, item.scope).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Grow(1).SingleLine().Ellipsis("…")
					for _, key := range item.keys {
						ui.Text(c, key).FontSize(p.textSize(smallText)).Padding(2, gap).Radius(4).Background(p.theme.SurfaceHover).SingleLine()
					}
				})
			}
		})
	}
}
