package ui

import (
	"context"
	"testing"
	"time"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestBackgroundPolicySchedulingAndRepositoryIsolation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	a.Model.Active().Repository.Remotes = []domain.Remote{{Name: "origin"}}
	a.syncBackgroundRepository()
	a.Layout.VersionControl.Git.AutoFetch = true
	var contexts []context.Context
	var callbacks []func(application.BackgroundRemoteResult, error)
	a.CheckRepositoryRemote = func(ctx context.Context, repo domain.Repository, policy application.BackgroundRemotePolicy, done func(application.BackgroundRemoteResult, error)) {
		if repo.Root != a.Model.Active().Repository.Root || !policy.AutoFetch || !policy.Network.NonInteractive {
			t.Fatalf("wrong background request: %+v", policy)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Minute {
			t.Fatal("missing timeout")
		}
		contexts = append(contexts, ctx)
		callbacks = append(callbacks, done)
	}
	now := a.background.nextRemote
	a.BackgroundPulse(a.background.root, false, now.Add(-time.Second))
	if len(callbacks) != 0 {
		t.Fatal("polled before schedule")
	}
	a.BackgroundPulse(a.background.root, false, now)
	a.BackgroundPulse(a.background.root, false, now.Add(time.Minute))
	if len(callbacks) != 1 {
		t.Fatal("duplicate in-flight request")
	}
	oldRoot := a.background.root
	a.run(selectRepo, commandTarget{index: 1})
	if contexts[0].Err() != context.Canceled {
		t.Fatal("switch did not cancel old work")
	}
	callbacks[0](application.BackgroundRemoteResult{Fetched: []string{"origin"}, Message: "stale remote result"}, nil)
	if a.background.root == oldRoot || a.background.message != "" || a.background.pending {
		t.Fatal("old result installed in new repository")
	}
}

func TestBackgroundPolicyDisabledAndCooldown(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	a.Model.Active().Repository.Remotes = []domain.Remote{{Name: "origin"}}
	a.syncBackgroundRepository()
	a.Layout.VersionControl.Git.AutoFetch = false
	a.Layout.VersionControl.Git.CheckIncoming = false
	calls := 0
	a.CheckRepositoryRemote = func(_ context.Context, _ domain.Repository, p application.BackgroundRemotePolicy, done func(application.BackgroundRemoteResult, error)) {
		calls++
		done(application.BackgroundRemoteResult{Message: "checked"}, nil)
	}
	now := time.Now().Add(time.Hour)
	a.BackgroundPulse(a.background.root, false, now)
	if calls != 0 {
		t.Fatal("disabled policy performed IO")
	}
	a.Layout.VersionControl.Git.CheckIncoming = true
	a.BackgroundPulse(a.background.root, false, now)
	a.BackgroundPulse(a.background.root, false, now.Add(4*time.Minute))
	if calls != 1 {
		t.Fatal("cooldown did not suppress polling")
	}
	a.BackgroundPulse(a.background.root, false, now.Add(5*time.Minute))
	if calls != 2 {
		t.Fatal("scheduled check was lost")
	}
}
