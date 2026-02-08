package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runtime executes AgentScript commands
type Runtime struct {
	gemini    *GeminiClient
	google    *GoogleClient
	github    *GitHubClient
	claude    *ClaudeClient
	verbose   bool
	searchKey string
}

// RuntimeConfig holds runtime configuration
type RuntimeConfig struct {
	GeminiAPIKey       string
	ClaudeAPIKey       string
	SearchAPIKey       string
	Model              string
	Verbose            bool
	GoogleCredsFile    string
	GoogleTokenFile    string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubTokenFile    string
}

// NewRuntime creates a new Runtime instance
func NewRuntime(ctx context.Context, cfg RuntimeConfig) (*Runtime, error) {
	var geminiClient *GeminiClient
	if cfg.GeminiAPIKey != "" {
		geminiClient = NewGeminiClient(cfg.GeminiAPIKey, cfg.Model)
	}

	var claudeClient *ClaudeClient
	if cfg.ClaudeAPIKey != "" {
		claudeClient = NewClaudeClient(cfg.ClaudeAPIKey)
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

	var githubClient *GitHubClient
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		tokenFile := cfg.GitHubTokenFile
		if tokenFile == "" {
			tokenFile = "github_token.json"
		}
		var err error
		githubClient, err = NewGitHubClient(ctx, cfg.GitHubClientID, cfg.GitHubClientSecret, tokenFile)
		if err != nil {
			// Don't fail, just log warning
			fmt.Fprintf(os.Stderr, "Warning: GitHub API not available: %v\n", err)
		}
	}

	return &Runtime{
		gemini:    geminiClient,
		google:    googleClient,
		github:    githubClient,
		claude:    claudeClient,
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
	case "stdin":
		result, err = r.readStdin(cmd.Arg)
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
	case "youtube_upload":
		result, err = r.youtubeUpload(ctx, cmd.Arg, input, false)
	case "youtube_shorts":
		result, err = r.youtubeUpload(ctx, cmd.Arg, input, true)
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
	case "text_to_speech":
		result, err = r.textToSpeech(ctx, cmd.Arg, input)
	case "audio_video_merge":
		result, err = r.audioVideoMerge(ctx, cmd.Arg, input)
	case "video_script":
		result, err = r.videoScript(ctx, cmd.Arg, input)
	case "confirm":
		result, err = r.confirm(ctx, cmd.Arg, input)
	case "github_pages":
		result, err = r.githubPages(ctx, cmd.Arg, input)
	case "github_pages_html":
		result, err = r.githubPagesHTML(ctx, cmd.Arg, input)
	default:
		err = fmt.Errorf("unknown action: %s", cmd.Action)
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

// readStdin reads from standard input
func (r *Runtime) readStdin(prompt string) (string, error) {
	if prompt != "" {
		fmt.Printf("%s: ", prompt)
	} else {
		fmt.Print("Enter text (Ctrl+D to end): ")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
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

	// Check if vertical/shorts video is requested
	isVertical := strings.Contains(strings.ToLower(prompt), "vertical") ||
		strings.Contains(strings.ToLower(prompt), "shorts") ||
		strings.Contains(strings.ToLower(prompt), "9:16") ||
		strings.Contains(strings.ToLower(prompt), "portrait")

	if isVertical {
		fmt.Println("🎬 Generating vertical video for Shorts (this may take a few minutes)...")
	} else {
		fmt.Println("🎬 Generating video (this may take a few minutes)...")
	}

	videoURI, err := r.gemini.GenerateVideo(ctx, fullPrompt, isVertical)
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

// textToSpeech converts text to speech using Gemini TTS
func (r *Runtime) textToSpeech(ctx context.Context, voice string, input string) (string, error) {
	r.log("TEXT_TO_SPEECH: voice=%s, input=%d bytes", voice, len(input))

	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for text-to-speech")
	}

	// Default voice if not specified
	if voice == "" {
		voice = "Kore"
	}

	// Use the piped input as the text to speak
	text := input
	if text == "" {
		return "", fmt.Errorf("no text to convert to speech - pipe text into text_to_speech")
	}

	fmt.Printf("🎙️ Converting text to speech (voice: %s)...\n", voice)

	audioPath, err := r.gemini.TextToSpeech(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("text-to-speech failed: %w", err)
	}

	fmt.Printf("✅ Audio generated: %s\n", audioPath)
	return audioPath, nil
}

// audioVideoMerge combines an audio file with a video file using ffmpeg
func (r *Runtime) audioVideoMerge(ctx context.Context, outputName string, input string) (string, error) {
	r.log("AUDIO_VIDEO_MERGE: output=%s, input=%s", outputName, input)

	// Parse input - expect "audio.wav video.mp4" or merged parallel output
	var audioPath, videoPath string

	// Try to extract paths from input
	parts := strings.Fields(input)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "===" || part == "Branch" {
			continue
		}
		if strings.HasSuffix(part, ".wav") || strings.HasSuffix(part, ".mp3") || strings.HasSuffix(part, ".m4a") {
			audioPath = part
		} else if strings.HasSuffix(part, ".mp4") || strings.HasSuffix(part, ".mov") || strings.HasSuffix(part, ".webm") {
			videoPath = part
		}
	}

	if audioPath == "" {
		return "", fmt.Errorf("no audio file found in input - need .wav, .mp3, or .m4a file")
	}
	if videoPath == "" {
		return "", fmt.Errorf("no video file found in input - need .mp4, .mov, or .webm file")
	}

	// Check files exist
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", audioPath)
	}
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// Default output name
	if outputName == "" {
		outputName = "merged_output.mp4"
	}
	if !strings.HasSuffix(outputName, ".mp4") {
		outputName = outputName + ".mp4"
	}

	fmt.Printf("🎬 Merging audio and video with ffmpeg...\n")
	fmt.Printf("   Audio: %s\n", audioPath)
	fmt.Printf("   Video: %s\n", videoPath)

	// Use ffmpeg to merge - replace video audio with our audio
	// -shortest makes output length match the shorter of the two inputs
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-shortest",
		outputName,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if ffmpeg is installed
		if strings.Contains(err.Error(), "executable file not found") {
			fmt.Printf("\n❌ ffmpeg not found.\n")
			fmt.Printf("\n📋 Install ffmpeg:\n")
			fmt.Printf("   macOS:  brew install ffmpeg\n")
			fmt.Printf("   Ubuntu: sudo apt install ffmpeg\n")
			fmt.Printf("   Windows: choco install ffmpeg\n\n")
			return "", fmt.Errorf("ffmpeg required for audio_video_merge - please install it")
		}
		return "", fmt.Errorf("ffmpeg failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("✅ Merged video saved: %s\n", outputName)
	return outputName, nil
}

// videoScript converts content into a Veo-optimized video prompt with synchronized dialogue
// This is the KEY command for creating narrated videos - Veo 3.1 will generate video WITH
// synchronized audio/speech, so we don't need separate TTS!
func (r *Runtime) videoScript(ctx context.Context, style string, input string) (string, error) {
	r.log("VIDEO_SCRIPT: style=%s, input=%d bytes", style, len(input))

	if r.gemini == nil && r.claude == nil {
		return "", fmt.Errorf("GEMINI_API_KEY or CLAUDE_API_KEY required for video script generation")
	}

	if input == "" {
		return "", fmt.Errorf("no content to convert - pipe content into video_script")
	}

	// Default style
	if style == "" {
		style = "news anchor"
	}

	fmt.Printf("📝 Converting to Veo video prompt (style: %s)...\n", style)
	fmt.Printf("   Veo 3.1 will generate synchronized audio automatically!\n")

	prompt := fmt.Sprintf(`Convert the following content into a video generation prompt for Veo 3.1 with SYNCHRONIZED DIALOGUE.

CONTENT TO CONVERT:
%s

STYLE: %s

CRITICAL REQUIREMENTS:
1. Veo 3.1 generates video WITH synchronized audio - the speaker's lips will move in sync with dialogue
2. Put ALL spoken words in quotes - Veo will generate speech for quoted text
3. Keep dialogue UNDER 20 words (must fit in 8 seconds at natural speaking pace)
4. Describe the visual scene, camera angle, lighting
5. Add SFX: for sound effects, Ambient: for background sounds
6. For vertical/shorts: include "portrait 9:16 aspect ratio"

OUTPUT FORMAT (return ONLY this prompt, no explanation):
[Visual scene description, camera angle, lighting]. [Speaker description] speaking directly to camera: "[DIALOGUE UNDER 20 WORDS]". SFX: [sound]. Ambient: [background].

EXAMPLE:
Modern news studio with blue accent lighting, medium close-up shot, portrait 9:16 aspect ratio. Professional female news anchor in business attire speaking directly to camera: "Breaking tonight - tech giants announce major layoffs as AI reshapes the workforce." SFX: subtle news intro tone. Ambient: quiet studio atmosphere.

NOW CONVERT THE CONTENT ABOVE:`, input, style)

	var result string
	var err error

	if r.claude != nil {
		result, err = r.claude.Chat(ctx, prompt)
	} else {
		result, err = r.geminiCall(ctx, prompt)
	}

	if err != nil {
		return "", fmt.Errorf("video script generation failed: %w", err)
	}

	result = strings.TrimSpace(result)

	// Validate it has quoted dialogue
	if !strings.Contains(result, "\"") {
		fmt.Printf("⚠️  Warning: No quoted dialogue found - Veo may not generate speech\n")
	}

	fmt.Printf("✅ Video prompt ready - Veo will sync lips to dialogue!\n")

	return result, nil
}

// youtubeUpload uploads a video to YouTube (or YouTube Shorts)
func (r *Runtime) youtubeUpload(ctx context.Context, title string, input string, isShorts bool) (string, error) {
	r.log("YOUTUBE_UPLOAD: title=%s, input=%s, shorts=%v", title, input, isShorts)

	if r.google == nil {
		return "", fmt.Errorf("GOOGLE_CREDENTIALS_FILE required for YouTube upload")
	}

	// Input should be a video file path
	videoPath := strings.TrimSpace(input)
	if videoPath == "" {
		return "", fmt.Errorf("no video file path provided - pipe a video path into youtube_upload")
	}

	// Check if file exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// For Shorts, add #Shorts to title if not present
	finalTitle := title
	description := "Uploaded via AgentScript"
	if isShorts {
		if !strings.Contains(title, "#Shorts") && !strings.Contains(title, "#shorts") {
			finalTitle = title + " #Shorts"
		}
		description = "Uploaded via AgentScript #Shorts"
		fmt.Printf("📱 Uploading to YouTube Shorts: %s...\n", finalTitle)
	} else {
		fmt.Printf("📺 Uploading to YouTube: %s...\n", finalTitle)
	}

	videoURL, err := r.google.UploadToYouTube(ctx, videoPath, finalTitle, description)
	if err != nil {
		return "", fmt.Errorf("YouTube upload failed: %w", err)
	}

	fmt.Printf("✅ Video uploaded: %s\n", videoURL)
	return videoURL, nil
}

// confirm prompts user for confirmation before continuing
func (r *Runtime) confirm(ctx context.Context, message string, input string) (string, error) {
	r.log("CONFIRM: %s", message)

	// Display what we're confirming
	if message == "" {
		message = "Continue with this action?"
	}

	fmt.Printf("\n⚠️  CONFIRMATION REQUIRED\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("%s\n", message)
	if input != "" {
		// Show truncated input if it's a file path or short
		if len(input) < 200 {
			fmt.Printf("Input: %s\n", input)
		} else {
			fmt.Printf("Input: %s... (%d bytes)\n", input[:100], len(input))
		}
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Proceed? [y/N]: ")

	var response string
	fmt.Scanln(&response)

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		fmt.Printf("✅ Confirmed. Continuing...\n")
		return input, nil // Pass through the input unchanged
	}

	return "", fmt.Errorf("operation cancelled by user")
}

// githubPages deploys content as a React SPA to GitHub Pages
func (r *Runtime) githubPages(ctx context.Context, title string, input string) (string, error) {
	r.log("GITHUB_PAGES: title=%s, input=%d bytes", title, len(input))

	if r.github == nil {
		fmt.Printf("\n❌ GitHub API not configured.\n")
		fmt.Printf("\n📋 Setup GitHub OAuth:\n")
		fmt.Printf("   1. Go to: https://github.com/settings/developers\n")
		fmt.Printf("   2. Click 'New OAuth App'\n")
		fmt.Printf("   3. Fill in app name, homepage URL, callback URL (http://localhost)\n")
		fmt.Printf("   4. Copy Client ID and Client Secret\n")
		fmt.Printf("\n💡 Add to your .env file:\n")
		fmt.Printf("   GITHUB_CLIENT_ID=your-client-id\n")
		fmt.Printf("   GITHUB_CLIENT_SECRET=your-client-secret\n\n")
		return "", fmt.Errorf("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET required")
	}

	if input == "" {
		return "", fmt.Errorf("no content to deploy - pipe content into github_pages")
	}

	if title == "" {
		title = "AgentScript Page"
	}

	// Create repo name from title
	repoName := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	repoName = strings.ReplaceAll(repoName, "'", "")
	repoName = strings.ReplaceAll(repoName, "\"", "")

	// Use Claude if available, otherwise fall back to Gemini
	var reactCode string
	var err error

	if r.claude != nil {
		fmt.Printf("🎨 Generating React SPA with Claude...\n")
		reactCode, err = r.claude.GenerateReactSPA(ctx, title, input)
		if err != nil {
			return "", fmt.Errorf("Claude React generation failed: %w", err)
		}
	} else if r.gemini != nil {
		fmt.Printf("🎨 Generating React SPA with Gemini...\n")
		reactCode, err = r.generateReactSPA(ctx, title, input)
		if err != nil {
			return "", fmt.Errorf("Gemini React generation failed: %w", err)
		}
	} else {
		fmt.Printf("\n❌ No AI API key configured for React SPA generation.\n")
		fmt.Printf("\n📋 Get your API keys:\n")
		fmt.Printf("   Claude (recommended): https://console.anthropic.com/settings/keys\n")
		fmt.Printf("   Gemini:               https://aistudio.google.com/apikey\n")
		fmt.Printf("\n💡 Then add to your .env file:\n")
		fmt.Printf("   CLAUDE_API_KEY=sk-ant-...\n")
		fmt.Printf("   GEMINI_API_KEY=...\n\n")
		return "", fmt.Errorf("CLAUDE_API_KEY or GEMINI_API_KEY required for github_pages")
	}

	fmt.Printf("🚀 Deploying to GitHub Pages: %s...\n", title)

	pagesURL, err := r.github.DeployReactSPA(ctx, repoName, title, reactCode)
	if err != nil {
		return "", fmt.Errorf("GitHub Pages deployment failed: %w", err)
	}

	fmt.Printf("✅ Deployed to: %s\n", pagesURL)
	fmt.Printf("   (Note: May take 1-2 minutes to go live)\n")

	return pagesURL, nil
}

// generateReactSPA uses Gemini to create a React single-page application
func (r *Runtime) generateReactSPA(ctx context.Context, title string, content string) (string, error) {
	prompt := fmt.Sprintf(`Generate a beautiful, modern React single-page application (SPA) for the following content.

TITLE: %s

CONTENT:
%s

REQUIREMENTS:
1. Output ONLY the complete HTML file with embedded React (using babel standalone)
2. Use React hooks (useState, useEffect)
3. Modern, dark theme UI with gradients and animations
4. Responsive design with Tailwind CSS (via CDN)
5. Include smooth scroll animations
6. Add a navigation header if content has sections
7. Use React icons or emojis for visual appeal
8. Make it visually stunning - this is for a hackathon demo!
9. Include a footer crediting "Built with AgentScript"

OUTPUT FORMAT:
Return ONLY the HTML code starting with <!DOCTYPE html> and ending with </html>
No markdown, no explanation, just the raw HTML/React code.`, title, content)

	result, err := r.geminiCall(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Clean up the response - remove any markdown code blocks if present
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```html")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	// Validate it looks like HTML
	if !strings.HasPrefix(result, "<!DOCTYPE html>") && !strings.HasPrefix(result, "<html") {
		// Try to find HTML in the response
		if idx := strings.Index(result, "<!DOCTYPE html>"); idx != -1 {
			result = result[idx:]
		} else if idx := strings.Index(result, "<html"); idx != -1 {
			result = result[idx:]
		} else {
			return "", fmt.Errorf("Gemini did not return valid HTML")
		}
	}

	return result, nil
}

// githubPagesHTML deploys content as simple HTML to GitHub Pages (no AI generation)
func (r *Runtime) githubPagesHTML(ctx context.Context, title string, input string) (string, error) {
	r.log("GITHUB_PAGES_HTML: title=%s, input=%d bytes", title, len(input))

	if r.github == nil {
		fmt.Printf("\n❌ GitHub API not configured.\n")
		fmt.Printf("\n📋 Setup GitHub OAuth:\n")
		fmt.Printf("   1. Go to: https://github.com/settings/developers\n")
		fmt.Printf("   2. Click 'New OAuth App'\n")
		fmt.Printf("   3. Fill in app name, homepage URL, callback URL (http://localhost)\n")
		fmt.Printf("   4. Copy Client ID and Client Secret\n")
		fmt.Printf("\n💡 Add to your .env file:\n")
		fmt.Printf("   GITHUB_CLIENT_ID=your-client-id\n")
		fmt.Printf("   GITHUB_CLIENT_SECRET=your-client-secret\n\n")
		return "", fmt.Errorf("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET required")
	}

	if input == "" {
		return "", fmt.Errorf("no content to deploy - pipe content into github_pages_html")
	}

	if title == "" {
		title = "AgentScript Page"
	}

	// Create repo name from title
	repoName := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	repoName = strings.ReplaceAll(repoName, "'", "")
	repoName = strings.ReplaceAll(repoName, "\"", "")

	fmt.Printf("🚀 Deploying simple HTML to GitHub Pages: %s...\n", title)

	pagesURL, err := r.github.DeployToPages(ctx, repoName, title, input)
	if err != nil {
		return "", fmt.Errorf("GitHub Pages deployment failed: %w", err)
	}

	fmt.Printf("✅ Deployed to: %s\n", pagesURL)
	fmt.Printf("   (Note: May take 1-2 minutes to go live)\n")

	return pagesURL, nil
}

// log prints verbose output if enabled
func (r *Runtime) log(format string, args ...any) {
	if r.verbose {
		fmt.Printf("[agentscript] "+format+"\n", args...)
	}
}
