package application

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxEditorDraftBytes = 2 << 20

// ValidateSessionDrafts bounds recovery data and accepts drafts only for
// relative Project file identities. Reject instead of truncating user input.
func ValidateSessionDrafts(session WorkspaceSnapshot) error {
	count, total := 0, 0
	for _, repo := range session.Repositories {
		seen := make(map[string]bool)
		for _, tab := range repo.Tabs {
			d := tab.Draft
			if d == nil {
				continue
			}
			count++
			total += len(d.Text) + len(d.BaseText)
			if repo.Root == "" || !tab.Code || tab.Source != "project" || tab.Comparison != nil || tab.Path == "" || strings.ContainsAny(tab.Path, "\\\x00:") || path.IsAbs(tab.Path) || path.Clean(tab.Path) != tab.Path || tab.Path == ".." || strings.HasPrefix(tab.Path, "../") || seen[tab.Path] {
				return fmt.Errorf("invalid Project draft identity: %q", tab.Path)
			}
			seen[tab.Path] = true
			if len(d.Text) > MaxEditorDraftBytes || len(d.BaseText) > MaxEditorDraftBytes || total > 32<<20 || count > 100 || !utf8.ValidString(d.Text) || !utf8.ValidString(d.BaseText) || strings.ContainsRune(d.Text, 0) || strings.ContainsRune(d.BaseText, 0) || d.DiskVersion == "" || len(d.DiskVersion) > 4096 || (d.LineEnding != "\n" && d.LineEnding != "\r\n") {
				return fmt.Errorf("invalid or oversized editor draft: %s (2 MiB per text, 32 MiB total including base, 100 drafts)", tab.Path)
			}
		}
	}
	return nil
}
