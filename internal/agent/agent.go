package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/agent/tools"
	"github.com/codejedi-ai/adkgobot/internal/genai"
)

type Request struct {
	Input string `json:"input"`
}

type Response struct {
	Reply string `json:"reply"`
}

type Agent struct {
	model *genai.Client
	tools *tools.Registry
}

func New(model string) *Agent {
	return &Agent{
		model: genai.NewClient(model),
		tools: tools.NewRegistry(),
	}
}

func (a *Agent) ToolNames() []string {
	return a.tools.Names()
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	systemPrompt := "You are adkbot, a task-focused assistant running through a websocket gateway. " +
		"Available tools: " + strings.Join(a.ToolNames(), ",") + ". " +
		"When you need a tool, respond ONLY as raw JSON (NO markdown, NO code fences): " +
		"{\"tool\":\"name\",\"args\":{...}}. " +
		"For image_generate, supported args: prompt (required), channel (\"cloudinary\" to upload), " +
		"cloudinary_public_id (string). For video_generate: prompt (required), channel, cloudinary_public_id, wait (bool)."

	modelOut, err := a.model.Generate(ctx, systemPrompt, input)
	if err != nil {
		return "", err
	}

	// Strip markdown code fences that models sometimes add despite instructions
	cleaned := stripCodeFences(strings.TrimSpace(modelOut))

	var call struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(cleaned), &call); err != nil || call.Tool == "" {
		return modelOut, nil
	}

	toolCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	res, err := a.tools.Execute(toolCtx, call.Tool, call.Args)
	if err != nil {
		return "", err
	}
	resBytes, _ := json.Marshal(res.Output)

	followupInput := fmt.Sprintf("User asked: %s\nTool used: %s\nTool output JSON: %s\nNow provide the final response to the user.", input, res.Name, string(resBytes))
	return a.model.Generate(ctx, "You are adkbot. Keep responses concise and practical.", followupInput)
}

// stripCodeFences removes markdown code fences (```json ... ``` or ``` ... ```)
// that LLMs sometimes wrap around JSON tool calls.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Remove opening fence (```json or ```)
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	// Remove closing fence
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func (a *Agent) RunTool(ctx context.Context, name string, args map[string]any) (tools.ToolResult, error) {
	return a.tools.Execute(ctx, name, args)
}
