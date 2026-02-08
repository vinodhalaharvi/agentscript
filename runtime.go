package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runtime executes AgentScript commands
type Runtime struct {
	gemini    *GeminiClient
	google    *GoogleClient
	verbose   bool
	searchKey string
}

// RuntimeConfig holds runtime configuration
type RuntimeConfig struct {
	GeminiAPIKey    string
	SearchAPIKey    string
	Model           string
	Verbose         bool
	GoogleCredsFile string
	GoogleTokenFile string
}

// NewRuntime creates a new Runtime instance
func NewRuntime(ctx context.Context, cfg RuntimeConfig) (*Runtime, error) {
	var geminiClient *GeminiClient
	if cfg.GeminiAPIKey != "" {
		geminiClient = NewGeminiClient(cfg.GeminiAPIKey, cfg.Model)
	}

	var googleClient *GoogleClient
	if cfg.GoogleCredsFile != "" {
		tokenFile := cfg.GoogleTokenFile
		if tokenFile == "" {
			tokenFile = "token.json"
		}
		var err error
		googleClient, err = NewGoogleClient(ctx, cfg.GoogleCredsFile, tokenFile)
		if err != nil {
			// Don't fail, just log warning
			fmt.Fprintf(os.Stderr, "Warning: Google API not available: %v\n", err)
		}
	}

	return &Runtime{
		gemini:    geminiClient,
		google:    googleClient,
		verbose:   cfg.Verbose,
		searchKey: cfg.SearchAPIKey,
	}, nil
}

// Execute runs a parsed program
func (r *Runtime) Execute(ctx context.Context, program *Program) (string, error) {
	var result string
	for _, stmt := range program.Statements {
		var err error
		result, err = r.executeStatement(ctx, stmt, result)
		if err != nil {
			return "", err
		}
	}
	return result, nil
}

// executeStatement executes a statement (command or parallel block)
func (r *Runtime) executeStatement(ctx context.Context, stmt *Statement, input string) (string, error) {
	var result string
	var err error

	if stmt.Parallel != nil {
		result, err = r.executeParallel(ctx, stmt.Parallel, input)
	} else if stmt.Command != nil {
		result, err = r.executeCommand(ctx, stmt.Command, input)
	}

	if err != nil {
		return "", err
	}

	// Follow the pipe chain
	if stmt.Pipe != nil {
		return r.executeStatement(ctx, stmt.Pipe, result)
	}

	return result, nil
}

// executeParallel runs multiple branches concurrently
func (r *Runtime) executeParallel(ctx context.Context, parallel *Parallel, input string) (string, error) {
	r.log("Executing PARALLEL with %d branches", len(parallel.Branches))

	// Results from each branch
	type branchResult struct {
		index  int
		result string
		err    error
	}

	results := make(chan branchResult, len(parallel.Branches))
	var wg sync.WaitGroup

	// Launch all branches concurrently
	for i, branch := range parallel.Branches {
		wg.Add(1)
		go func(idx int, stmt *Statement) {
			defer wg.Done()
			res, err := r.executeStatement(ctx, stmt, input)
			results <- branchResult{index: idx, result: res, err: err}
		}(i, branch)
	}

	// Wait for all branches to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	orderedResults := make([]string, len(parallel.Branches))
	for res := range results {
		if res.err != nil {
			return "", fmt.Errorf("parallel branch %d failed: %w", res.index, res.err)
		}
		orderedResults[res.index] = res.result
	}

	// Combine results with clear separators
	var combined []string
	for i, res := range orderedResults {
		combined = append(combined, fmt.Sprintf("=== Branch %d ===\n%s", i+1, res))
	}

	r.log("PARALLEL complete: %d branches finished", len(parallel.Branches))
	return strings.Join(combined, "\n\n"), nil
}

