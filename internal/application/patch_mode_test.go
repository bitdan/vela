package application

import "testing"

func TestImportedRegularModeChangesAreWholeFile(t *testing.T) {
	for _, text := range []string{
		"diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\n",
		"diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\nindex 1234567..7654321\n--- a/script.sh\n+++ b/script.sh\n@@ -1 +1 @@\n-old\n+new\n",
	} {
		doc, err := ParseImportedPatch(text)
		if err != nil || len(doc.Files) != 1 || ImportedPatchUnits(doc.Files[0]) != 1 {
			t.Fatalf("mode patch: %+v %v", doc, err)
		}
		selection, err := SelectImportedPatch(text, map[string][]int{"script.sh": {0}})
		if err != nil || selection.Text != text {
			t.Fatalf("whole-file mode selection: %+v %v", selection, err)
		}
	}
}

func TestImportedModesRejectLinksAndSubmodules(t *testing.T) {
	for _, mode := range []string{"120000", "160000", "100600", "000000"} {
		text := "diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode " + mode + "\n"
		if _, err := ParseImportedPatch(text); err == nil {
			t.Fatalf("unsafe mode accepted: %s", mode)
		}
	}
}
