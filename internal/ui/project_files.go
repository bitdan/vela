package ui

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

const applyProjectFile commandID = "project.apply-file-action"
const closeProjectFile commandID = "project.close-file-action"

type projectFileClipboard struct {
	root   string
	source domain.ProjectPathSnapshot
	cut    bool
}

type projectFileDialog struct {
	open, loading, confirmed   bool
	root, destination, message string
	id                         commandID
	op                         domain.ProjectFileOperation
	request                    *readRequest
}

func isProjectFileCommand(id commandID) bool {
	switch id {
	case newProjectFile, newProjectDirectory, renameFile, deleteFile, cutProjectFile, copyProjectFile, pasteProjectFile, addIgnore, openInExplorer, applyProjectFile, closeProjectFile:
		return true
	}
	return false
}

func (a *Workspace) projectFileTarget(t commandTarget) commandTarget {
	if t.file.Path != "" {
		return t
	}
	r := a.repo()
	if r.toolPage == "Project" && r.projectIndex >= 0 && r.projectIndex < r.project.Rows() {
		return commandTarget{source: "project", file: domain.File{Path: r.project.Item(r.projectIndex)}}
	}
	return commandTarget{source: "project", file: domain.File{Path: "."}}
}

func (a *Workspace) projectFileParent(t commandTarget) string {
	t = a.projectFileTarget(t)
	if _, file := a.repo().projectTree.byPath[t.file.Path]; file {
		return path.Dir(t.file.Path)
	}
	return t.file.Path
}

func projectPathContains(parent, item string) bool {
	return strings.EqualFold(parent, item) || strings.HasPrefix(strings.ToLower(item), strings.ToLower(parent)+"/")
}

func (a *Workspace) projectFileDirty(root, name string) bool {
	for i, session := range a.Model.Sessions {
		if session.Repository.Root != root {
			continue
		}
		r := &a.repos[i]
		for _, entry := range r.tabs {
			if entry.source == "project" && projectPathContains(name, entry.path) && entry.modified() {
				return true
			}
		}
		if r.split != nil && r.split.source == "project" && projectPathContains(name, r.split.path) && r.split.modified() {
			return true
		}
	}
	return false
}

