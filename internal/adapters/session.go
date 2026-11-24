package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"vela/internal/application"
)

var ErrInvalidSession = errors.New("invalid workspace session")

const maxSessionBytes = 128 << 20

func LoadSession(path string) (application.WorkspaceSnapshot, error) {
	var session application.WorkspaceSnapshot
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return session, nil
	}
	if err != nil {
		return session, fmt.Errorf("read workspace session: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionBytes+1))
	if err != nil {
		return session, fmt.Errorf("read workspace session: %w", err)
	}
	if len(data) > maxSessionBytes {
		return session, fmt.Errorf("%w: session exceeds 128 MiB", ErrInvalidSession)
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return application.WorkspaceSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}
	if err := application.ValidateSessionDrafts(session); err != nil {
		return application.WorkspaceSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}
	return session, nil
}

func SaveSession(path string, session application.WorkspaceSnapshot) error {
	if err := application.ValidateSessionDrafts(session); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}
	// Account for JSON escaping as well as raw draft limits. Anything saved
	// here must also fit the bounded reader on the next start.
	encoded, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode workspace session: %w", err)
	}
	if len(encoded)+1 > maxSessionBytes {
		return fmt.Errorf("%w: session exceeds 128 MiB", ErrInvalidSession)
	}
	if err := saveJSON(path, session); err != nil {
		return fmt.Errorf("save workspace session: %w", err)
	}
	return nil
}
