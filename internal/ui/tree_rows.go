package ui

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const folderIndent = 16

type folderKind int

const (
	folderNode folderKind = iota
	repositoryNode
	moduleNode
)

type folderEntry struct {
	name  string
	kind  folderKind
	count int
}

type folderTree struct {
	roots    []string
	children map[string][]string
	folders  map[string]folderEntry
	parent   map[string]string
}

func newFolderTree() folderTree {
	return folderTree{children: make(map[string][]string), folders: make(map[string]folderEntry), parent: make(map[string]string)}
}

func (t *folderTree) add(parent, item string) {
	t.parent[item] = parent
	if parent == "" {
		t.roots = append(t.roots, item)
		return
	}
	t.children[parent] = append(t.children[parent], item)
}

func changeTree(repo domain.Repository, files []domain.File, groupMode commandID) (folderTree, map[string]domain.File) {
	tree := newFolderTree()
	byPath := make(map[string]domain.File, len(files))
	modules := make(map[string]bool)
	if groupMode == groupModule {
		for _, file := range projectFiles(repo) {
			if path.Base(file.Path) == "go.mod" {
				modules[path.Dir(file.Path)] = true
			}
		}
	}
	for _, file := range files {
		parent := tree.folder("", repo.Name, repositoryNode)
		dir := path.Dir(file.Path)
		switch groupMode {
		case groupDirectory:
			if dir != "." {
				for segment := range strings.SplitSeq(dir, "/") {
					parent = tree.folder(parent, segment, folderNode)
				}
			}
		case groupModule:
			for candidate := dir; ; candidate = path.Dir(candidate) {
				if modules[candidate] || candidate == "." {
					name := candidate
					if candidate == "." {
						name = repo.Name
					}
					parent = tree.folder(parent, name, moduleNode)
					break
				}
			}
		}
		tree.add(parent, file.Path)
		byPath[file.Path] = file
	}
	if groupMode == groupDirectory {
		for _, root := range tree.roots {
			tree.compactDirectories(root)
		}
	}
	return tree, byPath
}

func (t *folderTree) compactDirectories(item string) {
	entry, isFolder := t.folders[item]
	if !isFolder {
		return
	}
	for entry.kind == folderNode && len(t.children[item]) == 1 {
		child := t.children[item][0]
		next, ok := t.folders[child]
		if !ok || next.kind != folderNode {
			break
		}
		entry.name += "/" + next.name
		t.children[item] = t.children[child]
		for _, grandchild := range t.children[item] {
			t.parent[grandchild] = item
		}
		delete(t.folders, child)
		delete(t.children, child)
		delete(t.parent, child)
	}
	t.folders[item] = entry
	for _, child := range t.children[item] {
		t.compactDirectories(child)
	}
}

func (t *folderTree) folder(parent, name string, kind folderKind) string {
	key := parent + "\x00" + name
	entry, exists := t.folders[key]
	if !exists {
		entry.name, entry.kind = name, kind
		t.add(parent, key)
	}
	entry.count++
	t.folders[key] = entry
	return key
}

type folderOutline struct {
	ui.OutlineState[string]
	seen          ui.Selection[string]
	defaultClosed bool
}

func (s *folderOutline) resetOpen(expanded bool) {
	s.defaultClosed = !expanded
	s.seen.Clear()
	s.Open.Clear()
}

func (s *folderOutline) prepare(c *ui.Context, p tokens, tree folderTree) {
	for key := range tree.folders {
		if !s.seen.Has(key) {
			s.seen.Add(key)
			if !s.defaultClosed {
				s.Open.Add(key)
			}
		}
	}
	folderPrefix(c, p, &s.OutlineState)
}

func folderPrefix(c *ui.Context, p tokens, state *ui.OutlineState[string]) {
	state.Prefix = func(row ui.OutlineRow) {
		ui.Box(c).Width(gap + float32(row.Depth)*folderIndent).Shrink(0)
		arrow := row.Arrow().Size(folderIndent, rowHeight).Center()
		if row.Branch {
			treeChevron(c, p, arrow, row.Open)
		}
	}
}

func treeChevron(c *ui.Context, p tokens, arrow ui.Element, expanded bool) {
	icon := chevronRightIcon
	if expanded {
		icon = chevronDownIcon
	}
	arrow.Children(func() { ui.Icon(c, icon).Size(12, 12).TextColor(p.theme.TextMuted) })
}

var (
	chevronRightIcon = changeIcon("M6 4l4 4-4 4")
	chevronDownIcon  = changeIcon("M4 6l4 4 4-4")
	moduleFolderIcon = changeIcon("M1.5 12V4a1 1 0 0 1 1-1H6l2 2h5a1 1 0 0 1 1 1v2M1.5 12a1 1 0 0 0 1 1H8")
	moduleBadgeIcon  = changeIcon("M10 9h5v5h-5z")
	documentIcon     = changeIcon("M4 1.5h5l3 3V14H4zM9 1.5v3h3M6 8h4m-4 3h4")
	codeFileIcon     = changeIcon("M5 4 1.5 8 5 12m6-8 3.5 4-3.5 4M9.5 3l-3 10")
	markdownFileIcon = changeIcon("M2 11V5l3 3 3-3v6m4-6v6m-2-2 2 2 2-2")
	ignoredFileIcon  = changeIcon("M14 8A6 6 0 1 1 2 8a6 6 0 0 1 12 0zM4 12l8-8")
)

func folderIcon(c *ui.Context, p tokens, kind folderKind) {
	switch kind {
	case repositoryNode:
		ui.Box(c).Size(12, 12).Radius(2).Background(p.treeRepository).Shrink(0)
	case moduleNode:
		ui.Box(c).Size(15, 15).Shrink(0).Children(func() {
			ui.Icon(c, moduleFolderIcon).Size(15, 15).TextColor(p.theme.TextMuted)
			ui.Icon(c, moduleBadgeIcon).Absolute().Left(0).Top(0).Size(15, 15).TextColor(p.treeModule)
		})
	default:
		ui.Icon(c, activityIcons[openProject]).Size(15, 15).TextColor(p.theme.TextMuted).Shrink(0)
	}
}

func fileIcon(c *ui.Context, p tokens, name, status string) {
	icon, color := documentIcon, p.theme.TextMuted
	switch strings.ToLower(path.Ext(name)) {
	case ".go", ".java", ".ts", ".js", ".vue", ".xml", ".sh", ".ps1":
		icon, color = codeFileIcon, p.treeModule
	case ".md":
		icon, color = markdownFileIcon, p.treeModule
	}
	if status == "I" {
		icon, color = ignoredFileIcon, p.theme.TextMuted
	}
	ui.Icon(c, icon).Size(15, 15).TextColor(color).Shrink(0)
}
