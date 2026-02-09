# 🚀 AgentScript

**A Domain-Specific Language for AI-Powered Automation**

AgentScript is a simple, powerful DSL that chains Gemini AI with Google APIs to automate complex workflows in just a few lines of code.

```
search "AI trends 2026" -> summarize -> translate "Spanish" -> email "team@company.com"
```

---

## 🎯 What is AgentScript?

AgentScript turns natural workflows into executable pipelines. Instead of writing hundreds of lines of code to:
1. Search the web
2. Summarize results with AI
3. Generate images
4. Create videos
5. Send emails

You write one line:
```
search "topic" -> summarize -> image_generate "visual" -> email "user@email.com"
```

---

## 🏆 Hackathon Submission

**Google Gemini API Developer Competition**

### Key Features
- **34 Commands** - Research, documents, multimedia, Google services
- **Parallel Execution** - Run multiple tasks concurrently
- **Pipeline Chaining** - Output of one command feeds into the next
- **Natural Language Mode** - Describe what you want in plain English
- **Gemini Integration** - Text, images, video, and speech generation

### Google APIs Used
| API | Commands |
|-----|----------|
| Gemini 2.5 Flash | `ask`, `summarize`, `analyze`, `translate` |
| Imagen 4 | `image_generate` |
| Veo 3.1 | `video_generate`, `images_to_video` |
| Gemini TTS | `text_to_speech` |
| Gmail | `email` |
| Calendar | `calendar`, `meet` |
| Drive | `drive_save` |
| Docs | `doc_create` |
| Sheets | `sheet_create`, `sheet_append` |
| Forms | `form_create`, `form_responses` |
| YouTube | `youtube_search`, `youtube_upload`, `youtube_shorts` |
| Tasks | `task` |
| People | `contact_find` |

---

## ⚡ Quick Start

### 1. Prerequisites
```bash
# Install Go 1.22+
brew install go  # macOS
# or download from https://go.dev

# Install ffmpeg (for video/audio)
brew install ffmpeg  # macOS
sudo apt install ffmpeg  # Ubuntu
```

### 2. Setup
```bash
# Clone the repository
git clone https://github.com/vinodhalaharvi/agentscript.git
cd agentscript

# Copy environment template
cp .env.example .env

# Add your API keys to .env
GEMINI_API_KEY=your_gemini_api_key
GOOGLE_CREDENTIALS_FILE=credentials.json
GOOGLE_TOKEN_FILE=token.json
```

### 3. Google OAuth Setup
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a project and enable APIs:
   - Gmail API
   - Google Calendar API
   - Google Drive API
   - Google Docs API
   - Google Sheets API
   - Google Forms API
   - YouTube Data API v3
3. Create OAuth 2.0 credentials (Desktop app)
4. Download as `credentials.json`

### 4. Build & Run
```bash
# Build
make build

# Run a simple command
make run EXPR='ask "Hello, what can you do?"'

# Run a script file
make run-file FILE=examples/simple-research.as

# Interactive natural language mode
make repl
```

---

## 📖 Language Syntax

### Basic Pipeline
```
command "arg" -> command "arg" -> command "arg"
```

### Parallel Execution
```
parallel {
    search "topic 1" -> summarize
    search "topic 2" -> summarize
    image_generate "visual"
}
-> merge
-> email "user@email.com"
```

### Comments
```
// This is a comment
search "query" -> summarize
```

---

## 🛠 All 34 Commands

### Core Commands
| Command | Description | Example |
|---------|-------------|---------|
| `search "query"` | Web search via Gemini | `search "AI news"` |
| `ask "prompt"` | Ask Gemini anything | `ask "Explain quantum computing"` |
| `summarize` | Summarize piped content | `search "topic" -> summarize` |
| `analyze` | Analyze piped content | `read "data.csv" -> analyze` |
| `save "file"` | Save to file | `-> save "output.txt"` |
| `read "file"` | Read file contents | `read "input.txt" -> summarize` |
| `stdin "prompt"` | Read user input | `stdin "Enter topic: " -> search` |
| `translate "lang"` | Translate text | `-> translate "Japanese"` |

