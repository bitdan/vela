package application

import (
	"context"
	"strings"
)

// CommitWarnings requires consent for this exact warning inspection digest.
// It is returned before Git commit executes, with no change to HEAD or Index.
type CommitWarnings struct {
	Messages []string
	Digest   string
}

func (w *CommitWarnings) Error() string {
	return "Commit warnings require confirmation:\n" + strings.Join(w.Messages, "\n")
}

type commitConsentKey struct{}

func WithCommitWarningConsent(ctx context.Context, digest string) context.Context {
	return context.WithValue(ctx, commitConsentKey{}, digest)
}
func CommitWarningConsent(ctx context.Context) string {
	value, _ := ctx.Value(commitConsentKey{}).(string)
	return value
}
