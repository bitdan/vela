package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"
	"vela/internal/application"
	"vela/internal/domain"
	"vela/internal/ui"
)

func connectReadTasks(a *ui.Workspace, invalidate func()) {
	originalReviewImportedPatch := a.ReviewImportedPatch
	a.ReviewImportedPatch = func(ctx context.Context, repo domain.Repository, input domain.ImportedPatch, done func(domain.ImportedPatch, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Check Selected Patch", taskKey("ReviewImportedPatch", input.Text, input.Target, input.Reverse, input.Strip, input.Directory, repo.Head, repo.IndexVersion, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(domain.ImportedPatch{}, err)
			return
		}
		originalReviewImportedPatch(ctx, repo, input, func(doc domain.ImportedPatch, err error) {
			if err == nil {
				err = ctx.Err()
			}
			finish(err, "")
			done(doc, err)
		})
	}
	originalRefreshRepository := a.RefreshRepository
	a.RefreshRepository = func(ctx context.Context, repo domain.Repository, done func(domain.Repository, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Background refresh", taskKey("RefreshRepository"), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.Repository), err)
			return
		}
		originalRefreshRepository(ctx, repo, func(v0 domain.Repository, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalCheckRepositoryRemote := a.CheckRepositoryRemote
	a.CheckRepositoryRemote = func(ctx context.Context, repo domain.Repository, policy application.BackgroundRemotePolicy, done func(application.BackgroundRemoteResult, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Background remote check / AutoFetch", taskKey("CheckRepositoryRemote"), 2*time.Minute, nil)
		if err != nil {
			done(*new(application.BackgroundRemoteResult), err)
			return
		}
		originalCheckRepositoryRemote(ctx, repo, policy, func(v0 application.BackgroundRemoteResult, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), v0.Message+fmt.Sprintf("\nFetched: %v; skipped: %v", v0.Fetched, v0.Skipped))
			done(v0, v1)
		})
	}
	originalInspectProjectPath := a.InspectProjectPath
	a.InspectProjectPath = func(ctx context.Context, repo domain.Repository, name string, done func(domain.ProjectPathSnapshot, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Inspect Project Path", taskKey("InspectProjectPath", name), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.ProjectPathSnapshot), err)
			return
		}
		originalInspectProjectPath(ctx, repo, name, func(v0 domain.ProjectPathSnapshot, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalPreviewImportedPatch := a.PreviewImportedPatch
	a.PreviewImportedPatch = func(ctx context.Context, repo domain.Repository, path, target string, done func(domain.ImportedPatch, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Preview Imported Patch", taskKey("PreviewImportedPatch", path, target, repo.Head, repo.IndexVersion, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.ImportedPatch), err)
			return
		}
		originalPreviewImportedPatch(ctx, repo, path, target, func(v0 domain.ImportedPatch, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryChangesPatch := a.ReadRepositoryChangesPatch
	a.ReadRepositoryChangesPatch = func(ctx context.Context, repo domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Read Repository Changes Patch", taskKey("ReadRepositoryChangesPatch", source, files, repo.Head, repo.IndexVersion, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.PatchExport), err)
			return
		}
		originalReadRepositoryChangesPatch(ctx, repo, source, files, func(v0 domain.PatchExport, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepository := a.ReadRepository
	a.ReadRepository = func(ctx context.Context, directory string, done func(domain.Repository, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, directory, "Read repository", taskKey("ReadRepository"), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.Repository), err)
			return
		}
		originalReadRepository(ctx, directory, func(v0 domain.Repository, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryLog := a.ReadRepositoryLog
	a.ReadRepositoryLog = func(ctx context.Context, root string, snapshot domain.CommitLogSnapshot, offset, limit int, done func(domain.CommitLogPage, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Read Repository Log", taskKey("ReadRepositoryLog", snapshot, offset, limit), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.CommitLogPage), err)
			return
		}
		originalReadRepositoryLog(ctx, root, snapshot, offset, limit, func(v0 domain.CommitLogPage, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryFile := a.ReadRepositoryFile
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Read File · "+file.Path, taskKey("ReadRepositoryFile", source, file.Path, file.OldPath, file.Status), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.File), err)
			return
		}
		originalReadRepositoryFile(ctx, root, source, file, func(v0 domain.File, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalSearchProjectFiles := a.SearchProjectFiles
	a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, input.Root, "Search Project Files", taskKey("SearchProjectFiles", input.Query, input.Paths, input.IncludeIgnored, input.Replacement, input.Overlays), 2*time.Minute, nil)
		if err != nil {
			done(*new(application.ProjectSearchResult), err)
			return
		}
		originalSearchProjectFiles(ctx, input, func(v0 application.ProjectSearchResult, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadMergeDocument := a.ReadMergeDocument
	a.ReadMergeDocument = func(ctx context.Context, repo domain.Repository, path string, done func(domain.MergeDocument, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Read Merge Document", taskKey("ReadMergeDocument", path, repo.Head, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.MergeDocument), err)
			return
		}
		originalReadMergeDocument(ctx, repo, path, func(v0 domain.MergeDocument, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryBlame := a.ReadRepositoryBlame
	a.ReadRepositoryBlame = func(ctx context.Context, root, revision, path string, done func(domain.BlameDocument, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Blame · "+path, taskKey("ReadRepositoryBlame", revision, path), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.BlameDocument), err)
			return
		}
		originalReadRepositoryBlame(ctx, root, revision, path, func(v0 domain.BlameDocument, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryPatch := a.ReadRepositoryPatch
	a.ReadRepositoryPatch = func(ctx context.Context, repo domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Read Repository Patch", taskKey("ReadRepositoryPatch", source, path, repo.Head, repo.IndexVersion, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.PatchDocument), err)
			return
		}
		originalReadRepositoryPatch(ctx, repo, source, path, func(v0 domain.PatchDocument, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryFileHistory := a.ReadRepositoryFileHistory
	a.ReadRepositoryFileHistory = func(ctx context.Context, root, revision, path string, offset, limit int, done func(domain.FileHistoryPage, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "File History · "+path, taskKey("ReadRepositoryFileHistory", revision, path, offset, limit), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.FileHistoryPage), err)
			return
		}
		originalReadRepositoryFileHistory(ctx, root, revision, path, offset, limit, func(v0 domain.FileHistoryPage, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryRevisionTree := a.ReadRepositoryRevisionTree
	a.ReadRepositoryRevisionTree = func(ctx context.Context, root, revision, directory string, done func(domain.RevisionTree, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Read Repository Revision Tree", taskKey("ReadRepositoryRevisionTree", revision, directory), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.RevisionTree), err)
			return
		}
		originalReadRepositoryRevisionTree(ctx, root, revision, directory, func(v0 domain.RevisionTree, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryRevisionChanges := a.ReadRepositoryRevisionChanges
	a.ReadRepositoryRevisionChanges = func(ctx context.Context, root, revision string, mainline int, done func([]domain.File, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Read Repository Revision Changes", taskKey("ReadRepositoryRevisionChanges", revision, mainline), 2*time.Minute, nil)
		if err != nil {
			done(*new([]domain.File), err)
			return
		}
		originalReadRepositoryRevisionChanges(ctx, root, revision, mainline, func(v0 []domain.File, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryComparison := a.ReadRepositoryComparison
	a.ReadRepositoryComparison = func(ctx context.Context, repo domain.Repository, ref, mode string, done func(domain.RevisionComparison, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Read Repository Comparison", taskKey("ReadRepositoryComparison", ref, mode, repo.Head, repo.WorktreeVersion), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.RevisionComparison), err)
			return
		}
		originalReadRepositoryComparison(ctx, repo, ref, mode, func(v0 domain.RevisionComparison, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalReadRepositoryComparisonCommits := a.ReadRepositoryComparisonCommits
	a.ReadRepositoryComparisonCommits = func(ctx context.Context, root string, comparison domain.RevisionComparison, offset, limit int, done func(domain.CommitLogPage, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, root, "Read Repository Comparison Commits", taskKey("ReadRepositoryComparisonCommits", comparison.Left, comparison.Right, comparison.Filter.Message, comparison.Filter.Author, comparison.Filter.Since, comparison.Filter.Until, comparison.Filter.Path, offset, limit), 2*time.Minute, nil)
		if err != nil {
			done(*new(domain.CommitLogPage), err)
			return
		}
		originalReadRepositoryComparisonCommits(ctx, root, comparison, offset, limit, func(v0 domain.CommitLogPage, v1 error) {
			if v1 == nil {
				v1 = ctx.Err()
			}
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
}
