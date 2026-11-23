package gitcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"vela/internal/application"
)

func TestImportedBinaryPatchTargetsAndReverse(t *testing.T) {
	for _, kind := range []string{"modify", "add", "delete", "rename"} {
		for _, target := range []string{"Working tree", "Index only"} {
			t.Run(kind+"/"+target, func(t *testing.T) {
				f := newWorkflow(t)
				old := append([]byte{0, 1, 2, 255}, bytes.Repeat([]byte("binary base\r\n"), 300)...)
				changed := bytes.Clone(old)
				changed[100] = 'X'
				changed[200] = 'Y'
				name, newName := "blob.bin", "new.bin"
				if kind != "add" {
					f.write(name, string(old))
					f.git("add", name)
					f.git("commit", "-m", "binary base")
				}
				before := f.read()
				switch kind {
				case "modify":
					f.write(name, string(changed))
					f.git("add", name)
				case "add":
					f.write(name, string(changed))
					f.git("add", name)
				case "delete":
					f.git("rm", name)
				case "rename":
					f.git("mv", name, newName)
					f.write(newName, string(changed))
					f.git("add", newName)
				}
				patchResult, patchErr := f.g.run(context.Background(), f.root, "diff", "--cached", "--binary", "--find-renames")
				if patchErr != nil {
					t.Fatal(patchErr)
				}
				text := patchResult.stdout
				f.git("reset", "--hard", "HEAD") // This fixture is an isolated temporary repository.
				if kind == "add" {
					if err := os.Remove(filepath.Join(f.root, name)); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
				}
				if kind == "rename" {
					if err := os.Remove(filepath.Join(f.root, newName)); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
				}
				r := f.read()
				doc, err := application.ParseImportedPatch(text)
				if err != nil {
					t.Fatalf("parse binary: %v\n%s", err, text)
				}
				doc.Target = target
				doc, err = f.g.ReviewImportedPatch(context.Background(), r, doc)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err != nil {
					t.Fatal(err)
				}
				resultPath := name
				if kind == "rename" {
					resultPath = newName
				}
				if kind != "delete" {
					var actual []byte
					if target == "Working tree" {
						actual, err = os.ReadFile(filepath.Join(f.root, resultPath))
					} else {
						var out result
						out, err = f.g.run(context.Background(), f.root, "cat-file", "blob", ":0:"+resultPath)
						actual = []byte(out.stdout)
					}
					if err != nil || !bytes.Equal(actual, changed) {
						t.Fatal("binary result differs from Git patch")
					}
				}
				current := f.read()
				if current.Head != before.Head || target == "Working tree" && current.IndexVersion != before.IndexVersion {
					t.Fatal("binary patch changed HEAD or wrong target")
				}
				doc.Reverse = true
				doc, err = f.g.ReviewImportedPatch(context.Background(), current, doc)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.g.ApplyImportedPatch(context.Background(), current, doc); err != nil {
					t.Fatal(err)
				}
				if after := f.read(); after.Head != before.Head || after.IndexVersion != before.IndexVersion || len(after.Working) != 0 || len(after.Staged) != 0 {
					t.Fatal("binary round trip lost original state")
				}
			})
		}
	}
}

func TestImportedModePatchIndexAndPlatformLimit(t *testing.T) {
	for _, withContent := range []bool{false, true} {
		t.Run(map[bool]string{false: "mode only", true: "mode and content"}[withContent], func(t *testing.T) {
			f := newWorkflow(t)
			f.write("script.sh", "old\n")
			f.git("add", "script.sh")
			f.git("commit", "-m", "script")
			before := f.read()
			if withContent {
				f.write("script.sh", "new\n")
				f.git("add", "script.sh")
			}
			f.git("update-index", "--chmod=+x", "script.sh")
			text := f.git("diff", "--cached", "--binary") + "\n"
			f.git("reset", "--hard", "HEAD")
			doc, err := application.ParseImportedPatch(text)
			if err != nil {
				t.Fatal(err)
			}
			doc.Target = "Working tree"
			if runtime.GOOS == "windows" {
				if _, err := f.g.ReviewImportedPatch(context.Background(), f.read(), doc); err == nil || !strings.Contains(err.Error(), "Index only") {
					t.Fatalf("missing Windows permission limit: %v", err)
				}
			}
			doc.Target = "Index only"
			doc, err = f.g.ReviewImportedPatch(context.Background(), f.read(), doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.g.ApplyImportedPatch(context.Background(), f.read(), doc); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(f.git("ls-files", "--stage", "--", "script.sh"), "100755 ") {
				t.Fatal("executable index mode not applied")
			}
			doc.Reverse = true
			doc, err = f.g.ReviewImportedPatch(context.Background(), f.read(), doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.g.ApplyImportedPatch(context.Background(), f.read(), doc); err != nil {
				t.Fatal(err)
			}
			if after := f.read(); after.Head != before.Head || after.IndexVersion != before.IndexVersion || len(after.Working) != 0 {
				t.Fatal("mode round trip changed original state")
			}
		})
	}
}

func TestImportedBinaryModeCombinationAndStaleRead(t *testing.T) {
	f := newWorkflow(t)
	old := []byte{0, 1, 2, 3, 255}
	changed := []byte{0, 4, 5, 6, 255}
	f.write("binary.bin", string(old))
	f.git("add", "binary.bin")
	f.git("commit", "-m", "binary base")
	f.write("binary.bin", string(changed))
	f.git("add", "binary.bin")
	f.git("update-index", "--chmod=+x", "binary.bin")
	out, err := f.g.run(context.Background(), f.root, "diff", "--cached", "--binary")
	if err != nil {
		t.Fatal(err)
	}
	f.git("reset", "--hard", "HEAD")
	r := f.read()
	doc, err := application.ParseImportedPatch(out.stdout)
	if err != nil {
		t.Fatal(err)
	}
	doc.Target = "Index only"
	doc, err = f.g.ReviewImportedPatch(context.Background(), r, doc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ApplyImportedPatch(ctx, r, doc); !errors.Is(err, context.Canceled) {
		t.Fatalf("binary cancellation: %v", err)
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("cancelled binary import changed repository")
	}
	if err := f.g.ApplyImportedPatch(context.Background(), r, doc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.git("ls-files", "--stage", "binary.bin"), "100755 ") {
		t.Fatal("combined binary mode was not applied")
	}
	actual, err := f.g.run(context.Background(), f.root, "cat-file", "blob", ":0:binary.bin")
	if err != nil || !bytes.Equal([]byte(actual.stdout), changed) {
		t.Fatal("combined binary payload was not applied")
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
	if after := f.read(); after.IndexVersion != r.IndexVersion {
		t.Fatal("combined mode/payload reverse did not restore Index")
	}
	// Stale raw bytes must be rejected even when the old preview is retained.
	doc.Reverse = false
	current = f.read()
	doc, err = f.g.ReviewImportedPatch(context.Background(), current, doc)
	if err != nil {
		t.Fatal(err)
	}
	f.write("binary.bin", string([]byte{0, 9, 8, 7, 255}))
	if err := f.g.ApplyImportedPatch(context.Background(), current, doc); err == nil {
		t.Fatal("stale binary preview was accepted")
	}
	if actual, _ := os.ReadFile(filepath.Join(f.root, "binary.bin")); !bytes.Equal(actual, []byte{0, 9, 8, 7, 255}) {
		t.Fatal("failed stale import overwrote raw disk bytes")
	}
}