### Google Workspace
| Command | Description | Example |
|---------|-------------|---------|
| `email "to"` | Send email | `-> email "user@gmail.com"` |
| `calendar "event"` | Create calendar event | `-> calendar "Meeting tomorrow 2pm"` |
| `meet "event"` | Create event with Meet link | `-> meet "Team sync"` |
| `drive_save "path"` | Save to Google Drive | `-> drive_save "reports/q1"` |
| `doc_create "title"` | Create Google Doc | `-> doc_create "Report"` |
| `sheet_create "title"` | Create spreadsheet | `-> sheet_create "Data"` |
| `sheet_append "id"` | Append to sheet | `-> sheet_append "sheet_id"` |
| `task "title"` | Create Google Task | `-> task "Follow up"` |
| `contact_find "name"` | Find contact | `contact_find "John"` |
| `form_create "title"` | Create Google Form | `-> form_create "Survey"` |
| `form_responses "id"` | Get form responses | `form_responses "form_id"` |

### YouTube
| Command | Description | Example |
|---------|-------------|---------|
| `youtube_search "query"` | Search YouTube | `youtube_search "Go tutorials"` |
| `youtube_upload "title"` | Upload video | `-> youtube_upload "My Video"` |
| `youtube_shorts "title"` | Upload as Short | `-> youtube_shorts "Quick Tip"` |

### Multimedia (Gemini)
| Command | Description | Example |
|---------|-------------|---------|
| `image_generate "prompt"` | Generate image (Imagen 4) | `image_generate "sunset"` |
| `image_analyze "path"` | Analyze image | `image_analyze "photo.jpg"` |
| `video_generate "prompt"` | Generate video (Veo 3.1) | `video_generate "ocean waves"` |
| `video_analyze "path"` | Analyze video | `video_analyze "clip.mp4"` |
| `images_to_video "paths"` | Images to video | `-> images_to_video` |
| `text_to_speech "voice"` | Convert text to speech | `-> text_to_speech "Kore"` |
| `audio_video_merge "out"` | Merge audio + video | `-> audio_video_merge "final.mp4"` |
| `image_audio_merge "out"` | Image + audio to video | `-> image_audio_merge "video.mp4"` |

### Travel & Places
| Command | Description | Example |
|---------|-------------|---------|
| `places_search "query"` | Search for places | `places_search "cafes Tokyo"` |
| `maps_trip "name"` | Create trip map URL | `-> maps_trip "Tokyo Trip"` |

### Control
| Command | Description | Example |
|---------|-------------|---------|
| `merge` | Merge parallel outputs | `parallel { ... } -> merge` |
| `list "path"` | List directory | `list "."` |

---

## 🎬 Example Workflows

### 1. Research Report
```bash
make run EXPR='search "renewable energy 2026" -> summarize -> doc_create "Energy Report" -> email "team@company.com"'
```

### 2. Travel Planner
```bash
make run-file FILE=examples/travel-planner.as
```
```
parallel {
    places_search "attractions Dubrovnik"
    places_search "restaurants Dubrovnik"
    places_search "hotels Dubrovnik"
}
-> merge
-> ask "Create 5-day itinerary"
-> maps_trip "Dubrovnik Adventure"
-> translate "Croatian"
-> email "traveler@email.com"
```

### 3. AI News Video (2 minutes)
```bash
make run-file FILE=examples/news-2min.as
```
```
parallel {
    search "top US news today"
    -> ask "Write 2-minute news script"
    -> text_to_speech "Charon"
    -> save "narration.wav"

    image_generate "news studio background" -> save "bg.png"
}
-> merge
-> image_audio_merge "news.mp4"
-> youtube_upload "Daily News Update"
```

### 4. Multilingual Content
```bash
make run EXPR='ask "Write a welcome message" -> translate "Spanish" -> translate "Japanese" -> translate "French" -> save "translations.txt"'
```

### 5. Event Planning with RSVP
```bash
make run EXPR='ask "Plan a team offsite agenda" -> form_create "Offsite RSVP" -> calendar "Team Offsite next Friday 9am" -> email "team@company.com"'
```

### 6. Competitor Analysis
```bash
make run-file FILE=examples/competitor-analysis.as
```

### 7. Interactive Mode
```bash
make repl
> search for latest AI research and summarize the top findings
> create an image of a futuristic city and save it
> translate "hello world" to 5 languages
```

