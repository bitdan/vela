package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

type searchCategory string

const (
	searchAll        searchCategory = "All"
	searchClasses    searchCategory = "Classes"
	searchFiles      searchCategory = "Files"
	searchSymbols    searchCategory = "Symbols"
	searchActions    searchCategory = "Actions"
	searchText       searchCategory = "Text"
	searchRecent     searchCategory = "Recent Files"
	searchLimit                     = 100
	shiftTapInterval                = 500 * time.Millisecond
)

type searchState struct {
	open           bool
	category       searchCategory
	query          string
	selected       int
	list           ui.ListState
	resultCategory searchCategory
	resultQuery    string
	results        []searchResult
	previousEditor *editor.State
}

type shiftGesture struct {
	down        ui.KeyCode
	start, last time.Time
}

func (g *shiftGesture) observe(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown, ui.InputText, ui.InputCompose, ui.InputCommand:
		*g = shiftGesture{}
		return false
	case ui.InputKeyDown, ui.InputKeyUp:
	default:
		return false
	}
	if ev.Code != ui.CodeShiftLeft && ev.Code != ui.CodeShiftRight {
		if ev.Kind == ui.InputKeyDown {
			*g = shiftGesture{}
		}
		return false
	}
	if ev.Repeat {
		return false
	}
	if ev.Kind == ui.InputKeyDown {
		if ev.Mods == ui.Shift && g.start.IsZero() {
			g.down, g.start = ev.Code, ev.Time
		} else {
			*g = shiftGesture{}
		}
		return false
	}
	if g.start.IsZero() || g.down != ev.Code || ev.Mods != 0 || ev.Time.Sub(g.start) > shiftTapInterval {
		*g = shiftGesture{}
		return false
	}
	second := !g.last.IsZero() && g.start.Sub(g.last) <= shiftTapInterval
	*g = shiftGesture{}
	if !second {
		g.last = ev.Time
	}
	return second
}

func (a *Workspace) observeSearchShortcut(ev ui.InputEvent) {
	if a.shiftTaps.observe(ev) {
		a.run(showSearch, commandTarget{})
	}
}

func (a *Workspace) openSearch(category searchCategory) {
	if a.settingsOpen || a.confirmExit || a.search.open {
		return
	}
	a.search = searchState{open: true, category: category, previousEditor: a.focusedEditor}
}

type searchDocument struct {
	file  domain.File
	tabID string
}

type searchResult struct {
	title, detail string
	kind          searchCategory
	document      searchDocument
	action        commandID
	start, end    int
}

func (a *Workspace) searchDocuments(recent bool) []searchDocument {
	var docs []searchDocument
	seen := make(map[string]bool)
	r := a.repo()
	addTab := func(entry tab) {
		if entry.code == nil || strings.HasPrefix(entry.source, "revision:") || seen[entry.path] {
			return
		}
		file := entry.file
		file.New = entry.code.Buffer.String()
		docs = append(docs, searchDocument{file: file, tabID: entry.id})
		seen[entry.path] = true
	}
	if len(r.tabs) != 0 {
		addTab(r.tabs[r.active])
	}
	if r.split != nil {
		addTab(*r.split)
	}
	for _, entry := range r.tabs {
		addTab(entry)
	}
	if recent {
		for i := len(r.closed) - 1; i >= 0; i-- {
			addTab(r.closed[i])
		}
		return docs
	}
	for _, file := range projectFiles(a.Model.Active().Repository) {
		if !seen[file.Path] {
			docs = append(docs, searchDocument{file: file})
		}
	}
	return docs
}

