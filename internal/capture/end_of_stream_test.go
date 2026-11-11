package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

// A window that closes while it is recorded ends the recording rather than
// failing it: what is on disk is the recording, and the window keeps it. The
// message is the backend's, wrapped and all.
func TestEndOfStreamKnowsTheSourceIsGone(t *testing.T) {
	gone := fmt.Errorf("capture stopped: %w", fmt.Errorf("%w: window:402f2 is not capturable any more", ErrSourceGone))
	if got := endOfStream(context.Background(), gone); got != streamGone {
		t.Errorf("a window that closed is read as %v, want streamGone", got)
	}
}

// The other two ends stay what they were: a stop we asked for is not an error,
// a real capture failure still fails the session.
func TestEndOfStreamTheOtherTwo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want streamEnd
	}{
		{"cancelled", ctx, errors.New("context canceled"), streamStopped},
		{"eof", context.Background(), io.EOF, streamStopped},
		{"permission", context.Background(), fmt.Errorf("%w: denied", ErrPermission), streamFailed},
		{"no frame", context.Background(), fmt.Errorf("%w: nothing came", ErrNoFrame), streamFailed},
	} {
		if got := endOfStream(tc.ctx, tc.err); got != tc.want {
			t.Errorf("%s: read as %v, want %v", tc.name, got, tc.want)
		}
	}
}
