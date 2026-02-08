package main

import (
	"context"
	"fmt"
	"strings"
)

// Translator converts natural language to AgentScript DSL
type Translator struct {
	gemini *GeminiClient
}

// NewTranslator creates a new Translator
func NewTranslator(ctx context.Context, apiKey string) (*Translator, error) {
	client := NewGeminiClient(apiKey, "")

	return &Translator{
		gemini: client,
	}, nil
}

const systemPrompt = `You are a translator that converts natural language into AgentScript DSL.

AgentScript is a simple command language with these commands:
- search "query" - Search the web for information
- summarize - Summarize the input content
- save "filename" - Save content to a file
- read "filename" - Read content from a file
- ask "question" - Ask a question, optionally with context from previous command
- analyze "focus" - Analyze content with optional focus area
- list "path" - List files in a directory
- merge - Combine results from parallel branches
- email "address" - Send an email with the content
- calendar "event" - Create a calendar event
- meet "meeting" - Create a Google Meet
- drive_save "path" - Save to Google Drive
- doc_create "title" - Create a Google Doc
- sheet_create "title" - Create a Google Sheet
- sheet_append "id" - Append to a Google Sheet
- task "todo" - Create a Google Task
- youtube_search "query" - Search YouTube videos
- image_generate "prompt" - Generate an image with AI
- image_analyze "file.jpg" - Analyze an image file
- video_analyze "file.mp4" - Analyze a video file
- video_generate "prompt" - Generate a video from text description
- images_to_video "img1.jpg,img2.jpg" - Generate video from images

Commands can be chained with -> (pipe) operator:
search "topic" -> summarize -> save "notes.md"

For comparing multiple things, use parallel blocks:
parallel {
  search "topic A" -> analyze
  search "topic B" -> analyze
} -> merge -> ask "compare these"

Rules:
1. Output ONLY the DSL commands, no explanation
2. Use double quotes for all string arguments
3. Chain commands logically with ->
4. Keep it simple - use minimum commands needed
5. Use parallel when comparing multiple items or doing independent research
6. Always use merge after parallel to combine results
7. All commands are lowercase

Examples:
- "find info about golang and save it" → search "golang" -> save "golang.txt"
- "summarize my notes file" → read "notes.txt" -> summarize
- "what's in the current directory" → list "."
- "research AI trends and give me a summary" → search "AI trends 2024" -> summarize
- "read config.json and explain it" → read "config.json" -> ask "explain this configuration"
- "compare Google and Microsoft" → parallel { search "Google company" -> analyze "strengths" search "Microsoft company" -> analyze "strengths" } -> merge -> ask "compare these companies"
- "research Apple and Samsung and email me the comparison" → parallel { search "Apple Inc" -> analyze search "Samsung Electronics" -> analyze } -> merge -> ask "which is better?" -> email "user@example.com"
- "schedule a meeting about the project" → search "project status" -> meet "Project Review Meeting"
- "save the report to Google Drive" → search "quarterly report" -> summarize -> drive_save "Reports/Q1.md"
- "generate an image of a sunset over mountains" → image_generate "a beautiful sunset over snow-capped mountains, photorealistic"
- "what's in this photo" → image_analyze "photo.jpg"
- "analyze the product demo video" → video_analyze "demo.mp4" -> summarize -> save "video-notes.md"
- "create a logo for my coffee shop" → image_generate "modern minimalist logo for a coffee shop called 'Bean There', clean lines"
- "make a video of a sunset" → video_generate "beautiful sunset over ocean, cinematic, 4k quality"
- "create a video from my product photos" → images_to_video "product1.jpg product2.jpg product3.jpg"
- "turn these vacation photos into a video" → images_to_video "beach.jpg mountain.jpg city.jpg"
`

// Translate converts natural language to AgentScript DSL
func (t *Translator) Translate(ctx context.Context, naturalLanguage string) (string, error) {
	prompt := fmt.Sprintf("%s\n\nConvert this to AgentScript:\n%s", systemPrompt, naturalLanguage)

	result, err := t.gemini.GenerateContent(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("translation failed: %w", err)
	}

	// Clean up the response
	dsl := strings.TrimSpace(result)

	// Remove markdown code blocks if present
	dsl = strings.TrimPrefix(dsl, "```agentscript")
	dsl = strings.TrimPrefix(dsl, "```")
	dsl = strings.TrimSuffix(dsl, "```")
	dsl = strings.TrimSpace(dsl)

	return dsl, nil
}
