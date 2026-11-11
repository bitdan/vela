package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/branding"
	"vela/internal/domain"
)

var projectIcon = func() *ui.Bitmap {
	icon, err := ui.DecodeBitmap([]byte(branding.IconPNG))
	if err != nil {
		panic(fmt.Errorf("decode embedded vela icon: %w", err))
	}
	return icon
}()

type tab struct {
	id, title, path, context, source string
	code                             *editor.State
	diff                             *editor.DiffState
	file                             domain.File
	pinned                           bool
}

type repoView struct {
	tabs                                              []tab
	closed                                            []tab
	split                                             *tab
	splitVertical, splitMoved                         bool
	splitSize                                         float32
	preview                                           *tab
	console                                           []string
	consoleOpen                                       bool
	toolPage                                          string
	project                                           ui.OutlineState[string]
	shelf                                             ui.ListState
	projectIndex, shelfIndex                          int
	projectReady, projectShowPaths                    bool
	projectLocate                                     string
	projectFocus                                      bool
	emptyEditorFocus                                  bool
	projectScratch                                    int
	shelfExpanded                                     map[int]bool
	shelfRename, shelfApply                           *commandTarget
	shelfName                                         string
	hideDeletedShelf                                  bool
	children                                          [][]int
	graphRows                                         []graphRow
	logScroll                                         bool
	logAuthor, logRef, logPath                        string
	logDate                                           logDateRange
	datePicker                                        logDatePicker
	logPathsOpen, logSearchFocus, logHideRoot         bool
	logTopo, logFirstParent, logNoMerges              bool
	logView                                           logViewOptions
	logFilePaths                                      []string
	logFirstParentRows                                []bool
	hideChangesToParents                              bool
	logExcludeRef                                     string
	historyGroup                                      commandID
	historyHideDetails, historyPreview                bool
	detailsBranches                                   commitBranchDetails
	logAuthors, logRefs                               []string
	active                                            int
	working, staged, branches, branchResults, files   folderOutline
	log                                               ui.ListState
	commitSource                                      string
	commitGroup                                       commandID
	showIgnored, locateSelected                       bool
	workingChosen, stagedChosen                       ui.Selection[string]
	workingIndex, stagedIndex, branchIndex, fileIndex int
	logQuery, branchQuery                             string
	branchSearchFocus                                 bool
	branchResultIndex                                 int
	filteredLog                                       []int
	lastQuery                                         string
}

type Workspace struct {
	Model                     *application.App
	Layout                    application.Layout
	repos                     []repoView
	status                    string
	pending                   func()
	search                    searchState
	shiftTaps                 shiftGesture
	searchFocus               *editor.State
	focusedEditor             *editor.State
	confirmExit, exitApproved bool
	CloseWindow               func()
	clipboard                 *string
	browserURL                string
	remotes                   remoteDialogState
	settingsOpen              bool
	settings                  settingsDraft
	systemDark                bool
	mainMenuExpanded          bool
	repositoryInit            *repositoryInitDialog
	InitializeRepository      func(context.Context, string, string, func(string, error))
	PickRepositoryDirectory   func(func(string, error))
}

func New(model *application.App, layout application.Layout) *Workspace {
	a := &Workspace{Model: model, Layout: layout, repos: make([]repoView, len(model.Sessions)), status: "Ready"}
	for i := range a.repos {
		r := &a.repos[i]
		r.workingIndex, r.stagedIndex, r.fileIndex = -1, -1, -1
		r.branchIndex = -1
		r.branchResultIndex = -1
		r.detailsBranches.index = -1
		r.projectIndex, r.shelfIndex = -1, -1
		r.shelfExpanded = make(map[int]bool)
		applyLogSettings(r, layout.VersionControl.Log)
		history := model.Sessions[i].Repository.History
		r.children = make([][]int, len(history))
		r.graphRows = buildGraph(history)
		r.logFirstParentRows = firstParentRows(history, model.Sessions[i].Repository.CurrentBranch)
		for child, commit := range history {
			if commit.Author != "" && !slices.Contains(r.logAuthors, commit.Author) {
				r.logAuthors = append(r.logAuthors, commit.Author)
			}
			for _, file := range commit.Files {
				r.logFilePaths = addLogValue(r.logFilePaths, file.Path)
			}
			for _, ref := range commit.Refs {
				if !slices.Contains(r.logRefs, ref) {
					r.logRefs = append(r.logRefs, ref)
				}
			}
			for _, parent := range commit.Parents {
				r.children[parent] = append(r.children[parent], child)
			}
		}
	}
	if files := model.Active().Repository.Working; len(files) > 0 {
		model.Active().Working = files[0].Path
		a.repo().workingIndex = 0
		a.openDiff("working", files[0])
	}
	return a
}

