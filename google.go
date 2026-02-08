package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/tasks/v1"
	"google.golang.org/api/youtube/v3"
)

// GoogleClient handles all Google APIs
type GoogleClient struct {
	gmail    *gmail.Service
	calendar *calendar.Service
	drive    *drive.Service
	docs     *docs.Service
	sheets   *sheets.Service
	tasks    *tasks.Service
	people   *people.Service
	youtube  *youtube.Service
}

// NewGoogleClient creates a new Google API client with OAuth2
func NewGoogleClient(ctx context.Context, credentialsFile, tokenFile string) (*GoogleClient, error) {
	// Read credentials
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	// Configure OAuth2 with all required scopes
	config, err := google.ConfigFromJSON(b,
		// Gmail - send and read profile
		gmail.GmailSendScope,
		gmail.GmailReadonlyScope,
		// Calendar - full access for Meet links
		calendar.CalendarEventsScope,
		calendar.CalendarScope,
		// Drive - file creation
		drive.DriveFileScope,
		drive.DriveScope,
		// Docs - full access
		docs.DocumentsScope,
		// Sheets - full access
		sheets.SpreadsheetsScope,
		// Tasks - full access
		tasks.TasksScope,
		// Contacts - read
		people.ContactsReadonlyScope,
		// YouTube - read
		youtube.YoutubeReadonlyScope,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	// Get token
	token, err := getToken(ctx, config, tokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to get token: %w", err)
	}

	// Create HTTP client
	client := config.Client(ctx, token)

	// Create all services
	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail service: %w", err)
	}

	calendarSvc, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Calendar service: %w", err)
	}

	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %w", err)
	}

	docsSvc, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Docs service: %w", err)
	}

	sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Sheets service: %w", err)
	}

	tasksSvc, err := tasks.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Tasks service: %w", err)
	}

	peopleSvc, err := people.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create People service: %w", err)
	}

	youtubeSvc, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create YouTube service: %w", err)
	}

	return &GoogleClient{
		gmail:    gmailSvc,
		calendar: calendarSvc,
		drive:    driveSvc,
		docs:     docsSvc,
		sheets:   sheetsSvc,
		tasks:    tasksSvc,
		people:   peopleSvc,
		youtube:  youtubeSvc,
	}, nil
}

// getToken retrieves token from file or initiates OAuth flow
func getToken(ctx context.Context, config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	// Try to load existing token
	token, err := loadToken(tokenFile)
	if err == nil {
		return token, nil
	}

	// No token, need to do OAuth dance
	token, err = getTokenFromWeb(ctx, config)
	if err != nil {
		return nil, err
	}

	// Save token for future use
	saveToken(tokenFile, token)
	return token, nil
}

// getTokenFromWeb starts OAuth flow
func getTokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	// Use localhost redirect for desktop app
	config.RedirectURL = "http://localhost:8085/callback"

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\n🔐 Google OAuth2 Authorization Required\n")
	fmt.Printf("1. Open this URL in your browser:\n\n%s\n\n", authURL)

	// Start local server to receive callback
	codeChan := make(chan string)
	errChan := make(chan error)

	server := &http.Server{Addr: ":8085"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			return
		}
		fmt.Fprintf(w, "<h1>✅ Authorization successful!</h1><p>You can close this window.</p>")
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	fmt.Println("2. Waiting for authorization...")

	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authorization timeout")
	}

	server.Shutdown(ctx)

	// Exchange code for token
	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange code: %w", err)
	}

	fmt.Println("✅ Authorization successful!")
	return token, nil
}

func loadToken(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

func saveToken(file string, token *oauth2.Token) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// ============================================================================
// Gmail
// ============================================================================

// SendEmail sends an email via Gmail API
func (g *GoogleClient) SendEmail(ctx context.Context, to, subject, body string) error {
	// Get user's email address
	profile, err := g.gmail.Users.GetProfile("me").Do()
	if err != nil {
		return fmt.Errorf("unable to get user profile: %w", err)
	}
	from := profile.EmailAddress

	// Create email message
	msgStr := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	msg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(msgStr)),
	}

	// Send
	_, err = g.gmail.Users.Messages.Send("me", msg).Do()
	if err != nil {
		return fmt.Errorf("unable to send email: %w", err)
	}

	return nil
}

