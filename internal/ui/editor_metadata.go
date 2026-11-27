package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
)

const (
	toggleBookmark   commandID = "editor.toggle-bookmark"
	nextBookmark     commandID = "editor.next-bookmark"
	previousBookmark commandID = "editor.previous-bookmark"
	clearBookmarks   commandID = "editor.clear-bookmarks"
	goBookmark       commandID = "editor.go-bookmark"
	applyFileType    commandID = "editor.apply-file-type"
	cancelFileType   commandID = "editor.cancel-file-type"
)

// editorDocument is shared by split views of the same source Buffer. Bookmarks
// are line numbers, not semantic anchors; edits do not move them with text.
type editorDocument struct {
	draftBase *string
	bookmarks []int
	fileType  string
}

type fileTypeDialog struct {
	open       bool
	repository int
	target     commandTarget
	document   *editorDocument
	buffer     *editor.Buffer
	choice     string
}

func isEditorMetadataCommand(id commandID) bool {
	switch id {
	case toggleBookmark, nextBookmark, previousBookmark, clearBookmarks, goBookmark, fileType, applyFileType, cancelFileType:
		return true
	}
	return false
}

func normalizeBookmarks(lines []int, count int) []int {
	var out []int
	for _, line := range lines {
		if line >= 0 && line < count {
			out = append(out, line)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (a *Workspace) editorMetadataEntry(target commandTarget) *tab {
	r := a.repo()
	state := target.editorState
	if target.tabID == "" && state == nil {
		state = a.focusedEditor
	}
	if split := r.split; split != nil && split.code != nil && split.code == state && (target.tabID == "" || split.id == target.tabID) {
		return split
	}
	if target.tabID == "" && r.preview != nil {
		return nil
	}
	i := r.tabIndex(target.tabID)
	if target.preview || i < 0 || i >= len(r.tabs) {
		return nil
	}
	entry := &r.tabs[i]
	if entry.code == nil || state != nil && entry.code != state {
		return nil
	}
	if entry.document == nil {
		entry.document = &editorDocument{}
	}
	return entry
}

func (a *Workspace) editorMetadataDisabled(id commandID, target commandTarget) string {
	if id == cancelFileType {
		if a.fileTypeDialog == nil {
			return "No file type dialog is open"
		}
		return ""
	}
	if id == applyFileType {
		s := a.fileTypeDialog
		if s == nil || s.repository != a.Model.Index {
			return "Return to the original editor"
		}
		entry := a.editorMetadataEntry(s.target)
		if entry == nil || entry.document != s.document || entry.code.Buffer != s.buffer {
			return "The original editor is no longer open"
		}
		if s.choice != "Automatic" && s.choice != "Plain Text" && editor.LanguageNamed(s.choice) == nil {
			return "Choose a supported file type"
		}
		return ""
	}
	entry := a.editorMetadataEntry(target)
	if entry == nil {
		return "Select a source editor"
	}
	lines := normalizeBookmarks(entry.document.bookmarks, entry.code.Buffer.LineCount())
	if (id == nextBookmark || id == previousBookmark || id == clearBookmarks) && len(lines) == 0 {
		return "This document has no bookmarks"
	}
	if id == goBookmark && !slices.Contains(lines, target.index) {
		return "The bookmark no longer exists"
	}
	return ""
}

func (a *Workspace) runEditorMetadata(id commandID, target commandTarget) {
	if id == cancelFileType {
		a.fileTypeDialog = nil
		return
	}
	if id == applyFileType {
		s := a.fileTypeDialog
		entry := a.editorMetadataEntry(s.target)
		value := s.choice
		if value == "Automatic" {
			value = ""
		} else if value != "Plain Text" {
			value = editor.LanguageNamed(value).Name
		}
		entry.document.fileType = value
		applyEditorFileType(entry)
		a.fileTypeDialog = nil
		return
	}
	entry := a.editorMetadataEntry(target)
	doc, code := entry.document, entry.code
	doc.bookmarks = normalizeBookmarks(doc.bookmarks, code.Buffer.LineCount())
	line := code.CursorPos().Line
	switch id {
	case fileType:
		choice := doc.fileType
		if choice == "" {
			choice = "Automatic"
		}
		target.tabID, target.editorState, target.file = entry.id, code, entry.file
		a.fileTypeDialog = &fileTypeDialog{open: true, repository: a.Model.Index, target: target, document: doc, buffer: code.Buffer, choice: choice}
	case toggleBookmark:
		if target.source == "gutter" {
			line = target.index
		}
		if line < 0 || line >= code.Buffer.LineCount() {
			return
		}
		if i := slices.Index(doc.bookmarks, line); i >= 0 {
			doc.bookmarks = slices.Delete(doc.bookmarks, i, i+1)
		} else {
			doc.bookmarks = append(doc.bookmarks, line)
			slices.Sort(doc.bookmarks)
		}
	case clearBookmarks:
		doc.bookmarks = nil
	case nextBookmark, previousBookmark:
		i, _ := slices.BinarySearch(doc.bookmarks, line)
		if id == nextBookmark {
			if i < len(doc.bookmarks) && doc.bookmarks[i] == line {
				i++
			}
			if i == len(doc.bookmarks) {
				i = 0
			}
		} else {
			i--
			if i < 0 {
				i = len(doc.bookmarks) - 1
			}
		}
		code.GoTo(editor.Pos{Line: doc.bookmarks[i]})
		a.focusBookmarkEditor(entry)
	case goBookmark:
		code.GoTo(editor.Pos{Line: target.index})
		a.focusBookmarkEditor(entry)
	}
}

func (a *Workspace) focusBookmarkEditor(entry *tab) {
	if split := a.repo().split; split == nil || split.code != entry.code {
		a.activate(entry.id)
	}
	a.searchFocus = entry.code
}

func applyEditorFileType(entry *tab) {
	if entry.code == nil || entry.document == nil {
		return
	}
	var language *editor.Language
	switch entry.document.fileType {
	case "":
		if !strings.HasPrefix(entry.source, "blame:") {
			language = editor.LanguageFor(entry.path)
		}
	case "Plain Text":
	default:
		language = editor.LanguageNamed(entry.document.fileType)
	}
	entry.code.Buffer.SetLanguage(language)
}

func (a *Workspace) prepareEditorMetadata(entry *tab, p tokens) {
	if entry.document == nil {
		entry.document = &editorDocument{}
	}
	entry.document.bookmarks = normalizeBookmarks(entry.document.bookmarks, entry.code.Buffer.LineCount())
	applyEditorFileType(entry)
	entry.code.GutterMarks = nil
	for _, line := range entry.document.bookmarks {
		entry.code.GutterMarks = append(entry.code.GutterMarks, editor.GutterMark{Line: line, Color: p.theme.Focus})
	}
	entry.code.OnGutterClick = func(line int) {
		a.run(toggleBookmark, commandTarget{tabID: entry.id, editorState: entry.code, source: "gutter", index: line})
	}
}

func (a *Workspace) restoreEditorMetadata(entry *tab, saved application.TabSnapshot) {
	fileType := saved.FileType
	if fileType != "" && fileType != "Plain Text" {
		if language := editor.LanguageNamed(fileType); language != nil {
			fileType = language.Name
		} else {
			fileType = ""
		}
	}
	entry.document = &editorDocument{bookmarks: normalizeBookmarks(saved.Bookmarks, entry.code.Buffer.LineCount()), fileType: fileType}
	applyEditorFileType(entry)
}

func (a *Workspace) editorBookmarkMenu(m *ui.Menu, target commandTarget) {
	m.Submenu("Bookmarks", func(sub *ui.Menu) {
		a.menuGroup(sub, target, toggleBookmark, nextBookmark, previousBookmark, clearBookmarks)
		if entry := a.editorMetadataEntry(target); entry != nil {
			lines := normalizeBookmarks(entry.document.bookmarks, entry.code.Buffer.LineCount())
			if len(lines) > 0 {
				sub.Separator()
			}
			for _, line := range lines[:min(20, len(lines))] {
				t := target
				t.index = line
				a.menuNamed(sub, goBookmark, t, fmt.Sprintf("Line %d", line+1), "")
			}
			if len(lines) > 20 {
				sub.Item("Use Next / Previous for more bookmarks").Disabled(true)
			}
		}
	})
}

func (a *Workspace) fileTypeView(c *ui.Context, p tokens) {
	s := a.fileTypeDialog
	ui.DialogBase(c, &s.open, func(_ ui.Element, panel ui.Element) {
		panel.Width(420).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("File type dialog").Children(func() {
			heading(c, p, "Override File Type", "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.target.file.Path).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, "Highlighting for this document and its split views.").TextColor(p.theme.TextMuted)
				choices := []string{"Automatic", "Plain Text"}
				for _, language := range editor.Languages() {
					choices = append(choices, language.Name)
				}
				ui.Select(c, &s.choice, choices).Label("Document file type").FillWidth()
				ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() {
					a.button(c, cancelFileType, "Cancel")
					a.button(c, applyFileType, "Apply")
				})
			})
		})
	})
	if !s.open {
		a.fileTypeDialog = nil
	}
}
