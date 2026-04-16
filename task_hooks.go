package main

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
)

// newTaskHooks wires observability callbacks for task lifecycle events emitted
// by mcp-go's task-augmented tool path. Logger-only; no side effects on task
// execution itself.
func newTaskHooks(logger Logger) *server.TaskHooks {
	h := &server.TaskHooks{}
	h.AddOnTaskCreated(func(_ context.Context, m server.TaskMetrics) {
		logger.Info("task %s started: tool=%s session=%s", m.TaskID, m.ToolName, m.SessionID)
	})
	h.AddOnTaskCompleted(func(_ context.Context, m server.TaskMetrics) {
		logger.Info("task %s completed: tool=%s duration=%v", m.TaskID, m.ToolName, m.Duration)
	})
	h.AddOnTaskFailed(func(_ context.Context, m server.TaskMetrics) {
		logger.Error("task %s failed: tool=%s duration=%v error=%v", m.TaskID, m.ToolName, m.Duration, m.Error)
	})
	h.AddOnTaskCancelled(func(_ context.Context, m server.TaskMetrics) {
		logger.Info("task %s cancelled: tool=%s duration=%v", m.TaskID, m.ToolName, m.Duration)
	})
	return h
}
