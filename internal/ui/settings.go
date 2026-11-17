package ui

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const (
	settingsWidth         = 1040
	settingsHeight        = 740
	settingsSidebar       = 205
	settingsNavArrowSize  = 12
	settingsNavLabelInset = inset + settingsNavArrowSize + gap
	settingsNavChildInset = settingsNavLabelInset + settingsNavArrowSize
)

type settingsDraft struct {
	keymap                                      map[string]string
	keymapRow                                   int
	keymapList                                  ui.ListState
	keymapCommand, keymapBinding, keymapMessage string
	shortcutReference                           bool
	vcs                                         application.VersionControlSettings
	appearance                                  application.AppearanceSettings
	editor                                      application.EditorSettings
	dark                                        bool
	query, page                                 string
	color                                       int
	colorPicker                                 bool
	versionControlClosed                        bool
	searchGroupClosed                           bool
}

var settingsPages = []string{"Changelists", "Commit", "Confirmation", "Directory Mappings", "File Status Colors", "Log", "Shelf", "Git"}

const versionControlGroup = "Version Control"

func (a *Workspace) openSettings() {
	if a.settingsOpen {
		return
	}
	page := a.settings.page
	if page == "" {
		page = appearancePage
	}
	a.settings = settingsDraft{vcs: a.Layout.VersionControl, dark: a.Layout.Dark, page: page,
		appearance: a.Layout.Appearance, editor: a.Layout.Editor, keymap: maps.Clone(layoutKeymap(a.Layout))}
	a.settingsOpen = true
}

func (a *Workspace) applySettings() {
	draft := a.settings
	bindings, err := application.NormalizeKeymap(draft.keymap)
	if err != nil {
		a.settings.keymapMessage = err.Error()
		return
	}
	if draft.vcs.Log != a.Layout.VersionControl.Log {
		for i := range a.repos {
			applyLogSettings(&a.repos[i], draft.vcs.Log)
		}
	}
	if draft.vcs.Git != a.Layout.VersionControl.Git {
		a.stopBackgroundTasks()
		a.background.nextRemote = time.Now()
		a.background.message = ""
	}
	a.Layout.VersionControl, a.Layout.Dark = draft.vcs, draft.dark
	a.Layout.Appearance, a.Layout.Editor = draft.appearance, draft.editor
	a.Layout.Keymap = nil
	if len(bindings) > 0 {
		a.Layout.Keymap = &application.KeymapSettings{Bindings: maps.Clone(bindings)}
	}
}

func applyLogSettings(r *repoView, settings application.LogSettings) {
	r.logView.expandedRefs = settings.ExpandedRefs
	r.logView.hideTags = settings.HideTags
	r.logView.timestamps = settings.Timestamp
	r.logView.refsLeft = settings.RefsLeft
	r.logView.hideAuthor = settings.HideAuthor
	r.logView.hideDate = settings.HideDate
	r.logView.showHash = settings.Hash
	r.logFirstParent = settings.FirstParent
	r.historyPreview = settings.Preview
	r.historyHideDetails = settings.HideDetails
}