func (a *Workspace) projectFileDisabled(id commandID, t commandTarget) string {
	if id == closeProjectFile {
		return ""
	}
	repo := a.Model.Active().Repository
	if repo.Root == "" {
		return "Open a local repository"
	}
	if a.InspectProjectPath == nil || a.ManageProjectFiles == nil {
		return "Project file operations are unavailable"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.repositoryOperation != nil || a.repositoryInit != nil || a.repositoryClone != nil {
		return "Wait for the current repository operation"
	}
	if repo.ResolvingConflicts() || repo.Bisect.Active() {
		return "Finish the current Git operation first"
	}
	if id == applyProjectFile {
		s := a.projectFile
		if s == nil || s.root != repo.Root || s.loading {
			return "Wait for the project item to load"
		}
		if s.message != "" {
			return s.message
		}
		if s.op.Action == domain.TrashProjectPath && !s.confirmed {
			return "Confirm moving the item to the recycle bin"
		}
		if s.op.Action != domain.TrashProjectPath && s.op.Action != domain.IgnoreProjectPath && s.destination == "" {
			return "Enter a destination path"
		}
		if s.destination != "" && a.projectFileDirty(repo.Root, s.destination) {
			return "Save or discard unsaved destination files first"
		}
		name := s.op.Source.Path
		if name != "" && a.projectFileDirty(repo.Root, name) {
			return "Save or discard unsaved source files first"
		}
		if s.op.Action == domain.IgnoreProjectPath && a.projectFileDirty(repo.Root, ".gitignore") {
			return "Save or discard the .gitignore editor first"
		}
		return ""
	}
	if id == openInExplorer && a.OpenProjectFolder == nil {
		return "System file manager is unavailable"
	}
	if id == pasteProjectFile {
		if a.projectClipboard == nil || a.projectClipboard.root != repo.Root {
			return "Copy or cut a project item in this repository first"
		}
		return ""
	}
	if id == newProjectFile || id == newProjectDirectory || id == openInExplorer {
		return ""
	}
	t = a.projectFileTarget(t)
	if t.file.Path == "." {
		return "Select a file or directory below the project root"
	}
	if t.source != "project" {
		return "Select a local Project item"
	}
	if a.projectFileDirty(repo.Root, t.file.Path) {
		return "Save or discard unsaved source files first"
	}
	return ""
}

func (a *Workspace) closeProjectFileDialog() {
	if s := a.projectFile; s != nil {
		stopRead(&s.request)
		a.projectFile = nil
	}
}

func (a *Workspace) runProjectFile(id commandID, t commandTarget) {
	if id == closeProjectFile {
		a.closeProjectFileDialog()
		return
	}
	if id == applyProjectFile {
		a.applyProjectFile()
		return
	}
	t = a.projectFileTarget(t)
	repo := a.Model.Active().Repository
	if id == openInExplorer {
		a.OpenProjectFolder(repo.Root, t.file.Path, func(err error) {
			if err != nil && a.Model.Active().Repository.Root == repo.Root {
				a.record("Could not open file manager · " + err.Error())
			}
		})
		return
	}
	a.closeProjectFileDialog()
	s := &projectFileDialog{open: true, root: repo.Root, id: id}
	a.projectFile = s
	switch id {
	case newProjectFile, newProjectDirectory:
		s.op.Action = domain.CreateProjectFile
		if id == newProjectDirectory {
			s.op.Action = domain.CreateProjectDirectory
		}
		s.destination = path.Join(a.projectFileParent(t), "untitled")
		return
	case pasteProjectFile:
		s.op.Source = a.projectClipboard.source
		s.op.Action = domain.CopyProjectPath
		if a.projectClipboard.cut {
			s.op.Action = domain.MoveProjectPath
		}
		s.destination = path.Join(a.projectFileParent(t), path.Base(s.op.Source.Path))
		return
	case renameFile:
		s.op.Action = domain.MoveProjectPath
		s.destination = t.file.Path
	case deleteFile:
		s.op.Action = domain.TrashProjectPath
	case addIgnore:
		s.op.Action = domain.IgnoreProjectPath
	}
	s.loading = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	a.InspectProjectPath(ctx, repo, t.file.Path, func(snapshot domain.ProjectPathSnapshot, err error) {
		if a.projectFile != s || s.request != request || a.Model.Active().Repository.Root != s.root {
			return
		}
		stopRead(&s.request)
		s.loading = false
		if err != nil {
			s.message = "Could not inspect project item · " + err.Error()
			return
		}
		s.op.Source = snapshot
		if id == cutProjectFile || id == copyProjectFile {
			a.projectClipboard = &projectFileClipboard{root: repo.Root, source: snapshot, cut: id == cutProjectFile}
			a.projectFile = nil
			a.status = "Copied project item · " + snapshot.Path
			if id == cutProjectFile {
				a.status = "Ready to move project item · " + snapshot.Path
			}
		}
	})
}

func (a *Workspace) applyProjectFile() {
	s := a.projectFile
	op := s.op
	op.Destination = s.destination
	repo := a.Model.Active().Repository
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	// Protect source buffers against new input while a disk action is in flight.
	locked := map[*editor.State]bool{}
	lock := func(entry *tab) {
		if entry == nil || entry.code == nil || entry.source != "project" {
			return
		}
		if projectPathContains(op.Source.Path, entry.path) || (op.Action == domain.IgnoreProjectPath && entry.path == ".gitignore") {
			if _, exists := locked[entry.code]; exists {
				return
			}
			locked[entry.code] = entry.code.ReadOnly
			entry.code.ReadOnly = true
		}
	}
	for i := range a.repo().tabs {
		lock(&a.repo().tabs[i])
	}
	lock(a.repo().split)
	a.projectFile = nil
	a.closeProjectSearchDialog()
	a.beginRepositoryWrite(repo.Root, "Updating project files…", cancel)
	a.projectFileWriting = repo.Root
	a.ManageProjectFiles(ctx, repo, op, func(updated domain.Repository, err, refreshErr error) {
		a.projectFileWriting = ""
		for state, readOnly := range locked {
			state.ReadOnly = readOnly
		}
		message := "Project file action completed"
		if err != nil {
			message = "Project file action failed · " + err.Error()
		}
		a.finishRepositoryWrite(repo.Root, updated, refreshErr, message)
		if err != nil {
			return
		}
		for i, session := range a.Model.Sessions {
			if session.Repository.Root != repo.Root {
				continue
			}
			r := &a.repos[i]
			if op.Action == domain.MoveProjectPath || op.Action == domain.TrashProjectPath {
				a.updateProjectFileTabs(r, repo.Root, op)
			}
			if op.Destination != "" {
				r.projectLocate = op.Destination
			}
		}
		if op.Action == domain.MoveProjectPath && a.projectClipboard != nil && a.projectClipboard.root == repo.Root && a.projectClipboard.source == op.Source {
			a.projectClipboard = nil
		}
		if op.Action == domain.IgnoreProjectPath {
			a.refreshOpenFileViews(repo.Root, ".gitignore")
		}
	})
}

func (a *Workspace) updateProjectFileTabs(r *repoView, root string, op domain.ProjectFileOperation) {
	update := func(entry *tab) bool {
		if !projectPathContains(op.Source.Path, entry.path) {
			return true
		}
		if entry.source != "project" && entry.source != "working" && entry.source != "staged" {
			return true
		}
		if op.Action == domain.TrashProjectPath || entry.source == "working" || entry.source == "staged" {
			return false
		}
		entry.path = op.Destination + strings.TrimPrefix(entry.path, op.Source.Path)
		entry.file.Path = entry.path
		entry.title = path.Base(entry.path)
		if entry.code != nil {
			entry.id = "code:" + entry.source + ":" + entry.path
			applyEditorFileType(entry)
		} else {
			entry.id = "diff:" + entry.source + ":" + entry.path
		}
		return true
	}
	active := ""
	if len(r.tabs) > 0 && r.active >= 0 && r.active < len(r.tabs) {
		active = r.tabs[r.active].id
	}
	kept := r.tabs[:0]
	for _, entry := range r.tabs {
		oldID := entry.id
		if update(&entry) {
			kept = append(kept, entry)
			if oldID == active {
				active = entry.id
			}
		}
	}
	r.tabs = kept
	r.active = max(0, min(r.active, len(r.tabs)-1))
	for i, entry := range r.tabs {
		if entry.id == active {
			r.active = i
		}
	}
	if r.split != nil && !update(r.split) {
		r.split = nil
	}
	if r.preview != nil && !update(r.preview) {
		r.preview = nil
	}
	closed := r.closed[:0]
	for _, entry := range r.closed {
		if update(&entry) {
			closed = append(closed, entry)
		}
	}
	r.closed = closed
	for _, entry := range r.tabs {
		if op.Action == domain.MoveProjectPath && entry.source == "project" && projectPathContains(op.Destination, entry.path) {
			a.refreshOpenFileViews(root, entry.path)
		}
	}
}

func (a *Workspace) projectFileView(c *ui.Context, p tokens) {
	s := a.projectFile
	width, _ := c.Size()
	title := commandLabel(s.id)
	ui.DialogBase(c, &s.open, func(_ ui.Element, panel ui.Element) {
		panel.Width(min(width-32, 640)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("Project file action dialog").Children(func() {
			heading(c, p, title, "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.root).FillWidth().SingleLine().Ellipsis("…").Tooltip(s.root)
				if s.loading {
					ui.Text(c, "Inspecting project item…")
				}
				if s.op.Source.Path != "" {
					ui.Text(c, "Source: "+s.op.Source.Path).FillWidth()
				}
				if s.op.Action != domain.TrashProjectPath && s.op.Action != domain.IgnoreProjectPath {
					ui.TextInput(c, &s.destination).Label("Project destination path").AutoFocus().FillWidth()
					ui.Text(c, "Path relative to the project root. Existing destinations are kept.").FillWidth().TextColor(p.theme.TextMuted)
				}
				if s.op.Action == domain.TrashProjectPath {
					ui.Checkbox(c, &s.confirmed, "Move this item and its contents to the recycle bin")
				}
				if s.op.Action == domain.IgnoreProjectPath {
					ui.Text(c, "Append a literal path rule to the root .gitignore. Tracked files remain tracked.").FillWidth()
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
				ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() { a.run(closeProjectFile, commandTarget{}) })
					ui.PrimaryButton(c, "Apply").Label("Apply project file action").Disabled(a.projectFileDisabled(applyProjectFile, commandTarget{}) != "").Tooltip(a.projectFileDisabled(applyProjectFile, commandTarget{})).OnClick(func() { a.run(applyProjectFile, commandTarget{}) })
				})
			})
		})
	})
	if !s.open && a.projectFile == s {
		a.closeProjectFileDialog()
	}
}
