package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/branding"
	"vela/internal/domain"
	velaruntime "vela/internal/runtime"
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
	document                         *editorDocument
	comparison                       *branchComparisonDialog
}

type repoView struct {
	shelfGroups                                       application.ShelfGrouping
	logChosen                                         ui.Selection[string]
	historicalChosen                                  ui.Selection[string]
	historicalSelectionRevision                       string
	changelistFilter                                  string
	pendingTabs                                       []application.TabSnapshot
	pendingActiveTab                                  string
	tabs                                              []tab
	closed                                            []tab
	split                                             *tab
	splitVertical, splitMoved                         bool
	splitSize                                         float32
	preview                                           *tab
	mergeDrafts                                       map[string]*mergeEditorState
	mergeActive                                       string
	console                                           []string
	consoleOpen                                       bool
	toolPage                                          string
	project                                           ui.OutlineState[string]
	projectTree                                       projectTreeData
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
	graphWidth                                        float32
	logScroll                                         bool
	logAuthor, logRef, logPath                        string
	logHistoryPath                                    string
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
	logError                                          string
}

type Workspace struct {
	squash                          *squashDialog
	PreviewRepositorySquash         func(context.Context, domain.Repository, []string, func(domain.SquashPreview, error))
	PreviewCommitRewrite            func(context.Context, domain.Repository, domain.RepositoryOperationKind, []string, func(domain.CommitRewritePreview, error))
	Tasks                           velaruntime.Tasks
	tasksOpen                       bool
	taskList                        ui.ListState
	taskSelected                    int
	taskSelection                   velaruntime.TaskID
	taskSplit                       float32
	updateWriting                   string
	UpdateRepository                func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, domain.UpdateResult, error, error))
	background                      backgroundState
	backgroundInFlight              int
	WatchRepository                 func(domain.Repository)
	RefreshRepository               func(context.Context, domain.Repository, func(domain.Repository, error))
	CheckRepositoryRemote           func(context.Context, domain.Repository, application.BackgroundRemotePolicy, func(application.BackgroundRemoteResult, error))
	branchComparison                *branchComparisonDialog
	ReadRepositoryComparison        func(context.Context, domain.Repository, string, string, func(domain.RevisionComparison, error))
	ReadRepositoryComparisonCommits func(context.Context, string, domain.RevisionComparison, int, int, func(domain.CommitLogPage, error))
	ReadRepositoryComparisonChanges func(context.Context, string, domain.RevisionComparison, []string, func([]domain.ComparisonChangeGroup, error))
	lastRealRoot                    string
	Model                           *application.App
	Layout                          application.Layout
	repos                           []repoView
	status                          string
	pending                         func()
	search                          searchState
	shiftTaps                       shiftGesture
	searchFocus                     *editor.State
	focusedEditor                   *editor.State
	inlineBlame                     inlineBlameState
	WatchEditorActivity             func(context.Context, func())
	confirmExit, exitApproved       bool
	exitDiscardDrafts               bool
	sessionSave                     *readRequest
	sessionSaveError                string
	SaveWorkspaceSession            func(context.Context, application.WorkspaceSnapshot, func(error))
	CloseWindow                     func()
	clipboard                       *string
	clipboardNotice                 string
	browserURL                      string
	remotes                         remoteDialogState
	settingsOpen                    bool
	settings                        settingsDraft
	systemDark                      bool
	mainMenuExpanded                bool
	repositoryInit                  *repositoryInitDialog
	repositoryClone                 *repositoryCloneDialog
	patchDialog                     *patchDialog
	fileHistory                     *fileHistoryDialog
	lineHistory                     *lineHistoryDialog
	ReadLineHistory                 func(context.Context, domain.Repository, domain.LineHistoryQuery, func(domain.LineHistoryPage, error))
	patchExport                     *patchExportDialog
	patchImport                     *patchImportDialog
	PickPatchSource                 func(func(string, error))
	PreviewImportedPatch            func(context.Context, domain.Repository, string, string, func(domain.ImportedPatch, error))
	ReviewImportedPatch             func(context.Context, domain.Repository, domain.ImportedPatch, func(domain.ImportedPatch, error))
	ApplyImportedPatch              func(context.Context, domain.Repository, domain.ImportedPatch, func(domain.Repository, error, error))
	patchCopyRead                   *readRequest
	sourceDecision                  *sourceFileDecision
	fileTypeDialog                  *fileTypeDialog
	projectSearch                   *projectSearchDialog
	projectFile                     *projectFileDialog
	projectClipboard                *projectFileClipboard
	projectFileWriting              string
	projectReplaceWriting           string
	ReplaceProjectFiles             func(context.Context, domain.Repository, application.ProjectReplaceInput, func(domain.Repository, application.ProjectReplaceResult, error, error))
	InspectProjectPath              func(context.Context, domain.Repository, string, func(domain.ProjectPathSnapshot, error))
	ManageProjectFiles              func(context.Context, domain.Repository, domain.ProjectFileOperation, func(domain.Repository, error, error))
	OpenProjectFolder               func(string, string, func(error))
	shelfGroupDialog                *shelfGroupDialog
	changelistDialog                *changelistDialog
	revisionBrowser                 *revisionBrowserDialog
	ReadRepositoryRevisionTree      func(context.Context, string, string, string, func(domain.RevisionTree, error))
	ReadRepositoryRevisionChanges   func(context.Context, string, string, int, func([]domain.File, error))
	SearchProjectFiles              func(context.Context, application.ProjectSearchInput, func(application.ProjectSearchResult, error))
	sourceRefreshes                 map[string]*readRequest
	CloneRepository                 func(context.Context, string, string, func(string, error))
	repositoryNavigation            uint64
	InitializeRepository            func(context.Context, string, string, func(string, error))
	PickRepositoryDirectory         func(func(string, error))
	ReadRepository                  func(context.Context, string, func(domain.Repository, error))
	ReadRepositoryLog               func(context.Context, string, domain.CommitLogSnapshot, int, int, func(domain.CommitLogPage, error))
	ReadRepositoryFile              func(context.Context, string, string, domain.File, func(domain.File, error))
	ReadRepositoryBlame             func(context.Context, string, string, string, func(domain.BlameDocument, error))
	ReadRepositoryPatch             func(context.Context, domain.Repository, string, string, func(domain.PatchDocument, error))
	ReadRepositoryFileHistory       func(context.Context, string, string, string, int, int, func(domain.FileHistoryPage, error))
	ReadRepositoryChangesPatch      func(context.Context, domain.Repository, string, []domain.File, func(domain.PatchExport, error))
	PickPatchDestination            func(func(string, error))
	WritePatchFile                  func(context.Context, domain.Repository, string, string, func(domain.Repository, error, error))
	SaveRepositoryFile              func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error))
	ReadMergeDocument               func(context.Context, domain.Repository, string, func(domain.MergeDocument, error))
	repositoryRead                  *readRequest
	logRead                         *readRequest
	repositoryFileRead              *readRequest
	pickingRepository               bool
	repositoryWrite                 string
	repositoryWriteCancel           context.CancelFunc
	repositoryCancelRequested       bool
	indexConfirmation               *indexChange
	commitConfirmation              *commitChange
	UpdateRepositoryIndex           func(context.Context, domain.Repository, bool, []domain.File, func(domain.Repository, error, error))
	CommitRepository                func(context.Context, domain.Repository, string, bool, func(domain.Repository, error, error))
	OperateRepository               func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error))
	repositoryOperation             *repositoryOperationDialog
	savedRepositories               []application.RepositorySnapshot
	restoreRoot                     string
	sessionRestore                  *readRequest
}