func (a *Workspace) settingsDialog(c *ui.Context, p tokens) {
	width, height := c.Size()
	ui.DialogBase(c, &a.settingsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Size(min(settingsWidth, width-32), min(settingsHeight, height-32)).
			Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Settings dialog").Column().Children(func() {
			ui.Row(c).FillWidth().Height(headerHeight).Padding(0, inset).Gap(gap).
				AlignItems(ui.Center).Background(p.chrome).Children(func() {
				ui.Text(c, "Settings").Bold().Grow(1)
				ui.Text(c, a.Model.Active().Repository.Name).TextColor(p.theme.TextMuted)
				titleTool(c, "×", "Close settings").OnClick(func() { a.settingsOpen = false })
			})
			divider(c, p)
			ui.Row(c).Grow(1).FillWidth().Children(func() {
				a.settingsNavigation(c, p)
				ui.Box(c).Width(1).FillHeight().Background(p.theme.Border)
				ui.Column(c).Grow(1).FillHeight().Children(func() {
					title := a.settings.page
					if isVersionControlPage(title) {
						title = versionControlGroup + "  ›  " + title
					} else {
						title = "Settings  ›  " + title
					}
					heading(c, p, title, "")
					ui.Scroll(c.Key(a.settings.page)).Grow(1).FillWidth().Children(func() {
						ui.Column(c).FillWidth().Padding(inset * 2).Gap(gap * 2).Children(func() { a.settingsPage(c, p) })
					})
				})
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Box(c).Grow(1)
				_, keymapErr := application.NormalizeKeymap(a.settings.keymap)
				invalid := a.settings.invalidColor() || keymapErr != nil
				changed := a.settings.vcs != a.Layout.VersionControl || a.settings.dark != a.Layout.Dark ||
					a.settings.appearance != a.Layout.Appearance || a.settings.editor != a.Layout.Editor || !maps.Equal(a.settings.keymap, layoutKeymap(a.Layout))
				ui.PrimaryButton(c, "OK").Width(76).Disabled(invalid).OnClick(func() { a.applySettings(); a.settingsOpen = false })
				ui.Button(c, "Cancel").Width(76).OnClick(func() { a.settingsOpen = false })
				ui.Button(c, "Apply").Width(76).Disabled(invalid || !changed).OnClick(a.applySettings)
			})
		})
	})
}

func (a *Workspace) settingsNavigation(c *ui.Context, p tokens) {
	s := &a.settings
	ui.Column(c).Width(settingsSidebar).FillHeight().Background(p.chrome).Children(func() {
		ui.Row(c).FillWidth().Padding(inset).Children(func() {
			ui.TextInput(c, &s.query).Placeholder("Search settings").Label("Search settings").Grow(1).
				OnChange(func() { s.searchGroupClosed = false })
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				query := strings.ToLower(strings.TrimSpace(s.query))
				item := func(page string, indent float32) {
					selected := page == s.page
					button := ui.ButtonBase(c.Key(page)).Label("Settings "+page).Height(rowHeight).FillWidth().
						Role(ui.RoleTab).AccessSelected(selected).Padding(0, inset, 0, indent).
						Row().Justify(ui.Start).AlignItems(ui.Center).Children(func() { ui.Text(c, page) })
					if selected {
						button.Background(p.settingsSelection)
					}
					button.OnClick(func() { s.page = page; s.colorPicker = false })
				}
				for _, page := range []string{appearancePage, editorPage, shortcutHelpPage} {
					if matchesSettingsPage(page, query) {
						item(page, settingsNavLabelInset)
					}
				}
				closed := &s.versionControlClosed
				if query != "" {
					closed = &s.searchGroupClosed
				}
				matchGroup := strings.Contains(strings.ToLower(versionControlGroup), query)
				groupShown := false
				for _, page := range settingsPages {
					if !matchGroup && !strings.Contains(strings.ToLower(page), query) {
						continue
					}
					if !groupShown {
						settingsDisclosure(c, p, closed)
						groupShown = true
					}
					if !*closed {
						item(page, settingsNavChildInset)
					}
				}
				if matchesSettingsPage(aboutPage, query) {
					item(aboutPage, settingsNavLabelInset)
				}
			})
		})
	})
}

func settingsDisclosure(c *ui.Context, p tokens, closed *bool) {
	chevron := chevronDownIcon
	if *closed {
		chevron = chevronRightIcon
	}
	ui.ButtonBase(c.Key(versionControlGroup)).Label("Settings "+versionControlGroup).
		Role(ui.RoleDisclosure).Expanded(!*closed).Height(rowHeight).FillWidth().
		Padding(0, inset).Gap(gap).Row().Justify(ui.Start).AlignItems(ui.Center).
		Children(func() {
			ui.Icon(c, chevron).Size(settingsNavArrowSize, settingsNavArrowSize).TextColor(p.theme.TextMuted)
			ui.Text(c, versionControlGroup).Bold()
		}).OnClick(func() { *closed = !*closed })
}

