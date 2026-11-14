package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/egoist/mygo"
	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/branding"
	"vela/internal/domain"
	"vela/internal/ui"
)

func Run() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config directory: %w", err)
	}
	return run(filepath.Join(dir, "vela", "layout.json"))
}

func InitializeRepository(ctx context.Context, directory, branch string) (string, error) {
	return gitcli.New().Initialize(ctx, directory, branch)
}

func run(layoutPath string) error {
	layout, err := adapters.LoadLayout(layoutPath)
	if err != nil {
		log.Printf("Using default layout: %v", err)
	}
	workspace := ui.New(application.New(), layout)
	sessionPath := filepath.Join(filepath.Dir(layoutPath), "workspace.json")
	session, sessionErr := adapters.LoadSession(sessionPath)
	if sessionErr != nil {
		log.Printf("Using default workspace: %v", sessionErr)
	} else {
		workspace.RestoreSession(session)
	}
	initial := workspace.SessionSnapshot()
	git := gitcli.New()
	var initializer application.RepositoryInitializer = git
	var cloner application.RepositoryCloner = git
	var reader application.RepositoryReader = git
	var logReader application.RepositoryLogReader = git
	var index application.IndexWriter = git
	var commits application.CommitWriter = git
	var operator application.RepositoryOperator = git
	var mergeDocuments application.MergeDocumentReader = git
	var blameReader application.RepositoryBlameReader = git
	var fileWriter application.RepositoryFileWriter = git
	var patchReader application.RepositoryPatchReader = git
	var fileHistoryReader application.RepositoryFileHistoryReader = git
	var patchExporter application.RepositoryPatchExporter = git
	var patchFiles application.PatchFileWriter = adapters.PatchFiles{}
	var projectSearch application.ProjectSearcher = adapters.ProjectFiles{}
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "vela", Width: 1440, Height: 900, MinWidth: 900, MinHeight: 620,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 40,
			Content: native.View(workspace.View),
		})
		if err := window.SetIcon([]byte(branding.IconPNG)); err != nil {
			log.Printf("Set vela window icon: %v", err)
		}
		workspace.InitializeRepository = func(ctx context.Context, directory, branch string, done func(string, error)) {
			go func() {
				path, err := initializer.Initialize(ctx, directory, branch)
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.CloneRepository = func(ctx context.Context, source, target string, done func(string, error)) {
			go func() {
				path, err := cloner.Clone(ctx, source, target)
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.PickRepositoryDirectory = func(done func(string, error)) {
			go func() {
				dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Directory: true, Title: "Choose Repository Directory"})
				var path string
				if len(dirs) > 0 {
					path = dirs[0]
				}
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.PickPatchDestination = func(done func(string, error)) {
			go func() {
				path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: window, Title: "Export Git Patch", DefaultPath: "changes.patch", ButtonLabel: "Create", Message: "Choose a new .patch or .diff file. Existing files are kept.", Filters: []mygo.FileFilter{{Name: "Git Patch", Extensions: []string{"patch", "diff"}}}})
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryChangesPatch = func(ctx context.Context, repo domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
			go func() {
				doc, err := patchExporter.ReadChangesPatch(ctx, repo, source, files)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
			}()
		}
		workspace.WritePatchFile = func(ctx context.Context, repo domain.Repository, path, text string, done func(domain.Repository, error, error)) {
			go func() {
				err := patchFiles.SaveNewPatch(ctx, repo, path, text)
				updated, refreshErr := reader.ReadRepository(context.Background(), repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.ReadRepository = func(ctx context.Context, directory string, done func(domain.Repository, error)) {
			go func() {
				repo, err := reader.ReadRepository(ctx, directory)
				mygo.RunOnMain(func() { done(repo, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryLog = func(ctx context.Context, root string, snapshot domain.CommitLogSnapshot, offset, limit int, done func(domain.CommitLogPage, error)) {
			go func() {
				page, err := logReader.ReadCommitLog(ctx, root, snapshot, offset, limit)
				mygo.RunOnMain(func() { done(page, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
			go func() {
				file, err := reader.ReadFile(ctx, root, source, file)
				mygo.RunOnMain(func() { done(file, err); window.Invalidate() })
			}()
		}
		workspace.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
			go func() {
				result, err := projectSearch.SearchProject(ctx, input)
				mygo.RunOnMain(func() { done(result, err); window.Invalidate() })
			}()
		}
		workspace.ReadMergeDocument = func(ctx context.Context, repo domain.Repository, path string, done func(domain.MergeDocument, error)) {
			go func() {
				doc, err := mergeDocuments.ReadMergeDocument(ctx, repo, path)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryBlame = func(ctx context.Context, root, revision, path string, done func(domain.BlameDocument, error)) {
			go func() {
				doc, err := blameReader.ReadBlame(ctx, root, revision, path)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryPatch = func(ctx context.Context, repo domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
			go func() {
				doc, err := patchReader.ReadPatch(ctx, repo, source, path)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryFileHistory = func(ctx context.Context, root, revision, path string, offset, limit int, done func(domain.FileHistoryPage, error)) {
			go func() {
				page, err := fileHistoryReader.ReadFileHistory(ctx, root, revision, path, offset, limit)
				mygo.RunOnMain(func() { done(page, err); window.Invalidate() })
			}()
		}
		workspace.SaveRepositoryFile = func(ctx context.Context, repo domain.Repository, file domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
			go func() {
				saved, err := fileWriter.SaveFile(ctx, repo, file, text)
				updated, refreshErr := reader.ReadRepository(context.Background(), repo.Root)
				mygo.RunOnMain(func() { done(updated, saved, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.UpdateRepositoryIndex = func(ctx context.Context, repo domain.Repository, stage bool, files []domain.File, done func(domain.Repository, error, error)) {
			go func() {
				err := index.UpdateIndex(ctx, repo, stage, files)
				updated, refreshErr := reader.ReadRepository(context.Background(), repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
			policy := application.CommitPolicy{Inspections: workspace.Layout.VersionControl.Commit}
			go func() {
				err := commits.Commit(ctx, repo, message, amend, policy)
				updated, refreshErr := reader.ReadRepository(context.Background(), repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.OperateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
			go func() {
				err := operator.ExecuteRepositoryOperation(ctx, repo, op)
				updated, refreshErr := reader.ReadRepository(context.Background(), repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.CloseWindow = func() { go mygo.RunOnMain(window.Close) }
		workspace.StartSessionRestore()
		window.OnClose(func(e *mygo.CloseEvent) {
			if !workspace.RequestClose() {
				e.PreventDefault()
				window.Invalidate()
			}
		})
	})
	runErr := mygo.App.Run()
	if runErr != nil {
		return fmt.Errorf("run native app: %w", runErr)
	}
	var saveLayout, saveSession error
	if workspace.Layout != layout {
		saveLayout = replaceConfig(layoutPath, err, adapters.ErrInvalidLayout, func() error {
			return adapters.SaveLayout(layoutPath, workspace.Layout)
		})
	}
	current := workspace.SessionSnapshot()
	if !reflect.DeepEqual(initial, current) {
		saveSession = replaceConfig(sessionPath, sessionErr, adapters.ErrInvalidSession, func() error {
			return adapters.SaveSession(sessionPath, current)
		})
	}
	return errors.Join(saveLayout, saveSession)
}

func replaceConfig(path string, loadErr, invalid error, save func() error) error {
	if loadErr != nil {
		if !errors.Is(loadErr, invalid) {
			return loadErr
		}
		backup, err := adapters.BackupLayout(path)
		if err != nil {
			return err
		}
		log.Printf("Backed up invalid config to %s", backup)
	}
	return save()
}