func (a *Workspace) repo() *repoView { return &a.repos[a.Model.Index] }

func (a *Workspace) View(c *ui.Context) {
	a.focusedEditor = nil
	systemTheme := c.Theme()
	a.systemDark = systemTheme.Dark
	p := design(a.darkTheme())
	p.theme.Font = preferredFont(a.Layout.Appearance.Font, systemTheme.Font)
	if size := a.Layout.Appearance.FontSize; size > 0 {
		p.theme.FontSize = float32(min(max(size, 11), 18))
	}
	scale := a.Layout.Appearance.Scale
	if scale == 0 {
		scale = 100
	}
	c.SetZoom(float32(min(max(scale, 80), 125)) / 100)
	c.SetTheme(p.theme)
	c.SetMenuPresentation(ui.PopoverMenus)
	if !c.WindowActive() {
		a.shiftTaps = shiftGesture{}
	}
	ui.Column(c).Fill().Background(p.editor).TabStop(false).AutoFocus().
		ObserveInput(a.observeSearchShortcut).Children(func() {
		a.toolbar(c, p)
		divider(c, p)
		ui.Row(c).FillWidth().Grow(1).Children(func() {
			a.activity(c, p)
			ui.Box(c).Width(1).FillHeight().Background(p.theme.Border)
			if a.Layout.HideGit {
				a.upper(c, p)
				return
			}
			ui.SplitVerticalPercent(c, &a.Layout.Top, func() {
				a.upper(c, p)
			}, func() {
				a.git(c, p)
			}).SplitReset(58).Fill()
		})
		divider(c, p)
		a.statusBar(c, p)
	})
	if a.pending != nil {
		action := a.pending
		a.pending = nil
		action()
	}
	if a.clipboard != nil {
		value := *a.clipboard
		a.clipboard = nil
		c.WriteClipboardThen(value, func(err error) {
			if err != nil {
				a.record("Could not copy to clipboard")
				return
			}
			a.record("Copied " + value)
		})
	}
	if a.browserURL != "" {
		link := a.browserURL
		a.browserURL = ""
		c.OpenURLThen(link, func(err error) {
			if err != nil {
				a.record("Could not open repository in browser")
			}
		})
	}
	if a.settingsOpen {
		a.settingsDialog(c, p)
	}
	if a.remotes.open {
		a.remotesDialog(c, p)
	}
	if a.repositoryInit != nil {
		a.repositoryInitView(c, p)
	}
	if a.search.open {
		a.searchDialog(c, p)
	} else if !a.settingsOpen && !a.remotes.open && a.repositoryInit == nil && a.repo().preview != nil && c.Shortcut(0, ui.KeyEscape) {
		a.run(closePreview, commandTarget{})
	}
	if a.confirmExit {
		if ui.AlertDialog(c, &a.confirmExit, "Discard mock edits?", "Edited mock files are kept in memory for this session.", "Cancel", "Discard and Close") == 1 {
			a.exitApproved = true
			if a.CloseWindow != nil {
				a.CloseWindow()
			}
		}
	}
	if c.Shortcut(ui.Alt, ui.Key0) {
		a.run(toggleGit, commandTarget{})
	}
	if c.Shortcut(ui.Alt, ui.Key1) {
		a.run(toggleCommit, commandTarget{})
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyT) {
		a.run(toggleTheme, commandTarget{})
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyS) {
		a.run(showSettings, commandTarget{})
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		a.run(showSearchActions, commandTarget{})
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		a.run(showCommit, commandTarget{})
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		a.run(goToFile, commandTarget{})
	}
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		a.run(showRecentFiles, commandTarget{})
	}
	if c.Shortcut(ui.Alt, ui.KeyHome) {
		a.run(focusProject, commandTarget{})
	}
	if c.Shortcut(ui.Cmd, ui.KeyW) || c.Shortcut(ui.Ctrl, ui.KeyF4) {
		id := closeTab
		if a.repo().preview != nil {
			id = closePreview
		}
		a.run(id, commandTarget{})
	}
}

