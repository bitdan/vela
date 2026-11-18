package domain

import (
	"encoding/hex"
	"errors"
	"strings"
)

// CommitSigningPolicy is a request-scoped OpenPGP override. Disabled means
// inherit Git's configuration, never a request to create an unsigned commit.
type CommitSigningPolicy struct {
	Enabled bool
	Key     string
}

func (p CommitSigningPolicy) KeyID() string { return strings.ToUpper(strings.TrimSpace(p.Key)) }

func (p CommitSigningPolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	key := strings.TrimSuffix(p.KeyID(), "!")
	if len(key) != 16 && len(key) != 40 && len(key) != 64 {
		return errors.New("enter a 16-digit OpenPGP key ID or a 40/64-digit fingerprint (optional trailing ! for an exact subkey)")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return errors.New("OpenPGP key ID/fingerprint must contain hexadecimal digits only")
	}
	return nil
}
