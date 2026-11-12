package ui

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func (a *Workspace) commit(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		a.commitToolbar(c, p)
		divider(c, p)
		if s.Repository.ResolvingConflicts() {
			a.mergeControls(c, p)
		}
		working := s.Repository.Working
		if r.showIgnored {
			working = appendIgnored(working)
		}
		a.fileList(c.Key("working"), p, "Working Changes", "working", "No working changes",
			working, &r.working, &r.workingIndex, &s.Working, &r.workingChosen)
		a.fileList(c.Key("staged"), p, "Staged Changes", "staged", "No staged changes",
			s.Repository.Staged, &r.staged, &r.stagedIndex, &s.Staged, &r.stagedChosen)
		if s.Repository.Root == "" {
			divider(c, p)
			ui.Column(c).FillWidth().Padding(gap, inset).Gap(gap).Children(func() {
				ui.Checkbox(c, &s.Amend, "Amend last commit")
				ui.TextArea(c, &s.Message).Label("Commit message").Placeholder("Commit message").Height(66).FillWidth()
				a.button(c, commitMock, "Preview Commit")
			})
		} else if !s.Repository.ResolvingConflicts() {
			a.commitForm(c, p)
		}
	})
}

func (a *Workspace) fileList(c *ui.Context, p tokens, title, source, empty string, files []domain.File,
	outline *folderOutline, index *int, selected *string, chosen *ui.Selection[string]) {
	working := source == "working" || source == "staged"
	if working {
		heading(c, p, title, fileCount(len(files)))
	}
	r := a.repo()
	groupMode := r.commitGroup
	if !working {
		groupMode = r.historyGroup
		if groupMode == "" {
			groupMode = groupDirectory
		}
	}
	repo := a.Model.Active().Repository
	tree, byPath := changeTree(repo, files, groupMode)
	outline.prepare(c, p, tree)
	locating := working && r.locateSelected && a.selectedChange().source == source
	if locating {
		for parent := tree.parent[*selected]; parent != ""; parent = tree.parent[parent] {
			outline.Open.Add(parent)
		}
	}
	state := &outline.List
	state.Selected = index
	state.Label = func(i int) string {
		item := outline.Item(i)
		if group, ok := tree.folders[item]; ok {
			return group.name
		}
		return item
	}
	if chosen != nil {
		state.Selection = chosen
	}
	currentFile := func() (domain.File, bool) {
		if *index < 0 || *index >= outline.Rows() {
			return domain.File{}, false
		}
		f, ok := byPath[outline.Item(*index)]
		return f, ok
	}
	list := ui.Outline(c, &outline.OutlineState, tree.roots, func(item string) []string { return tree.children[item] }, func(item string) {
		if group, ok := tree.folders[item]; ok {
			ui.Row(c).Height(rowHeight).Grow(1).Gap(gap).AlignItems(ui.Center).Children(func() {
				folderIcon(c, p, group.kind)
				ui.Text(c, group.name).Label(title + " folder " + group.name).SingleLine().Ellipsis("…")
				ui.Text(c, fileCount(group.count)).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Shrink(0)
				if working && group.kind == repositoryNode {
					ui.Text(c, repo.CurrentBranch).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).
						Background(p.theme.SurfaceHover).Radius(2).Padding(1, 4).Shrink(0)
				}
			})
			return
		}
		f := byPath[item]
		color := fileStatusColor(p, &a.Layout.VersionControl.Colors, f.Status)
		ui.Row(c).Height(rowHeight).Grow(1).Gap(gap).AlignItems(ui.Center).
			ContextMenu(func(m *ui.Menu) { a.fileMenu(m, source, f) }).Children(func() {
			fileIcon(c, p, f.Path, f.Status)
			label := f.Path
			if !working {
				label = "Historical " + f.Path
			}
			ui.Text(c, path.Base(f.Path)).Label(label).Tooltip(f.Path).TextColor(color).SingleLine().Ellipsis("…")
			if dir := path.Dir(f.Path); working && groupMode != groupDirectory && dir != "." {
				ui.Text(c, dir).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Grow(1).SingleLine().Ellipsis("…")
			}
		})
	}).Grow(1).FillWidth().Label(title).ContextMenu(func(m *ui.Menu) {
		if f, ok := currentFile(); ok {
			a.fileMenu(m, source, f)
		}
	})
	if len(files) == 0 {
		list.Children(func() { ui.Text(c, empty).Padding(inset).TextColor(p.theme.TextMuted) })
	}
	if working && state.FocusWithin(c) {
		a.repo().commitSource = source
	}
	if locating {
		for i := range outline.Rows() {
			if outline.Item(i) == *selected {
				*index = i
				state.ScrollTo(i, ui.Center)
				break
			}
		}
		r.locateSelected = false
	}
	if f, ok := currentFile(); ok {
		target := commandTarget{source: source, file: f}
		if list.Changed() {
			*selected = f.Path
		}
		if list.Submitted() {
			id := previewDiff
			if working {
				id = openCode
			}
			if source == "working" && f.Status == "U" && a.ReadMergeDocument != nil && repo.ResolvingConflicts() {
				id = openMergeEditor
			}
			a.run(id, target)
		} else if list.Changed() && (working || r.historyPreview) {
			a.run(previewDiff, target)
		}
		if state.Shortcut(c, ui.Cmd, ui.KeyD) {
			a.run(previewDiff, target)
		}
		if source == "working" && state.Shortcut(c, ui.Ctrl|ui.Alt, ui.KeyA) {
			a.run(stageChanges, target)
		}
		if working && state.Shortcut(c, ui.Ctrl|ui.Alt, ui.KeyP) {
			a.run(groupDirectory, commandTarget{})
		}
		if working && state.Shortcut(c, ui.Ctrl|ui.Alt, ui.KeyM) {
			a.run(groupModule, commandTarget{})
		}
		if state.Shortcut(c, 0, ui.KeyF4) {
			a.run(openCode, target)
		}
	}
}

