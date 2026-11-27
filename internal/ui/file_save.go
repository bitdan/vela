package ui

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

const saveSourceFile commandID = "editor.save-source-file"

func (a *Workspace) hasUnsavedRealFiles() bool {
	for i, s := range a.Model.Sessions {
		if s.Repository.Root != "" {
			if a.repos[i].split != nil && a.repos[i].split.modified() {
				return true
			}
			for _, t := range a.repos[i].tabs {
				if t.modified() {
					return true
				}
			}
		}
	}
	return false
}

func (a *Workspace) sourceReadOnly(source string, file domain.File) bool {
	if strings.HasPrefix(source, "revision:") || strings.HasPrefix(source, "blame:") {
		return true
	}
	if a.Model.Active().Repository.Root == "" {
		return false
	}
	return source != "project" || file.DiskVersion == "" || a.SaveRepositoryFile == nil
}

func (a *Workspace) sourceSaveEntry(target commandTarget) *tab {
	r := a.repo()
	if target.tabID != "" {
		if i := r.tabIndex(target.tabID); i >= 0 {
			return &r.tabs[i]
		}
		return nil
	}
	if r.split != nil && r.split.code != nil && r.split.code == a.focusedEditor {
		return r.split
	}
	if r.preview != nil || len(r.tabs) == 0 {
		return nil
	}
	return &r.tabs[r.active]
}

func (a *Workspace) sourceSaveDisabled(target commandTarget) string {
	entry := a.sourceSaveEntry(target)
	if a.SaveRepositoryFile == nil || entry == nil || entry.code == nil || entry.code.ReadOnly || entry.source != "project" || entry.file.DiskVersion == "" {
		return "Select an editable project file"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil {
		return "Wait for the current repository task"
	}
	if !entry.code.Buffer.Modified() {
		return "No unsaved changes"
	}
	return ""
}

func bufferDiskText(buffer *editor.Buffer) string {
	text := buffer.String()
	if buffer.LineEnding() == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func (a *Workspace) saveSource(target commandTarget, after ...func(error)) {
	entry := a.sourceSaveEntry(target)
	repo := a.Model.Active().Repository
	file := entry.file
	buffer := entry.code.Buffer
	text := bufferDiskText(buffer)
	ctx, cancel := context.WithCancel(context.Background())
	a.beginRepositoryWrite(repo.Root, "Saving "+file.Path+"…", cancel)
	a.SaveRepositoryFile(ctx, repo, file, text, func(updated domain.Repository, saved domain.File, err, refreshErr error) {
		message := "Saved " + file.Path
		if err != nil {
			message = "Could not save " + file.Path + " · " + err.Error()
		}
		a.finishRepositoryWrite(repo.Root, updated, refreshErr, message)
		if err != nil {
			for _, done := range after {
				done(err)
			}
			return
		}
		for i, s := range a.Model.Sessions {
			if s.Repository.Root != repo.Root {
				continue
			}
			r := &a.repos[i]
			for j := range r.tabs {
				if t := &r.tabs[j]; t.code != nil && t.code.Buffer == buffer {
					t.file = saved
					if t.document != nil {
						t.document.draftBase = nil
					}
				}
			}
			if r.split != nil && r.split.code != nil && r.split.code.Buffer == buffer {
				r.split.file = saved
				if r.split.document != nil {
					r.split.document.draftBase = nil
				}
			}
		}
		if bufferDiskText(buffer) == text {
			buffer.MarkSaved()
		}
		if refreshErr == nil {
			a.refreshOpenFileViews(repo.Root, file.Path)
		}
		for _, done := range after {
			done(nil)
		}
	})
}
