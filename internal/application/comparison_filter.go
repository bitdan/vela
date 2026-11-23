package application

import (
	"errors"
	"fmt"
	"regexp"
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
		return errors.New("message and author filters must be single-line text")
	}
	if filter.Hash != "" && (len(filter.Hash) < 4 || len(filter.Hash) > 64 || strings.Trim(strings.ToLower(filter.Hash), "0123456789abcdef") != "") {
		return errors.New("hash must be a hexadecimal prefix of 4 to 64 characters")
	}
	if filter.Regex {
		for _, value := range []string{filter.Message, filter.Author} {
			if _, err := regexp.Compile(value); err != nil {
				return fmt.Errorf("invalid regular expression: %w", err)
			}
		}
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

// ComparisonTextMatcher validates and compiles once per query. Using Go's
// bounded-time regexp engine gives the UI and adapter the same regex syntax.
func ComparisonTextMatcher(filter domain.ComparisonLogFilter) (func(domain.Commit) bool, error) {
	if err := ValidateComparisonLogFilter(filter); err != nil {
		return nil, err
	}
	pattern := func(value string) *regexp.Regexp {
		if !filter.Regex {
			value = regexp.QuoteMeta(value)
		}
		if !filter.CaseSensitive {
			value = "(?i)" + value
		}
		return regexp.MustCompile("(?m)" + value)
	}
	message, author := pattern(filter.Message), pattern(filter.Author)
	return func(c domain.Commit) bool {
		return strings.HasPrefix(c.Hash, strings.ToLower(filter.Hash)) && message.MatchString(c.Subject+"\n"+c.Body) && author.MatchString(c.Author+" <"+c.AuthorEmail+">")
	}, nil
}