func fileCount(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

func (a *Workspace) git(c *ui.Context, p tokens) {
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		r := a.repo()
		ui.Row(c).Height(headerHeight+4).FillWidth().Padding(0, inset).Gap(gap).AlignItems(ui.Center).Background(p.chrome).Children(func() {
			ui.Text(c, "Git").Bold()
			toolTab(c, p, "Log", !r.consoleOpen, func() { a.run(showLog, commandTarget{}) })
			toolTab(c, p, "Console", r.consoleOpen, func() { a.run(toggleConsole, commandTarget{}) })
			ui.Box(c).Grow(1)
			if r.consoleOpen {
				a.panelAction(c, clearConsole, "×")
			} else {
				titleTool(c, "⋯", "Git View Options").Menu(func(m *ui.Menu) {
					a.menuGroup(m, commandTarget{}, clearLogFilters, toggleBranches, toggleDetails)
				})
			}
			a.panelAction(c, toggleGit, "−")
		})
		divider(c, p)
		if r.consoleOpen {
			a.consolePanel(c, p)
			return
		}
		logAndDetails := func() {
			if a.Layout.HideDetails {
				a.commitLog(c, p)
				return
			}
			ui.SplitPercent(c, &a.Layout.Log, func() {
				a.commitLog(c, p)
			}, func() {
				a.commitDetails(c, p)
			}).SplitReset(64).Fill()
		}
		if a.Layout.HideBranches {
			ui.Column(c).Grow(1).FillWidth().Children(logAndDetails)
			return
		}
		ui.SplitPercent(c, &a.Layout.Branches, func() {
			a.branchList(c, p)
		}, logAndDetails).SplitReset(22).Grow(1).FillWidth()
	})
}

func displayBranch(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, "refs/tags/"), "refs/remotes/")
}

func branchGroup(name string) string {
	if strings.HasPrefix(name, "refs/tags/") {
		return "Tags"
	}
	if strings.HasPrefix(name, "origin/") {
		return "Remote"
	}
	return "Local"
}

