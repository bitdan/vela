package application

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

type RepositorySquashReader interface {
	PreviewSquash(context.Context, domain.Repository, []string) (domain.SquashPreview, error)
}

func ValidateSquashSelection(ids []string) error {
	return ValidateCommitRangeSelection(ids, 2)
}

func ValidateCommitRangeSelection(ids []string, minimum int) error {
	if len(ids) < minimum || len(ids) > 1000 {
		return errors.New("select the required number of consecutive private commits (at most 1000)")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if (len(id) != 40 && len(id) != 64) || seen[id] || strings.ToLower(id) != id {
			return errors.New("select unique full commit IDs")
		}
		if _, err := hex.DecodeString(id); err != nil {
			return errors.New("invalid commit ID")
		}
		seen[id] = true
	}
	return nil
}

func ValidateSquashMessage(message string) error {
	if strings.TrimSpace(message) == "" || len(message) > 64<<10 || !utf8.ValidString(message) || strings.ContainsRune(message, 0) {
		return errors.New("enter a nonempty UTF-8 commit message of at most 64 KiB without NUL")
	}
	return nil
}

func ValidateInteractiveRebasePlan(preview domain.CommitRewritePreview, steps []domain.InteractiveRebaseStep) error {
	if len(preview.RebaseCommits) != len(preview.Selected) || len(steps) != len(preview.Selected) || len(steps) == 0 {
		return errors.New("preview the complete interactive Rebase commit range")
	}
	original := make(map[string]bool, len(preview.RebaseCommits))
	for _, commit := range preview.RebaseCommits {
		original[commit.Commit] = true
	}
	kept := 0
	for index, step := range steps {
		found := original[step.Commit]
		if !found {
			return errors.New("interactive Rebase plan contains an unknown or repeated commit")
		}
		delete(original, step.Commit)
		switch step.Action {
		case domain.RebasePick, domain.RebaseSquash, domain.RebaseFixup:
			if step.Message != "" {
				return errors.New("only Reword steps may carry a replacement message")
			}
		case domain.RebaseReword:
			if err := ValidateSquashMessage(step.Message); err != nil {
				return err
			}
		case domain.RebaseDrop:
			if step.Message != "" {
				return errors.New("Drop steps cannot carry a replacement message")
			}
		default:
			return errors.New("unknown interactive Rebase action")
		}
		if index == 0 && (step.Action == domain.RebaseSquash || step.Action == domain.RebaseFixup) {
			return errors.New("the first commit cannot be Squash or Fixup")
		}
		if step.Action != domain.RebaseDrop {
			kept++
		}
	}
	if len(original) != 0 {
		return errors.New("interactive Rebase plan omits a selected commit")
	}
	if kept == 0 && len(preview.Replay) == 0 && preview.Parent == "" {
		return errors.New("keep at least one commit; interactive Rebase cannot delete the entire branch")
	}
	return nil
}
