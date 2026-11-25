package application

import (
	"strings"
	"testing"

	"vela/internal/domain"
)

func linePatchDoc(t *testing.T, body string) domain.PatchDocument {
	t.Helper()
	d, e := ParseTextPatch("diff --git a/f b/f\nindex aa..bb 100644\n--- a/f\n+++ b/f\n"+body, "f")
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestSelectPatchLinesPreservesOnlyChosenChanges(t *testing.T) {
	d := linePatchDoc(t, "@@ -1,3 +1,3 @@\n context\n-old A\n-old B\n+new A\n+new B\n")
	for _, c := range []struct {
		reverse  bool
		selected []int
		want     string
	}{
		{false, []int{0, 2}, "@@ -1,3 +1,3 @@\n context\n-old A\n+new A\n old B\n"},
		{true, []int{0, 2}, "@@ -1,3 +1,3 @@\n context\n-old A\n+new A\n new B\n"},
		{false, []int{2}, "@@ -1,3 +1,4 @@\n context\n old A\n+new A\n old B\n"},
		{true, []int{2}, "@@ -1,2 +1,3 @@\n context\n+new A\n new B\n"},
	} {
		got, err := SelectPatchLines(d, 0, c.selected, c.reverse)
		if err != nil || got != d.Header+c.want {
			t.Fatalf("reverse=%v selected=%v: %q %v", c.reverse, c.selected, got, err)
		}
	}
	for _, bad := range [][]int{nil, {}, {-1}, {4}, {0, 0}} {
		if _, e := SelectPatchLines(d, 0, bad, false); e == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestSelectPatchLinesCRLFAndNoNewline(t *testing.T) {
	d := linePatchDoc(t, "@@ -4 +4 @@\n-old\r\n+new\r\n")
	got, e := SelectPatchLines(d, 0, []int{0, 1}, false)
	if e != nil || !strings.HasSuffix(got, "-old\r\n+new\r\n") {
		t.Fatal("CRLF changed", e)
	}
	d = linePatchDoc(t, "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n")
	if _, e := SelectPatchLines(d, 0, []int{0, 1}, false); e != nil {
		t.Fatal(e)
	}
	if _, e := SelectPatchLines(d, 0, []int{1}, false); e == nil {
		t.Fatal("accepted impossible unterminated internal context")
	}
	if _, e := SelectPatchLines(d, 0, []int{0}, true); e == nil {
		t.Fatal("accepted impossible reverse unterminated context")
	}
}
