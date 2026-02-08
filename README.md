# AgentScript 🚀

A DSL for orchestrating AI agents with Google integrations. Built for the **Gemini API Hackathon**.

## Quick Start

```bash
export GEMINI_API_KEY="your-key"

# Build
go build -o agentscript .

# Simple search
./agentscript -e 'search "golang best practices" -> summarize -> save "golang.md"'

# Parallel comparison
./agentscript -e 'parallel { search "React" -> analyze search "Vue" -> analyze } -> merge -> ask "which is better?"'

# From file
./agentscript -f examples/competitor-analysis.as

# Natural language (translates to DSL)
./agentscript -n "compare AWS and Azure and email me the results"

# Interactive REPL
./agentscript -i
```

## DSL Syntax

### Basic Commands
```bash
search "query"              # Search via Gemini
summarize                   # Summarize input
ask "question"              # Ask with context
analyze "focus"             # Analyze with focus
save "file.md"              # Save to file
read "file.txt"             # Read from file
list "."                    # List directory
```

### Pipelines
```bash
search "topic" -> summarize -> save "output.md"
read "notes.txt" -> ask "what are the key points?" -> save "summary.md"
```

### Parallel Execution
```bash
parallel {
    search "Google" -> analyze "strengths"
    search "Microsoft" -> analyze "strengths"
} -> merge -> ask "compare these companies" -> save "comparison.md"
```

### Nested Parallel
```bash
parallel {
    parallel {
        search "AWS" -> analyze
        search "Azure" -> analyze
        search "GCP" -> analyze
    } -> merge -> ask "summarize cloud providers"
    
    parallel {
        search "PostgreSQL" -> analyze
        search "MongoDB" -> analyze
    } -> merge -> ask "summarize databases"
} -> merge -> ask "infrastructure recommendation" -> save "report.md"
```

## Google Integrations

### Setup
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create project, enable APIs: Gmail, Calendar, Drive, Docs, Sheets, Tasks, YouTube
3. Create OAuth credentials (Desktop app)
4. Download as `credentials.json`

### Commands
```bash
# Gmail
search "report" -> email "boss@company.com"

# Calendar
search "agenda" -> calendar "Team Meeting Monday 10am"

# Google Meet
search "topics" -> meet "Sprint Review Friday 2pm"

# Google Drive
search "analysis" -> drive_save "Reports/analysis.md"

# Google Docs
search "research" -> doc_create "Research Report"

# Google Sheets
search "data" -> sheet_create "Data Tracking"
search "metrics" -> sheet_append "spreadsheet_id/Sheet1"

# Google Tasks
search "todos" -> task "Review action items"

# YouTube
youtube_search "golang tutorial" -> summarize -> save "videos.md"

# Contacts
contact_find "John Smith"
```

## Multimodal (Image & Video)

```bash
# Generate an image
image_generate "a futuristic city skyline at sunset, cyberpunk style"

# Generate image from research context
search "minimalist logo design trends" -> image_generate "modern minimalist tech startup logo"

# Analyze an image
image_analyze "photo.jpg" -> save "image-description.md"

# Analyze with custom prompt
image_analyze "product.png" -> ask "what improvements would you suggest for this UI?"

# Analyze video
video_analyze "demo.mp4" -> summarize -> save "video-notes.md"

# Generate video from text prompt
video_generate "a serene lake at sunrise with mist, cinematic drone shot"

# Create video from multiple images (space or comma separated)
images_to_video "photo1.jpg photo2.jpg photo3.jpg"

# Full creative pipeline
search "product marketing video styles" -> image_generate "sleek product on white background" -> video_generate "product rotating slowly, professional lighting"

# Compare images
parallel {
    image_analyze "design1.jpg" -> analyze "style"
    image_analyze "design2.jpg" -> analyze "style"
} -> merge -> ask "which design is better and why?"
```

### Full Workflow Example
```bash
parallel {
    search "Q1 sales data" -> analyze "trends"
    search "competitor analysis" -> analyze "threats"
    search "market forecast" -> analyze "opportunities"
} -> merge -> ask "create executive summary" -> doc_create "Q1 Board Report" -> meet "Board Meeting Friday 9am" -> email "board@company.com"
```

## All Commands

| Command | Args | Description |
|---------|------|-------------|
| `search` | "query" | Search via Gemini |
| `summarize` | - | Summarize input |
| `ask` | "question" | Ask question with context |
| `analyze` | "focus" | Analyze with optional focus |
| `save` | "file" | Save to local file |
| `read` | "file" | Read from local file |
| `list` | "path" | List directory |
| `merge` | - | Combine parallel results |
| `email` | "to@email" | Send via Gmail |
| `calendar` | "event info" | Create calendar event |
| `meet` | "meeting info" | Create Meet with link |
| `drive_save` | "path/file" | Save to Google Drive |
| `doc_create` | "title" | Create Google Doc |
| `sheet_create` | "title" | Create Google Sheet |
| `sheet_append` | "id/sheet" | Append to Sheet |
| `task` | "todo" | Create Google Task |
| `contact_find` | "name" | Search contacts |
| `youtube_search` | "query" | Search YouTube |
| `image_generate` | "prompt" | Generate image with Imagen |
| `image_analyze` | "file.jpg" | Analyze image with Gemini |
| `video_analyze` | "file.mp4" | Analyze video with Gemini |
| `video_generate` | "prompt" | Generate video with Veo |
| `images_to_video` | "img1 img2" | Create video from images |
| `parallel { }` | commands | Run concurrently | |
| `youtube_search` | "query" | Search YouTube |
| `parallel { }` | commands | Run concurrently |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Natural Language                         │
│                "compare AWS and Azure"                       │
└─────────────────────────┬───────────────────────────────────┘
                          │ Gemini translates
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      AgentScript DSL                         │
│  parallel { search "AWS" -> analyze                          │
│             search "Azure" -> analyze } -> merge -> ask      │
└─────────────────────────┬───────────────────────────────────┘
                          │ Participle parser
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                          AST                                 │
└─────────────────────────┬───────────────────────────────────┘
                          │ Runtime executes
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                    Parallel Execution                        │
│  ┌─────────┐         ┌─────────┐                            │
│  │ Branch1 │  sync   │ Branch2 │                            │
│  │ AWS     │ ─────── │ Azure   │                            │
│  └────┬────┘ WaitGrp └────┬────┘                            │
│       └────────┬──────────┘                                 │
│                ▼                                             │
│           merge → ask                                        │
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

```
agentscript/
├── main.go          # CLI & REPL
├── grammar.go       # Participle parser
├── runtime.go       # Execution engine
├── google.go        # Google API integrations
├── translator.go    # Natural language → DSL
├── client.go        # Gemini API client
└── examples/
    ├── simple-research.as
    ├── competitor-analysis.as
    ├── ai-comparison.as
    ├── ev-market-report.as
    ├── executive-report.as
    ├── google-workflow.as
    └── nested-parallel.as
```

## License

MIT

---

Built with 🚀 for the Gemini API Hackathon
