package application

import "testing"

func TestBackgroundDefaultsRequireFetchOptIn(t *testing.T) {
	settings := DefaultVersionControlSettings()
	if settings.Git.AutoFetch || !settings.Git.CheckIncoming {
		t.Fatal("defaults must check incoming without automatic fetch")
	}
	// A request captures the selected network policy, independently of later edits.
	policy := BackgroundRemotePolicy{AutoFetch: settings.Git.AutoFetch, CheckIncoming: settings.Git.CheckIncoming}
	settings.Git.AutoFetch, settings.Git.CheckIncoming = true, false
	if policy.AutoFetch || !policy.CheckIncoming {
		t.Fatal("settings edits changed an existing request")
	}
}