func (a *Workspace) searchResults(category searchCategory, query string) []searchResult {
	lowerQuery := strings.ToLower(query)
	match := func(text string) bool { return strings.Contains(strings.ToLower(text), lowerQuery) }
	var results []searchResult
	if category != searchActions {
		docs := a.searchDocuments(category == searchRecent)
		if category == searchAll || category == searchFiles || category == searchRecent {
			for _, doc := range docs {
				if match(doc.file.Path) {
					detail := path.Dir(doc.file.Path)
					if detail == "." {
						detail = a.Model.Active().Repository.Name
					}
					results = append(results, searchResult{title: path.Base(doc.file.Path), detail: detail, kind: searchFiles, document: doc})
					if len(results) == searchLimit {
						break
					}
				}
			}
		}
		if category == searchClasses || category == searchSymbols || (category == searchAll && query != "") {
			for _, doc := range docs {
				results = searchDeclarations(results, doc, category, match)
			}
		}
		if query != "" && (category == searchAll || category == searchText) {
			expression := regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
			for _, doc := range docs {
				offset := 0
				for i, line := range strings.Split(doc.file.New, "\n") {
					if len(results) >= searchLimit {
						break
					}
					if m := expression.FindStringIndex(line); m != nil {
						results = append(results, searchResult{title: strings.TrimSpace(line), detail: fmt.Sprintf("%s:%d", doc.file.Path, i+1),
							kind: searchText, document: doc, start: offset + m[0], end: offset + m[1]})
					}
					offset += len(line) + 1
				}
			}
		}
	}
	if category == searchActions || (category == searchAll && query != "") {
		for _, id := range []commandID{switchRepo, toggleCommit, toggleGit, toggleBranches, toggleDetails, toggleTheme,
			closeTab, openProject, openShelf, toggleConsole, showLog, refreshLog, showSettings, showLoading, showError, retryHistory,
			goToFile, showRecentFiles, focusProject, manageRemotes, openLocalRepository, newBranch, fetch, pull, push, updateProject} {
			label := commandLabel(id)
			if match(label) && a.disabledFor(id, commandTarget{}) == "" {
				results = append(results, searchResult{title: label, detail: "Action", kind: searchActions, action: id})
			}
		}
	}
	return results[:min(len(results), searchLimit)]
}

func searchDeclarations(results []searchResult, doc searchDocument, category searchCategory, match func(string) bool) []searchResult {
	if doc.file.New == "" || path.Ext(doc.file.Path) != ".go" || len(results) >= searchLimit {
		return results
	}
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, doc.file.Path, doc.file.New, parser.SkipObjectResolution)
	if file == nil {
		return results
	}
	add := func(name *ast.Ident, kind searchCategory) {
		if (category == searchAll || category == searchSymbols || category == kind) && match(name.Name) && len(results) < searchLimit {
			pos := fset.Position(name.Pos())
			results = append(results, searchResult{title: name.Name, detail: fmt.Sprintf("%s:%d", doc.file.Path, pos.Line), kind: kind,
				document: doc, start: pos.Offset, end: pos.Offset + len(name.Name)})
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			add(decl.Name, searchSymbols)
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					add(spec.Name, searchClasses)
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						add(name, searchSymbols)
					}
				}
			}
		}
	}
	return results
}

func (a *Workspace) chooseSearchResult(result searchResult, split bool) {
	a.search.open = false
	if result.kind == searchActions {
		a.run(result.action, commandTarget{})
		return
	}
	target := commandTarget{source: "project", file: result.document.file, tabID: result.document.tabID}
	r := a.repo()
	if target.tabID != "" && r.tabIndex(target.tabID) < 0 {
		closed := slices.IndexFunc(r.closed, func(entry tab) bool { return entry.id == target.tabID })
		if closed >= 0 {
			previous := ""
			if len(r.tabs) > 0 {
				previous = r.tabs[r.active].id
			}
			a.reopenClosedTabAt(closed)
			if split {
				a.activate(previous)
			}
		}
	}
	if split {
		a.run(openRightSplit, target)
		return
	}
	if !a.activate(result.document.tabID) {
		if r.split != nil && r.split.id == result.document.tabID {
			r.preview = nil
			if result.kind != searchFiles {
				r.split.code.Select(result.start, result.end)
			}
			a.searchFocus = r.split.code
			return
		}
		a.run(openCode, target)
		if a.Model.Active().Repository.Root != "" {
			return
		}
	}
	r.preview = nil
	if result.kind != searchFiles {
		r.tabs[r.active].code.Select(result.start, result.end)
	}
	a.searchFocus = r.tabs[r.active].code
}

