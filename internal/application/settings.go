package application

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
	"vela/internal/domain"
)

// VersionControlSettings contains local preferences. Operational Git settings
// are stored here without configuring or invoking Git.
type VersionControlSettings struct {
	Changelists  ChangelistSettings
	Commit       CommitSettings
	Confirmation ConfirmationSettings
	Colors       FileColors
	Log          LogSettings
	Shelf        ShelfSettings
	Git          GitSettings
}

type ChangelistSettings struct {
	AutoCreate, SplitFileChanges, HighlightInactive, WarnInactive bool
	EmptyInactive                                                 string
}

type CommitSettings struct {
	ClearMessage, BlankLine, LimitSubject, LimitBody, CheckTODO bool
	SubjectLength, BodyLength                                   float64
}

type ConfirmationSettings struct {
	CreatedFiles, DeletedFiles, PatchCreated string
	Checkout, Update, LimitHistory           bool
	HistoryRows                              float64
}

type FileColors struct {
	Added                       string
	AddedInactive               string
	Conflict                    string
	Copied                      string
	Deleted                     string
	DeletedFromFileSystem       string
	External                    string
	ChangedDescendants          string
	ChangedChildren             string
	Hijacked                    string
	Ignored                     string
	Merged                      string
	MergedConflicts             string
	MergedPropertyConflicts     string
	MergedTextPropertyConflicts string
	Modified                    string
	ModifiedInactive            string
	Obsolete                    string
	Obstructed                  string
	Renamed                     string
	Replaced                    string
	Suppressed                  string
	Switched                    string
	Unknown                     string
	UpToDate                    string
}

type LogSettings struct {
	ExpandedRefs, HideTags, Timestamp, RefsLeft, HideAuthor, HideDate, Hash, FirstParent, Preview, HideDetails bool
}

type ShelfSettings struct {
	RemoveApplied, StoreBase bool
}

type GitSettings struct {
	SignCommits                                                                                        bool
	SigningKey                                                                                         string
	WarnLarge, WarnNames, CherryPickSuffix, ShowPushDialog, AutoFetch, CheckIncoming, CredentialHelper bool
	LargeFileMB                                                                                        float64
	ProtectedBranches, UpdateMethod, CleanWorkingTree, FetchTags                                       string
}

func (g GitSettings) SafetyPolicy() domain.RepositorySafetyPolicy {
	return domain.RepositorySafetyPolicy{ProtectedBranches: g.ProtectedBranches, CherryPickSuffix: g.CherryPickSuffix}
}

func (g GitSettings) SigningPolicy() domain.CommitSigningPolicy {
	return domain.CommitSigningPolicy{Enabled: g.SignCommits, Key: g.SigningKey}
}

func (g GitSettings) Validate() error {
	if err := g.SigningPolicy().Validate(); err != nil {
		return err
	}
	if _, err := domain.ProtectedBranch(g.ProtectedBranches, ""); err != nil {
		return err
	}
	if g.WarnLarge && (math.IsNaN(g.LargeFileMB) || math.IsInf(g.LargeFileMB, 0) || g.LargeFileMB < 1 || g.LargeFileMB > 100000) {
		return fmt.Errorf("large file threshold must be between 1 and 100000 MiB")
	}
	return nil
}

func DefaultVersionControlSettings() VersionControlSettings {
	return VersionControlSettings{
		Changelists:  ChangelistSettings{AutoCreate: true, SplitFileChanges: true, EmptyInactive: "Show options"},
		Commit:       CommitSettings{SubjectLength: 72, BodyLength: 100},
		Confirmation: ConfirmationSettings{CreatedFiles: "Ask", DeletedFiles: "Ask", PatchCreated: "Ask", Checkout: true, Update: true, HistoryRows: 1000},
		Shelf:        ShelfSettings{StoreBase: true},
		Git: GitSettings{WarnLarge: true, WarnNames: true, CherryPickSuffix: true, ShowPushDialog: true,
			CheckIncoming: true, CredentialHelper: true, LargeFileMB: 50, ProtectedBranches: "master;main",
			UpdateMethod: "Merge", CleanWorkingTree: "Shelve", FetchTags: "Auto"},
	}
}

// ColorHex accepts an optional leading hash and exactly six RGB digits.
func ColorHex(value string) (string, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return "#" + strings.ToUpper(value), true
}

func (s CommitSettings) Inspect(message string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(message), "\r\n", "\n"), "\n")
	if s.LimitSubject && utf8.RuneCountInString(lines[0]) > int(s.SubjectLength) {
		return fmt.Sprintf("Subject exceeds %d characters", int(s.SubjectLength))
	}
	if s.BlankLine && len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
		return "Add a blank line between the subject and body"
	}
	if s.LimitBody {
		for _, line := range lines[1:] {
			if utf8.RuneCountInString(line) > int(s.BodyLength) {
				return fmt.Sprintf("Body line exceeds %d characters", int(s.BodyLength))
			}
		}
	}
	return ""
}