func repoBranchGroup(repo domain.Repository, name string) string {
	if repo.Root == "" || strings.HasPrefix(name, "refs/tags/") {
		return branchGroup(name)
	}
	if repo.RemoteBranches[name] {
		return "Remote"
	}
	return "Local"
}

func (a *Workspace) branchList(c *ui.Context, p tokens) {
	ui.Row(c).Fill().Label("Branches").Children(func() {
		a.branchToolbar(c, p)
		ui.Box(c).Width(1).FillHeight().Background(p.theme.Border)
		a.branchRows(c, p)
	})
}

func (a *Workspace) branchRows(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Height(headerHeight).FillWidth().Padding(0, gap).AlignItems(ui.Center).Children(func() {
			search := ui.TextInput(c, &r.branchQuery).Label("Filter branches").Placeholder("Branch or tag").Grow(1)
			if r.branchSearchFocus {
				search.Focus()
				r.branchSearchFocus = false
			}
		})
		divider(c, p)
		query := strings.ToLower(strings.TrimSpace(r.branchQuery))
		outline := r.branchOutline()
		tree := newFolderTree()
		for _, group := range []string{"Local", "Remote", "Tags"} {
			for _, name := range s.Repository.Branches {
				if repoBranchGroup(s.Repository, name) != group || !strings.Contains(strings.ToLower(displayBranch(name)), query) {
					continue
				}
				parent := tree.folder("", group, folderNode)
				dir := path.Dir(displayBranch(name))
				if dir != "." {
					for segment := range strings.SplitSeq(dir, "/") {
						parent = tree.folder(parent, segment, folderNode)
					}
				}
				tree.add(parent, name)
			}
		}
		outline.prepare(c, p, tree)
		outline.List.Selected = &r.branchIndex
		if query != "" {
			outline.List.Selected = &r.branchResultIndex
		}
		outline.List.Label = func(i int) string {
			item := outline.Item(i)
			if group, ok := tree.folders[item]; ok {
				return group.name
			}
			return displayBranch(item)
		}
		ui.Text(c, "HEAD  ·  "+s.Repository.CurrentBranch).FontSize(p.textSize(smallText)).Bold().Padding(gap, inset)
		currentBranch := func() string {
			i := *outline.List.Selected
			if i >= 0 && i < outline.Rows() {
				item := outline.Item(i)
				if _, isGroup := tree.folders[item]; !isGroup {
					return item
				}
			}
			return ""
		}
		list := ui.Outline(c.Key(query != ""), &outline.OutlineState, tree.roots, func(item string) []string { return tree.children[item] }, func(name string) {
			if group, ok := tree.folders[name]; ok {
				ui.Row(c).Grow(1).Height(rowHeight).Gap(gap).AlignItems(ui.Center).Children(func() {
					if tree.parent[name] != "" {
						folderIcon(c, p, folderNode)
					}
					ui.Text(c, group.name).Label("Branch folder " + strings.ReplaceAll(strings.TrimPrefix(name, "\x00"), "\x00", "/")).SingleLine().Ellipsis("…")
				})
				return
			}
			icon, color := branchIcon, p.theme.TextMuted
			if branchGroup(name) == "Tags" {
				icon = tagIcon
			}
			if name == s.Repository.CurrentBranch {
				icon, color = favoriteIcon, p.favorite
			}
			ui.Row(c).Grow(1).Height(rowHeight).Gap(5).AlignItems(ui.Center).
				ContextMenu(func(m *ui.Menu) { a.branchMenu(m, name) }).Children(func() {
				ui.Icon(c, icon).Size(14, 14).TextColor(color)
				ui.Text(c, path.Base(displayBranch(name))).Label(displayBranch(name)).Tooltip(displayBranch(name)).SingleLine().Ellipsis("…")
			})
		}).Grow(1).FillWidth().Label("Branches list").ContextMenu(func(m *ui.Menu) {
			if name := currentBranch(); name != "" {
				a.branchMenu(m, name)
			}
		})
		if name := currentBranch(); list.Changed() && name != "" {
			a.run(selectBranch, commandTarget{source: name})
		}
		if len(tree.roots) == 0 {
			list.Children(func() { ui.Text(c, "No matching branches").Padding(inset).TextColor(p.theme.TextMuted) })
		}
	})
}