// ============================================================================
// Calendar
// ============================================================================

// CreateCalendarEvent creates an event in Google Calendar
func (g *GoogleClient) CreateCalendarEvent(ctx context.Context, summary, description, startTime, endTime string) (*calendar.Event, error) {
	event := &calendar.Event{
		Summary:     summary,
		Description: description,
		Start: &calendar.EventDateTime{
			DateTime: startTime,
			TimeZone: "America/Los_Angeles",
		},
		End: &calendar.EventDateTime{
			DateTime: endTime,
			TimeZone: "America/Los_Angeles",
		},
	}

	event, err := g.calendar.Events.Insert("primary", event).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create event: %w", err)
	}

	return event, nil
}

// CreateMeetEvent creates a Google Calendar event with Meet link
func (g *GoogleClient) CreateMeetEvent(ctx context.Context, summary, description, startTime, endTime string) (*calendar.Event, error) {
	event := &calendar.Event{
		Summary:     summary,
		Description: description,
		Start: &calendar.EventDateTime{
			DateTime: startTime,
			TimeZone: "America/Los_Angeles",
		},
		End: &calendar.EventDateTime{
			DateTime: endTime,
			TimeZone: "America/Los_Angeles",
		},
		ConferenceData: &calendar.ConferenceData{
			CreateRequest: &calendar.CreateConferenceRequest{
				RequestId: fmt.Sprintf("meet-%d", time.Now().UnixNano()),
				ConferenceSolutionKey: &calendar.ConferenceSolutionKey{
					Type: "hangoutsMeet",
				},
			},
		},
	}

	event, err := g.calendar.Events.Insert("primary", event).ConferenceDataVersion(1).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create Meet event: %w", err)
	}

	return event, nil
}

// ============================================================================
// Google Drive
// ============================================================================

// SaveToDrive saves content to Google Drive
func (g *GoogleClient) SaveToDrive(ctx context.Context, path, content string) (*drive.File, error) {
	// Parse path to get folder and filename
	parts := strings.Split(path, "/")
	filename := parts[len(parts)-1]
	folderPath := parts[:len(parts)-1]

	// Find or create folder hierarchy
	parentID := "root"
	for _, folderName := range folderPath {
		if folderName == "" {
			continue
		}
		// Search for existing folder
		query := fmt.Sprintf("name='%s' and '%s' in parents and mimeType='application/vnd.google-apps.folder' and trashed=false", folderName, parentID)
		result, err := g.drive.Files.List().Q(query).Fields("files(id, name)").Do()
		if err != nil {
			return nil, fmt.Errorf("unable to search for folder: %w", err)
		}

		if len(result.Files) > 0 {
			parentID = result.Files[0].Id
		} else {
			// Create folder
			folder := &drive.File{
				Name:     folderName,
				MimeType: "application/vnd.google-apps.folder",
				Parents:  []string{parentID},
			}
			created, err := g.drive.Files.Create(folder).Do()
			if err != nil {
				return nil, fmt.Errorf("unable to create folder: %w", err)
			}
			parentID = created.Id
		}
	}

	// Create file
	file := &drive.File{
		Name:    filename,
		Parents: []string{parentID},
	}

	created, err := g.drive.Files.Create(file).Media(strings.NewReader(content)).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create file: %w", err)
	}

	return created, nil
}

// ============================================================================
// Google Docs
// ============================================================================

// CreateDoc creates a Google Doc with content
func (g *GoogleClient) CreateDoc(ctx context.Context, title, content string) (*docs.Document, error) {
	// Create the document
	doc := &docs.Document{
		Title: title,
	}

	created, err := g.docs.Documents.Create(doc).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create document: %w", err)
	}

	// Insert content
	requests := []*docs.Request{
		{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{
					Index: 1,
				},
				Text: content,
			},
		},
	}

	_, err = g.docs.Documents.BatchUpdate(created.DocumentId, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to insert content: %w", err)
	}

	return created, nil
}

