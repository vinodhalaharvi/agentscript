# AgentScript Examples

Each `.as` file is a runnable AgentScript pipeline. Run one with:

```bash
agentscript -f examples/<name>.as
```

Most examples need `GEMINI_API_KEY`. Some need additional keys (Google
Workspace OAuth, Slack webhook, stock/news API keys) or an MCP server.
The tables below note the requirements per example.

> **Key-free starters:** `morning-briefing.as` and the `weather`/`crypto`/
> `reddit`/`rss` portions of pipelines run with no API keys at all. Start
> there if you just want to see the operators work.

## Getting started

| Example | What it does | Needs |
|---------|--------------|-------|
| `morning-briefing.as` | Weather + crypto + headlines fan-out, no keys for the free parts | Gemini |
| `simple-research.as` | Search → summarize, the canonical first pipeline | Gemini |
| `daily-briefing.as` | Full morning briefing: weather, crypto, stocks, news, HN, Reddit, jobs | Gemini + data keys |
| `tech-digest.as` | Multiple RSS/Reddit feeds → merge → summarize → email | Gemini + Google |
| `showcase.as` | Multimodal: parallel image generation → video | Gemini multimodal |

## Data & reports

| Example | What it does | Needs |
|---------|--------------|-------|
| `crypto-report.as` | Fetch prices, analyze, email | Gemini + Google |
| `mag7-report.as` | Magnificent 7 stock report → Slack | Gemini + Slack |
| `ev-market-report.as` | EV market analysis report | Gemini + Google |
| `sentiment-stock.as` | Sentiment-aware stock monitor | Gemini + stock key |
| `competitor-analysis.as` | Multi-source competitor research | Gemini |
| `executive-report.as` | Executive summary report | Gemini + Google |
| `go-job-hunt.as` | Go contract job search → formatted table | Gemini + SerpAPI |
| `nova-gigs.as` | Side-gig hunter for a specific area | Gemini |

## Multimodal (Gemini Imagen / Veo / TTS)

| Example | What it does | Needs |
|---------|--------------|-------|
| `dc-art.as` | Generate 5 AI art images of DC landmarks → Slack | Gemini multimodal + Slack |
| `multimodal.as` | Image + audio + video pipeline | Gemini multimodal |
| `youtube-shorts.as` | Research → script → TTS → video → YouTube | Gemini multimodal + Google |
| `news-shorts.as` / `local-news-shorts.as` / `news-2min.as` | News → narrated short video | Gemini multimodal |
| `mega-showcase.as` | The kitchen-sink demo of most commands | Gemini + Google + multimodal |
| `full-demo.as` | Broad capability demo | Gemini + Google + multimodal |

## MCP (Model Context Protocol)

These connect to an external MCP server via `mcp_connect`, then drive it
with `mcp` or `mcp_agent`. The relevant server is launched with `npx`/`uvx`
as written in each script.

| Example | MCP server | What it does |
|---------|-----------|--------------|
| `mcp-filesystem.as` | filesystem | Read/inspect local files |
| `mcp-document-code.as` | filesystem | Document a codebase |
| `mcp-fetch-news.as` | fetch | Fetch and summarize web content |
| `mcp-github.as` / `mcp-github-pages.as` | github | Create repos, push files, Pages |
| `mcp-vite-project.as` | github | Scaffold a Vite + React app into a new repo |
| `mcp-sqlite-report.as` | sqlite | Query a database and report |
| `sqlite-maps.as` / `sqlite-maps-slack.as` | sqlite + maps | Combine DB + maps data |
| `mcp-maps-*.as` | maps | Commute, compare, explore, research, restaurants, visual |
| `airbnb-search.as` / `airbnb-maps.as` | airbnb | Listings search and mapping |
| `ha-*.as` | Home Assistant | Control lights, list tools, demo control |
| `ring-*.as` | Home Assistant | Ring doorbell status, motion, siren, arm |
| `review-to-issue.as` | github | Code review → file an issue |

## Google Workspace

| Example | What it does | Needs |
|---------|--------------|-------|
| `calendar-demo.as` | Create calendar events | Google OAuth |
| `google-workflow.as` | Multi-service Workspace pipeline | Google OAuth |
| `grand-canyon-trip.as` / `quick-trip.as` / `travel-planner.as` | Trip planning → Workspace | Gemini + Google |
| `multi-news.as` / `tech-pulse.as` | News aggregation → email/docs | Gemini + Google |

## Infrastructure & utilities

| Example | What it does | Needs |
|---------|--------------|-------|
| `network-dashboard.as` / `network-dashboard-full.as` | SSL/ping/DNS/HTTP diagnostics → dashboard | Gemini |
| `ssl-monitor.as` / `ssl-dashboard.as` | Monitor TLS certs, render a dashboard | Gemini |
| `pdf-inspect.as` | Inspect a PDF's form fields | none |
| `pdf-fill.as` / `pdf-fill-advanced.as` | AI-fill a PDF form | Gemini |
| `nested-parallel.as` | Demonstrates nested `<*>` fan-out | Gemini |
| `ai-comparison.as` | Compare AI model outputs | Gemini |
| `youtube-research.as` | Research a topic via YouTube | Gemini + Google |

## A note on examples and the grammar

Every example here parses against the current grammar
(`internal/agentscript/grammar.go`). If you add a new example that uses a
command, make sure that command is registered — the CLI will report
`unknown action` otherwise. The operators are just two: `>=>` for sequential
stages and `<*>` for parallel fan-out inside parentheses. See the top-level
[README](../README.md) for the full command reference.
