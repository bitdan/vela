package application

import "vela/internal/domain"

type HistoryState uint8

const (
	HistoryReady HistoryState = iota
	HistoryLoading
	HistoryError
)

type Session struct {
	Changelists                         ChangelistState
	Repository                          domain.Repository
	Working, Staged, Branch, Historical string
	Commit                              int
	Message                             string
	HistoryState                        HistoryState
	Amend                               bool
}

type App struct {
	Sessions []Session
	Index    int
}

func New() *App {
	app := &App{}
	for _, repo := range domain.MockRepositories() {
		app.Sessions = append(app.Sessions, Session{Repository: repo, Branch: repo.CurrentBranch})
		app.Sessions[len(app.Sessions)-1].Changelists.Normalize(repo)
	}
	return app
}

func (a *App) Active() *Session { return &a.Sessions[a.Index] }

func (a *App) Switch(index int) {
	if index >= 0 && index < len(a.Sessions) {
		a.Index = index
	}
}

// MoveChanges moves only the selected mock files between the two change sets.
func (s *Session) MoveChanges(stage bool, selected func(string) bool) int {
	from, to := &s.Repository.Staged, &s.Repository.Working
	if stage {
		from, to = to, from
	}
	var moved int
	remaining := (*from)[:0]
	for _, file := range *from {
		if selected(file.Path) {
			*to = append(*to, file)
			moved++
		} else {
			remaining = append(remaining, file)
		}
	}
	clear((*from)[len(remaining):])
	*from = remaining
	return moved
}

type Layout struct {
	Dark, HideCommit, HideGit, HideBranches, HideDetails bool
	Top, Commit, Branches, Log, Files                    float32
	VersionControl                                       VersionControlSettings
	Appearance                                           AppearanceSettings
	Editor                                               EditorSettings
	Keymap                                               *KeymapSettings `json:",omitempty"`
}

const SystemFont = "System Default"

type AppearanceSettings struct {
	FollowSystem    bool
	Scale, FontSize int
	Font            string
}

type EditorSettings struct {
	FontSize, TabSize int
	Font              string
}

func DefaultLayout() Layout {
	return Layout{Dark: true, Top: 58, Commit: 26, Branches: 22, Log: 64, Files: 54,
		VersionControl: DefaultVersionControlSettings(),
		Appearance:     AppearanceSettings{Scale: 100, FontSize: 13, Font: SystemFont},
		Editor:         EditorSettings{FontSize: 13, TabSize: 4, Font: SystemFont},
	}
}
