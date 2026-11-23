package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestRemoteManagementRoundtrip(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	bare := filepath.Join(t.TempDir(), "远端 with spaces.git")
	f.git("init", "--bare", bare)
	head, index, work := f.git("rev-parse", "HEAD"), f.git("ls-files", "--stage"), f.git("status", "--porcelain=v2")
	if err := f.run(domain.RepositoryOperation{Kind: domain.AddRemote, Remote: "upstream", Target: bare}); err != nil {
		t.Fatal(err)
	}
	if got := f.git("remote", "get-url", "upstream"); got != bare {
		t.Fatalf("wrong URL %q", got)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.EditRemote, Remote: "upstream", Target: "https://example.test/team/repo.git"}); err != nil {
		t.Fatal(err)
	}
	if got := f.git("remote", "get-url", "upstream"); got != "https://example.test/team/repo.git" {
		t.Fatalf("URL not edited %q", got)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "upstream"}); err != nil {
		t.Fatal(err)
	}
	if f.git("remote") != "" || f.git("rev-parse", "HEAD") != head || f.git("ls-files", "--stage") != index || f.git("status", "--porcelain=v2") != work {
		t.Fatal("remote management changed checkout or failed to remove config")
	}
}

func TestRemoteManagementSafety(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("remote", "add", "origin", "https://example.test/old.git")
	snapshot := f.read()
	f.git("remote", "set-url", "origin", "https://example.test/external.git")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.EditRemote, Remote: "origin", Target: "https://example.test/new.git"}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale config accepted: %v", err)
	}
	for _, op := range []domain.RepositoryOperation{
		{Kind: domain.AddRemote, Remote: "origin", Target: "https://example.test/new.git"},
		{Kind: domain.AddRemote, Remote: "--upload-pack=evil", Target: "https://example.test/new.git"},
		{Kind: domain.AddRemote, Remote: "new", Target: "ext::evil"},
		{Kind: domain.AddRemote, Remote: "new", Target: "https://secret@example.test/repo.git"},
		{Kind: domain.RemoveRemote, Remote: "missing"},
	} {
		if err := f.run(op); err == nil {
			t.Fatalf("unsafe operation accepted: %+v", op)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(canceled, f.read(), domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "origin"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if got := f.git("remote", "get-url", "origin"); got != "https://example.test/external.git" {
		t.Fatal("failed operation changed URL")
	}
	f.git("config", "--add", "remote.origin.url", "https://example.test/second.git")
	if err := f.run(domain.RepositoryOperation{Kind: domain.EditRemote, Remote: "origin", Target: "https://example.test/replacement.git"}); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous multi-URL edit accepted: %v", err)
	}
}

func TestRemoteRemovalVerifiesTrackingRefs(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("remote", "add", "origin", "https://example.test/old.git")
	f.git("update-ref", "refs/remotes/origin/main", "HEAD")
	snapshot := f.read()
	f.git("commit", "--allow-empty", "-m", "next")
	f.git("update-ref", "refs/remotes/origin/main", "HEAD")
	// Keep checkout current while the remote refs in the old confirmation differ.
	current := f.read()
	snapshot.Head, snapshot.IndexVersion = current.Head, current.IndexVersion
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "origin"}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale tracking ref removal accepted: %v", err)
	}
}

func TestRemoteManagementRejectsInheritedAndCustomRefspecs(t *testing.T) {
	f := newWorkflow(t)
	global := filepath.Join(t.TempDir(), "global-gitconfig")
	if err := os.WriteFile(global, []byte("[remote \"inherited\"]\n url = https://example.test/global.git\n fetch = +refs/heads/*:refs/remotes/inherited/*\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	f.git("config", "remote.inherited.prune", "true")
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "inherited"}); err == nil {
		t.Fatal("inherited remote was partially removed")
	}
	f.git("remote", "add", "custom", "https://example.test/custom.git")
	f.git("config", "remote.custom.fetch", "+refs/heads/*:refs/custom/*")
	f.git("update-ref", "refs/custom/main", "HEAD")
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "custom"}); err == nil {
		t.Fatal("custom fetch mapping deletion was accepted without modeled impact")
	}
	if f.git("rev-parse", "refs/custom/main") != f.git("rev-parse", "HEAD") {
		t.Fatal("custom ref was removed")
	}
}

func TestRemoteManagementRejectsLocalInclude(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	include := filepath.Join(t.TempDir(), "included-config")
	if err := os.WriteFile(include, []byte("[remote \"included\"]\n url = https://example.test/include.git\n fetch = +refs/heads/*:refs/remotes/included/*\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.git("config", "include.path", filepath.ToSlash(include))
	f.git("config", "remote.included.prune", "true")
	for _, kind := range []domain.RepositoryOperationKind{domain.EditRemote, domain.RemoveRemote} {
		if err := f.run(domain.RepositoryOperation{Kind: kind, Remote: "included", Target: "https://example.test/replacement.git"}); err == nil {
			t.Fatalf("included remote mutation accepted: %v", kind)
		}
	}
	if f.git("config", "--get", "remote.included.prune") != "true" {
		t.Fatal("direct config was partially removed")
	}
}

func TestRemoteRemovalRejectsStaleFullConfiguration(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"remote.origin.prune", "branch.topic.remote"} {
		t.Run(setting, func(t *testing.T) {
			f := newWorkflow(t)
			f.git("remote", "add", "origin", "https://example.test/original.git")
			snapshot := f.read()
			value := "true"
			if setting == "branch.topic.remote" {
				value = "origin"
			}
			f.git("config", setting, value)
			if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.RemoveRemote, Remote: "origin"}); !errors.Is(err, ErrStaleIndex) {
				t.Fatalf("stale destructive config edit accepted: %v", err)
			}
			if f.git("config", "--get", setting) != value {
				t.Fatal("newer configuration deleted")
			}
		})
	}
}
