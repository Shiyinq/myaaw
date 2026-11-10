package agent

import (
	"context"
	"errors"
)

// ErrRunStopped is the cancellation cause used when the client stops a run, or
// when the client connection goes away mid-stream. It is a control signal, not
// a failure: the partial transcript is kept and no error is shown. The stop
// button and a closed browser tab both report it, so the run never keeps
// spending provider tokens in the background.
var ErrRunStopped = errors.New("run stopped by client")

// StoppedByClient reports whether ctx was cancelled through ErrRunStopped.
// A plain ctx.Err() cannot tell a user stop from a timeout, so the cause is the
// only reliable signal.
func StoppedByClient(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrRunStopped)
}
