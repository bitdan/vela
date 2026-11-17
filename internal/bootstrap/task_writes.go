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

func connectWriteTasks(a *ui.Workspace, invalidate func()) {
	originalInitializeRepository := a.InitializeRepository
	a.InitializeRepository = func(ctx context.Context, directory, branch string, done func(string, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, directory, "Initialize Repository", taskKey("InitializeRepository"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(*new(string), err)
			return
		}
		originalInitializeRepository(ctx, directory, branch, func(v0 string, v1 error) {
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalManageProjectFiles := a.ManageProjectFiles
	a.ManageProjectFiles = func(ctx context.Context, repo domain.Repository, op domain.ProjectFileOperation, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Manage Project Files", taskKey("ManageProjectFiles"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalManageProjectFiles(ctx, repo, op, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
	originalCloneRepository := a.CloneRepository
	a.CloneRepository = func(ctx context.Context, source, target string, done func(string, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, target, "Clone Repository", taskKey("CloneRepository"), 10*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(*new(string), err)
			return
		}
		originalCloneRepository(ctx, source, target, func(v0 string, v1 error) {
			finish(errors.Join(v1), "")
			done(v0, v1)
		})
	}
	originalApplyImportedPatch := a.ApplyImportedPatch
	a.ApplyImportedPatch = func(ctx context.Context, repo domain.Repository, doc domain.ImportedPatch, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Apply Imported Patch", taskKey("ApplyImportedPatch"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalApplyImportedPatch(ctx, repo, doc, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
	originalWritePatchFile := a.WritePatchFile
	a.WritePatchFile = func(ctx context.Context, repo domain.Repository, path, text string, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Write Patch File", taskKey("WritePatchFile"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalWritePatchFile(ctx, repo, path, text, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
	originalReplaceProjectFiles := a.ReplaceProjectFiles
	a.ReplaceProjectFiles = func(ctx context.Context, repo domain.Repository, input application.ProjectReplaceInput, done func(domain.Repository, application.ProjectReplaceResult, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Replace Project Files", taskKey("ReplaceProjectFiles"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, application.ProjectReplaceResult{}, err, nil)
			return
		}
		originalReplaceProjectFiles(ctx, repo, input, func(v0 domain.Repository, v1 application.ProjectReplaceResult, v2 error, v3 error) {
			finish(errors.Join(v2, v3), writeTaskResult(v2, v3)+fmt.Sprintf("\nReplacements completed: %d\nWritten paths: %v", v1.Replacements, v1.Paths))
			done(v0, v1, v2, v3)
		})
	}
	originalSaveRepositoryFile := a.SaveRepositoryFile
	a.SaveRepositoryFile = func(ctx context.Context, repo domain.Repository, file domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Save file", taskKey("SaveRepositoryFile"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, domain.File{}, err, nil)
			return
		}
		originalSaveRepositoryFile(ctx, repo, file, text, func(v0 domain.Repository, v1 domain.File, v2 error, v3 error) {
			finish(errors.Join(v2, v3), writeTaskResult(v2, v3))
			done(v0, v1, v2, v3)
		})
	}
	originalUpdateRepositoryIndex := a.UpdateRepositoryIndex
	a.UpdateRepositoryIndex = func(ctx context.Context, repo domain.Repository, stage bool, files []domain.File, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Update index", taskKey("UpdateRepositoryIndex"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalUpdateRepositoryIndex(ctx, repo, stage, files, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
	originalCommitRepository := a.CommitRepository
	a.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Commit", taskKey("CommitRepository"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalCommitRepository(ctx, repo, message, amend, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
	originalUpdateRepository := a.UpdateRepository
	a.UpdateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, domain.UpdateResult, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, "Update branch", taskKey("UpdateRepository"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, domain.UpdateResult{}, err, nil)
			return
		}
		originalUpdateRepository(ctx, repo, op, func(v0 domain.Repository, v1 domain.UpdateResult, v2 error, v3 error) {
			finish(errors.Join(v2, v3), updateTaskResult(v1)+"\n"+writeTaskResult(v2, v3))
			done(v0, v1, v2, v3)
		})
	}
	originalOperateRepository := a.OperateRepository
	a.OperateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		ctx, finish, err := trackTask(a, invalidate, ctx, repo.Root, a.TaskOperationName(op), taskKey("OperateRepository"), 5*time.Minute, a.TaskWriteCancellation())
		if err != nil {
			done(repo, err, nil)
			return
		}
		originalOperateRepository(ctx, repo, op, func(v0 domain.Repository, v1 error, v2 error) {
			finish(errors.Join(v1, v2), writeTaskResult(v1, v2))
			done(v0, v1, v2)
		})
	}
}