func (a *Workspace) commitLog(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	history := s.Repository.History
	ui.Column(c).Fill().Label("Commit Log / Graph").Children(func() {
		a.logFilters(c, p)
		switch s.HistoryState {
		case application.HistoryLoading:
			ui.Text(c, "Loading mock history…").Padding(inset).TextColor(p.theme.TextMuted)
			a.button(c, retryHistory, "Retry history")
			return
		case application.HistoryError:
			ui.Text(c, "Mock history could not be loaded").Padding(inset).TextColor(p.theme.Danger)
			a.button(c, retryHistory, "Retry history")
			return
		}
		query := strings.ToLower(strings.TrimSpace(r.logQuery))
		pathQuery := strings.ToLower(strings.TrimSpace(r.logPath))
		if len(history) == 0 {
			ui.Text(c, "No commits yet").Padding(inset).TextColor(p.theme.TextMuted)
			return
		}
		if r.logHistoryPath != "" {
			ui.Row(c).FillWidth().Padding(0, gap).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "History · "+r.logHistoryPath).Grow(1).SingleLine().Ellipsis("…")
				ui.Button(c, "Clear File History").OnClick(func() { r.logHistoryPath = "" })
			})
		}
		key := fmt.Sprintf("%q:%q:%q:%q:%q:%q:%t:%t:%t:%t:%t:%v", query, r.logAuthor, r.logRef, pathQuery, r.logHistoryPath, r.logExcludeRef, r.logHideRoot, r.logFirstParent, r.logNoMerges, r.logTopo, r.logView.timestamps, r.logDate)
		filteredView := query != "" || r.logAuthor != "" || r.logRef != "" || r.logExcludeRef != "" || r.logDate.preset != "" || pathQuery != "" || r.logHistoryPath != "" || r.logHideRoot || r.logFirstParent || r.logNoMerges || (s.Repository.Root != "" && !r.logTopo)
		if key != r.lastQuery {
			r.filteredLog = nil
			if filteredView {
				reachable := reachableCommits(history, r.logRef)
				excluded := reachableCommits(history, r.logExcludeRef)
				firstParents := r.logFirstParentRows
				if r.logFirstParent && r.logRef != "" {
					firstParents = firstParentRows(history, r.logRef)
				}
				for i, commit := range history {
					if r.logHideRoot || (r.logNoMerges && commit.Merge) || (r.logFirstParent && !firstParents[i]) {
						continue
					}
					if (strings.Contains(strings.ToLower(commit.Subject), query) || strings.Contains(commit.Hash, query)) &&
						(r.logAuthor == "" || r.logAuthor == commit.Author) &&
						r.logDate.includes(logTimestamp(commit, r.logView.timestamps), history[0].Timestamp) &&
						(reachable == nil || reachable[i]) && (excluded == nil || !excluded[i]) &&
						(r.logHistoryPath == "" || slices.ContainsFunc(commit.Files, func(file domain.File) bool {
							return file.Path == r.logHistoryPath || file.OldPath == r.logHistoryPath
						})) &&
						(pathQuery == "" || slices.ContainsFunc(commit.Files, func(file domain.File) bool {
							return strings.Contains(strings.ToLower(file.Path), pathQuery)
						})) {
						r.filteredLog = append(r.filteredLog, i)
					}
				}
			}
			if !r.logTopo {
				slices.SortStableFunc(r.filteredLog, func(i, j int) int { return history[j].Timestamp.Compare(history[i].Timestamp) })
			}
			r.lastQuery = key
		}
		count := len(history)
		selected := s.Commit
		index := func(i int) int { return i }
		if filteredView {
			filtered := r.filteredLog
			count = len(filtered)
			index = func(i int) int { return filtered[i] }
			selected = -1
			for i, j := range r.filteredLog {
				if j == s.Commit {
					selected = i
					break
				}
			}
		}
		if settings := a.Layout.VersionControl.Confirmation; settings.LimitHistory {
			count = min(count, max(1, int(settings.HistoryRows)))
			if selected >= count {
				selected = -1
			}
		}
		r.log.Selected = &selected
		r.log.Key = func(i int) any { return history[index(i)].Hash }
		r.log.Label = func(i int) string { return history[index(i)].Subject }
		list := ui.List(c, &r.log, count, func(i int) {
			a.logRow(c, p, history[index(i)], history[0].Timestamp, i, index(i), index(i) == s.Commit, filteredView)
		}).Grow(1).FillWidth().ContextMenu(func(m *ui.Menu) {
			if selected >= 0 && selected < count {
				a.logMenu(m, index(selected), history[index(selected)])
			}
		})
		if list.Changed() && selected >= 0 && selected < count {
			a.run(selectCommit, commandTarget{index: index(selected)})
		}
		if r.log.Shortcut(c, ui.Ctrl, ui.KeyF5) {
			a.run(refreshLog, commandTarget{})
		}
		if selected >= 0 && selected < count {
			commitIndex := index(selected)
			entry := history[commitIndex]
			if r.log.Shortcut(c, 0, ui.KeyRight) && len(entry.Parents) > 0 {
				a.run(goParent, commandTarget{index: entry.Parents[0]})
			}
			if r.log.Shortcut(c, 0, ui.KeyLeft) && len(r.children[commitIndex]) > 0 {
				a.run(goChild, commandTarget{index: r.children[commitIndex][0]})
			}
			if r.log.Shortcut(c, ui.Cmd|ui.Alt|ui.Shift, ui.KeyC) {
				a.run(copyHash, commandTarget{source: entry.Hash})
			}
			if r.logScroll && commitIndex == s.Commit {
				r.log.ScrollTo(selected, ui.Center)
				r.logScroll = false
			}
		}
		if count == 0 {
			list.Children(func() { ui.Text(c, "No matching commits").Padding(inset).TextColor(p.theme.TextMuted) })
		}
	})
}

