# AgentScript

**A domain-specific language for chaining AI and APIs in simple, readable pipelines.**

One line replaces hundreds of lines of code.

```
search "AI trends" >=> summarize >=> email "you@gmail.com"
```

## What It Does

AgentScript lets you chain Gemini AI with 100+ commands using Morpheus category-theory operators. Research topics, generate images and videos, send emails, check stocks, monitor Reddit, read RSS feeds, get weather forecasts, search jobs, run MCP tools, query knowledge graphs — all in one script.

```bash
# Morning briefing in one command
( weather "San Francisco"
  <*> crypto "BTC,ETH,SOL"
  <*> stock "AAPL,NVDA,MSFT"
  <*> news_headlines "technology"
  <*> rss "hn"
  <*> reddit "r/golang"
  <*> job_search "golang contract" "remote"
)
>=> merge
>=> ask "morning briefing with weather, markets, headlines, and jobs"
>=> notify "slack"
>=> email "you@gmail.com"
```

## Quick Start

```bash
git clone https://github.com/vinodhalaharvi/agentscript
cd agentscript
go mod tidy
go build -o agentscript ./cmd/agentscript

export GEMINI_API_KEY="your-key"

# Try it
./agentscript -e 'ask "hello world"'
./agentscript -e 'search "golang trends" >=> summarize'
./agentscript -e 'crypto "BTC,ETH,SOL"'
./agentscript -e 'weather "New York"'
```

## The Grammar

```
Program     = Statement*
Statement   = ( "(" Parallel ")" | Command ) ( ">=>" Statement )?
Parallel    = Statement ( "<*>" Statement )*
Command     = Action String*
```

Two operators define the entire language:

| Operator | Meaning |
|----------|---------|
| `>=>` | Kleisli composition — sequential pipeline stage |
| `<*>` | Fan-out — run branches concurrently (inside parentheses) |

## Commands

AgentScript ships 100+ commands. The most common are grouped below; the full keyword list lives in `internal/agentscript/grammar.go`.

### Core
| Command | Example | Description |
|---------|---------|-------------|
| `search` | `search "topic"` | Web search via Gemini |
| `summarize` | `>=> summarize` | Summarize piped input |
| `ask` | `>=> ask "question"` | Ask with context |
| `analyze` | `>=> analyze "focus"` | Deep analysis |
| `save` | `>=> save "file.md"` | Save to local file |
| `read` | `read "file.txt"` | Read local file |
| `list` | `list "."` | List directory |
| `merge` | `>=> merge` | Merge fan-out results |

### Data
| Command | Example | API | Key? |
|---------|---------|-----|------|
| `weather` | `weather "NYC"` | Open-Meteo | **No** |
| `crypto` | `crypto "BTC,ETH"` | CoinGecko | **No** |
| `reddit` | `reddit "r/golang"` | Reddit JSON | **No** |
| `rss` | `rss "hn"` | Direct HTTP | **No** |
| `news` | `news "AI"` | GNews | Yes |
| `news_headlines` | `news_headlines "tech"` | GNews | Yes |
| `stock` | `stock "AAPL,NVDA"` | Finnhub | Yes |
| `job_search` | `job_search "golang"` | SerpAPI | Yes |
| `twitter` | `twitter "golang"` | Twitter API | Yes |

### Google Workspace
| Command | Example | API |
|---------|---------|-----|
| `email` | `>=> email "to@gmail.com"` | Gmail |
| `calendar` | `>=> calendar "Meeting 2pm"` | Calendar |
| `meet` | `>=> meet "Sprint Review"` | Calendar+Meet |
| `drive_save` | `>=> drive_save "path/file"` | Drive |
| `doc_create` | `>=> doc_create "Title"` | Docs |
| `sheet_create` | `>=> sheet_create "Title"` | Sheets |
| `sheet_append` | `>=> sheet_append "id/sheet"` | Sheets |
| `task` | `>=> task "todo"` | Tasks |
| `contact_find` | `contact_find "John"` | People |
| `youtube_search` | `youtube_search "query"` | YouTube |

### Multimodal (Gemini)
| Command | Example | Model |
|---------|---------|-------|
| `image_generate` | `image_generate "robot"` | Imagen 4 |
| `image_analyze` | `image_analyze "describe"` | Gemini |
| `video_generate` | `video_generate "sunset"` | Veo 3.1 |
| `video_analyze` | `video_analyze "summarize"` | Gemini |
| `images_to_video` | `>=> images_to_video` | ffmpeg |
| `text_to_speech` | `>=> text_to_speech "en"` | Gemini TTS |
| `translate` | `>=> translate "Japanese"` | Gemini |

### Notifications
| Command | Example | Service |
|---------|---------|---------|
| `email` | `>=> email "you@gmail.com"` | Gmail |
| `notify` | `>=> notify "slack"` | Slack/Discord/Telegram |
| `whatsapp` | `>=> whatsapp "+1234567890"` | Twilio |

