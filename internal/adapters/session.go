package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"vela/internal/application"
)

var ErrInvalidSession = errors.New("invalid workspace session")

func LoadSession(path string) (application.WorkspaceSnapshot, error) {
	var session application.WorkspaceSnapshot
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return session, nil
	}
	if err != nil {
		return session, fmt.Errorf("read workspace session: %w", err)
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return application.WorkspaceSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}
	return session, nil
}

func SaveSession(path string, session application.WorkspaceSnapshot) error {
	if err := saveJSON(path, session); err != nil {
		return fmt.Errorf("save workspace session: %w", err)
	}
	return nil
}