// ============================================================================
// Google Sheets
// ============================================================================

// AppendToSheet appends data to a Google Sheet
func (g *GoogleClient) AppendToSheet(ctx context.Context, spreadsheetID, sheetName, content string) error {
	// Parse content into rows (split by newlines, columns by tabs or |)
	lines := strings.Split(content, "\n")
	var values [][]interface{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Try tab first, then pipe
		var cells []string
		if strings.Contains(line, "\t") {
			cells = strings.Split(line, "\t")
		} else if strings.Contains(line, "|") {
			cells = strings.Split(line, "|")
		} else {
			cells = []string{line}
		}
		var row []interface{}
		for _, cell := range cells {
			row = append(row, strings.TrimSpace(cell))
		}
		values = append(values, row)
	}

	// If no values, just add the content as a single cell
	if len(values) == 0 {
		values = [][]interface{}{{content}}
	}

	rangeStr := sheetName
	if sheetName == "" {
		rangeStr = "Sheet1"
	}

	vr := &sheets.ValueRange{
		Values: values,
	}

	_, err := g.sheets.Spreadsheets.Values.Append(spreadsheetID, rangeStr, vr).
		ValueInputOption("USER_ENTERED").
		InsertDataOption("INSERT_ROWS").
		Do()
	if err != nil {
		return fmt.Errorf("unable to append to sheet: %w", err)
	}

	return nil
}

// CreateSheet creates a new Google Sheet
func (g *GoogleClient) CreateSheet(ctx context.Context, title string) (*sheets.Spreadsheet, error) {
	spreadsheet := &sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{
			Title: title,
		},
	}

	created, err := g.sheets.Spreadsheets.Create(spreadsheet).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create spreadsheet: %w", err)
	}

	return created, nil
}

// ============================================================================
// Google Tasks
// ============================================================================

// CreateTask creates a task in Google Tasks
func (g *GoogleClient) CreateTask(ctx context.Context, title, notes string) (*tasks.Task, error) {
	// Get default task list
	taskLists, err := g.tasks.Tasklists.List().Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get task lists: %w", err)
	}

	var taskListID string
	if len(taskLists.Items) > 0 {
		taskListID = taskLists.Items[0].Id
	} else {
		// Create a task list
		newList := &tasks.TaskList{Title: "AgentScript Tasks"}
		created, err := g.tasks.Tasklists.Insert(newList).Do()
		if err != nil {
			return nil, fmt.Errorf("unable to create task list: %w", err)
		}
		taskListID = created.Id
	}

	// Create task
	task := &tasks.Task{
		Title: title,
		Notes: notes,
	}

	created, err := g.tasks.Tasks.Insert(taskListID, task).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create task: %w", err)
	}

	return created, nil
}

// ============================================================================
// Google Contacts (People API)
// ============================================================================

// FindContact finds a contact by name
func (g *GoogleClient) FindContact(ctx context.Context, name string) ([]*people.Person, error) {
	// Search contacts
	result, err := g.people.People.SearchContacts().Query(name).ReadMask("names,emailAddresses").Do()
	if err != nil {
		return nil, fmt.Errorf("unable to search contacts: %w", err)
	}

	var contacts []*people.Person
	for _, res := range result.Results {
		contacts = append(contacts, res.Person)
	}

	return contacts, nil
}

// ============================================================================
// YouTube
// ============================================================================

// SearchYouTube searches for videos
func (g *GoogleClient) SearchYouTube(ctx context.Context, query string, maxResults int64) ([]*youtube.SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}

	call := g.youtube.Search.List([]string{"snippet"}).
		Q(query).
		Type("video").
		MaxResults(maxResults)

	response, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to search YouTube: %w", err)
	}

	return response.Items, nil
}