### Control Flow
| Command | Example | Description |
|---------|---------|-------------|
| `( <*> )` | `( a <*> b <*> c )` | Run branches concurrently |
| `if` | `if "rain > 50"` | Conditional execution |
| `foreach` | `>=> foreach "line"` | Iterate over items |
| `match` | `>=> match` | Pattern-match on piped input |
| `fmap` / `pfmap` | `>=> fmap "cmd"` | Map a command over items (parallel variant: `pfmap`) |

### MCP (Model Context Protocol)
| Command | Example | Description |
|---------|---------|-------------|
| `mcp_connect` | `mcp_connect "name" "cmd"` | Connect to an MCP server |
| `mcp` | `mcp "server:tool" "args"` | Call an MCP tool |
| `mcp_list` | `mcp_list` | List connected servers and tools |
| `mcp_search` | `mcp_search "query"` | Search the MCP registry |
| `mcp_agent` | `>=> mcp_agent "task"` | AI-driven MCP tool selection |

### Knowledge & Retrieval
| Command | Example | Description |
|---------|---------|-------------|
| `rag_connect` / `rag_index` / `rag_query` | `rag_query "question"` | Postgres + LLM RAG pipeline |
| `kg_extract` / `kg_query` / `kg_cypher` | `kg_query "question"` | Knowledge graph + GraphRAG |
| `perplexity` | `perplexity "query"` | Perplexity-backed search (`_pro`, `_recent`, `_domain` variants) |

### AI Backends & Tooling
| Command | Example | Description |
|---------|---------|-------------|
| `claude` | `>=> claude "prompt"` | Claude (Anthropic) completion |
| `ollama` | `>=> ollama "prompt"` | Local Ollama model |
| `agent` | `>=> agent "task"` | Natural-language → DSL agent |
| `codereview` | `>=> codereview` | Multi-model code-review debate |
| `hf_*` | `hf_generate "prompt"` | Hugging Face inference (generate, classify, NER, QA, embeddings, image, speech, ...) |

### Infrastructure
| Command | Example | Description |
|---------|---------|-------------|
| `exec` | `>=> exec "go build"` | Run a shell command in a pipeline |
| `ssl_check` / `ping` / `dns_lookup` / `http_check` / `whois` | `ssl_check "example.com"` | Network diagnostics (pure Go) |
| `deploy` / `schedule` / `undeploy` | `>=> deploy` | Cloud Run job deploy + Cloud Scheduler |
| `pdf_fields` / `pdf_fill` | `>=> pdf_fill "form.pdf"` | AI-powered PDF form filling |
| `github_pages_html` | `>=> github_pages_html "Title"` | Deploy HTML to GitHub Pages |

## Example Pipelines

### Daily Job Hunt
```
( job_search "golang contract" "remote"
  <*> job_search "go microservices" "remote"
)
>=> merge
>=> ask "deduplicate, format as table, sort by rate"
>=> email "you@gmail.com"
```

### Stock Alert
```
stock "NVDA" >=> if "change > 5" >=> notify "slack"
```

### Tech Digest
```
( rss "hn"
  <*> rss "lobsters"
  <*> reddit "r/golang" "top"
  <*> news_headlines "technology"
)
>=> merge >=> summarize >=> email "you@gmail.com"
```

### Multimodal Pipeline
```
search "butterflies migration"
>=> summarize
>=> text_to_speech "en"
>=> image_generate "monarch butterflies migrating"
>=> images_to_video
>=> youtube_upload "Butterfly Migration"
```

### Nested Fan-out
```
( ( search "React pros cons" >=> analyze
    <*> search "Vue pros cons" >=> analyze
    <*> search "Angular pros cons" >=> analyze
  ) >=> merge >=> ask "summarize frontend frameworks"
  <*> ( search "Node.js backend" >=> analyze
        <*> search "Go backend" >=> analyze
      ) >=> merge >=> ask "summarize backend options"
)
>=> merge
>=> ask "full-stack recommendation"
>=> save "recommendation.md"
```

### Natural Language Mode
```bash
./agentscript -n "find golang jobs and email me a summary"
# Gemini translates English -> DSL -> executes
```

## RSS Feed Shortcuts

No URL needed — just use the shortcut name:

| Shortcut | Feed |
|----------|------|
| `hn` | Hacker News |
| `golang` | Go Blog |
| `lobsters` | Lobste.rs |
| `techcrunch` | TechCrunch |
| `kubernetes` | Kubernetes Blog |
| `anthropic` | Anthropic Blog |
| `github-blog` | GitHub Blog |
| `reddit-golang` | r/golang RSS |
| `dev-to` | Dev.to |

30+ shortcuts available. Run `rss` with no args to see all.

## Architecture

```
Natural Language ─── Gemini/Claude translates ──→ AgentScript DSL
                                                      │
                                                 Participle parser
                                                 (Morpheus grammar)
                                                      │
                                                     AST
                                                      │
                                                Runtime executor
                                                ┌─────┴─────┐
                                           Sequential    Fan-out
                                             (>=>)        (<*>)
                                                │            │
                                          Plugin registry  goroutines
                                                │
                          ┌──────────┬──────────┼──────────┬──────────┐
                       Gemini      MCP         data       cloud      AI
                        APIs      servers     plugins    plugins   backends
```