---

## 📁 Project Structure

```
agentscript/
├── main.go           # Entry point
├── grammar.go        # DSL parser (Participle)
├── runtime.go        # Command execution engine
├── client.go         # Gemini API client
├── google.go         # Google Workspace APIs
├── github.go         # GitHub Pages deployment
├── translator.go     # Natural language to DSL
├── claude.go         # Claude API (optional)
├── examples/         # Example scripts
│   ├── mega-showcase.as      # 100-line comprehensive demo
│   ├── travel-planner.as     # Travel planning workflow
│   ├── news-2min.as          # 2-minute news video
│   ├── youtube-shorts.as     # YouTube Shorts creation
│   └── ...
├── Makefile          # Build commands
├── .env.example      # Environment template
└── README.md         # This file
```

---

## 🔧 Makefile Commands

```bash
make build                    # Build binary
make run EXPR='...'          # Run inline expression
make run-file FILE=path.as   # Run script file  
make repl                    # Interactive mode
make test                    # Run tests
make clean                   # Clean build artifacts
```

---

## 🎤 TTS Voices

Available voices for `text_to_speech`:
- `Kore` - Female, warm
- `Charon` - Male, deep (great for news)
- `Puck` - Male, energetic
- `Aoede` - Female, calm

```
ask "Hello world" -> text_to_speech "Kore" -> save "greeting.wav"
```

---

## 🌐 Environment Variables

```bash
# Required
GEMINI_API_KEY=your_gemini_api_key

# For Google Workspace integration
GOOGLE_CREDENTIALS_FILE=credentials.json
GOOGLE_TOKEN_FILE=token.json

# Optional - for GitHub Pages deployment
GITHUB_CLIENT_ID=your_client_id
GITHUB_CLIENT_SECRET=your_client_secret

# Optional - for Claude as alternative LLM
CLAUDE_API_KEY=your_claude_key
```

---

## 🚨 Troubleshooting

### "GEMINI_API_KEY not set"
```bash
export GEMINI_API_KEY=your_key
# or add to .env file
```

### "ffmpeg not found"
```bash
brew install ffmpeg  # macOS
sudo apt install ffmpeg  # Ubuntu
```

### "OAuth scope insufficient"
```bash
rm token.json  # Delete old token
make run EXPR='...'  # Re-authenticate with new scopes
```

### "Veo quota exceeded"
- Use `image_audio_merge` instead of `images_to_video`
- Or wait for daily quota reset
- Or request quota increase in Google Cloud Console

---

## 📊 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      AgentScript                            │
├─────────────────────────────────────────────────────────────┤
│  Natural Language ──► Translator ──► DSL Parser            │
│         │                               │                   │
│         ▼                               ▼                   │
│  "summarize AI news"        search "AI" -> summarize       │
├─────────────────────────────────────────────────────────────┤
│                     Runtime Engine                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│  │ Gemini  │  │ Google  │  │ GitHub  │  │ ffmpeg  │       │
│  │  APIs   │  │  APIs   │  │  API    │  │         │       │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘       │
│       │            │            │            │              │
│       ▼            ▼            ▼            ▼              │
│  Text/Image    Gmail/Cal     Pages      Audio/Video        │
│  Video/TTS     Drive/Docs    Deploy      Processing        │
└─────────────────────────────────────────────────────────────┘
```

---

## 🏅 Why AgentScript?

| Traditional Approach | AgentScript |
|---------------------|-------------|
| 500+ lines of Python | 5 lines of DSL |
| Multiple API libraries | One unified syntax |
| Complex async handling | Built-in parallelism |
| Manual error handling | Automatic retries |
| Separate scripts | Chainable pipelines |

---

## 📜 License

MIT License - See LICENSE file

---

## 👨‍💻 Author

**Vinod Halaharvi**
- GitHub: [@vinodhalaharvi](https://github.com/vinodhalaharvi)
- Email: vinod.halaharvi@gmail.com

---

## 🙏 Acknowledgments

- Google Gemini API Team
- Anthropic Claude (alternative LLM support)
- Participle Parser Library

---

**Built for the Google Gemini API Developer Competition 2026** 🏆