func settingsSection(c *ui.Context, p tokens, label string) {
	ui.Text(c, label).Bold().Padding(gap, 0)
	divider(c, p)
}

func settingsSelect(c *ui.Context, label string, value *string, options ...string) {
	ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label)
		ui.Select(c, value, options).Label(label)
	})
}

func settingsNumber(c *ui.Context, label string, value *float64, lo, hi float64) {
	ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label)
		ui.NumberInput(c, value, lo, hi, 1).Width(100).Label(label)
	})
}

func (a *Workspace) settingsPage(c *ui.Context, p tokens) {
	s := &a.settings.vcs
	future := func(text string) { ui.Text(c, text).TextColor(p.theme.TextMuted).FontSize(p.textSize(smallText)) }
	switch a.settings.page {
	case appearancePage:
		a.appearanceSettings(c, p)
	case editorPage:
		a.editorSettings(c, p)
	case aboutPage:
		aboutSettings(c, p)
	case shortcutHelpPage:
		a.keymapSettings(c, p)
	case "Changelists":
		ui.Checkbox(c, &s.Changelists.AutoCreate, "Create changelists automatically")
		ui.Checkbox(c, &s.Changelists.SplitFileChanges, "Allow changes within a file in different changelists")
		settingsSection(c, p, "Inactive Changelist")
		ui.Checkbox(c, &s.Changelists.HighlightInactive, "Highlight files from inactive changelists")
		ui.Checkbox(c, &s.Changelists.WarnInactive, "Show dialog before editing inactive changes").Disabled(!s.Changelists.HighlightInactive)
		settingsSelect(c, "When an empty changelist becomes inactive", &s.Changelists.EmptyInactive, "Show options", "Keep", "Remove")
		future("Whole-file lists, membership and active list are implemented. Automatic creation, hunk splitting and inactive-edit policies remain unapplied.")
	case "Commit":
		ui.Checkbox(c, &s.Commit.ClearMessage, "Clear commit message after preview")
		settingsSection(c, p, "Commit message inspections")
		ui.Checkbox(c, &s.Commit.BlankLine, "Blank line between subject and body")
		ui.Checkbox(c, &s.Commit.LimitSubject, "Limit subject line")
		if s.Commit.LimitSubject {
			settingsNumber(c, "Subject length", &s.Commit.SubjectLength, 1, 500)
		}
		ui.Checkbox(c, &s.Commit.LimitBody, "Limit body line")
		if s.Commit.LimitBody {
			settingsNumber(c, "Body line length", &s.Commit.BodyLength, 1, 500)
		}
		settingsSection(c, p, "Commit Checks")
		ui.Checkbox(c, &s.Commit.CheckTODO, "Check TODO in staged files")
		for _, label := range []string{"Reformat code", "Optimize imports", "Run configuration", "Analyze code"} {
			unavailable := false
			ui.Checkbox(c.Key(label), &unavailable, label).Disabled(true).Tooltip("Requires IDE code tooling")
		}
		future("Message inspections apply to real commits and Mock previews. TODO checks inspect added staged lines; fix the finding or disable the check to commit.")
	case "Confirmation":
		settingsSection(c, p, "Confirmation")
		for _, field := range []struct {
			label string
			value *string
		}{
			{"When files are created", &s.Confirmation.CreatedFiles}, {"When files are deleted", &s.Confirmation.DeletedFiles},
			{"When a patch is created", &s.Confirmation.PatchCreated},
		} {
			settingsSelect(c, field.label, field.value, "Ask", "Do not ask")
		}
		ui.Checkbox(c, &s.Confirmation.Checkout, "Show options before Checkout")
		ui.Checkbox(c, &s.Confirmation.Update, "Show options before Update")
		future("Destructive Git operations require confirmation.")
		settingsSection(c, p, "History")
		ui.Checkbox(c, &s.Confirmation.LimitHistory, "Limit history rows")
		if s.Confirmation.LimitHistory {
			settingsNumber(c, "Maximum history rows", &s.Confirmation.HistoryRows, 1, 100000)
		}
	case "Directory Mappings":
		settingsSection(c, p, "Mock repository mappings")
		for _, repo := range a.Model.Sessions {
			ui.Text(c, fmt.Sprintf("%s  ›  Git (Mock)", repo.Repository.Name))
		}
	case "File Status Colors":
		a.settingsColors(c, p)
	case "Log":
		ui.Checkbox(c, &s.Log.ExpandedRefs, "Show reference categories")
		settingsInverted(c, "Show tag names", &s.Log.HideTags)
		ui.Checkbox(c, &s.Log.Timestamp, "Show commit timestamp")
		ui.Checkbox(c, &s.Log.RefsLeft, "Show references on the left")
		ui.Checkbox(c, &s.Log.FirstParent, "Follow first parent")
		settingsSection(c, p, "Visible Columns")
		settingsInverted(c, "Show Author column", &s.Log.HideAuthor)
		ui.Checkbox(c, &s.Log.Hash, "Show Hash column")
		settingsInverted(c, "Show Date column", &s.Log.HideDate)
		settingsSection(c, p, "File History")
		settingsInverted(c, "Display details panel", &s.Log.HideDetails)
		ui.Checkbox(c, &s.Log.Preview, "Show the diff preview panel")
	case "Shelf":
		ui.Checkbox(c, &s.Shelf.RemoveApplied, "Remove successfully applied files from the shelf")
		ui.Checkbox(c, &s.Shelf.StoreBase, "Shelve base revisions of files").Disabled(true).
			Tooltip("Base revisions are always kept for safe unshelve")
		if a.Model.Active().Repository.Root != "" {
			future("Shelf location: local Git objects, separate from Git Stash. Successful removal moves whole entries to Recently Deleted.")
		} else {
			future("Shelf location: in-memory Mock workspace")
		}
	case "Git":
		a.gitSettings(c, p)
	}
}