func New(model *application.App, layout application.Layout) *Workspace {
	a := &Workspace{Model: model, Layout: layout, repos: make([]repoView, len(model.Sessions)), status: "Ready", taskSelected: -1, taskSplit: 52}
	for i := range a.repos {
		a.repos[i] = newRepoView(model.Sessions[i].Repository, layout)
	}
	if files := model.Active().Repository.Working; len(files) > 0 && model.Active().Repository.Root == "" {
		model.Active().Working = files[0].Path
		a.repo().workingIndex = 0
		a.openDiff("working", files[0])
	}
	return a
}

func newRepoView(repo domain.Repository, layout application.Layout) repoView {
	r := &repoView{}
	r.workingIndex, r.stagedIndex, r.fileIndex = -1, -1, -1
	r.branchIndex = -1
	r.branchResultIndex = -1
	r.detailsBranches.index = -1
	r.projectIndex, r.shelfIndex = -1, -1
	r.shelfExpanded = make(map[int]bool)
	applyLogSettings(r, layout.VersionControl.Log)
	r.logTopo = repo.Root != ""
	if repo.Root != "" {
		r.projectTree = buildProjectTree(repo.Project, repo.ProjectDirectories...)
	}
	r.rebuildCommitLog(repo)
	return *r
}

// rebuildCommitLog updates derived history data without replacing the list's
// scroll/focus state, selections or editor buffers when another page arrives.
func (r *repoView) rebuildCommitLog(repo domain.Repository) {
	history := repo.History
	r.children = make([][]int, len(history))
	r.graphRows = buildGraph(history)
	r.graphWidth = graphWidth
	r.logFirstParentRows = firstParentRows(history, repo.CurrentBranch)
	r.logAuthors, r.logRefs, r.logFilePaths = nil, nil, nil
	r.filteredLog, r.lastQuery = nil, ""
	paths := make(map[string]bool)
	authors := make(map[string]bool)
	refs := make(map[string]bool)
	for child, commit := range history {
		r.graphWidth = max(r.graphWidth, float32(graphStart+min(commit.Lane, lastGraphLane)*graphStep+graphStart))
		if commit.Author != "" && !authors[commit.Author] {
			authors[commit.Author] = true
			r.logAuthors = append(r.logAuthors, commit.Author)
		}
		for _, file := range commit.Files {
			if !paths[file.Path] {
				paths[file.Path] = true
				r.logFilePaths = append(r.logFilePaths, file.Path)
			}
		}
		for _, ref := range commit.Refs {
			if !refs[ref] {
				refs[ref] = true
				r.logRefs = append(r.logRefs, ref)
			}
		}
		for _, parent := range commit.Parents {
			r.children[parent] = append(r.children[parent], child)
		}
	}
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
			if a.welcomeActive() {
				if a.tasksOpen && !a.Layout.HideGit {
					ui.SplitVerticalPercent(c, &a.Layout.Top, func() { a.welcomeView(c, p) }, func() { a.git(c, p) }).SplitReset(58).Fill()
				} else {
					a.welcomeView(c, p)
				}
				return
			}
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
		notice := a.clipboardNotice
		a.clipboardNotice = ""
		if notice == "" {
			notice = "Copied " + value
		}
		a.clipboard = nil
		c.WriteClipboardThen(value, func(err error) {
			if err != nil {
				a.record("Could not copy to clipboard")
				return
			}
			a.record(notice)
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
	if a.repositoryClone != nil {
		a.repositoryCloneView(c, p)
	}
	if a.patchDialog != nil {
		a.patchDialogView(c, p)
	}
	if a.shelfGroupDialog != nil {
		a.shelfGroupView(c, p)
	}
	if a.squash != nil {
		a.squashView(c, p)
	}
	if a.inlineBlame.detailsOpen {
		a.inlineDetailsView(c, p)
	}
	if a.lineHistory != nil {
		a.lineHistoryView(c, p)
	}
	if a.fileHistory != nil {
		a.fileHistoryView(c, p)
	}
	if a.revisionBrowser != nil {
		a.revisionBrowserView(c, p)
	}
	if a.branchComparison != nil {
		a.branchComparisonView(c, p)
	}
	if a.patchExport != nil {
		a.patchExportView(c, p)
	}
	if a.patchImport != nil {
		a.patchImportView(c, p)
	}
	if a.sourceDecision != nil {
		a.sourceDecisionView(c, p)
	}
	if a.fileTypeDialog != nil {
		a.fileTypeView(c, p)
	}
	if a.projectSearch != nil {
		a.projectSearchView(c, p)
	}
	if a.projectFile != nil {
		a.projectFileView(c, p)
	}
	if a.changelistDialog != nil {
		a.changelistView(c, p)
	}
	if a.indexConfirmation != nil {
		a.confirmIndexChange(c)
	}
	if a.commitConfirmation != nil {
		a.confirmAmend(c)
	}
	if a.repositoryOperation != nil {
		a.repositoryOperationView(c, p)
	}
	if a.search.open {
		a.searchDialog(c, p)
	} else if !a.settingsOpen && !a.remotes.open && a.repositoryInit == nil &&
		(a.repo().preview != nil || a.repositoryFileRead != nil) && c.Shortcut(0, ui.KeyEscape) {
		a.run(closePreview, commandTarget{})
	}
	if a.confirmExit {
		if a.canKeepSessionDrafts() || a.sessionSave != nil || a.sessionSaveError != "" {
			a.exitDraftDialog(c, p)
		} else {
			title, message := "Discard mock edits?", "Edited mock files are kept in memory for this session."
			if a.hasUnsavedMergeDrafts() || a.hasUnsavedRealFiles() {
				title, message = "Discard unsaved edits?", "Unsaved editor changes and merge drafts will be lost."
			}
			if ui.AlertDialog(c, &a.confirmExit, title, message, "Cancel", "Discard and Close") == 1 {
				a.exitApproved = true
				a.exitDiscardDrafts = true
				if a.CloseWindow != nil {
					a.CloseWindow()
				}
			}
		}
	}
	a.dispatchWorkspaceShortcuts(c)
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
			name := s.Repository.Name
			if a.welcomeActive() {
				name = "Open Repository"
			}
			flatToolMenu(c, p, "Switch repository", name, nil, func(m *ui.Menu) {
				if a.PickRepositoryDirectory != nil && a.ReadRepository != nil {
					a.menu(m, openLocalRepository, commandTarget{})
					m.Separator()
				}
				for i, session := range a.Model.Sessions {
					if !a.canSelectSession(i) {
						continue
					}
					name := session.Repository.Name
					if root := session.Repository.Root; root != "" {
						name += " · " + root
					}
					if m.Item(name).ID(fmt.Sprint(i)).Radio(i == a.Model.Index).Chosen() {
						a.pending = func() { a.run(selectRepo, commandTarget{index: i}) }
					}
				}
				a.recentRepositoryMenu(m)
				m.Separator()
				a.menu(m, openDemo, commandTarget{})
				if a.demoActive() {
					a.menu(m, leaveDemo, commandTarget{})
				}
			})
			if a.demoActive() && a.Model.Production {
				ui.Text(c, "Demo Mode").TextColor(p.theme.TextMuted)
			}
			if !a.welcomeActive() {
				flatToolMenu(c, p, "Current branch", s.Repository.CurrentBranch, branchIcon, func(m *ui.Menu) {
					a.branchMenu(m, s.Repository.CurrentBranch)
				})
			}
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
				a.menuGroup(m, commandTarget{}, openProject, showCommit, openShelf, showLog, toggleConsole, showTasks)
				m.Separator()
				a.menu(m, restoreLayout, commandTarget{})
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
		if a.Model.Production && a.demoActive() {
			context = "Demo Mode · " + context
		}
		if entry != nil {
			context += " › " + strings.ReplaceAll(entry.path, "/", " › ")
		}
		ui.Text(c, context).FontSize(p.textSize(smallText)).SingleLine().Ellipsis("…")
		ui.Text(c, a.status).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Grow(1).SingleLine().Ellipsis("…")
		ui.ButtonBase(c.Key("task-center")).Height(statusHeight-2).Padding(0, gap).Center().Label("Task Center").Children(func() {
			ui.Text(c, fmt.Sprintf("Tasks (%d)", a.Tasks.ActiveCount())).FontSize(p.textSize(smallText))
		}).OnClick(func() { a.run(showTasks, commandTarget{}) })
		if a.background.task != "" {
			ui.Text(c, a.background.task).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Tooltip(a.background.message)
		} else if a.background.message != "" {
			ui.Text(c, a.background.message).Width(220).SingleLine().Ellipsis("…").FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Tooltip(a.background.message)
		}
		if a.repositoryWriteCancel != nil || a.patchCopyRead != nil || a.background.refresh != nil || a.background.remote != nil || a.activeCancellableTask() != 0 {
			ui.ButtonBase(c.Key("cancel-git-task")).Height(statusHeight-2).Padding(0, gap).Center().Label("Cancel Git Task").Tooltip("Cancel Git task in " + a.repositoryWrite).
				Disabled(a.repositoryCancelRequested).Children(func() { ui.Text(c, "Cancel Task").FontSize(p.textSize(smallText)) }).OnClick(func() { a.run(cancelGitTask, commandTarget{}) })
		}
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
