package application

import "context"

type taskProgressKey struct{}

// WithTaskProgress installs an optional phase reporter. Bootstrap
// marshals reports onto the UI thread; adapters never retain UI state.
func WithTaskProgress(ctx context.Context, report func(string)) context.Context {
	return context.WithValue(ctx, taskProgressKey{}, report)
}

func ReportTaskProgress(ctx context.Context, phase string) {
	if report, ok := ctx.Value(taskProgressKey{}).(func(string)); ok {
		report(phase)
	}
}
