package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"time"

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
	workspace := ui.New(application.NewWorkspace(), layout)
	sessionPath := filepath.Join(filepath.Dir(layoutPath), "workspace.json")
	session, sessionErr := adapters.LoadSession(sessionPath)
	if sessionErr != nil {
		log.Printf("Using default workspace: %v", sessionErr)
	} else {
		workspace.RestoreSession(session)
	}
	initial := workspace.SessionSnapshot()
	git := gitcli.New()
	var projectFiles application.ProjectFileManager = gitcli.ProjectFiles{Git: git, Trash: mygo.Shell.TrashItem}
	var initializer application.RepositoryInitializer = git
	var cloner application.RepositoryNetworkCloner = git
	var reader application.RepositoryReader = git
	var comparisonReader application.RepositoryComparisonReader = git
	var revisionTreeReader application.RepositoryRevisionTreeReader = git
	var logReader application.RepositoryLogReader = git
	var index application.IndexWriter = git
	var commits application.CommitWriter = git
	var operator application.RepositoryOperator = git
	var updater application.RepositoryUpdater = git
	var mergeDocuments application.MergeDocumentReader = git
	var blameReader application.RepositoryBlameReader = git
	var fileWriter application.RepositoryFileWriter = git
	var patchReader application.RepositoryPatchReader = git
	var fileHistoryReader application.RepositoryFileHistoryReader = git
	var patchExporter application.RepositoryPatchExporter = git
	var patchImporter application.RepositoryPatchImporter = git
	var patchReviewer application.RepositoryPatchReviewer = git
	var patchFiles application.PatchFileWriter = adapters.PatchFiles{}
	var projectSearch application.ProjectSearcher = adapters.ProjectFiles{}
	var projectReplacer application.ProjectReplacer = git
	var background application.BackgroundRepository = git
	monitor := &repositoryWatch{}
	defer monitor.close()
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "vela", Width: 1440, Height: 900, MinWidth: 900, MinHeight: 620,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 40,
			Content: native.View(workspace.View),
		})
		if err := window.SetIcon([]byte(branding.IconPNG)); err != nil {
			log.Printf("Set vela window icon: %v", err)
		}
		workspace.WatchRepository = func(repo domain.Repository) {
			monitor.configure(repo, func(root string, dirty bool, now time.Time) {
				workspace.BackgroundPulse(root, dirty, now)
				window.Invalidate()
			}, func(root, message string) {
				workspace.BackgroundNotice(root, message)
				window.Invalidate()
			})
		}
		workspace.RefreshRepository = func(ctx context.Context, repo domain.Repository, done func(domain.Repository, error)) {
			go func() {
				updated, err := background.RefreshRepository(ctx, repo)
				mygo.RunOnMain(func() { done(updated, err); window.Invalidate() })
			}()
		}
		workspace.CheckRepositoryRemote = func(ctx context.Context, repo domain.Repository, policy application.BackgroundRemotePolicy, done func(application.BackgroundRemoteResult, error)) {
			go func() {
				result, err := background.CheckRepositoryRemote(ctx, repo, policy)
				mygo.RunOnMain(func() { done(result, err); window.Invalidate() })
			}()
		}
		workspace.InitializeRepository = func(ctx context.Context, directory, branch string, done func(string, error)) {
			go func() {
				path, err := initializer.Initialize(ctx, directory, branch)
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.InspectProjectPath = func(ctx context.Context, repo domain.Repository, name string, done func(domain.ProjectPathSnapshot, error)) {
			go func() {
				snapshot, err := projectFiles.InspectProjectPath(ctx, repo, name)
				mygo.RunOnMain(func() { done(snapshot, err); window.Invalidate() })
			}()
		}
		workspace.ManageProjectFiles = func(ctx context.Context, repo domain.Repository, op domain.ProjectFileOperation, done func(domain.Repository, error, error)) {
			go func() {
				err := projectFiles.ExecuteProjectFileOperation(ctx, repo, op)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := git.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.OpenProjectFolder = func(root, name string, done func(error)) {
			go func() {
				folder, err := gitcli.ResolveProjectFolder(root, name)
				if err == nil {
					err = mygo.Shell.OpenPath(folder)
				}
				mygo.RunOnMain(func() { done(err); window.Invalidate() })
			}()
		}
		workspace.CloneRepository = func(ctx context.Context, source, target string, done func(string, error)) {
			preferences := workspace.Layout.VersionControl.Git
			policy := domain.NetworkPolicy{FetchTags: preferences.FetchTags, DisableCredentialHelper: !preferences.CredentialHelper}
			go func() {
				path, err := cloner.CloneWithPolicy(ctx, source, target, policy)
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
		workspace.PickPatchSource = func(done func(string, error)) {
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "Import Git Patch", Filters: []mygo.FileFilter{{Name: "Git Patch", Extensions: []string{"patch", "diff"}}}})
				var path string
				if len(paths) > 0 {
					path = paths[0]
				}
				mygo.RunOnMain(func() { done(path, err); window.Invalidate() })
			}()
		}
		workspace.PreviewImportedPatch = func(ctx context.Context, repo domain.Repository, path, target string, done func(domain.ImportedPatch, error)) {
			go func() {
				doc, err := patchImporter.PreviewImportedPatch(ctx, repo, path, target)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
			}()
		}
		workspace.ApplyImportedPatch = func(ctx context.Context, repo domain.Repository, doc domain.ImportedPatch, done func(domain.Repository, error, error)) {
			go func() {
				err := patchImporter.ApplyImportedPatch(ctx, repo, doc)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.ReviewImportedPatch = func(ctx context.Context, repo domain.Repository, input domain.ImportedPatch, done func(domain.ImportedPatch, error)) {
			go func() {
				doc, err := patchReviewer.ReviewImportedPatch(ctx, repo, input)
				mygo.RunOnMain(func() { done(doc, err); window.Invalidate() })
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
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
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
		workspace.ReplaceProjectFiles = func(ctx context.Context, repo domain.Repository, input application.ProjectReplaceInput, done func(domain.Repository, application.ProjectReplaceResult, error, error)) {
			go func() {
				result, err := projectReplacer.ReplaceProject(ctx, repo, input)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, result, err, refreshErr); window.Invalidate() })
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
		workspace.ReadRepositoryRevisionTree = func(ctx context.Context, root, revision, directory string, done func(domain.RevisionTree, error)) {
			go func() {
				page, err := revisionTreeReader.ReadRevisionTree(ctx, root, revision, directory)
				mygo.RunOnMain(func() { done(page, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryRevisionChanges = func(ctx context.Context, root, revision string, mainline int, done func([]domain.File, error)) {
			go func() {
				files, err := revisionTreeReader.ReadRevisionChanges(ctx, root, revision, mainline)
				mygo.RunOnMain(func() { done(files, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryComparison = func(ctx context.Context, repo domain.Repository, ref, mode string, done func(domain.RevisionComparison, error)) {
			go func() {
				var result domain.RevisionComparison
				var err error
				if mode == "missing" {
					result, err = comparisonReader.ReadMissingBranchComparison(ctx, repo, ref)
				} else {
					result, err = comparisonReader.ReadBranchComparison(ctx, repo, ref, mode == "working")
				}
				mygo.RunOnMain(func() { done(result, err); window.Invalidate() })
			}()
		}
		workspace.ReadRepositoryComparisonCommits = func(ctx context.Context, root string, comparison domain.RevisionComparison, offset, limit int, done func(domain.CommitLogPage, error)) {
			go func() {
				page, err := comparisonReader.ReadComparisonCommits(ctx, root, comparison, offset, limit)
				mygo.RunOnMain(func() { done(page, err); window.Invalidate() })
			}()
		}
		workspace.SaveRepositoryFile = func(ctx context.Context, repo domain.Repository, file domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
			go func() {
				saved, err := fileWriter.SaveFile(ctx, repo, file, text)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, saved, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.UpdateRepositoryIndex = func(ctx context.Context, repo domain.Repository, stage bool, files []domain.File, done func(domain.Repository, error, error)) {
			go func() {
				err := index.UpdateIndex(ctx, repo, stage, files)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
			preferences := workspace.Layout.VersionControl.Git
			policy := application.CommitPolicy{Signing: preferences.SigningPolicy(), Inspections: workspace.Layout.VersionControl.Commit, WarnLarge: preferences.WarnLarge, WarnNames: preferences.WarnNames, LargeFileMB: preferences.LargeFileMB, ProtectedBranches: preferences.ProtectedBranches, WarningConsent: application.CommitWarningConsent(ctx)}
			go func() {
				err := commits.Commit(ctx, repo, message, amend, policy)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.UpdateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, domain.UpdateResult, error, error)) {
			preferences := workspace.Layout.VersionControl.Git
			op.Safety = preferences.SafetyPolicy()
			op.Network = domain.NetworkPolicy{FetchTags: preferences.FetchTags, DisableCredentialHelper: !preferences.CredentialHelper}
			go func() {
				result, err := updater.UpdateRepository(ctx, repo, op)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, result, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.OperateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
			preferences := workspace.Layout.VersionControl.Git
			op.Safety = preferences.SafetyPolicy()
			op.Network = domain.NetworkPolicy{FetchTags: preferences.FetchTags, DisableCredentialHelper: !preferences.CredentialHelper}
			go func() {
				err := operator.ExecuteRepositoryOperation(ctx, repo, op)
				application.ReportTaskProgress(ctx, "Refreshing original repository after operation")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				updated, refreshErr := reader.ReadRepository(refreshCtx, repo.Root)
				mygo.RunOnMain(func() { done(updated, err, refreshErr); window.Invalidate() })
			}()
		}
		workspace.CloseWindow = func() { go mygo.RunOnMain(window.Close) }
		connectReadTasks(workspace, window.Invalidate)
		connectWriteTasks(workspace, window.Invalidate)
		workspace.StartSessionRestore()
		window.OnClose(func(e *mygo.CloseEvent) {
			if !workspace.RequestClose() {
				e.PreventDefault()
				window.Invalidate()
			} else {
				monitor.close()
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
