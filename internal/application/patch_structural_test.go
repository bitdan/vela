package application

import (
	"strings"
	"testing"
)

func TestImportedStructuralTextPatches(t *testing.T) {
	for _, tc := range []struct{ name, text, path string }{
		{"add", "diff --git a/new.txt b/new.txt\nnew file mode 100644\nindex 0000000..1234567\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+new\n", "new.txt"},
		{"delete", "diff --git a/old.txt b/old.txt\ndeleted file mode 100644\nindex 1234567..0000000\n--- a/old.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n", "old.txt"},
		{"rename", "diff --git a/old name.txt b/new name.txt\nsimilarity index 100%\nrename from old name.txt\nrename to new name.txt\n", "new name.txt"},
		{"empty add", "diff --git a/empty.txt b/empty.txt\nnew file mode 100644\nindex 0000000..e69de29\n", "empty.txt"},
		{"rename with edit", "diff --git a/old.txt b/new.txt\nsimilarity index 50%\nrename from old.txt\nrename to new.txt\nindex 1234567..7654321 100644\n--- a/old.txt\n+++ b/new.txt\n@@ -1,2 +1,2 @@\n same\n-old\n+new\n", "new.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ParseImportedPatch(tc.text)
			if err != nil || len(doc.Files) != 1 || doc.Files[0].Path != tc.path {
				t.Fatalf("parse: %+v %v", doc, err)
			}
			selected, err := SelectImportedPatch(tc.text, map[string][]int{tc.path: {0}})
			if err != nil || selected.Text != tc.text {
				t.Fatalf("whole-file selection changed structural patch: %+v %v", selected, err)
			}
		})
	}
}

func TestImportedModificationSelectionAndRenameRelocation(t *testing.T) {
	header := "diff --git a/file.txt b/file.txt\nindex 123..456 100644\n--- a/file.txt\n+++ b/file.txt\n"
	first := "@@ -1 +1 @@\n-old\n+new\n"
	last := "@@ -20 +20 @@\n-before\n+after\n"
	selected, err := SelectImportedPatch(header+first+last, map[string][]int{"file.txt": {1}})
	if err != nil || selected.Text != header+last || selected.Hunks != 1 {
		t.Fatalf("ordinary hunk selection regressed: %+v %v", selected, err)
	}
	rename := "diff --git a/src/old.txt b/src/new.txt\nsimilarity index 100%\nrename from src/old.txt\nrename to src/new.txt\n"
	doc, err := RelocateImportedPatch(rename, 2, "target")
	if err != nil || len(doc.Paths) != 2 || doc.Paths[0] != "target/new.txt" || doc.Paths[1] != "target/old.txt" || doc.Text != rename {
		t.Fatalf("rename mapping: %+v %v", doc, err)
	}
	if _, err := SelectImportedPatch(rename, map[string][]int{"src/new.txt": {1}}); err == nil {
		t.Fatal("partial structural selection accepted")
	}
	if _, err := ParseImportedPatch(rename + strings.ReplaceAll(rename, "src/old.txt", "src/other.txt")); err == nil {
		t.Fatal("overlapping rename destination accepted")
	}
}

func TestImportedStructuralPathsAndHeadersMustAgree(t *testing.T) {
	for _, text := range []string{
		"diff --git a/a b/b\nnew file mode 100644\nindex 000..123\n--- /dev/null\n+++ b/b\n@@ -0,0 +1 @@\n+x\n",
		"diff --git a/a b/b\nsimilarity index 100%\nrename from a\nrename to ../../outside\n",
		"diff --git a/a b/b\nsimilarity index 100%\nrename from a\nrename to b\nold mode 100644\nnew mode 100755\n",
	} {
		if _, err := ParseImportedPatch(text); err == nil {
			t.Fatalf("unsafe structural headers accepted: %s", text)
		}
	}
}
