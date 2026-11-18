package application

import (
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestComparisonLogFilterValidation(t *testing.T) {
	for _, filter := range []domain.ComparisonLogFilter{
		{}, {Message: "修复 [literal]", Author: "A.*", Path: "目录/file name.go", Since: "2026-01-01", Until: "2026-01-31"},
		{Path: "-option.go"}, {Path: "literal[1].go"},
	} {
		if err := ValidateComparisonLogFilter(filter); err != nil {
			t.Fatalf("valid filter rejected: %v", err)
		}
	}
	for _, filter := range []domain.ComparisonLogFilter{
		{Message: "bad\x00query"}, {Author: strings.Repeat("x", 513)}, {Since: "2026-02-30"},
		{Message: "one\ntwo"}, {Message: "trailing\n"}, {Author: "a\rb"},
		{Until: "yesterday"}, {Since: "2026-02-01", Until: "2026-01-01"}, {Path: "../outside"}, {Path: "/absolute"},
	} {
		if err := ValidateComparisonLogFilter(filter); err == nil {
			t.Fatalf("invalid filter accepted: %#v", filter)
		}
	}
}