func (a *Workspace) upper(c *ui.Context, p tokens) {
	if a.Layout.HideCommit {
		a.editor(c, p)
		return
	}
	ui.SplitPercent(c, &a.Layout.Commit, func() {
		a.changeTools(c, p)
	}, func() {
		a.editor(c, p)
	}).SplitReset(26).Fill()
}

func (a *Workspace) toolbar(c *ui.Context, p tokens) {
	s := a.Model.Active()
	bar := c.TitleBar()
	ui.Row(c).Height(max(toolbarHeight, bar.Height)).FillWidth().
		Padding(0, bar.Right+inset, 0, bar.Left+inset).Gap(gap).AlignItems(ui.Center).
		Background(p.chrome).Children(func() {
		ui.Image(c, projectIcon).Size(projectIconSize, projectIconSize)
		menu := ui.ButtonBase(c.Key(string(toggleMainMenu))).Size(controlHeight, controlHeight).Center().Radius(5).
			Label(commandLabel(toggleMainMenu)).Role(ui.RoleDisclosure).Expanded(a.mainMenuExpanded).
			Children(func() { ui.Icon(c, mainMenuIcon).Size(16, 16).TextColor(p.theme.TextMuted) }).
			OnClick(func() { a.run(toggleMainMenu, commandTarget{}) })
		if a.mainMenuExpanded {
			menu.Background(p.theme.Accent)
			a.mainMenuBar(c, p)
		} else {
			if menu.Hovered() {
				menu.Background(p.theme.SurfaceHover)
			}
			flatToolMenu(c, p, "Switch repository", s.Repository.Name, nil, func(m *ui.Menu) {
				for i, session := range a.Model.Sessions {
					name := session.Repository.Name
					if m.Item(name).ID(name).Radio(i == a.Model.Index).Chosen() {
						a.pending = func() { a.run(selectRepo, commandTarget{index: i}) }
					}
				}
			})
			flatToolMenu(c, p, "Current branch", s.Repository.CurrentBranch, branchIcon, func(m *ui.Menu) {
				a.branchMenu(m, s.Repository.CurrentBranch)
			})
		}
		ui.Box(c).Grow(1).FillHeight().DragWindow()
		commitIcon(c, logSearchIcon, "Search Everywhere").Tooltip("Search Everywhere  Double Shift").
			OnClick(func() { a.run(showSearch, commandTarget{}) })
		titleTool(c, "⚙", "Settings").OnClick(func() { a.run(showSettings, commandTarget{}) })
	})
}

var mainMenuIcon = changeIcon("M2 4h12M2 8h12M2 12h12")

func (a *Workspace) mainMenuBar(c *ui.Context, p tokens) {
	ui.Row(c.Key("main-menu-bar")).Gap(2).Role(ui.RoleMenuBar).Label("Main menu bar").Children(func() {
		for _, group := range []struct {
			name  string
			build func(*ui.Menu)
		}{
			{"View", func(m *ui.Menu) {
				a.menuGroup(m, commandTarget{}, openProject, showCommit, openShelf, showLog, toggleConsole)
			}},
			{"Git", a.mainGitMenu},
		} {
			button := ui.ButtonBase(c.Key(group.name)).Label(group.name+" menu").
				Height(controlHeight).Padding(0, inset).Radius(5).Center()
			open := ui.Local(button, flatMenuOpenKey{}, func() bool { return false })
			button.Children(func() { ui.Text(c, group.name) }).PopoverMenuButton(group.build, ui.PopoverMenuButtonOptions{Open: open})
			if *open {
				button.Background(p.theme.Accent)
			} else if button.Hovered() {
				button.Background(p.theme.SurfaceHover)
			}
		}
	})
}

