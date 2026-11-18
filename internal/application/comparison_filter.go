package application

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"vela/internal/domain"
)

// ValidateComparisonLogFilter applies the same bounds at UI and adapter entry.
func ValidateComparisonLogFilter(filter domain.ComparisonLogFilter) error {
	for _, value := range []string{filter.Message, filter.Author, filter.Since, filter.Until} {
		if len(value) > 512 || strings.ContainsRune(value, 0) || !utf8.ValidString(value) {
			return errors.New("filter text must be valid UTF-8, at most 512 bytes, without NUL")
		}
	}
	if strings.ContainsAny(filter.Message, "\r\n") || strings.ContainsAny(filter.Author, "\r\n") {
		return errors.New("message and author filters must be single-line literal text")
	}
	for _, value := range []string{filter.Since, filter.Until} {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return errors.New("use dates in YYYY-MM-DD format (UTC)")
			}
		}
	}
	if filter.Since != "" && filter.Until != "" && filter.Since > filter.Until {
		return errors.New("start date must not follow end date")
	}
	if len(filter.Path) > 4096 || !utf8.ValidString(filter.Path) || strings.ContainsRune(filter.Path, 0) || filter.Path != "" && !ValidProjectSearchPath(filter.Path) {
		return errors.New("choose a literal repository-relative file or directory")
	}
	return nil
}
