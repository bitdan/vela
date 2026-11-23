package bootstrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"vela/internal/application"
	"vela/internal/domain"
	"vela/internal/ui"
)

// trackTask is called and completed on the main thread. Existing ports keep
// responsibility for execution, locks and stale-result guards. A canceled task
// stays active until its actual callback (including repository refresh) returns.
func trackTask(a *ui.Workspace, invalidate func(), parent context.Context, root, name, key string, timeout time.Duration, cancelOwner context.CancelFunc) (context.Context, func(error, string), error) {
	ctx, id, err := a.Tasks.Start(parent, root, name, key, timeout, cancelOwner)
	if err != nil {
		return nil, nil, err
	}
	ctx = application.WithTaskProgress(ctx, func(phase string) {
		mygo.RunOnMain(func() { a.Tasks.Phase(id, phase); invalidate() })
	})
	return ctx, func(err error, result string) { a.Tasks.Finish(id, err, result) }, nil
}

func taskKey(kind string, parts ...any) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%q", kind)
	for _, part := range parts {
		fmt.Fprintf(hash, "\x00%#v", part)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func updateTaskResult(result domain.UpdateResult) string {
	text := fmt.Sprintf("Method: %s\nFetch completed: %t\nTracked changes cleaned: %t\nBranch integrated: %t", result.Method, result.Fetched, result.Cleaned, result.Integrated)
	if result.BackupOID != "" && result.BackupID != "" {
		text += "\nRestore: manual Unshelve (backup retained)"
		text += fmt.Sprintf("\nRetained %s backup: %s\nBackup OID: %s\nBackup name: %s", result.BackupKind, result.BackupID, result.BackupOID, result.BackupName)
	} else if result.BackupName != "" {
		text += "\nBackup publication unconfirmed; inspect Shelf for: " + result.BackupName
	}
	return text
}

func writeTaskResult(operationErr, refreshErr error) string {
	return fmt.Sprintf("Operation completed: %t\nOriginal repository refresh completed: %t\nCancellation does not roll back changes.", operationErr == nil, refreshErr == nil)
}