func titleTool(c *ui.Context, glyph, label string) ui.Element {
	return ui.ButtonBase(c.Key(label)).Size(controlHeight, controlHeight).Center().
		Label(label).Tooltip(label).Children(func() { ui.Text(c, glyph).FontSize(16) })
}

func (a *Workspace) activity(c *ui.Context, p tokens) {
	ui.Column(c).Width(activityWidth).FillHeight().Padding(gap).Gap(gap).Background(p.chrome).Children(func() {
		page := a.repo().toolPage
		for _, id := range []commandID{openProject, showCommit} {
			sameTool := (page == commandLabel(openProject)) == (id == openProject)
			active := !a.Layout.HideCommit && sameTool
			label := commandLabel(id)
			if id == showCommit {
				label = "Commit tool window"
			}
			color := p.theme.TextMuted
			if active {
				color = p.theme.AccentText
			}
			button := ui.ButtonBase(c.Key(string(id))).Size(activityWidth-2*gap, activityWidth-2*gap).
				Center().Label(label).Tooltip(label).Role(ui.RoleToggleButton).Checked(active).Children(func() {
				ui.Icon(c, activityIcons[id]).Size(18, 18).TextColor(color)
			})
			if active {
				button.Background(p.theme.Accent).Radius(6)
			}
			button.OnClick(func() {
				if sameTool {
					a.run(toggleCommit, commandTarget{})
					return
				}
				a.run(id, commandTarget{})
			})
		}
		ui.Box(c).Grow(1)
		label := commandLabel(toggleGit)
		color := p.theme.TextMuted
		if !a.Layout.HideGit {
			color = p.theme.AccentText
		}
		button := ui.ButtonBase(c.Key(string(toggleGit))).Size(activityWidth-2*gap, activityWidth-2*gap).
			Center().Label(label).Tooltip(label).Role(ui.RoleToggleButton).Checked(!a.Layout.HideGit).Children(func() { ui.Icon(c, activityIcons[toggleGit]).Size(18, 18).TextColor(color) })
		if !a.Layout.HideGit {
			button.Background(p.theme.Accent).Radius(6)
		}
		button.OnClick(func() { a.run(toggleGit, commandTarget{}) })
	})
}

func (a *Workspace) statusBar(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	entry := r.preview
	if entry == nil && len(r.tabs) > 0 {
		entry = &r.tabs[r.active]
	}
	ui.Row(c).Height(statusHeight).FillWidth().Padding(0, inset).Gap(inset).AlignItems(ui.Center).Background(p.chrome).Children(func() {
		context := s.Repository.Name
		if entry != nil {
			context += " › " + strings.ReplaceAll(entry.path, "/", " › ")
		}
		ui.Text(c, context).FontSize(p.textSize(smallText)).SingleLine().Ellipsis("…")
		ui.Text(c, a.status).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Grow(1).SingleLine().Ellipsis("…")
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, branchIcon).Size(13, 13).TextColor(p.theme.TextMuted)
			ui.Text(c, s.Repository.CurrentBranch).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted)
		})
		if entry == nil {
			return
		}
		buffer := entry.diff
		var text *editor.Buffer
		var pos editor.Pos
		if entry.code != nil {
			text, pos = entry.code.Buffer, entry.code.CursorPos()
		} else if buffer != nil {
			text, pos = buffer.New, buffer.CursorPos()
		}
		if text == nil {
			return
		}
		ending := "LF"
		if text.LineEnding() == "\r\n" {
			ending = "CRLF"
		}
		indent := "Spaces: 4"
		if strings.Contains(entry.file.New, "\n\t") || strings.HasPrefix(entry.file.New, "\t") {
			indent = "Tabs: 4"
		}
		language := "Plain Text"
		if lang := text.Language(); lang != nil && lang.Name != "" {
			language = lang.Name
		}
		for _, label := range []string{fmt.Sprintf("%d:%d", pos.Line+1, pos.Col+1), ending, "UTF-8", indent, language} {
			ui.Text(c, label).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).SingleLine()
		}
	})
}
