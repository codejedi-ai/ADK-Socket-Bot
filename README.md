# adkgobot

`adkgobot` is a Go websocket AI agent with a command structure inspired by gateway-first bot systems.

It includes:
- `adkgobot gateway start|stop|restart`
- `adkgobot tui` for interactive chat
- `adkgobot onboard` for provider and API key onboarding
- Tooling support (`time_now`, `http_get`, `shell_exec`, `echo`)
- Media tooling: Cloudinary upload/transform + Gemini image generation
- Gemini-backed responses through the Google Generative Language API

## Requirements

- Go 1.25+
- Onboarding setup via `adkgobot onboard`

## Quick Start

```bash
go mod tidy
go build ./cmd/adkgobot
./adkgobot onboard
./adkgobot gateway start
./adkgobot tui
```

## One-Command Install

```bash
curl -fsSL https://raw.githubusercontent.com/codejedi-ai/adkgobot/main/install.sh | bash
```

This installs prerequisites, installs `adkgobot`, then you run onboarding.

## Providers

- Google: fully implemented end-to-end.
- OpenAI: onboarding scaffold only (runtime integration pending).
- Anthropic: onboarding scaffold only (runtime integration pending).

## Media Tooling

Available tool names:

- `cloudinary_upload_remote`: upload remote image/video URL to Cloudinary.
- `cloudinary_transform_url`: build transformed delivery URL for image or video.
- `gemini_generate_image`: generate image data from prompt (returns base64 + data URL).
- `gemini_generate_video_job`: starts a Google video generation job using your configured endpoint/token.

Direct CLI usage is also Go-native:

```bash
adkgobot image --prompt "cinematic neon city" --output city.png
adkgobot image --prompt "studio portrait" --upload --public-id portrait_demo
```

Onboarding now supports storing `CLOUDINARY_URL` in `~/.adkgobot/config.json`.

For video job start (Google), set:

- `GOOGLE_VIDEO_GEN_ENDPOINT`
- `GOOGLE_OAUTH_ACCESS_TOKEN`

## Gateway Commands

```bash
./adkgobot gateway start --host 127.0.0.1 --port 8080
./adkgobot gateway stop
./adkgobot gateway restart
```

Foreground mode:

```bash
./adkgobot gateway start --detach=false
```

## TUI

```bash
./adkgobot tui --ws ws://127.0.0.1:8080/ws
```

Type `/quit` to exit.

## Websocket Protocol

See `docs/MANUAL.md` for message formats and examples.
