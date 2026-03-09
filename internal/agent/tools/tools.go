package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ToolResult struct {
	Name   string      `json:"name"`
	Output interface{} `json:"output"`
}

type ToolHandler func(ctx context.Context, args map[string]any) (interface{}, error)

type Registry struct {
	handlers map[string]ToolHandler
}

func NewRegistry() *Registry {
	r := &Registry{handlers: map[string]ToolHandler{}}
	r.handlers["time_now"] = timeNow
	r.handlers["echo"] = echo
	r.handlers["media_image"] = mediaImageTool
	r.handlers["media_video"] = mediaVideoTool
	r.handlers["image_generate"] = imageGenerateTool
	r.handlers["video_generate"] = videoGenerateTool
	r.handlers["cli"] = cliTool
	r.handlers["filesystem"] = filesystemTool

	// Backward-compatible aliases.
	r.handlers["shell_exec"] = cliTool
	r.handlers["gemini_generate_image"] = mediaImageTool
	r.handlers["gemini_generate_video_job"] = mediaVideoTool
	r.handlers["cloudinary_upload_remote"] = mediaImageTool
	r.handlers["cloudinary_transform_url"] = mediaImageTool
	return r
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		out = append(out, name)
	}
	return out
}

func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	h, ok := r.handlers[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
	res, err := h(ctx, args)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Name: name, Output: res}, nil
}

func timeNow(_ context.Context, args map[string]any) (interface{}, error) {
	tz := "UTC"
	if val, ok := args["timezone"].(string); ok && val != "" {
		tz = val
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q", tz)
	}
	return map[string]any{
		"timezone": tz,
		"now":      time.Now().In(loc).Format(time.RFC3339),
	}, nil
}

func echo(_ context.Context, args map[string]any) (interface{}, error) {
	v := ""
	if s, ok := args["message"].(string); ok {
		v = s
	}
	if v == "" {
		enc, _ := json.Marshal(args)
		v = string(enc)
	}
	return map[string]any{"message": v}, nil
}
