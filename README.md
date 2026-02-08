# AgentScript DSL

**A simple, powerful DSL for orchestrating AI agents with Gemini 3**

AgentScript lets you build complex AI workflows in a single line of code. Think "LangGraph without the Python boilerplate."

## 🚀 Quick Start

```bash
# Set your API key
export GEMINI_API_KEY="your-key"

# Run a simple query
make run EXPR='search "AI trends" -> summarize'

# Run from file
make run-file FILE=examples/showcase.as
```

## 📖 Language Overview

### Core Syntax

```
command "argument" -> command "argument" -> command "argument"
```

Commands are chained with `->` (pipe). Each command's output becomes the next command's input.

### Parallel Execution

```
parallel {
    branch1 -> commands
    branch2 -> commands
} -> merge -> next_command
```

Run multiple branches concurrently. Use `merge` to combine results.

### Nested Parallel (Unique Feature!)

```
parallel {
    parallel {
        search "topic A"
        search "topic B"
    } -> merge -> summarize
    
    parallel {
        search "topic C"
        search "topic D"  
    } -> merge -> summarize
} -> merge -> ask "synthesize all findings"
```

## 🛠 All 25 Commands

### Core Commands (8)
| Command | Description | Example |
|---------|-------------|---------|
| `search` | Web search via Gemini | `search "AI news"` |
| `summarize` | Summarize piped input | `search "topic" -> summarize` |
| `ask` | Query Gemini with context | `ask "explain this"` |
| `analyze` | Deep analysis | `read "data.csv" -> analyze` |
| `save` | Save to file | `-> save "output.md"` |
| `read` | Read file contents | `read "input.txt" -> summarize` |
| `list` | List directory | `list "."` |
| `merge` | Combine parallel outputs | `parallel { ... } -> merge` |

### Google Workspace Commands (10)
| Command | Description | Example |
|---------|-------------|---------|
| `email` | Send email via Gmail | `-> email "user@gmail.com"` |
| `calendar` | Create calendar event | `calendar "Meeting tomorrow 2pm"` |
| `meet` | Create Google Meet | `meet "Team sync"` |
| `drive_save` | Upload to Google Drive | `-> drive_save "report"` |
| `doc_create` | Create Google Doc | `-> doc_create "My Document"` |
| `sheet_create` | Create spreadsheet | `sheet_create "Budget 2026"` |
| `sheet_append` | Append to sheet | `-> sheet_append "sheet_id"` |
| `task` | Create Google Task | `task "Review PR by Friday"` |
| `contact_find` | Search contacts | `contact_find "John"` |
| `youtube_search` | Search YouTube | `youtube_search "AI tutorials"` |

### Multimodal Commands (5)
| Command | Description | Example |
|---------|-------------|---------|
| `image_generate` | Generate image (Imagen 4) | `image_generate "sunset over mountains"` |
| `image_analyze` | Analyze image | `image_analyze "photo.jpg"` |
| `video_generate` | Generate video (Veo 3.1) | `video_generate "drone over city"` |
| `video_analyze` | Analyze video | `video_analyze "clip.mp4"` |
| `images_to_video` | Images to video transition | `images_to_video "a.png b.png"` |

### Control Commands (1)
| Command | Description | Example |
|---------|-------------|---------|
| `parallel` | Concurrent execution | `parallel { branch1 branch2 }` |

## 🎬 Example Workflows

### 1. Simple Research
```bash
make run EXPR='search "quantum computing 2026" -> summarize -> save "quantum.md"'
```

### 2. Parallel Research with Email
```bash
make run EXPR='parallel { search "AI safety" -> summarize search "AI ethics" -> summarize } -> merge -> ask "synthesize into report" -> email "team@company.com"'
```

### 3. Image Generation Pipeline
```bash
make run EXPR='image_generate "cyberpunk cityscape at night" -> save "city.png"'
```

### 4. Video Generation
```bash
make run EXPR='video_generate "drone flying over mountains at sunset, cinematic" -> save "drone.mp4"'
```

### 5. 🦋 Full Multimodal Pipeline (Showcase!)
```bash
make run EXPR='parallel { image_generate "mountain lake at sunrise, butterflies" -> save "dawn.png" image_generate "mountain lake at sunset, butterflies" -> save "sunset.png" } -> merge -> images_to_video "dawn.png sunset.png" -> save "butterflies.mp4" -> drive_save "butterflies" -> email "you@gmail.com"'
```

This single command:
1. Generates 2 images in parallel (Imagen 4)
2. Creates a video transitioning between them (Veo 3.1)
3. Saves locally
4. Uploads to Google Drive
5. Emails you the link

### 6. Competitor Analysis
```bash
make run-file FILE=examples/competitor-analysis.as
```

### 7. Nested Parallel Research
```bash
make run-file FILE=examples/nested-parallel.as
```

## 🔧 Setup

### Prerequisites
- Go 1.22+
- Gemini API key

### Installation
```bash
git clone https://github.com/vinodhalaharvi/agentscript
cd agentscript
go mod tidy
```

### Environment Variables
```bash
# Required
export GEMINI_API_KEY="your-gemini-api-key"

# Optional: Google Workspace integration
export GOOGLE_CREDENTIALS_FILE="credentials.json"

# Optional: Web search
export SERPAPI_KEY="your-serpapi-key"
```

### Google OAuth Setup (for Workspace commands)
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create OAuth 2.0 credentials (Desktop app)
3. Download as `credentials.json`
4. Run any Google command - browser will open for authorization

## 📁 Project Structure
```
agentscript/
├── main.go           # CLI entry point
├── grammar.go        # Participle grammar definition
├── runtime.go        # Command execution engine
├── client.go         # Gemini API client (Imagen 4, Veo 3.1)
├── google.go         # Google Workspace integrations
├── translator.go     # Natural language to DSL
├── Makefile          # Build commands
└── examples/         # Example .as files
    ├── showcase.as           # Butterflies video pipeline
    ├── full-demo.as          # All features demo
    ├── simple-research.as    # Basic search
    ├── nested-parallel.as    # 3-level nesting
    ├── multimodal.as         # Image/video generation
    ├── google-workflow.as    # Workspace integration
    └── competitor-analysis.as
```

## 🏃 Running

```bash
# Expression mode
make run EXPR='search "topic" -> summarize'

# File mode
make run-file FILE=examples/showcase.as

# REPL mode
make repl

# Natural language mode
make natural
> "research AI and email me a summary"
```

## 🎯 Why AgentScript?

| Feature | LangGraph | AgentScript |
|---------|-----------|-------------|
| Lines for parallel research | 50+ | 1 |
| Setup complexity | High | Low |
| Nested parallelism | Complex | Native |
| Google Workspace | Manual | Built-in |
| Multimodal (Imagen/Veo) | Manual | Built-in |
| Learning curve | Steep | Minutes |

## 🏆 Built for Gemini 3 Hackathon

AgentScript demonstrates:
- **Gemini 3** for intelligent text processing
- **Imagen 4** for image generation
- **Veo 3.1** for video generation (including image-to-video)
- **10 Google Workspace APIs** integrated
- **Nested parallel execution** for complex workflows
- **Single-line DSL** replacing 100s of lines of code

## 📜 License

MIT

---

**Made with ❤️ for the Gemini 3 Hackathon**
