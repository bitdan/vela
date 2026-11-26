package application

import (
	"strings"
	"testing"
	"vela/internal/domain"
)

func TestLineHistoryValidationAndNativeHunkProjection(t *testing.T) {
	q := domain.LineHistoryQuery{Revision: strings.Repeat("a", 40), Path: "中文 file.txt", Text: "one\nsecond\n", DiskVersion: "disk", Start: 2, End: 2, Limit: 100}
	if err := ValidateLineHistoryQuery(q); err != nil {
		t.Fatal(err)
	}
	bad := q
	bad.End = 3
	if ValidateLineHistoryQuery(bad) == nil {
		t.Fatal("EOF virtual line accepted")
	}
	for _, raw := range []string{"diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -2,1 +2,1 @@\n-old\r\n+new\r\n", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -2 +2 @@\n-old\n+new\n\\ No newline at end of file\n"} {
		hunks, err := ProjectLineHistoryPatch(raw)
		if err != nil || len(hunks) != 1 || hunks[0].OldStart != 2 {
			t.Fatal(hunks, err)
		}
		if strings.Contains(raw, "\r") {
			if hunks[0].Before != "old\r\n" || hunks[0].After != "new\r\n" {
				t.Fatal("CRLF lost")
			}
		} else if hunks[0].After != "new" {
			t.Fatal("EOF marker lost")
		}
	}
	if _, err := ProjectLineHistoryPatch("diff --git a/f b/f\n@@ -1 +1 @@\n-old\n"); err == nil {
		t.Fatal("invalid native hunk counts accepted")
	}
}
