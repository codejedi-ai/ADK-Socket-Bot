package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/config"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

func cloudinaryClient() (*cloudinary.Cloudinary, error) {
	cloudURL, err := config.ResolveCloudinaryURL()
	if err != nil {
		return nil, err
	}
	_ = os.Setenv("CLOUDINARY_URL", cloudURL)
	cld, err := cloudinary.New()
	if err != nil {
		return nil, err
	}
	cld.Config.URL.Secure = true
	return cld, nil
}

func UploadRemote(ctx context.Context, sourceURL, publicID, resourceType string) (map[string]any, error) {
	if strings.TrimSpace(sourceURL) == "" {
		return nil, errors.New("source_url is required")
	}
	if strings.TrimSpace(publicID) == "" {
		publicID = fmt.Sprintf("adkbot_%d", time.Now().Unix())
	}
	if strings.TrimSpace(resourceType) == "" {
		resourceType = "image"
	}

	cld, err := cloudinaryClient()
	if err != nil {
		return nil, err
	}
	resp, err := cld.Upload.Upload(ctx, sourceURL, uploader.UploadParams{
		PublicID:       publicID,
		ResourceType:   resourceType,
		Folder:         "adkbot",
		UniqueFilename: api.Bool(false),
		Overwrite:      api.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"public_id":     resp.PublicID,
		"resource_type": resp.ResourceType,
		"secure_url":    resp.SecureURL,
		"bytes":         resp.Bytes,
	}, nil
}

func UploadBytes(ctx context.Context, data []byte, mimeType, publicID, resourceType string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, errors.New("upload data is empty")
	}
	if strings.TrimSpace(publicID) == "" {
		publicID = fmt.Sprintf("adkbot_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(resourceType) == "" {
		resourceType = "image"
	}

	cld, err := cloudinaryClient()
	if err != nil {
		return nil, err
	}
	resp, err := cld.Upload.Upload(ctx, bytes.NewReader(data), uploader.UploadParams{
		PublicID:       publicID,
		ResourceType:   resourceType,
		Folder:         "adkbot",
		UniqueFilename: api.Bool(false),
		Overwrite:      api.Bool(true),
		Format:         strings.TrimPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/"),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"public_id":     resp.PublicID,
		"resource_type": resp.ResourceType,
		"secure_url":    resp.SecureURL,
		"bytes":         resp.Bytes,
	}, nil
}

func TransformURL(publicID, transformation, resourceType string) (map[string]any, error) {
	if strings.TrimSpace(publicID) == "" {
		return nil, errors.New("public_id is required")
	}
	if strings.TrimSpace(transformation) == "" {
		return nil, errors.New("transformation is required")
	}
	if strings.TrimSpace(resourceType) == "" {
		resourceType = "image"
	}

	cld, err := cloudinaryClient()
	if err != nil {
		return nil, err
	}

	switch resourceType {
	case "video":
		asset, err := cld.Video(publicID)
		if err != nil {
			return nil, err
		}
		asset.Transformation = transformation
		u, err := asset.String()
		if err != nil {
			return nil, err
		}
		return map[string]any{"url": u, "resource_type": resourceType, "public_id": publicID}, nil
	default:
		asset, err := cld.Image(publicID)
		if err != nil {
			return nil, err
		}
		asset.Transformation = transformation
		u, err := asset.String()
		if err != nil {
			return nil, err
		}
		return map[string]any{"url": u, "resource_type": "image", "public_id": publicID}, nil
	}
}
