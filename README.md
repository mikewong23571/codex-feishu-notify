# Codex Feishu Notify

Go-based notifier that transforms Codex `notify` events into Feishu (Lark) interactive card messages.

## Requirements
- Go 1.21+ to build from source.
- A Feishu custom bot webhook URL and optional secret (enable signature verification in the bot’s security settings).

## Configuration
Create a `.env` file in the project root (or alongside the installed binary) with:

```
FEISHU_WEBHOOK_URL=https://open.feishu.cn/open-apis/bot/v2/hook/<your-webhook-id>
FEISHU_SECRET=optional-secret-if-enabled
```

Because `.bashrc` automatically sources `./.env`, starting a new shell (or running `source ~/.bashrc`) will export those variables for Codex. When the secret is empty, signature verification is skipped automatically.

## Build

```bash
go build -o codex-feishu-notify ./codex_feishu.go
```

Copy the resulting binary anywhere on your `PATH` (e.g. `~/.codex/bin`) so Codex can invoke it directly.

## Codex Integration

Edit `~/.codex/config.toml` and set:

```toml
notify = ["/home/<user>/.codex/bin/codex-feishu-notify"]
```

Codex will execute the binary for every `agent-turn-complete` event, passing a single JSON string argument. The notifier parses the payload, builds a Feishu card with input messages, execution summary, and session metadata, signs the request if a secret is configured, and posts it to the configured webhook.

## Testing Locally

You can simulate a Codex event with:

```bash
./codex-feishu-notify '{"type":"agent-turn-complete","thread-id":"demo","turn-id":"1","cwd":"/tmp","input-messages":["demo task"],"last-assistant-message":"all done"}'
```

If the webhook returns an error (e.g., signature mismatch), the process exits non-zero with the Feishu error code for easier troubleshooting.
