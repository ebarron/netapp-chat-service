# netapp-chat-service

A standalone, product-agnostic chat service that provides an agentic tool-use loop powered by LLMs and MCP (Model Context Protocol) tool servers.

## Features

- **Multi-provider LLM support**: OpenAI, Anthropic, AWS Bedrock, custom endpoints
- **MCP tool routing**: Connect to multiple MCP servers with capability-based tool filtering
- **Interest system**: Pattern-matched prompts that scope tools to relevant capabilities
- **Autonomy modes**: Read-only, read-write, and per-capability ask/allow/off states
- **SSE streaming**: Real-time event streaming for chat responses
- **Tool approval workflow**: Ask-mode tools require user approval before execution

## Quick Start

```bash
# Build
go build -o chat-service ./cmd/chat-service

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your LLM provider and MCP server details

# Run
./chat-service -config config.yaml
```

## Configuration

See [config.example.yaml](config.example.yaml) for a complete example with comments.

Environment variables are expanded in the config file (e.g. `$ANTHROPIC_API_KEY`).

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/chat/message` | Send a message, receive SSE event stream |
| DELETE | `/chat/session` | Clear session history |
| GET | `/chat/capabilities` | List capabilities and tool counts |
| POST | `/chat/capabilities` | Update capability states |
| POST | `/chat/approve` | Approve a pending tool call |
| POST | `/chat/deny` | Deny a pending tool call |
| POST | `/chat/stop` | Cancel an in-progress chat |
| GET | `/health` | Health check |

## Docker

```bash
docker build -t chat-service .
docker run -p 8090:8090 -v ./config.yaml:/etc/chat-service/config.yaml chat-service
```

## Architecture

```
cmd/chat-service/       Main entrypoint
internal/
  agent/                Agentic tool-use loop orchestration
  capability/           Capability state model (off/ask/allow)
  config/               YAML configuration loading
  interest/             Interest pattern matching and catalog
  llm/                  LLM provider abstraction (OpenAI, Anthropic, Bedrock)
  mcpclient/            MCP server connection and tool routing
  render/               Output rendering helpers
  server/               HTTP server and chat handlers
  session/              Conversation session management
```
