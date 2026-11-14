package application

import (
	"strings"
	"testing"
)

func TestParseTextPatchPreservesGitHunks(t *testing.T) {
	header := "diff --git a/file.txt b/file.txt\nindex 123..456 100644\n--- a/file.txt\n+++ b/file.txt\n"
	first := "@@ -1,2 +1,2 @@\n-old\n+new\n context\n"
	second := "@@ -10 +10 @@\n-before\n\\ No newline at end of file\n+after\n\\ No newline at end of file\n"
	doc, err := ParseTextPatch(header+first+second, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Header != header || len(doc.Hunks) != 2 || doc.Hunks[0].Text != first || doc.Hunks[1].Text != second || doc.Hunks[1].OldStart != 10 || doc.Hunks[1].NewCount != 1 {
		t.Fatalf("patch changed %+v", doc)
	}
	if !strings.Contains(doc.Header+doc.Hunks[1].Text, "No newline") {
		t.Fatal("newline marker lost")
	}
}

func TestParseTextPatchRejectsUnsupportedOrMalformed(t *testing.T) {
	for _, raw := range []string{
		"GIT binary patch\n",
		"diff --git a/a b/a\nold mode 100644\nnew mode 100755\n",
		"diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,2 +1 @@\n-x\n+y\n",
		"diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ invalid @@\n-x\n+y\n",
	} {
		if _, err := ParseTextPatch(raw, "a"); err == nil {
			t.Fatalf("invalid patch accepted %q", raw)
		}
	}
}

func TestParseTextPatchPreservesQuotedSpecialPaths(t *testing.T) {
	header := "diff --git \"a/文件\\tname\\n.txt\" \"b/文件\\tname\\n.txt\"\nindex 123..456 100644\n--- \"a/文件\\tname\\n.txt\"\n+++ \"b/文件\\tname\\n.txt\"\n"
	doc, err := ParseTextPatch(header+"@@ -1 +1 @@\n-before\n+after\n", "文件\tname\n.txt")
	if err != nil || doc.Header != header {
		t.Fatalf("quoted special path changed: %+v %v", doc, err)
	}
}
