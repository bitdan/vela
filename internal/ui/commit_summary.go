package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type commitBranchDetails struct {
	index    int
	names    []string
	expanded bool
}

// containingBranches walks towards branch tips from the selected commit.
// Only references reached through the parent graph can contain the commit.
func containingBranches(repo domain.Repository, children [][]int, index int) []string {
	visited := make([]bool, len(children))
	visited[index] = true
	stack := []int{index}
	found := make(map[string]bool)
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, ref := range repo.History[i].Refs {
			found[ref] = true
		}
		for _, child := range children[i] {
			if !visited[child] {
				visited[child] = true
				stack = append(stack, child)
			}
		}
	}
	var names []string
	if found[repo.CurrentBranch] {
		names = append(names, "HEAD")
	}
	for _, name := range repo.Branches {
		if found[name] && branchGroup(name) != "Tags" {
			names = append(names, name)
		}
	}
	return names
}

func (a *Workspace) commitSummary(c *ui.Context, p tokens, repo domain.Repository, index int) {
	r := a.repo()
	entry := repo.History[index]
	branches := &r.detailsBranches
	if branches.index != index {
		*branches = commitBranchDetails{index: index, names: containingBranches(repo, r.children, index)}
	}
	ui.Scroll(c.Key(entry.Hash)).Fill().Label("Commit Details").Background(p.panel).Children(func() {
		ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
			ui.Column(c).FillWidth().Gap(inset).Selectable().Children(func() {
				ui.Text(c, entry.Subject).FillWidth().Font("monospace").Bold()
				if entry.Body != "" {
					ui.Text(c, entry.Body).FillWidth().Font("monospace")
				}
			})
			commitMetadata(c, p, entry, func() { a.run(copyHash, commandTarget{source: entry.Hash}) })
			commitReferenceLabels(c, p, entry.Refs, repo.CurrentBranch)
			if len(branches.names) > 0 {
				commitBranchList(c, p, branches)
			}
		})
	})
}

func commitMetadata(c *ui.Context, p tokens, entry domain.Commit, copyRevision func()) {
	ui.Row(c).FillWidth().Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
		folderIcon(c, p, repositoryNode)
		ui.ButtonBase(c.Key("revision")).Height(rowHeight - 4).Center().Label("Copy commit revision").
			Tooltip("Copy revision " + entry.Hash).Children(func() {
			ui.Text(c, entry.Hash[:min(len(entry.Hash), 7)]).FontSize(p.textSize(smallText))
		}).OnClick(copyRevision)
		if entry.Author != "" {
			ui.Text(c, entry.Author).FontSize(p.textSize(smallText))
		}
		if entry.AuthorEmail != "" {
			ui.Text(c, "<"+entry.AuthorEmail+">").FontSize(p.textSize(smallText)).TextColor(p.treeModule).Selectable()
		}
		date := entry.Date
		if !entry.Timestamp.IsZero() {
			date = entry.Timestamp.Format("2006/01/02 at 15:04")
		}
		if date != "" {
			ui.Text(c, "on "+date).FontSize(p.textSize(smallText))
		}
	})
}

func commitReferenceLabels(c *ui.Context, p tokens, refs []string, currentBranch string) {
	if len(refs) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(gap).AlignItems(ui.Center).Children(func() {
		label := func(name string, color ui.Color) {
			ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, tagIcon).Size(13, 13).TextColor(color).Shrink(0)
				ui.Text(c, name).Label("Commit reference " + name).FontSize(p.textSize(smallText)).SingleLine().Ellipsis("…")
			})
		}
		if slices.Contains(refs, currentBranch) {
			label("HEAD", p.favorite)
		}
		for _, ref := range refs {
			color := p.graph[2]
			switch branchGroup(ref) {
			case "Remote":
				color = p.graph[1]
			case "Tags":
				color = p.treeModule
			}
			label(displayBranch(ref), color)
		}
	})
}

func commitBranchList(c *ui.Context, p tokens, branches *commitBranchDetails) {
	ui.Row(c).FillWidth().Gap(3).AlignItems(ui.Center).Children(func() {
		count := "In 1 branch"
		if len(branches.names) > 1 {
			count = fmt.Sprintf("In %d branches", len(branches.names))
		}
		ui.Text(c, count+":").FontSize(p.textSize(smallText))
		verb := "Show"
		if branches.expanded {
			verb = "Hide"
		}
		ui.ButtonBase(c.Key("containing-branches")).Height(rowHeight - 4).Center().
			Label(verb + " containing branches").Role(ui.RoleDisclosure).Expanded(branches.expanded).
			Children(func() {
				ui.Text(c, verb).FontSize(p.textSize(smallText)).TextColor(p.treeModule)
			}).OnClick(func() { branches.expanded = !branches.expanded })
	})
	if branches.expanded {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for _, name := range branches.names {
				ui.Text(c, name).Label("Containing branch " + name).FontSize(p.textSize(smallText)).FillWidth()
			}
		})
	}
}
