package application

import (
	"strings"
	"testing"
)

func TestImportedPatchCRLFPreservesExactBody(t *testing.T) {
	header := "diff --git a/file.txt b/file.txt\nindex a..b 100644\n--- a/file.txt\n+++ b/file.txt\n"
	body := "@@ -1,2 +1,2 @@\n context\r\n-old\r\n+new\r\n"
	raw := header + body
	doc, err := ParseImportedPatch(raw)
	if err != nil || len(doc.Files) != 1 || doc.Files[0].Hunks[0].Text != body || doc.Text != raw {
		t.Fatalf("CRLF bytes rejected or normalized: %+v %v", doc, err)
	}
	selected, err := SelectImportedPatch(raw, map[string][]int{"file.txt": {0}})
	if err != nil || selected.Text != raw {
		t.Fatal("selection altered CRLF patch", err)
	}
	for _, bad := range []string{strings.ReplaceAll(raw, "\n", "\r\n"), header + strings.Replace(body, "+new\r\n", "+new\n", 1), header + strings.Replace(body, "+new\r\n", "+new\rmiddle\r\n", 1), strings.Replace(header, "--- a/file.txt\n", "--- a/file.txt\r\n", 1) + body} {
		if _, err := ParseImportedPatch(bad); err == nil {
			t.Fatalf("accepted envelope/bare/mixed CR: %q", bad)
		}
	}
	// An unterminated last line is not an LF line in a CRLF file.
	noNewline := header + "@@ -1,2 +1,2 @@\n context\r\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	if _, err := ParseImportedPatch(noNewline); err != nil {
		t.Fatal("rejected CRLF with unterminated final line", err)
	}
}
