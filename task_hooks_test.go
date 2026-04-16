package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingLogger captures log calls by level so tests can assert on them.
type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	level string
	msg   string
}

func (l *recordingLogger) record(level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: level, msg: fmt.Sprintf(format, args...)})
}

func (l *recordingLogger) Debug(format string, args ...any) { l.record("DEBUG", format, args...) }
func (l *recordingLogger) Info(format string, args ...any)  { l.record("INFO", format, args...) }
func (l *recordingLogger) Warn(format string, args ...any)  { l.record("WARN", format, args...) }
func (l *recordingLogger) Warnf(format string, args ...any) { l.record("WARN", format, args...) }
func (l *recordingLogger) Error(format string, args ...any) { l.record("ERROR", format, args...) }

func (l *recordingLogger) snapshot() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]logEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

func TestTaskHooks(t *testing.T) {
	metrics := server.TaskMetrics{
		TaskID:   "task-123",
		ToolName: "gemini_ask",
		Status:   mcp.TaskStatusCompleted,
		Duration: 250 * time.Millisecond,
	}

	t.Run("on_task_created_logs_info", func(t *testing.T) {
		logger := &recordingLogger{}
		hooks := newTaskHooks(logger)
		require.Len(t, hooks.OnTaskCreated, 1)

		m := metrics
		m.SessionID = "sess-1"
		hooks.OnTaskCreated[0](context.Background(), m)

		entries := logger.snapshot()
		require.Len(t, entries, 1)
		assert.Equal(t, "INFO", entries[0].level)
		assert.Contains(t, entries[0].msg, "task task-123 started")
		assert.Contains(t, entries[0].msg, "tool=gemini_ask")
		assert.Contains(t, entries[0].msg, "session=sess-1")
	})

	t.Run("on_task_completed_logs_info", func(t *testing.T) {
		logger := &recordingLogger{}
		hooks := newTaskHooks(logger)
		require.Len(t, hooks.OnTaskCompleted, 1)

		hooks.OnTaskCompleted[0](context.Background(), metrics)

		entries := logger.snapshot()
		require.Len(t, entries, 1)
		assert.Equal(t, "INFO", entries[0].level)
		assert.Contains(t, entries[0].msg, "task task-123 completed")
		assert.Contains(t, entries[0].msg, "duration=250ms")
	})

	t.Run("on_task_failed_logs_error", func(t *testing.T) {
		logger := &recordingLogger{}
		hooks := newTaskHooks(logger)
		require.Len(t, hooks.OnTaskFailed, 1)

		m := metrics
		m.Error = errors.New("boom")
		hooks.OnTaskFailed[0](context.Background(), m)

		entries := logger.snapshot()
		require.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].level)
		assert.Contains(t, entries[0].msg, "task task-123 failed")
		assert.Contains(t, entries[0].msg, "error=boom")
	})

	t.Run("on_task_cancelled_logs_info", func(t *testing.T) {
		logger := &recordingLogger{}
		hooks := newTaskHooks(logger)
		require.Len(t, hooks.OnTaskCancelled, 1)

		hooks.OnTaskCancelled[0](context.Background(), metrics)

		entries := logger.snapshot()
		require.Len(t, entries, 1)
		assert.Equal(t, "INFO", entries[0].level)
		assert.Contains(t, entries[0].msg, "task task-123 cancelled")
		assert.Contains(t, entries[0].msg, "duration=250ms")
	})
}
