package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
	"vela/internal/domain"
)

func structuralPatchFixture(t *testing.T, kind string) (*workflowFixture, string, string, string) {
	t.Helper()
	f := newWorkflow(t)
	oldName, newName := "文件 name.txt", "new 文件 name.txt"
	content := strings.Repeat("unchanged line\n", 20) + "old\n"
	f.write(oldName, content)
	f.git("commit", "-am", "content")
	switch kind {
	case "add":
		f.write(newName, "new\n")
		f.git("add", "--", newName)
	case "empty add":
		f.write(newName, "")
		f.git("add", "--", newName)
	case "delete":
		f.git("rm", "--", oldName)
	case "rename", "rename edit":
		f.git("mv", "--", oldName, newName)
		if kind == "rename edit" {
			f.write(newName, strings.Replace(content, "old\n", "new\n", 1))
			f.git("add", "--", newName)
		}
	}
	text := f.git("diff", "--cached", "--no-ext-diff", "--no-textconv", "--binary", "--find-renames") + "\n"
	f.git("reset", "--mixed", "HEAD")
	if kind != "delete" {
		if err := os.Remove(filepath.Join(f.root, newName)); err != nil {
			t.Fatal(err)
		}
	}
	f.write(oldName, content)
	return f, text, oldName, newName
}

