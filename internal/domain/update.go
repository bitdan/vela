package domain

// UpdateResult reports durable checkpoints even when the operation stops.
// A retained Shelf is never applied or deleted by the Update workflow.
type UpdateResult struct {
	Fetched, Cleaned, Integrated, Restored                         bool
	Method, TargetOID, BackupKind, BackupID, BackupOID, BackupName string
}
