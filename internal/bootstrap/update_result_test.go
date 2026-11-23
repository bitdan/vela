package bootstrap

import (
	"strings"
	"testing"
	"vela/internal/domain"
)

func TestUpdateTaskReportsManualShelfRestore(t *testing.T) {
	result := domain.UpdateResult{Method: "Merge", Fetched: true, Cleaned: true, Integrated: true, BackupKind: "Shelve", BackupID: "saved", BackupOID: "oid", BackupName: "Update backup"}
	text := updateTaskResult(result)
	if !strings.Contains(text, "manual Unshelve") || !strings.Contains(text, "Backup OID: oid") || strings.Contains(text, "Backup restored") {
		t.Fatalf("misleading backup result: %s", text)
	}
	if strings.Contains(updateTaskResult(domain.UpdateResult{Fetched: true, Integrated: true}), "Restore:") {
		t.Fatal("clean update claimed a backup to restore")
	}
}

func TestUpdateTaskDoesNotClaimUnconfirmedBackup(t *testing.T) {
	text := updateTaskResult(domain.UpdateResult{Method: "Merge", Fetched: true, BackupKind: "Shelve", BackupName: "possible backup"})
	if strings.Contains(text, "backup retained") || strings.Contains(text, "Retained Shelve") || strings.Contains(text, "manual Unshelve") || !strings.Contains(text, "possible backup") {
		t.Fatalf("unconfirmed backup reported as retained: %s", text)
	}
}
