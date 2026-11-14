package ui

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

type refreshViewIdentity struct {
	code, old, next *editor.Buffer
	diskVersion     string
}

func (a *Workspace) stopSourceRefreshes() {
	for _, request := range a.sourceRefreshes {
		request.cancel()
	}
	a.sourceRefreshes = nil
}

// refreshOpenFileViews updates live source/index/worktree views after a file
// write, preserving view identities, focus and any edits made while it reads.
func (a *Workspace) refreshOpenFileViews(root, path string) {
	if a.ReadRepositoryFile == nil {
		return
	}
	index := -1
	for i, s := range a.Model.Sessions {
		if s.Repository.Root == root {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	r := &a.repos[index]
	identities := map[string]map[string][]refreshViewIdentity{}
	queryFiles := map[string]domain.File{}
	collect := func(entry *tab) {
		if entry == nil || entry.path != path || (entry.source != "project" && entry.source != "working" && entry.source != "staged") {
			return
		}
		identity := refreshViewIdentity{diskVersion: entry.file.DiskVersion}
		if entry.code != nil {
			if entry.source != "project" || entry.code.Buffer.Modified() {
				return
			}
			identity.code = entry.code.Buffer
		} else if entry.diff != nil {
			identity.old, identity.next = entry.diff.Old, entry.diff.New
		} else {
			return
		}
		if identities[entry.source] == nil {
			identities[entry.source] = map[string][]refreshViewIdentity{}
		}
		identities[entry.source][entry.id] = append(identities[entry.source][entry.id], identity)
		queryFiles[entry.source] = entry.file
	}
	for i := range r.tabs {
		collect(&r.tabs[i])
	}
	collect(r.split)
	collect(r.preview)
	for source, expected := range identities {
		if a.sourceRefreshes == nil {
			a.sourceRefreshes = map[string]*readRequest{}
		}
		key := root + "\x00" + source + "\x00" + path
		if previous := a.sourceRefreshes[key]; previous != nil {
			previous.cancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		request := &readRequest{cancel: cancel}
		a.sourceRefreshes[key] = request
		queryFile := queryFiles[source]
		if current, ok := findFile(a.Model.Sessions[index].Repository, source, path); ok {
			queryFile = current
		}
		queryFile.Old, queryFile.New, queryFile.DiskVersion = "", "", ""
		a.ReadRepositoryFile(ctx, root, source, queryFile, func(file domain.File, err error) {
			if a.sourceRefreshes[key] != request || ctx.Err() == context.Canceled {
				return
			}
			delete(a.sourceRefreshes, key)
			cancel()
			if err != nil || file.Path != path {
				if err != nil {
					message := "Could not refresh open file · " + path + " · " + err.Error()
					for i, s := range a.Model.Sessions {
						if s.Repository.Root == root {
							a.repos[i].addConsole(message)
							break
						}
					}
					if a.Model.Active().Repository.Root == root {
						a.status = message
					}
				}
				return
			}
			for i, s := range a.Model.Sessions {
				if s.Repository.Root != root {
					continue
				}
				view := &a.repos[i]
				update := func(entry *tab) {
					if entry == nil || entry.source != source || entry.path != path {
						return
					}
					candidates := expected[entry.id]
					if len(candidates) == 0 {
						return
					}
					if entry.code != nil {
						buffer := entry.code.Buffer
						matched := false
						for _, identity := range candidates {
							if buffer == identity.code && entry.file.DiskVersion == identity.diskVersion {
								matched = true
								break
							}
						}
						if !matched || buffer.Modified() {
							return
						}
						cursor := entry.code.Cursor()
						if bufferDiskText(buffer) != file.New {
							buffer.Reset(file.New)
						}
						cursor = min(cursor, buffer.Len())
						entry.code.Select(cursor, cursor)
					} else if entry.diff != nil {
						matched := false
						for _, identity := range candidates {
							if entry.diff.Old == identity.old && entry.diff.New == identity.next {
								matched = true
								break
							}
						}
						if !matched {
							return
						}
						entry.diff.Old.Reset(strings.ReplaceAll(file.Old, "\r\n", "\n"))
						entry.diff.New.Reset(strings.ReplaceAll(file.New, "\r\n", "\n"))
					} else {
						return
					}
					entry.file = file
				}
				for j := range view.tabs {
					update(&view.tabs[j])
				}
				update(view.split)
				update(view.preview)
				break
			}
		})
	}
}