func settingsInverted(c *ui.Context, label string, target *bool) {
	value := !*target
	if ui.Checkbox(c, &value, label).Changed() {
		*target = !value
	}
}

func (a *Workspace) gitSettings(c *ui.Context, p tokens) {
	git := &a.settings.vcs.Git
	ui.Text(c, "Automatic Fetch / incoming checks run for the active repository every five minutes; incoming-only checks do not fetch. Changes apply to the next task.").FillWidth().FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted)
	ui.Checkbox(c, &git.WarnLarge, "Warn when committing large files")
	if git.WarnLarge {
		settingsNumber(c, "Large file threshold (MB)", &git.LargeFileMB, 1, 100000)
	}
	ui.Checkbox(c, &git.WarnNames, "Warn about file names incompatible with other systems")
	ui.Checkbox(c, &git.CherryPickSuffix, "Add source hash when cherry-picking to a protected branch")
	ui.Button(c, "Configure GPG Key…").Width(160).Disabled(true).Tooltip("Commit signing is available in a later Git phase")
	settingsSection(c, p, "Push")
	ui.Checkbox(c, &git.ShowPushDialog, "Show Push dialog for Commit and Push")
	ui.Text(c, "Protected branches")
	ui.TextInput(c, &git.ProtectedBranches).Label("Protected branches").FillWidth()
	settingsSection(c, p, "Update")
	settingsSelect(c, "Update method", &git.UpdateMethod, "Merge", "Rebase")
	settingsSelect(c, "Clean working tree using", &git.CleanWorkingTree, "Stash", "Shelve")
	ui.Checkbox(c, &git.AutoFetch, "Fetch remote changes automatically")
	ui.Checkbox(c, &git.CheckIncoming, "Check remote for incoming changes")
	settingsSelect(c, "Fetch tags", &git.FetchTags, "Auto", "Always", "Never")
	ui.Checkbox(c, &git.CredentialHelper, "Use credential helper")
	ui.Text(c, "Fetch tags and credential helper preferences apply to repository Fetch / Pull / Push / Update and background remote tasks. Auto respects Git's tag configuration. Update method and cleanup are defaults in the explicit Update confirmation; protected-branch options and GPG configuration remain pending.").FillWidth().TextColor(p.theme.TextMuted)
}