// executeCommand executes a single command
func (r *Runtime) executeCommand(ctx context.Context, cmd *Command, input string) (string, error) {
	r.log("Executing: %s %q (input: %d bytes)", cmd.Action, cmd.Arg, len(input))

	var result string
	var err error

	switch cmd.Action {
	case "search":
		result, err = r.search(ctx, cmd.Arg)
	case "summarize":
		result, err = r.geminiCall(ctx, "Summarize the following content concisely:\n\n"+input)
	case "ask":
		prompt := cmd.Arg
		if input != "" {
			prompt = cmd.Arg + "\n\nContext:\n" + input
		}
		result, err = r.geminiCall(ctx, prompt)
	case "analyze":
		prompt := "Analyze the following"
		if cmd.Arg != "" {
			prompt += " focusing on " + cmd.Arg
		}
		prompt += ":\n\n" + input
		result, err = r.geminiCall(ctx, prompt)
	case "save":
		result, err = r.save(cmd.Arg, input)
	case "read":
		result, err = r.read(cmd.Arg)
	case "list":
		result, err = r.list(cmd.Arg)
	case "merge":
		// merge just passes through the input - it's used after parallel
		// to signal that we want to combine results (which parallel already does)
		result = input
	case "email":
		result, err = r.email(ctx, cmd.Arg, input)
	case "calendar":
		result, err = r.calendar(ctx, cmd.Arg, input)
	case "meet":
		result, err = r.meet(ctx, cmd.Arg, input)
	case "drive_save":
		result, err = r.driveSave(ctx, cmd.Arg, input)
	case "doc_create":
		result, err = r.docCreate(ctx, cmd.Arg, input)
	case "sheet_append":
		result, err = r.sheetAppend(ctx, cmd.Arg, input)
	case "sheet_create":
		result, err = r.sheetCreate(ctx, cmd.Arg, input)
	case "task":
		result, err = r.task(ctx, cmd.Arg, input)
	case "contact_find":
		result, err = r.contactFind(ctx, cmd.Arg)
	case "youtube_search":
		result, err = r.youtubeSearch(ctx, cmd.Arg)
	case "image_generate":
		result, err = r.imageGenerate(ctx, cmd.Arg, input)
	case "image_analyze":
		result, err = r.imageAnalyze(ctx, cmd.Arg, input)
	case "video_analyze":
		result, err = r.videoAnalyze(ctx, cmd.Arg, input)
	case "video_generate":
		result, err = r.videoGenerate(ctx, cmd.Arg, input)
	case "images_to_video":
		result, err = r.imagesToVideo(ctx, cmd.Arg, input)
	default:
		err = fmt.Errorf("unknown action: %s", cmd.Action)
	}

	if err != nil {
		return "", fmt.Errorf("%s failed: %w", cmd.Action, err)
	}

	r.log("Result: %d bytes", len(result))
	return result, nil
}

// geminiCall makes a call to the Gemini API
func (r *Runtime) geminiCall(ctx context.Context, prompt string) (string, error) {
	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY not set - required for this command")
	}
	return r.gemini.GenerateContent(ctx, prompt)
}

// search performs a web search
func (r *Runtime) search(ctx context.Context, query string) (string, error) {
	// If no search API key, use Gemini to generate a response
	if r.searchKey == "" {
		return r.geminiCall(ctx, "Please provide information about: "+query)
	}

	// Use SerpAPI or similar
	searchURL := fmt.Sprintf(
		"https://serpapi.com/search.json?q=%s&api_key=%s",
		url.QueryEscape(query),
		r.searchKey,
	)

	resp, err := http.Get(searchURL)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read search response: %w", err)
	}

	// Parse and extract relevant snippets
	var searchResult map[string]any
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return "", fmt.Errorf("failed to parse search response: %w", err)
	}

	// Extract organic results
	var snippets []string
	if organic, ok := searchResult["organic_results"].([]any); ok {
		for i, item := range organic {
			if i >= 5 {
				break
			}
			if result, ok := item.(map[string]any); ok {
				title := result["title"]
				snippet := result["snippet"]
				link := result["link"]
				snippets = append(snippets, fmt.Sprintf("- %v\n  %v\n  %v", title, snippet, link))
			}
		}
	}

	if len(snippets) == 0 {
		return string(body), nil
	}

	return strings.Join(snippets, "\n\n"), nil
}

