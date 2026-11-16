package domain

import (
	"fmt"
	"time"
)

type File struct {
	Path, Status, Old, New string
	OldPath                string
	DiskVersion            string
}

type Commit struct {
	Hash, Subject, Body, Author, AuthorEmail, Date string
	Refs                                           []string
	Timestamp                                      time.Time
	AuthoredAt                                     time.Time
	Lane                                           int
	Merge                                          bool
	Files                                          []File
	Parents                                        []int
	ParentHashes                                   []string
}

type Shelf struct {
	ID, OID, Base, Index string
	Deleted              bool
	Title                string
	Files                []File
}

type Repository struct {
	Name, CurrentBranch     string
	Root                    string
	Head, IndexVersion      string
	WorktreeVersion         string
	StashVersion            string
	Stashes                 []StashEntry
	GitDir, CommonDir       string
	WorktreesVersion        string
	Worktrees               []Worktree
	Bisect                  BisectState
	HeadMessage             string
	Project                 []File
	ProjectDirectories      []string
	RemoteBranches          map[string]bool
	Refs                    map[string]string
	Upstreams               map[string]Upstream
	RemoteVersion           string
	RemoteManagementVersion string
	MergeHeads              []string
	Rebase                  RebaseState
	Sequencer               SequencerState
	Conflicts               []MergeConflict
	Branches                []string
	Remotes                 []Remote
	Working, Staged         []File
	History                 []Commit
	LogSnapshot             CommitLogSnapshot
	LogHasMore              bool
	Shelf                   []Shelf
}

func (r Repository) ResolvingConflicts() bool {
	return len(r.MergeHeads) != 0 || r.Rebase.Active() || r.Sequencer.Active() || len(r.Conflicts) != 0
}

func MockRepositories() []Repository {
	main := File{Path: "cmd/vela/main.go", Status: "M", Old: "package main\n\nfunc main() {\n\tprintln(\"Starting application\")\n}\n", New: "package main\n\nimport \"vela/internal/bootstrap\"\n\nfunc main() {\n\tbootstrap.Run()\n}\n"}
	notes := File{Path: "README.md", Status: "M", Old: "# vela\n\nGit client.\n", New: "# vela\n\nA native Git workspace.\n"}
	mod := File{Path: "go.mod", Status: "M", Old: "module vela\n\ngo 1.27.3\n", New: "module vela\n\ngo 1.27.3\n\nrequire github.com/egoist/mygo v0.8.3\n"}
	theme := File{Path: "internal/ui/theme.go", Status: "A", New: "package ui\n\n// Colors are shared by all workspace panels.\n"}
	subjects := []string{
		"feat(ui): wire up the native workspace", "test(ui): cover keyboard focus and panel toggles",
		"Merge branch 'feature/native-ui' into main", "feat(editor): preserve the active diff tab",
		"refactor(ui): share workspace design tokens", "docs: record MyGo technical validation",
		"feat(log): virtualize commit and change lists", "chore: pin MyGo 0.8.3",
		"feat(layout): restore panel sizes", "docs: define IDEA-style workspace",
		"test(editor): verify independent cursor and scroll", "chore: add initial project assets",
	}
	authors := []string{"Lin", "Alex", "Morgan"}
	authorEmails := []string{"lin@example.test", "alex@example.test", "morgan@example.test"}
	latest := time.Date(2026, time.November, 10, 16, 0, 0, 0, time.UTC)
	history := make([]Commit, 10000)
	for i := range history {
		files := []File{main, mod}
		if i%2 == 1 {
			files = []File{notes, theme}
		}
		// The main line skips two commits at each merge. Those commits
		// form a side branch that forks from the same main-line parent.
		var parents []int
		if i+1 < len(history) {
			parents = append(parents, i+1)
		}
		lane := 0
		if i%6 == 2 && i+3 < len(history) {
			parents = []int{i + 3, i + 1}
		}
		if i%6 == 3 || i%6 == 4 {
			lane = 1
		}
		date := latest.Add(-time.Duration(i) * 4 * time.Hour)
		history[i] = Commit{Hash: fmt.Sprintf("%07x", 0x9af281d-i), Subject: subjects[i%len(subjects)],
			Author: authors[i%len(authors)], AuthorEmail: authorEmails[i%len(authorEmails)], Date: date.Format("Jan 2"), Timestamp: date,
			Lane: lane, Merge: len(parents) > 1, Files: files, Parents: parents}
	}
	history[0].Refs = []string{"main", "origin/stable"}
	history[2].Refs = []string{"origin/main"}
	history[3].Refs = []string{"feature/native-ui"}
	history[4].Refs = []string{"origin/feature/native-ui"}
	history[6].Refs = []string{"feature/editor-tabs"}
	history[11].Refs = []string{"refs/tags/v0.1.0"}
	history[0].Body = "- Build the native workspace\n- Share editor and diff tabs\n- Keep pane selections independent"
	history[0].AuthoredAt = latest.Add(-15 * time.Minute)
	history[1].Body = "- Verify keyboard navigation in the project tree\n- Cover panel toggles and focus restoration"
	branchVersion := main
	branchVersion.New = "package main\n\nfunc main() {\n\tprintln(\"Native UI preview\")\n}\n"
	history[3].Files = []File{branchVersion, notes}
	return []Repository{
		{Name: "vela", CurrentBranch: "main", Branches: []string{"main", "feature/native-ui", "feature/editor-tabs", "origin/main", "origin/feature/native-ui", "origin/stable", "refs/tags/v0.1.0"},
			Remotes: []Remote{{"origin", "https://github.com/egoist/mygo.git"}, {"gitee", "https://gitee.com/openharmony/docs.git"}, {"gitlab", "git@gitlab.com:gitlab-org/gitlab.git"}},
			Working: []File{main, theme, mod}, Staged: []File{notes}, History: history,
			Shelf: []Shelf{{Title: "UI baseline changes", Files: []File{{Path: "docs/ui-notes.md", Status: "A", New: "# UI baseline\n\nMock layout notes.\n"}}}}},
		{Name: "sandbox", CurrentBranch: "trunk", Branches: []string{"trunk", "demo/empty", "origin/trunk"},
			Remotes: []Remote{{"origin", "https://github.com/egoist/mygo.git"}},
			History: []Commit{{Hash: "ab1500f", Subject: "Initialize sandbox", Author: "Lin", AuthorEmail: "lin@example.test", Date: "Nov 10", Timestamp: latest, Refs: []string{"trunk"}, Files: []File{notes}}}},
	}
}
