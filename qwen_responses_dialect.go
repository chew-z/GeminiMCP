package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// qwenResponsesDialect shapes Responses-API requests for Qwen models.
// thinkingForced marks thinking-only models (e.g. qwen3.8-max) that
// reject enable_thinking=false with a 400. Their documented effort values are
// low/high/xhigh (default xhigh, 131072-token thinking budget), and they get
// low effort on every request: measured 2026-07-19, low serves review-class
// work in ~45s (~1.1K reasoning tokens) while high thinks unboundedly and
// overruns multi-minute timeout budgets on the same task. The Responses API
// exposes no thinking_budget cap — effort is the only lever. Raise this
// deliberately if a future policy wants deeper reasoning for large contexts.
//
// The x-dashscope-session-cache header is kept for single-turn use: measured
// 2026-08-16, a repeated request hit the cache for 5144/5153 tokens despite
// no previous_response_id chain. The envelope places stable context before
// the query so shared prefixes cache across calls (billed at 10% on hit vs
// 125% on creation).
type qwenResponsesDialect struct {
	thinkingForced bool
}

func (qwenResponsesDialect) name() string { return "qwen" }

func (d qwenResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	// Effort floor is low, not none: DashScope rejects none for thinking-only
	// models, and low+json_object is verified compatible for prequalify.
	// JSON format suppresses max down to the floor.
	switch {
	case d.thinkingForced:
		params.Reasoning.Effort = shared.ReasoningEffortLow
	case req.Thinking.Enabled && req.ResponseFormat != "json_object":
		params.Reasoning.Effort = shared.ReasoningEffortMax
	default:
		params.Reasoning.Effort = shared.ReasoningEffortLow
	}
	return []option.RequestOption{option.WithHeader("x-dashscope-session-cache", "enable")}
}