func firstParentRows(history []domain.Commit, ref string) []bool {
	rows := make([]bool, len(history))
	for i := slices.IndexFunc(history, func(entry domain.Commit) bool { return slices.Contains(entry.Refs, ref) }); i >= 0; {
		rows[i] = true
		if len(history[i].Parents) == 0 {
			break
		}
		i = history[i].Parents[0]
	}
	return rows
}

func branchTip(history []domain.Commit, ref string) (domain.Commit, bool) {
	for _, entry := range history {
		if slices.Contains(entry.Refs, ref) {
			return entry, true
		}
	}
	return domain.Commit{}, false
}

func reachableCommits(history []domain.Commit, ref string) []bool {
	if ref == "" {
		return nil
	}
	seen := make([]bool, len(history))
	if i := slices.IndexFunc(history, func(entry domain.Commit) bool { return slices.Contains(entry.Refs, ref) || entry.Hash == ref }); i >= 0 {
		work := []int{i}
		for len(work) > 0 {
			current := work[len(work)-1]
			work = work[:len(work)-1]
			if !seen[current] {
				seen[current] = true
				work = append(work, history[current].Parents...)
			}
		}
	}
	return seen
}

func (a *Workspace) logRow(c *ui.Context, p tokens, entry domain.Commit, latest time.Time, rowIndex, commitIndex int, selected, filtered bool) {
	r := a.repo()
	row := ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center)
	row.ContextMenu(func(m *ui.Menu) { a.logMenu(m, commitIndex, entry) })
	if rowIndex%2 == 1 && !selected {
		row.Background(p.stripe)
	}
	row.Children(func() {
		meta, ref := p.theme.TextMuted, p.graph[0]
		if selected {
			meta, ref = p.theme.AccentText, p.theme.AccentText
		}
		v := r.logView
		if !v.hideAuthor {
			author := ui.Text(c, entry.Author).Label("Commit author " + entry.Hash).Width(84).Shrink(0).
				FontSize(p.textSize(smallText)).TextColor(meta).SingleLine().Ellipsis("…")
			if selected {
				author.Bold()
			}
		}
		graph := graphRow{}
		if !filtered {
			graph = r.graphRows[commitIndex]
		}
		ui.Box(c).Width(r.graphWidth).Shrink(0).FillHeight().Label("Commit graph " + entry.Hash).Draw(func(painter *ui.Painter, box ui.Rect) {
			drawCommit(painter, box, p, entry, graph)
		})
		refLabel := func() {
			var labels []string
			for _, name := range entry.Refs {
				group := repoBranchGroup(a.Model.Active().Repository, name)
				if v.hideTags && group == "Tags" {
					continue
				}
				label := displayBranch(name)
				if v.expandedRefs {
					label = group + " · " + label
				}
				labels = append(labels, label)
				if !v.expandedRefs {
					break
				}
			}
			if len(labels) != 0 {
				ui.Text(c, strings.Join(labels, ", ")).FontSize(p.textSize(smallText)).TextColor(ref).Padding(0, gap).MaxWidth(135).SingleLine().Ellipsis("…")
			}
		}
		if v.root {
			ui.Text(c, a.Model.Active().Repository.Name).FontSize(p.textSize(smallText)).TextColor(meta)
		}
		if v.refsLeft {
			refLabel()
		}
		subject := ui.Text(c, entry.Subject).Grow(1).Ellipsis("…")
		if (!v.noMyHighlight && entry.Author == "Lin") || (!v.noMergeHighlight && entry.Merge) ||
			(!v.noBranchHighlight && slices.Contains(entry.Refs, a.Model.Active().Repository.CurrentBranch)) {
			subject.Bold()
		}
		if !v.refsLeft {
			refLabel()
		}
		if v.showHash {
			ui.Text(c, entry.Hash).FontSize(p.textSize(smallText)).TextColor(meta)
		}
		if !v.hideDate {
			date := entry.Date
			timestamp := logTimestamp(entry, v.timestamps)
			if !timestamp.IsZero() {
				date = formatLogTimestamp(timestamp, latest)
			}
			ui.Text(c, date).Label("Commit date " + entry.Hash).Width(112).Shrink(0).
				FontSize(p.textSize(smallText)).TextColor(meta).SingleLine().FontFeatures("tnum")
		}
	})
}