// save writes content to a file
func (r *Runtime) save(path, content string) (string, error) {
	// Check if content is a temp image file from imageGenerate
	if strings.HasPrefix(content, "IMAGEFILE:") {
		tempPath := strings.TrimPrefix(content, "IMAGEFILE:")
		// Move temp file to final destination
		if err := os.Rename(tempPath, path); err != nil {
			// If rename fails (cross-device), try copy
			data, err := os.ReadFile(tempPath)
			if err != nil {
				return "", fmt.Errorf("failed to read temp image: %w", err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return "", fmt.Errorf("failed to write image: %w", err)
			}
			os.Remove(tempPath)
		}
		fmt.Printf("✅ Image saved to %s\n", path)
		return path, nil
	}

	// Check if content is a Gemini file URI that needs downloading
	if strings.HasPrefix(content, "https://generativelanguage.googleapis.com/") && strings.Contains(content, "/files/") {
		if r.gemini != nil {
			fmt.Printf("📥 Downloading to %s...\n", path)
			_, err := r.gemini.DownloadFile(context.Background(), content, path)
			if err != nil {
				return "", fmt.Errorf("failed to download file: %w", err)
			}
			fmt.Printf("✅ Saved to %s\n", path)
			// Return just the path for chaining
			return path, nil
		}
		return "", fmt.Errorf("GEMINI_API_KEY required to download file")
	}

	// Regular file save
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("✅ Saved %d bytes to %s\n", len(content), path)
	// Return just the path for chaining
	return path, nil
}

// read reads content from a file
func (r *Runtime) read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

// list lists files in a directory
func (r *Runtime) list(path string) (string, error) {
	if path == "" {
		path = "."
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}

	var lines []string
	for _, entry := range entries {
		prefix := "📄"
		if entry.IsDir() {
			prefix = "📁"
		}
		lines = append(lines, fmt.Sprintf("%s %s", prefix, entry.Name()))
	}

	return strings.Join(lines, "\n"), nil
}

// email sends an email via Gmail API
func (r *Runtime) email(ctx context.Context, to string, content string) (string, error) {
	r.log("EMAIL to: %s", to)

	// Use Gemini to format the email and extract subject
	prompt := fmt.Sprintf(`Format this content as a professional email. 
Return ONLY a JSON object with "subject" and "body" fields. No markdown.

Content to include:
%s`, content)

	var subject, body string

	if r.gemini != nil {
		formatted, err := r.geminiCall(ctx, prompt)
		if err == nil {
			// Try to parse JSON response
			var emailData struct {
				Subject string `json:"subject"`
				Body    string `json:"body"`
			}
			if json.Unmarshal([]byte(formatted), &emailData) == nil && emailData.Subject != "" {
				subject = emailData.Subject
				body = emailData.Body
			} else {
				// Fallback: use first line as subject
				lines := strings.SplitN(formatted, "\n", 2)
				subject = strings.TrimPrefix(lines[0], "Subject: ")
				if len(lines) > 1 {
					body = lines[1]
				} else {
					body = content
				}
			}
		}
	}

	if subject == "" {
		subject = "AgentScript Report"
		body = content
	}

	// If Google client is available, send real email
	if r.google != nil {
		err := r.google.SendEmail(ctx, to, subject, body)
		if err != nil {
			return "", fmt.Errorf("failed to send email: %w", err)
		}
		return fmt.Sprintf("✅ Email sent to %s", to), nil
	}

	// Fallback: simulate sending
	fmt.Printf("\n📧 ========== EMAIL TO: %s ==========\n", to)
	fmt.Printf("Subject: %s\n\n%s\n", subject, body)
	fmt.Println("📧 ======================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real email)")

	return fmt.Sprintf("Email simulated to %s", to), nil
}

// calendar creates a Google Calendar event
func (r *Runtime) calendar(ctx context.Context, eventInfo string, content string) (string, error) {
	r.log("CALENDAR: %s", eventInfo)

	// Use Gemini to parse event details
	prompt := fmt.Sprintf(`Parse this event information and return ONLY a JSON object with these fields:
- summary: event title
- description: event description
- start: start time in RFC3339 format (e.g., 2024-01-15T10:00:00-08:00)
- end: end time in RFC3339 format

Event info: %s

Additional context:
%s`, eventInfo, content)

	var summary, description, startTime, endTime string

	if r.gemini != nil {
		parsed, err := r.geminiCall(ctx, prompt)
		if err == nil {
			var eventData struct {
				Summary     string `json:"summary"`
				Description string `json:"description"`
				Start       string `json:"start"`
				End         string `json:"end"`
			}
			if json.Unmarshal([]byte(parsed), &eventData) == nil {
				summary = eventData.Summary
				description = eventData.Description
				startTime = eventData.Start
				endTime = eventData.End
			}
		}
	}

	if summary == "" {
		summary = eventInfo
		description = content
		// Default: 1 hour from now
		now := time.Now()
		startTime = now.Add(1 * time.Hour).Format(time.RFC3339)
		endTime = now.Add(2 * time.Hour).Format(time.RFC3339)
	}

	// If Google client is available, create real event
	if r.google != nil {
		event, err := r.google.CreateCalendarEvent(ctx, summary, description, startTime, endTime)
		if err != nil {
			return "", fmt.Errorf("failed to create calendar event: %w", err)
		}
		return fmt.Sprintf("✅ Calendar event created: %s\nLink: %s", event.Summary, event.HtmlLink), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📅 ========== CALENDAR EVENT ==========\n")
	fmt.Printf("Summary: %s\n", summary)
	fmt.Printf("Start: %s\n", startTime)
	fmt.Printf("End: %s\n", endTime)
	fmt.Printf("Description: %s\n", description)
	fmt.Println("📅 ======================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real calendar)")

	return fmt.Sprintf("Calendar event simulated: %s", summary), nil
}

// meet creates a Google Meet event
func (r *Runtime) meet(ctx context.Context, eventInfo string, content string) (string, error) {
	r.log("MEET: %s", eventInfo)

	// Use Gemini to parse event details
	prompt := fmt.Sprintf(`Parse this meeting information and return ONLY a JSON object with these fields:
- summary: meeting title
- description: meeting description/agenda
- start: start time in RFC3339 format (e.g., 2024-01-15T10:00:00-08:00)
- end: end time in RFC3339 format

Meeting info: %s

Additional context:
%s`, eventInfo, content)

	var summary, description, startTime, endTime string

	if r.gemini != nil {
		parsed, err := r.geminiCall(ctx, prompt)
		if err == nil {
			var eventData struct {
				Summary     string `json:"summary"`
				Description string `json:"description"`
				Start       string `json:"start"`
				End         string `json:"end"`
			}
			if json.Unmarshal([]byte(parsed), &eventData) == nil {
				summary = eventData.Summary
				description = eventData.Description
				startTime = eventData.Start
				endTime = eventData.End
			}
		}
	}

	if summary == "" {
		summary = eventInfo
		description = content
		now := time.Now()
		startTime = now.Add(1 * time.Hour).Format(time.RFC3339)
		endTime = now.Add(2 * time.Hour).Format(time.RFC3339)
	}

	// If Google client is available, create real Meet event
	if r.google != nil {
		event, err := r.google.CreateMeetEvent(ctx, summary, description, startTime, endTime)
		if err != nil {
			return "", fmt.Errorf("failed to create Meet event: %w", err)
		}
		meetLink := ""
		if event.ConferenceData != nil && len(event.ConferenceData.EntryPoints) > 0 {
			meetLink = event.ConferenceData.EntryPoints[0].Uri
		}
		return fmt.Sprintf("✅ Meet created: %s\nMeet Link: %s\nCalendar: %s", event.Summary, meetLink, event.HtmlLink), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📹 ========== GOOGLE MEET ==========\n")
	fmt.Printf("Summary: %s\n", summary)
	fmt.Printf("Start: %s\n", startTime)
	fmt.Printf("Meet Link: (would be generated)\n")
	fmt.Println("📹 ====================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Meet)")

	return fmt.Sprintf("Meet simulated: %s", summary), nil
}

// driveSave saves content to Google Drive
func (r *Runtime) driveSave(ctx context.Context, path string, content string) (string, error) {
	r.log("DRIVE_SAVE: %s", path)

	if r.google != nil {
		file, err := r.google.SaveToDrive(ctx, path, content)
		if err != nil {
			return "", fmt.Errorf("failed to save to Drive: %w", err)
		}
		return fmt.Sprintf("✅ Saved to Google Drive: %s\nFile ID: %s", file.Name, file.Id), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📁 ========== GOOGLE DRIVE ==========\n")
	fmt.Printf("Path: %s\n", path)
	fmt.Printf("Content: %d bytes\n", len(content))
	fmt.Println("📁 ====================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Drive)")

	return fmt.Sprintf("Drive save simulated: %s", path), nil
}

// docCreate creates a Google Doc
func (r *Runtime) docCreate(ctx context.Context, title string, content string) (string, error) {
	r.log("DOC_CREATE: %s", title)

	if r.google != nil {
		doc, err := r.google.CreateDoc(ctx, title, content)
		if err != nil {
			return "", fmt.Errorf("failed to create Doc: %w", err)
		}
		return fmt.Sprintf("✅ Google Doc created: %s\nLink: https://docs.google.com/document/d/%s", doc.Title, doc.DocumentId), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📄 ========== GOOGLE DOC ==========\n")
	fmt.Printf("Title: %s\n", title)
	fmt.Printf("Content: %d bytes\n", len(content))
	fmt.Println("📄 ==================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Docs)")

	return fmt.Sprintf("Doc create simulated: %s", title), nil
}

// sheetAppend appends data to a Google Sheet
func (r *Runtime) sheetAppend(ctx context.Context, sheetRef string, content string) (string, error) {
	r.log("SHEET_APPEND: %s", sheetRef)

	// Parse sheetRef: "spreadsheetId/SheetName" or just "spreadsheetId"
	parts := strings.SplitN(sheetRef, "/", 2)
	spreadsheetID := parts[0]
	sheetName := ""
	if len(parts) > 1 {
		sheetName = parts[1]
	}

	if r.google != nil {
		err := r.google.AppendToSheet(ctx, spreadsheetID, sheetName, content)
		if err != nil {
			return "", fmt.Errorf("failed to append to Sheet: %w", err)
		}
		return fmt.Sprintf("✅ Data appended to Google Sheet: %s", spreadsheetID), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📊 ========== GOOGLE SHEET ==========\n")
	fmt.Printf("Spreadsheet: %s\n", spreadsheetID)
	fmt.Printf("Sheet: %s\n", sheetName)
	fmt.Printf("Data: %d bytes\n", len(content))
	fmt.Println("📊 ====================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Sheets)")

	return fmt.Sprintf("Sheet append simulated: %s", sheetRef), nil
}

// sheetCreate creates a new Google Sheet
func (r *Runtime) sheetCreate(ctx context.Context, title string, content string) (string, error) {
	r.log("SHEET_CREATE: %s", title)

	if r.google != nil {
		sheet, err := r.google.CreateSheet(ctx, title)
		if err != nil {
			return "", fmt.Errorf("failed to create Sheet: %w", err)
		}

		// If there's content, append it
		if content != "" {
			err = r.google.AppendToSheet(ctx, sheet.SpreadsheetId, "", content)
			if err != nil {
				return "", fmt.Errorf("failed to add data to Sheet: %w", err)
			}
		}

		return fmt.Sprintf("✅ Google Sheet created: %s\nLink: %s", sheet.Properties.Title, sheet.SpreadsheetUrl), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📊 ========== CREATE GOOGLE SHEET ==========\n")
	fmt.Printf("Title: %s\n", title)
	fmt.Printf("Initial data: %d bytes\n", len(content))
	fmt.Println("📊 ==========================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Sheets)")

	return fmt.Sprintf("Sheet create simulated: %s", title), nil
}

// task creates a Google Task
func (r *Runtime) task(ctx context.Context, title string, notes string) (string, error) {
	r.log("TASK: %s", title)

	if r.google != nil {
		task, err := r.google.CreateTask(ctx, title, notes)
		if err != nil {
			return "", fmt.Errorf("failed to create Task: %w", err)
		}
		return fmt.Sprintf("✅ Task created: %s", task.Title), nil
	}

	// Fallback: simulate
	fmt.Printf("\n✓ ========== GOOGLE TASK ==========\n")
	fmt.Printf("Title: %s\n", title)
	fmt.Printf("Notes: %s\n", notes)
	fmt.Println("✓ ==================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Tasks)")

	return fmt.Sprintf("Task simulated: %s", title), nil
}

// contactFind finds a contact by name
func (r *Runtime) contactFind(ctx context.Context, name string) (string, error) {
	r.log("CONTACT_FIND: %s", name)

	if r.google != nil {
		contacts, err := r.google.FindContact(ctx, name)
		if err != nil {
			return "", fmt.Errorf("failed to find contact: %w", err)
		}

		if len(contacts) == 0 {
			return fmt.Sprintf("No contacts found for: %s", name), nil
		}

		var results []string
		for _, contact := range contacts {
			var contactName, email string
			if len(contact.Names) > 0 {
				contactName = contact.Names[0].DisplayName
			}
			if len(contact.EmailAddresses) > 0 {
				email = contact.EmailAddresses[0].Value
			}
			results = append(results, fmt.Sprintf("%s <%s>", contactName, email))
		}
		return strings.Join(results, "\n"), nil
	}

	// Fallback: simulate
	fmt.Printf("\n👤 ========== GOOGLE CONTACTS ==========\n")
	fmt.Printf("Searching for: %s\n", name)
	fmt.Println("👤 ======================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real Contacts)")

	return fmt.Sprintf("Contact search simulated: %s", name), nil
}

// youtubeSearch searches YouTube
func (r *Runtime) youtubeSearch(ctx context.Context, query string) (string, error) {
	r.log("YOUTUBE_SEARCH: %s", query)

	if r.google != nil {
		results, err := r.google.SearchYouTube(ctx, query, 5)
		if err != nil {
			return "", fmt.Errorf("failed to search YouTube: %w", err)
		}

		if len(results) == 0 {
			return fmt.Sprintf("No videos found for: %s", query), nil
		}

		var videos []string
		for _, item := range results {
			videoURL := fmt.Sprintf("https://youtube.com/watch?v=%s", item.Id.VideoId)
			videos = append(videos, fmt.Sprintf("📺 %s\n   %s\n   %s", item.Snippet.Title, item.Snippet.Description[:min(100, len(item.Snippet.Description))], videoURL))
		}
		return strings.Join(videos, "\n\n"), nil
	}

	// Fallback: simulate
	fmt.Printf("\n📺 ========== YOUTUBE SEARCH ==========\n")
	fmt.Printf("Query: %s\n", query)
	fmt.Println("📺 =====================================")
	fmt.Println("(Simulated - set GOOGLE_CREDENTIALS_FILE for real YouTube)")

	return fmt.Sprintf("YouTube search simulated: %s", query), nil
}

// imageGenerate generates an image using Gemini Imagen
func (r *Runtime) imageGenerate(ctx context.Context, prompt string, input string) (string, error) {
	r.log("IMAGE_GENERATE: %s", prompt)

	// Combine prompt with any input context
	fullPrompt := prompt
	if input != "" {
		fullPrompt = prompt + ". Context: " + input
	}

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for image generation")
	}

	imageBytes, err := r.gemini.GenerateImage(ctx, fullPrompt)
	if err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	// Store the image bytes in a temp file and return a special marker
	// that save() can detect and handle properly
	tempFile := fmt.Sprintf(".temp_image_%d.png", time.Now().UnixNano())
	if err := os.WriteFile(tempFile, imageBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to save temp image: %w", err)
	}

	fmt.Printf("✅ Image generated (%d bytes)\n", len(imageBytes))

	// Return the temp file path - save command will move it to the final location
	return "IMAGEFILE:" + tempFile, nil
}

// imageAnalyze analyzes an image file
func (r *Runtime) imageAnalyze(ctx context.Context, imagePath string, prompt string) (string, error) {
	r.log("IMAGE_ANALYZE: %s", imagePath)

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for image analysis")
	}

	// Default prompt if none provided
	analysisPrompt := prompt
	if analysisPrompt == "" {
		analysisPrompt = "Describe this image in detail. What do you see?"
	}

	result, err := r.gemini.AnalyzeImage(ctx, imagePath, analysisPrompt)
	if err != nil {
		return "", fmt.Errorf("image analysis failed: %w", err)
	}

	return result, nil
}

// videoAnalyze analyzes a video file
func (r *Runtime) videoAnalyze(ctx context.Context, videoPath string, prompt string) (string, error) {
	r.log("VIDEO_ANALYZE: %s", videoPath)

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for video analysis")
	}

	// Default prompt if none provided
	analysisPrompt := prompt
	if analysisPrompt == "" {
		analysisPrompt = "Describe what happens in this video. Summarize the key moments and content."
	}

	result, err := r.gemini.AnalyzeVideo(ctx, videoPath, analysisPrompt)
	if err != nil {
		return "", fmt.Errorf("video analysis failed: %w", err)
	}

	return result, nil
}

// videoGenerate generates a video from a text prompt
func (r *Runtime) videoGenerate(ctx context.Context, prompt string, input string) (string, error) {
	r.log("VIDEO_GENERATE: %s", prompt)

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for video generation")
	}

	// Combine prompt with input context if available
	fullPrompt := prompt
	if input != "" {
		fullPrompt = prompt + ". Additional context: " + input
	}

	fmt.Println("🎬 Generating video (this may take a few minutes)...")

	videoURI, err := r.gemini.GenerateVideo(ctx, fullPrompt)
	if err != nil {
		return "", fmt.Errorf("video generation failed: %w", err)
	}

	fmt.Printf("✅ Video generated!\n")

	// Return the URI - can be piped to save command
	return videoURI, nil
}

// imagesToVideo generates a video from multiple images
func (r *Runtime) imagesToVideo(ctx context.Context, imagesArg string, input string) (string, error) {
	r.log("IMAGES_TO_VIDEO: arg=%s, input=%s", imagesArg, input)

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for video generation")
	}

	// Parse image paths from both the argument and piped input
	var imagePaths []string

	// Helper to extract image paths from text
	extractPaths := func(text string) []string {
		var paths []string
		// Replace newlines and commas with spaces, then split
		normalized := strings.ReplaceAll(text, "\n", " ")
		normalized = strings.ReplaceAll(normalized, ",", " ")
		normalized = strings.ReplaceAll(normalized, "===", " ")
		normalized = strings.ReplaceAll(normalized, "Branch", " ")
		for _, p := range strings.Fields(normalized) {
			path := strings.TrimSpace(p)
			// Check for image extensions
			lower := strings.ToLower(path)
			if strings.HasSuffix(lower, ".jpg") ||
				strings.HasSuffix(lower, ".jpeg") ||
				strings.HasSuffix(lower, ".png") ||
				strings.HasSuffix(lower, ".webp") ||
				strings.HasSuffix(lower, ".gif") {
				paths = append(paths, path)
			}
		}
		return paths
	}

	// First try to get paths from the argument
	if imagesArg != "" {
		imagePaths = append(imagePaths, extractPaths(imagesArg)...)
	}

	// Also extract from piped input (merged parallel output)
	if input != "" {
		imagePaths = append(imagePaths, extractPaths(input)...)
	}

	// Deduplicate paths while preserving order
	seen := make(map[string]bool)
	var uniquePaths []string
	for _, p := range imagePaths {
		if !seen[p] {
			seen[p] = true
			uniquePaths = append(uniquePaths, p)
		}
	}
	imagePaths = uniquePaths

	if len(imagePaths) == 0 {
		return "", fmt.Errorf("no image paths found. Use: images_to_video \"img1.png img2.png\" or pipe from parallel image generation")
	}

	// Use a default video prompt, or use piped input if it looks like a description
	videoPrompt := "Create a smooth cinematic video transitioning between these images"
	if input != "" {
		// Check if input contains a description (not just file paths)
		words := strings.Fields(input)
		descWords := 0
		for _, word := range words {
			lower := strings.ToLower(word)
			if !strings.HasSuffix(lower, ".png") &&
				!strings.HasSuffix(lower, ".jpg") &&
				!strings.Contains(lower, "===") &&
				!strings.Contains(lower, "branch") &&
				len(word) > 2 {
				descWords++
			}
		}
		// If there's substantial text beyond file paths, might be a description
		if descWords > 5 {
			// Extract potential description
			for _, keyword := range []string{"smooth", "transition", "cinematic", "pan", "zoom", "animate"} {
				if strings.Contains(strings.ToLower(input), keyword) {
					videoPrompt = input
					break
				}
			}
		}
	}

	fmt.Printf("🎬 Generating video from %d images (this may take a few minutes)...\n", len(imagePaths))
	for i, p := range imagePaths {
		fmt.Printf("   %d. %s\n", i+1, p)
	}
	fmt.Printf("   Prompt: %s\n", videoPrompt)

	videoURI, err := r.gemini.GenerateVideoFromImages(ctx, imagePaths, videoPrompt)
	if err != nil {
		return "", fmt.Errorf("video generation failed: %w", err)
	}

	fmt.Printf("✅ Video generated from %d images!\n", len(imagePaths))

	// Return URI so it can be piped to save
	return videoURI, nil
}

// log prints verbose output if enabled
func (r *Runtime) log(format string, args ...any) {
	if r.verbose {
		fmt.Printf("[agentscript] "+format+"\n", args...)
	}
}
