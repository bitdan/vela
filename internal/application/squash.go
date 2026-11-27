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