func (a *Workspace) commitDetails(c *ui.Context, p tokens) {
	s, r := a.Model.Active(), a.repo()
	history := s.Repository.History
	if s.HistoryState != application.HistoryReady || s.Commit < 0 || s.Commit >= len(history) {
		heading(c, p, "Commit Details", "")
		ui.Text(c, "Select a commit").Padding(inset).TextColor(p.theme.TextMuted)
		return
	}
	entry := history[s.Commit]
	files := entry.Files
	if r.hideChangesToParents && len(entry.Parents) > 1 {
		parent := history[entry.Parents[0]]
		files = nil
		for _, file := range entry.Files {
			if previous, ok := matchFile(parent.Files, file.Path); ok {
				if previous.New == file.New {
					continue
				}
				file.Old = previous.New
			}
			files = append(files, file)
		}
	}
	filePanel := func() {
		ui.Column(c).Fill().Children(func() {
			a.historicalToolbar(c, p, entry.Hash, files)
			a.fileList(c.Key(entry.Hash), p, "Changed Files", entry.Hash, "No changed files", files, &r.files, &r.fileIndex, &s.Historical, nil)
		})
	}
	if r.historyHideDetails {
		filePanel()
		return
	}
	ui.SplitVerticalPercent(c, &a.Layout.Files, filePanel, func() {
		a.commitSummary(c, p, s.Repository, s.Commit)
	}).SplitReset(54).Fill()
}
