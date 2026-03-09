package main

import (
	"github.com/codejedi-ai/adkgobot/internal/agent/tools"
	"context"
	"fmt"
	"log"
)

func main() {
	registry := tools.NewRegistry()
	ctx := context.Background()

	args := map[string]any{
		"prompt":               "A futuristic city under a neon sky, cinematic lighting, digital art style",
		"channel":              "cloudinary",
		"cloudinary_public_id": "test_image_gen",
	}

	fmt.Println("Testing image_generate tool...")
	result, err := registry.Execute(ctx, "image_generate", args)
	if err != nil {
		log.Fatalf("Error executing image_generate: %v", err)
	}

	fmt.Printf("Result: %+v\n", result)
}
