package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type qwenResponsesDialect struct{}

func (qwenResponsesDialect) name() string { return "qwen" }

func (qwenResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	if req.Thinking.Enabled && req.ResponseFormat != "json_object" {
		params.Reasoning.Effort = shared.ReasoningEffortMax
	} else {
		params.Reasoning.Effort = shared.ReasoningEffortNone
	}
	return []option.RequestOption{option.WithHeader("x-dashscope-session-cache", "enable")}
}
