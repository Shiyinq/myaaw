//go:build !darwin

package computer

import (
	"context"
	"fmt"
	"time"
)

type unsupportedBackend struct{}

func newBackend() Backend { return unsupportedBackend{} }

func unsupported() error {
	return fmt.Errorf("%w: desktop automation is only available on macOS; use the cua-driver MCP route instead", ErrUnsupportedPlatform)
}

func (unsupportedBackend) Screenshot(context.Context) (Frame, error)           { return Frame{}, unsupported() }
func (unsupportedBackend) MouseMove(context.Context, int, int) error           { return unsupported() }
func (unsupportedBackend) Click(context.Context, int, int, string) error       { return unsupported() }
func (unsupportedBackend) DoubleClick(context.Context, int, int) error         { return unsupported() }
func (unsupportedBackend) RightClick(context.Context, int, int) error          { return unsupported() }
func (unsupportedBackend) Drag(context.Context, int, int, int, int) error      { return unsupported() }
func (unsupportedBackend) Scroll(context.Context, int, int, int, string) error { return unsupported() }
func (unsupportedBackend) Type(context.Context, string) error                  { return unsupported() }
func (unsupportedBackend) Key(context.Context, string) error                   { return unsupported() }
func (unsupportedBackend) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (unsupportedBackend) ListWindows(context.Context) ([]Window, error) { return nil, unsupported() }
func (unsupportedBackend) FocusApp(context.Context, string) error        { return unsupported() }
