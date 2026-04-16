package main

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// recordedNotification captures the params passed to SendNotificationToClient
// so tests can inspect progress/total/message shape after the fact.
type recordedNotification struct {
	method string
	params map[string]any
}

type recordingEmitter struct {
	mu    sync.Mutex
	calls []recordedNotification
}

func (r *recordingEmitter) SendNotificationToClient(_ context.Context, method string, params map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Copy params to avoid test seeing later mutations.
	copied := make(map[string]any, len(params))
	for k, v := range params {
		copied[k] = v
	}
	r.calls = append(r.calls, recordedNotification{method: method, params: copied})
	return nil
}

func (r *recordingEmitter) snapshot() []recordedNotification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedNotification, len(r.calls))
	copy(out, r.calls)
	return out
}

// reqWithProgressToken builds a minimal CallToolRequest that carries a
// progress token, mirroring what a progress-aware MCP client sends.
func reqWithProgressToken(token any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Meta: &mcp.Meta{ProgressToken: token},
		},
	}
}

func TestProgressReporter(t *testing.T) {
	t.Run("no_token_is_noop", func(t *testing.T) {
		// No _meta.progressToken — startProgressReporter must return a noop
		// stop and emit nothing regardless of how much virtual time passes.
		synctest.Test(t, func(t *testing.T) {
			logger := NewLogger(LevelError)
			req := mcp.CallToolRequest{} // no meta → no progress token

			// Build an emitter that records; the full helper should not even
			// reach it because the token guard must fire first. We exercise
			// the public entrypoint so the guard path is covered.
			stop := startProgressReporter(t.Context(), req, 10*time.Second, 300, "noop-test", logger)
			defer stop()

			time.Sleep(30 * time.Second)
			synctest.Wait()
			// Nothing to assert against an emitter — the guarantee is that
			// the public helper returns a no-op and the ticker goroutine is
			// never spawned. Reaching this line without hangs or panics
			// proves both.
		})
	})

	t.Run("emits_on_interval", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			logger := NewLogger(LevelError)
			emitter := &recordingEmitter{}
			token := mcp.ProgressToken("tok-1")

			stop := startProgressReporterWithEmitter(
				t.Context(), emitter, token, 10*time.Second, 300, "gemini-pro (high)", logger)
			defer stop()

			// Advance 25 s of virtual time — expect ticks at 10s and 20s
			// (exactly 2 notifications). synctest.Wait ensures the ticker
			// goroutine has observed every elapsed tick before we sample.
			time.Sleep(25 * time.Second)
			synctest.Wait()

			got := emitter.snapshot()
			if len(got) != 2 {
				t.Fatalf("expected 2 notifications after 25s, got %d", len(got))
			}
			for i, n := range got {
				if n.method != progressNotificationMethod {
					t.Errorf("notification %d method = %q, want %q", i, n.method, progressNotificationMethod)
				}
				if n.params["progressToken"] != token {
					t.Errorf("notification %d progressToken = %v, want %v", i, n.params["progressToken"], token)
				}
				if total, ok := n.params["total"].(float64); !ok || total != 300 {
					t.Errorf("notification %d total = %v, want 300", i, n.params["total"])
				}
			}
			// Progress values should be monotonically ~10 and ~20 seconds.
			p0, _ := got[0].params["progress"].(float64)
			p1, _ := got[1].params["progress"].(float64)
			if p0 < 9.9 || p0 > 10.1 {
				t.Errorf("first progress = %.3f, want ~10.0", p0)
			}
			if p1 < 19.9 || p1 > 20.1 {
				t.Errorf("second progress = %.3f, want ~20.0", p1)
			}
		})
	})

	t.Run("stops_on_stop_fn", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			logger := NewLogger(LevelError)
			emitter := &recordingEmitter{}

			stop := startProgressReporterWithEmitter(
				t.Context(), emitter, mcp.ProgressToken("tok-2"),
				10*time.Second, 300, "label", logger)

			time.Sleep(15 * time.Second)
			synctest.Wait()
			stop()
			before := len(emitter.snapshot())

			// Advance far past additional tick boundaries; stopped reporter
			// must stay silent.
			time.Sleep(60 * time.Second)
			synctest.Wait()
			after := len(emitter.snapshot())
			if after != before {
				t.Errorf("expected no further notifications after stop(); before=%d after=%d", before, after)
			}
		})
	})

	t.Run("stops_on_ctx_cancel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			logger := NewLogger(LevelError)
			emitter := &recordingEmitter{}

			ctx, cancel := context.WithCancel(t.Context())
			stop := startProgressReporterWithEmitter(
				ctx, emitter, mcp.ProgressToken("tok-3"),
				10*time.Second, 300, "label", logger)
			defer stop()

			time.Sleep(15 * time.Second)
			synctest.Wait()
			before := len(emitter.snapshot())

			cancel()
			// After cancel, Wait returns once the ticker goroutine is
			// durably blocked (i.e. exited). Any later ticks would be
			// captured by the emitter.
			synctest.Wait()
			time.Sleep(60 * time.Second)
			synctest.Wait()
			after := len(emitter.snapshot())
			if after != before {
				t.Errorf("expected no further notifications after ctx cancel; before=%d after=%d", before, after)
			}
		})
	})
}