Each integration is a **plugin** that registers its DSL commands with the
runtime. Plugins live under `plugins/<vendor>/`; shared API clients and the
plugin interface live under `pkg/`.

### DSL → Sibyl translator (in progress)

A second execution path is being built: an arrow-first translator that
compiles AgentScript into [Sibyl](https://github.com/vinodhalaharvi/sibyl)
DAGs for durable, Temporal-backed execution. The design is documented in
[`docs/dsl-to-sibyl-translator.md`](docs/dsl-to-sibyl-translator.md); the
translator code is under `internal/agentscript/script/`. The existing
in-process runtime continues to serve as the fast "memory" backend.

## Project Structure

```
agentscript/
├── cmd/
│   ├── agentscript/       # CLI entry point (-e, -f, -i, -n modes)
│   └── geminilive/        # Gemini live audio demo
├── internal/agentscript/  # Runtime, grammar, registry, translator
│   ├── grammar.go         # Morpheus DSL grammar (>=>, <*>, parentheses)
│   ├── runtime.go         # Command execution engine
│   ├── registry.go        # Plugin registration
│   ├── translator.go      # Natural language → DSL
│   └── script/            # DSL → Sibyl translator (Parse, Resolve, ...)
├── pkg/                   # Shared clients + plugin interface
│   ├── plugin/            # The plugin.Plugin contract
│   ├── claude/            # Claude API client
│   ├── gemini/            # Gemini API client
│   ├── geminilive/        # Gemini live client
│   ├── google/            # Google Workspace helpers
│   └── openai/            # OpenAI API client
├── plugins/               # One directory per integration
│   ├── weather/ crypto/ stock/ news/ reddit/ rss/ twitter/ jobsearch/
│   ├── mcp/ mcpagent/ mcpsearch/ rag/ kg/ perplexity/ huggingface/
│   ├── github/ cloudrun/ network/ pdffill/ datatable/ ollama/
│   ├── agent/ review/ plugagent/ notify/ whatsapp/ exec/
│   └── cache/             # Shared file-based result cache
├── docs/
│   └── dsl-to-sibyl-translator.md
├── examples/              # *.as scripts
└── Makefile
```

## Environment Variables

```bash
# Required
export GEMINI_API_KEY="..."

# Data APIs (all have free tiers)
export SERPAPI_KEY="..."           # Jobs, news/stock fallback (100/mo free)
export FINNHUB_API_KEY="..."      # Stocks (60/min free)
export GNEWS_API_KEY="..."        # News (100/day free)
export TWITTER_BEARER_TOKEN="..." # Twitter search

# Google Workspace (OAuth)
export GOOGLE_CREDENTIALS_FILE="credentials.json"

# Notifications
export SLACK_WEBHOOK_URL="..."
export DISCORD_WEBHOOK_URL="..."
export TELEGRAM_BOT_TOKEN="..."
export TELEGRAM_CHAT_ID="..."

# WhatsApp (Twilio)
export TWILIO_ACCOUNT_SID="..."
export TWILIO_AUTH_TOKEN="..."
export TWILIO_WHATSAPP_FROM="whatsapp:+14155238886"

# 4 commands need ZERO keys: weather, crypto, reddit, rss
```

## Running

```bash
# Expression mode
./agentscript -e 'search "topic" >=> summarize'

# File mode
./agentscript -f examples/daily-briefing.as

# REPL mode
./agentscript -i

# Natural language mode
./agentscript -n "research AI and email me a summary"

# Verbose mode (debug)
./agentscript -v -e 'crypto "BTC"'
```

## Testing

```bash
# Run the whole suite
go test ./...

# Race detector (matches CI)
go test -race ./...

# A single package
go test ./internal/agentscript/script/...
```

CI (GitHub Actions) runs `go mod tidy`, `go vet -structtag=false`, `gofmt`,
`staticcheck`, `go test -race`, and `go build` on every push and PR. See
`.github/workflows/ci.yml`.

## Built With

- [Go 1.23](https://golang.org/) — Runtime engine
- [Participle v2](https://github.com/alecthomas/participle) — Parser generator
- [Gemini API](https://ai.google.dev/) — Text, Imagen 4, Veo 3.1, TTS
- [Claude API](https://www.anthropic.com/) — Completions, code review, agents
- [Model Context Protocol](https://modelcontextprotocol.io/) — Tool servers
- [Google Workspace APIs](https://developers.google.com/workspace) — Gmail, Calendar, Drive, Docs, Sheets, Forms, YouTube, Tasks
- [Open-Meteo](https://open-meteo.com/) — Weather (free)
- [CoinGecko](https://www.coingecko.com/) — Crypto prices (free)
- [SerpAPI](https://serpapi.com/) — Job search, news, finance
- [Finnhub](https://finnhub.io/) — Stock quotes
- [GNews](https://gnews.io/) — News articles
- [Twilio](https://www.twilio.com/) — WhatsApp messaging

## Links

- **Demo:** https://youtube.com/watch?v=6yJa5LiUS9U
- **DevPost:** https://devpost.com/software/agent-script
- **Author:** [Vinod Halaharvi](https://github.com/vinodhalaharvi)

## License

MIT
