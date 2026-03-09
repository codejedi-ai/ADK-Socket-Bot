package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/codejedi-ai/adkgobot/internal/media"
)

func mediaImageTool(ctx context.Context, args map[string]any) (interface{}, error) {
	operation := strArg(args, "operation")
	if operation == "" {
		switch {
		case strArg(args, "source_url") != "":
			operation = "upload_remote"
		case strArg(args, "data_base64") != "":
			operation = "upload_base64"
		case strArg(args, "public_id") != "" && strArg(args, "transformation") != "":
			operation = "transform_url"
		default:
			operation = "generate"
		}
	}

	switch operation {
	case "upload_remote":
		return media.UploadRemote(ctx, strArg(args, "source_url"), strArg(args, "public_id"), "image")
	case "upload_base64":
		b64 := strArg(args, "data_base64")
		if b64 == "" {
			return nil, errors.New("data_base64 is required")
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, err
		}
		mime := strArg(args, "mime_type")
		if mime == "" {
			mime = "image/png"
		}
		return media.UploadBytes(ctx, raw, mime, strArg(args, "public_id"), "image")
	case "transform_url":
		return media.TransformURL(strArg(args, "public_id"), strArg(args, "transformation"), "image")
	case "generate":
		return imageGenerateTool(ctx, args)
	default:
		return nil, errors.New("unsupported media_image operation")
	}
}

func mediaVideoTool(ctx context.Context, args map[string]any) (interface{}, error) {
	operation := strArg(args, "operation")
	if operation == "" {
		switch {
		case strArg(args, "operation_name") != "":
			operation = "poll"
		case strArg(args, "source_url") != "":
			operation = "upload_remote"
		case strArg(args, "data_base64") != "":
			operation = "upload_base64"
		case strArg(args, "public_id") != "" && strArg(args, "transformation") != "":
			operation = "transform_url"
		default:
			operation = "generate"
		}
	}

	switch operation {
	case "upload_remote":
		return media.UploadRemote(ctx, strArg(args, "source_url"), strArg(args, "public_id"), "video")
	case "upload_base64":
		b64 := strArg(args, "data_base64")
		if b64 == "" {
			return nil, errors.New("data_base64 is required")
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, err
		}
		mime := strArg(args, "mime_type")
		if mime == "" {
			mime = "video/mp4"
		}
		return media.UploadBytes(ctx, raw, mime, strArg(args, "public_id"), "video")
	case "transform_url":
		return media.TransformURL(strArg(args, "public_id"), strArg(args, "transformation"), "video")
	case "poll":
		poll, err := media.PollVideoOperation(ctx, strArg(args, "operation_name"), strArg(args, "backend"))
		if err != nil {
			return nil, err
		}
		if channelArg(args) != "cloudinary" {
			return poll, nil
		}
		return uploadVideoResults(ctx, poll, strArg(args, "cloudinary_public_id"))
	case "generate":
		return videoGenerateTool(ctx, args)
	default:
		return nil, errors.New("unsupported media_video operation")
	}
}

func imageGenerateTool(ctx context.Context, args map[string]any) (interface{}, error) {
	prompt, _ := args["prompt"].(string)
	model, _ := args["model"].(string)
	channel := channelArg(args)

	opt := media.ImageOptions{
		Model:          model,
		Backend:        strArg(args, "backend"),
		AspectRatio:    strArg(args, "aspect_ratio"),
		NegativePrompt: strArg(args, "negative_prompt"),
		NumberOfImages: int32Arg(args, "number_of_images"),
	}
	res, err := media.GenerateImages(ctx, prompt, opt)
	if err != nil {
		return nil, err
	}

	if channel != "cloudinary" {
		return res, nil
	}

	images, ok := res["images"].([]map[string]any)
	if !ok {
		return res, nil
	}
	uploaded := make([]map[string]any, 0, len(images))
	basePublicID := strArg(args, "cloudinary_public_id")
	for i, img := range images {
		b64, _ := img["image_base64"].(string)
		if b64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, err
		}
		mime, _ := img["mime_type"].(string)
		publicID := basePublicID
		if publicID != "" && len(images) > 1 {
			publicID = publicID + "_" + strings.TrimSpace(strconv.Itoa(i+1))
		}
		up, err := media.UploadBytes(ctx, data, mime, publicID, "image")
		if err != nil {
			return nil, err
		}
		// Log the Cloudinary URL to the CSV
		secureURL, _ := up["secure_url"].(string)
		localPath, _ := img["local_path"].(string)
		media.LogMediaMetadata(prompt, localPath, secureURL, "image")
		uploaded = append(uploaded, up)
	}
	return map[string]any{
		"model":       res["model"],
		"channel":     "cloudinary",
		"uploaded":    uploaded,
		"image_count": len(uploaded),
	}, nil
}

func videoGenerateTool(ctx context.Context, args map[string]any) (interface{}, error) {
	prompt := strArg(args, "prompt")
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	channel := channelArg(args)
	opt := media.VideoOptions{
		Model:           strArg(args, "model"),
		Backend:         strArg(args, "backend"),
		AspectRatio:     strArg(args, "aspect_ratio"),
		Resolution:      strArg(args, "resolution"),
		DurationSeconds: int32Arg(args, "duration_seconds"),
		NumberOfVideos:  int32Arg(args, "number_of_videos"),
		NegativePrompt:  strArg(args, "negative_prompt"),
		Wait:            boolArg(args, "wait"),
		PollIntervalSec: int(int32Arg(args, "poll_interval_seconds")),
		TimeoutSec:      int(int32Arg(args, "timeout_seconds")),
	}
	out, err := media.GenerateVideos(ctx, prompt, opt)
	if err != nil {
		return nil, err
	}
	if channel != "cloudinary" {
		return out, nil
	}
	return uploadVideoResults(ctx, out, strArg(args, "cloudinary_public_id"))
}

func uploadVideoResults(ctx context.Context, in map[string]any, publicID string) (map[string]any, error) {
	vAny, ok := in["videos"].([]map[string]any)
	if !ok {
		return in, nil
	}
	uploaded := make([]map[string]any, 0, len(vAny))
	for i, v := range vAny {
		if uri, _ := v["uri"].(string); strings.TrimSpace(uri) != "" {
			id := publicID
			if id != "" && len(vAny) > 1 {
				id = id + "_" + strings.TrimSpace(strconv.Itoa(i+1))
			}
			up, err := media.UploadRemote(ctx, uri, id, "video")
			if err != nil {
				return nil, err
			}
			uploaded = append(uploaded, up)
			continue
		}
		if b64, _ := v["video_base64"].(string); b64 != "" {
			raw, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return nil, err
			}
			id := publicID
			if id != "" && len(vAny) > 1 {
				id = id + "_" + strings.TrimSpace(strconv.Itoa(i+1))
			}
			up, err := media.UploadBytes(ctx, raw, "video/mp4", id, "video")
			if err != nil {
				return nil, err
			}
			uploaded = append(uploaded, up)
		}
	}
	in["channel"] = "cloudinary"
	in["uploaded"] = uploaded
	return in, nil
}

func channelArg(args map[string]any) string {
	ch := strings.ToLower(strings.TrimSpace(strArg(args, "channel")))
	if ch == "" {
		return "cloudinary"
	}
	return ch
}

func strArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func int32Arg(args map[string]any, key string) int32 {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return int32(n)
	case int32:
		return n
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	default:
		return 0
	}
}

func boolArg(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if ok {
		return b
	}
	return false
}