func TestImportedModificationKeepsPartialHunkSelection(t *testing.T) {
	for _, target := range []string{"Working tree", "Index only"} {
		t.Run(target, func(t *testing.T) {
			f := newWorkflow(t)
			name := "文件 name.txt"
			var lines []string
			for i := 0; i < 30; i++ {
				lines = append(lines, fmt.Sprintf("line %02d", i))
			}
			base := strings.Join(lines, "\n") + "\n"
			f.write(name, base)
			f.git("commit", "-am", "two hunk fixture")
			first := strings.Replace(base, "line 02", "first change", 1)
			f.write(name, strings.Replace(first, "line 27", "second change", 1))
			text := f.git("diff", "--no-ext-diff", "--no-textconv") + "\n"
			f.write(name, base)
			r := f.read()
			doc, err := application.SelectImportedPatch(text, map[string][]int{name: {0}})
			if err != nil {
				t.Fatal(err)
			}
			doc.Target = target
			doc, err = f.g.ReviewImportedPatch(context.Background(), r, doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err != nil {
				t.Fatal(err)
			}
			if target == "Working tree" {
				actual, _ := os.ReadFile(filepath.Join(f.root, name))
				if string(actual) != first {
					t.Fatal("partial import changed other hunks")
				}
			} else {
				if f.git("show", ":0:"+name) != strings.TrimSuffix(first, "\n") {
					t.Fatal("partial index import changed other hunks")
				}
				actual, _ := os.ReadFile(filepath.Join(f.root, name))
				if string(actual) != base {
					t.Fatal("index import changed disk")
				}
			}
			if f.git("rev-parse", "HEAD") != r.Head {
				t.Fatal("import moved HEAD")
			}
		})
	}
}

func TestImportedStructuralPatchApplyAndReverse(t *testing.T) {
	for _, kind := range []string{"add", "empty add", "delete", "rename", "rename edit"} {
		for _, target := range []string{"Working tree", "Index only"} {
			t.Run(kind+"/"+target, func(t *testing.T) {
				f, text, oldName, newName := structuralPatchFixture(t, kind)
				r := f.read()
				head, index := r.Head, r.IndexVersion
				patch := filepath.Join(t.TempDir(), "changes.patch")
				if err := os.WriteFile(patch, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
				doc, err := f.g.PreviewImportedPatch(context.Background(), r, patch, target)
				if err != nil || doc.Version == "" {
					t.Fatalf("preview %s: %v\n%s", kind, err, text)
				}
				if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err != nil {
					t.Fatal(err)
				}
				after := f.read()
				if after.Head != head || target == "Working tree" && after.IndexVersion != index {
					t.Fatal("patch changed HEAD or wrong target")
				}
				name := newName
				if kind == "delete" {
					name = oldName
				}
				if target == "Working tree" {
					_, statErr := os.Stat(filepath.Join(r.Root, name))
					if (kind == "delete") != os.IsNotExist(statErr) {
						t.Fatalf("unexpected worktree result: %v", statErr)
					}
				} else {
					_, blobErr := f.g.readBlob(context.Background(), r.Root, ":0:"+name)
					if (kind == "delete") != (blobErr != nil) {
						t.Fatalf("unexpected index result: %v", blobErr)
					}
					if current, _ := os.ReadFile(filepath.Join(r.Root, oldName)); !strings.HasSuffix(string(current), "old\n") {
						t.Fatal("Index only changed disk")
					}
				}
				doc.Reverse = true
				doc, err = f.g.ReviewImportedPatch(context.Background(), after, doc)
				if err != nil {
					t.Fatalf("reverse preview: %v", err)
				}
				if err := f.g.ApplyImportedPatch(context.Background(), after, doc); err != nil {
					t.Fatal(err)
				}
				restored := f.read()
				if restored.Head != head || restored.IndexVersion != index || len(restored.Working) != 0 || len(restored.Staged) != 0 {
					t.Fatalf("round trip lost original state: %+v", restored)
				}
			})
		}
	}
}

func TestImportedAdditionRefusesExistingAndStaleTargets(t *testing.T) {
	f, text, _, newName := structuralPatchFixture(t, "add")
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := os.WriteFile(patch, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	f.write(newName, "keep untracked\n")
	if _, err := f.g.PreviewImportedPatch(context.Background(), f.read(), patch, "Working tree"); err == nil {
		t.Fatal("addition accepted existing untracked target")
	}
	if err := os.Remove(filepath.Join(f.root, newName)); err != nil {
		t.Fatal(err)
	}
	r := f.read()
	doc, err := f.g.PreviewImportedPatch(context.Background(), r, patch, "Working tree")
	if err != nil {
		t.Fatal(err)
	}
	f.write(newName, "arrived after preview\n")
	if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err == nil {
		t.Fatal("stale addition overwrote target")
	}
	if actual, _ := os.ReadFile(filepath.Join(f.root, newName)); string(actual) != "arrived after preview\n" {
		t.Fatal("failed patch overwrote user content")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ApplyImportedPatch(ctx, r, domain.ImportedPatch{}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestImportedAdditionProtectsIgnoredDestinationAfterPreview(t *testing.T) {
	f, text, _, name := structuralPatchFixture(t, "add")
	f.write(".gitignore", name+"\n")
	f.git("add", ".gitignore")
	f.git("commit", "-m", "ignore target")
	r := f.read()
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := os.WriteFile(patch, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := f.g.PreviewImportedPatch(context.Background(), r, patch, "Working tree")
	if err != nil {
		t.Fatal(err)
	}
	f.write(name, "ignored content\n")
	if after := f.read(); after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("fixture did not isolate ignored target fingerprint")
	}
	if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err == nil {
		t.Fatal("ignored destination overwritten")
	}
	if actual, _ := os.ReadFile(filepath.Join(f.root, name)); string(actual) != "ignored content\n" {
		t.Fatal("failed apply lost ignored content")
	}
}

func TestImportedRenameRelocationUsesBothMappedEndpoints(t *testing.T) {
	f := newWorkflow(t)
	if err := os.Mkdir(filepath.Join(f.root, "target"), 0755); err != nil {
		t.Fatal(err)
	}
	f.write("target/old.txt", "keep content\n")
	f.git("add", ".")
	f.git("commit", "-m", "relocation fixture")
	r := f.read()
	text := "diff --git a/src/old.txt b/src/new.txt\nsimilarity index 100%\nrename from src/old.txt\nrename to src/new.txt\n"
	doc, err := f.g.ReviewImportedPatch(context.Background(), r, domain.ImportedPatch{Text: text, Target: "Working tree", Strip: 2, Directory: "target"})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 2 || doc.Paths[0] != "target/new.txt" || doc.Paths[1] != "target/old.txt" {
		t.Fatalf("wrong mapped endpoints: %+v", doc.Paths)
	}
	if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err != nil {
		t.Fatal(err)
	}
	if value, err := os.ReadFile(filepath.Join(f.root, "target/new.txt")); err != nil || string(value) != "keep content\n" {
		t.Fatalf("rename result: %q %v", value, err)
	}
	doc.Reverse = true
	current := f.read()
	doc, err = f.g.ReviewImportedPatch(context.Background(), current, doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.g.ApplyImportedPatch(context.Background(), current, doc); err != nil {
		t.Fatal(err)
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || len(after.Working) != 0 {
		t.Fatal("relocation round trip changed repository")
	}
}