func (a *Workspace) searchDialog(c *ui.Context, p tokens) {
	s := &a.search
	width, height := c.Size()
	dialog := ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .10))
		panel.Size(min(width-32, 690), min(height-48, 340)).Translate(0, -min(height/8, 100)).
			Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Search Everywhere dialog").
			Column().Children(func() {
			query := strings.TrimSpace(s.query)
			category := s.category
			if category != searchRecent {
				if after, ok := strings.CutPrefix(query, "/"); ok {
					category, query = searchActions, strings.TrimSpace(after)
				}
			}
			ui.Row(c).FillWidth().Padding(0, inset).Gap(gap).Background(p.chrome).Children(func() {
				if category == searchRecent {
					ui.Text(c, "Recent Files").Height(headerHeight + 4).Bold()
					return
				}
				for _, tab := range []searchCategory{searchAll, searchClasses, searchFiles, searchSymbols, searchActions, searchText} {
					toolTab(c, p, string(tab), category == tab, func() {
						if category != s.category {
							s.query = query
						}
						s.category, s.selected = tab, 0
						s.list.ScrollTo(0, ui.Start)
					}).KeepFocus()
				}
			})
			divider(c, p)
			var input ui.Element
			placeholder := "Search files, symbols, actions, or text"
			if category == searchRecent {
				placeholder = "Filter recent files"
			} else if category == searchFiles {
				placeholder = "Search by file name or path"
			}
			ui.Row(c).FillWidth().Padding(gap, inset).Children(func() {
				input = ui.TextInput(c, &s.query).Placeholder(placeholder).
					Label("Search Everywhere query").Grow(1).AutoFocus().OnChange(func() {
					s.selected = 0
					s.list.ScrollTo(0, ui.Start)
				})
				if s.category == searchAll && query == "" && category == searchAll {
					ui.Text(c, "Type / to see commands").FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Padding(gap, 0)
				}
			})
			if category != s.resultCategory || query != s.resultQuery {
				s.results = a.searchResults(category, query)
				s.resultCategory, s.resultQuery = category, query
			}
			results := s.results
			if len(results) == 0 {
				message := "No matches"
				if category == searchText && query == "" {
					message = "Enter text to search the project"
				} else if category == searchRecent && query == "" {
					message = "No recent files in this workspace"
				}
				ui.Text(c, message).Grow(1).Padding(inset).TextColor(p.theme.TextMuted)
				return
			}
			s.selected = min(max(s.selected, 0), len(results)-1)
			s.list.Selected = &s.selected
			s.list.Label = func(i int) string { return results[i].title }
			list := ui.List(c, &s.list, len(results), func(i int) {
				result := results[i]
				ui.Row(c).Height(rowHeight).FillWidth().Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
					if result.kind == searchActions {
						ui.Icon(c, commitIcons[logSearchIcon]).Size(15, 15).TextColor(p.theme.TextMuted)
					} else {
						fileIcon(c, p, result.document.file.Path, result.document.file.Status)
					}
					ui.Text(c, result.title).SingleLine().Ellipsis("…")
					ui.Text(c, result.detail).Grow(1).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).SingleLine().Ellipsis("…")
				})
			}).Grow(1).FillWidth().Label("Search results")
			if input.Shortcut(0, ui.KeyDown) {
				s.selected = min(s.selected+1, len(results)-1)
				s.list.ScrollTo(s.selected, ui.Center)
			} else if input.Shortcut(0, ui.KeyUp) {
				s.selected = max(s.selected-1, 0)
				s.list.ScrollTo(s.selected, ui.Center)
			}
			result := results[s.selected]
			input.OnTextKey(func(key ui.TextKey) bool {
				if key.Key != ui.KeyEnter || key.Mods != ui.Shift || !s.open ||
					s.selected < 0 || s.selected >= len(results) || results[s.selected].kind != searchFiles {
					return false
				}
				a.chooseSearchResult(results[s.selected], true)
				return true
			})
			canSplit := result.kind == searchFiles
			if canSplit && panel.OverlayShortcut(ui.Shift, ui.KeyEnter) {
				a.chooseSearchResult(result, true)
			} else if list.Submitted() || input.Submitted() {
				a.chooseSearchResult(result, false)
			}
			divider(c, p)
			ui.Row(c).Height(headerHeight).FillWidth().Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
				hint := result.document.file.Path
				if result.kind == searchActions {
					hint = "Enter to run action"
				}
				ui.Text(c, hint).Grow(1).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).SingleLine().Ellipsis("…")
				if canSplit {
					ui.ButtonBase(c).Height(rowHeight).Padding(0, gap).Center().Label("Open in Right Split").
						Tooltip("Shift+Enter").Children(func() { ui.Text(c, "Open in Right Split") }).
						OnClick(func() { a.chooseSearchResult(result, true) })
				}
			})
		})
	})
	if dialog.Dismissed() {
		a.searchFocus = s.previousEditor
	}
}
