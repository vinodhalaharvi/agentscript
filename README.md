# AgentScript DSL

**A simple, powerful DSL for orchestrating AI agents with Gemini 3 + Claude**

AgentScript lets you build complex AI workflows in a single line of code. Think "LangGraph without the Python boilerplate."

## 🚀 Quick Start

```bash
# Set your API keys
export GEMINI_API_KEY="your-gemini-key"
export CLAUDE_API_KEY="your-anthropic-key"  # Optional, for React SPA generation

# Run a simple query
make run EXPR='search "AI trends" -> summarize'

# Deploy a React SPA to GitHub Pages
make run EXPR='search "AI trends" -> summarize -> github_pages "AI Report"'
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

### Nested Parallel (Unique Feature!)

```
parallel {
    parallel { search "A" search "B" } -> merge -> summarize
    parallel { search "C" search "D" } -> merge -> summarize
} -> merge -> ask "synthesize"
```

## 🛠 All 31 Commands

### Core (9)
`search`, `summarize`, `ask`, `analyze`, `save`, `read`, `list`, `merge`, `confirm`

### Google Workspace (10)
`email`, `calendar`, `meet`, `drive_save`, `doc_create`, `sheet_create`, `sheet_append`, `task`, `contact_find`, `youtube_search`

### Multimodal (6)
`image_generate`, `image_analyze`, `video_generate`, `video_analyze`, `images_to_video`, `text_to_speech`

### Publishing (5)
`youtube_upload`, `youtube_shorts`, `github_pages`, `github_pages_html`

### Control (1)
`parallel`

## 🎬 Key Workflows

### 🚀 Deploy React SPA to GitHub Pages (Claude-powered!)
```bash
make run EXPR='search "quantum computing" -> summarize -> github_pages "Quantum Report"'
```

This command:
1. Searches for quantum computing content
2. Summarizes it with Gemini
3. **Claude generates a beautiful React SPA** with Tailwind, animations, dark theme
4. Deploys to GitHub Pages
5. Returns: `https://yourusername.github.io/quantum-report`

### 🦋 Multimodal Video Pipeline
```bash
make run EXPR='parallel { image_generate "sunrise" -> save "a.png" image_generate "sunset" -> save "b.png" } -> merge -> images_to_video "a.png b.png" -> save "timelapse.mp4"'
```

### 📱 YouTube Shorts
```bash
make run EXPR='video_generate "vertical shorts: cute cat" -> save "cat.mp4" -> confirm "Upload?" -> youtube_shorts "Cute Cat"'
```

## 🔧 Setup

```bash
git clone https://github.com/vinodhalaharvi/agentscript
cd agentscript
go mod tidy && go build -o agentscript .
```

### Environment Variables
```bash
# Required
export GEMINI_API_KEY="your-gemini-key"

# Recommended: Claude for React SPA generation (github_pages)
export CLAUDE_API_KEY="your-anthropic-key"

# Optional: Google Workspace
export GOOGLE_CREDENTIALS_FILE="credentials.json"

# Optional: GitHub Pages deployment
export GITHUB_CLIENT_ID="your-github-client-id"
export GITHUB_CLIENT_SECRET="your-github-client-secret"
```

## 🎯 Why AgentScript?

| Feature | LangGraph | AgentScript |
|---------|-----------|-------------|
| Lines for parallel research | 50+ | 1 |
| Nested parallelism | Complex | Native |
| Google Workspace | Manual | Built-in |
| Multimodal (Imagen/Veo) | Manual | Built-in |
| YouTube Upload | Manual | Built-in |
| **Claude-powered React SPAs** | Manual | Built-in |
| GitHub Pages Deploy | Manual | Built-in |

## 🏆 Built for Gemini 3 Hackathon

- **Gemini 3** for search, summarization, multimodal
- **Claude** for React SPA code generation
- **Imagen 4** for image generation
- **Veo 3.1** for video generation
- **12 Google APIs** integrated
- **GitHub API** for deployment
- **31 commands** in a single-line DSL

---

**Made with ❤️ for the Gemini 3 Hackathon**
