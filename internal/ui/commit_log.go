package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	loadMoreCommits  commandID = "history.load-more-commits"
	cancelLogLoading commandID = "history.cancel-log-loading"
)

func (a *Workspace) commitLogDisabled(id commandID) string {
	if id == cancelLogLoading {
		if a.logRead == nil {
			return "No commit log query is running"
		}
		return ""
	}
	if a.ReadRepositoryLog == nil || a.Model.Active().Repository.Root == "" {
		return "Open a local repository"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil {
		return "Wait for the repository operation to finish"
	}
	if a.logRead != nil {
		return "History is loading"
	}
	if !a.Model.Active().Repository.LogHasMore {
		return "All history is loaded"
	}
	return ""
}

func (a *Workspace) loadMoreCommits() {
	if a.commitLogDisabled(loadMoreCommits) != "" {
		return
	}
	repo := a.Model.Active().Repository
	offset := len(repo.History)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	a.logRead = request
	a.repo().logError = ""
	a.ReadRepositoryLog(ctx, repo.Root, repo.LogSnapshot, offset, 500, func(page domain.CommitLogPage, err error) {
		if a.logRead != request || a.Model.Active().Repository.Root != repo.Root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&a.logRead)
		s, r := a.Model.Active(), a.repo()
		if err != nil {
			r.logError = "Could not load commits · " + err.Error()
			return
		}
		if len(s.Repository.History) != offset || len(page.Commits) > 500 || len(page.Commits) == 0 && page.HasMore {
			r.logError = "Git returned an invalid commit log page; refresh the Log"
			return
		}
		seen := make(map[string]bool, offset+len(page.Commits))
		for _, commit := range s.Repository.History {
			seen[commit.Hash] = true
		}
		for _, commit := range page.Commits {
			if commit.Hash == "" || seen[commit.Hash] {
				r.logError = "Git returned a repeated commit; refresh the Log"
				return
			}
			seen[commit.Hash] = true
		}
		// Keep snapshots captured by other operations immutable.
		history := make([]domain.Commit, offset+len(page.Commits))
		copy(history, s.Repository.History)
		copy(history[offset:], page.Commits)
		domain.ReconnectCommitLog(history)
		s.Repository.History, s.Repository.LogHasMore = history, page.HasMore
		r.rebuildCommitLog(s.Repository)
	})
}

func (a *Workspace) commitLogFooter(c *ui.Context, p tokens) {
	r, repo := a.repo(), a.Model.Active().Repository
	ui.Column(c).FillWidth().Padding(0, gap).Gap(2).Children(func() {
		if r.logError != "" {
			ui.Text(c, r.logError).FillWidth().SingleLine().Ellipsis("…").Tooltip(r.logError).TextColor(p.theme.Danger)
		}
		ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
			label := fmt.Sprintf("%d commits loaded", len(repo.History))
			if repo.LogHasMore {
				label += " · filters apply to loaded commits"
			}
			ui.Text(c, label).Grow(1).SingleLine().Ellipsis("…").Tooltip(label).TextColor(p.theme.TextMuted)
			if a.logRead != nil {
				ui.Text(c, "Loading commits…").TextColor(p.theme.TextMuted)
				a.button(c, cancelLogLoading, "Cancel")
			} else if repo.LogHasMore {
				label := "Load More"
				if r.logError != "" {
					label = "Retry"
				}
				a.button(c, loadMoreCommits, label)
			}
		})
	})
}
