package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const baseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiClient is a simple HTTP client for the Gemini API
type GeminiClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGeminiClient creates a new Gemini client
func NewGeminiClient(apiKey, model string) *GeminiClient {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiClient{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// Request structures
type generateRequest struct {
	Contents         []content         `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
	FileData   *fileData   `json:"file_data,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type fileData struct {
	MimeType string `json:"mime_type"`
	FileURI  string `json:"file_uri"`
}

type generationConfig struct {
	ResponseMimeType string `json:"response_mime_type,omitempty"`
}

// Response structures
type generateResponse struct {
	Candidates []candidate `json:"candidates"`
	Error      *apiError   `json:"error,omitempty"`
}

type candidate struct {
	Content content `json:"content"`
}

type apiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// GenerateContent sends a prompt to Gemini and returns the response text
func (c *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, c.model, c.apiKey)

	reqBody := generateRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: prompt},
				},
			},
		},
	}

	return c.doRequest(ctx, url, reqBody)
}

// AnalyzeImage analyzes an image file with a prompt
func (c *GeminiClient) AnalyzeImage(ctx context.Context, imagePath, prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, c.model, c.apiKey)

	// Read and encode image
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	mimeType := getMimeType(imagePath)
	encoded := base64.StdEncoding.EncodeToString(imageData)

	reqBody := generateRequest{
		Contents: []content{
			{
				Parts: []part{
					{
						InlineData: &inlineData{
							MimeType: mimeType,
							Data:     encoded,
						},
					},
					{Text: prompt},
				},
			},
		},
	}

	return c.doRequest(ctx, url, reqBody)
}

// AnalyzeVideo analyzes a video with a prompt (using File API for larger files)
func (c *GeminiClient) AnalyzeVideo(ctx context.Context, videoPath, prompt string) (string, error) {
	// For videos, we need to upload to File API first, then reference
	// For now, support small videos via inline data (< 20MB)

	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat video: %w", err)
	}

	// Check file size (limit to 20MB for inline)
	if fileInfo.Size() > 20*1024*1024 {
		return "", fmt.Errorf("video too large for inline processing (max 20MB). Use File API for larger videos")
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, c.model, c.apiKey)

	// Read and encode video
	videoData, err := os.ReadFile(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to read video: %w", err)
	}

	mimeType := getMimeType(videoPath)
	encoded := base64.StdEncoding.EncodeToString(videoData)

	reqBody := generateRequest{
		Contents: []content{
			{
				Parts: []part{
					{
						InlineData: &inlineData{
							MimeType: mimeType,
							Data:     encoded,
						},
					},
					{Text: prompt},
				},
			},
		},
	}

	return c.doRequest(ctx, url, reqBody)
}

// GenerateImage generates an image using Imagen model
func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	// Use Imagen 4 - Imagen 3 has been shut down
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/imagen-4.0-generate-001:predict?key=%s", c.apiKey)

	reqBody := map[string]interface{}{
		"instances": []map[string]string{
			{"prompt": prompt},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != 200 {
		var errResp struct {
			Error *apiError `json:"error,omitempty"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != nil {
			return nil, fmt.Errorf("API error: %s (code %d)", errResp.Error.Message, errResp.Error.Code)
		}
		return nil, fmt.Errorf("API error: status %d - %s", resp.StatusCode, string(body))
	}

	// Parse response
	var imgResp struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
		} `json:"predictions"`
	}

	if err := json.Unmarshal(body, &imgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(imgResp.Predictions) == 0 || imgResp.Predictions[0].BytesBase64Encoded == "" {
		return nil, fmt.Errorf("no image generated")
	}

	// Decode base64 image
	imageBytes, err := base64.StdEncoding.DecodeString(imgResp.Predictions[0].BytesBase64Encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return imageBytes, nil
}

func (c *GeminiClient) doRequest(ctx context.Context, url string, reqBody generateRequest) (string, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var genResp generateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if genResp.Error != nil {
		return "", fmt.Errorf("API error: %s (code %d)", genResp.Error.Message, genResp.Error.Code)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}

func getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mpeg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// GenerateVideo generates a video using Veo model
func (c *GeminiClient) GenerateVideo(ctx context.Context, prompt string) (string, error) {
	// Use Veo 3.1 for video generation with predictLongRunning endpoint
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/veo-3.1-generate-preview:predictLongRunning?key=%s", c.apiKey)

	reqBody := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"aspectRatio": "16:9",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != 200 {
		var errResp struct {
			Error *apiError `json:"error,omitempty"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != nil {
			return "", fmt.Errorf("API error: %s (code %d)", errResp.Error.Message, errResp.Error.Code)
		}
		return "", fmt.Errorf("API error: status %d - %s", resp.StatusCode, string(body))
	}

	// Parse response - this returns an operation name for polling
	var opResp struct {
		Name  string    `json:"name"`
		Error *apiError `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &opResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if opResp.Error != nil {
		return "", fmt.Errorf("API error: %s (code %d)", opResp.Error.Message, opResp.Error.Code)
	}

	if opResp.Name == "" {
		return "", fmt.Errorf("no operation name returned. Response: %s", string(body))
	}

	// Poll for completion
	return c.pollVideoOperation(ctx, opResp.Name)
}

// pollVideoOperation polls for video generation completion
func (c *GeminiClient) pollVideoOperation(ctx context.Context, operationName string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s?key=%s", operationName, c.apiKey)

	for i := 0; i < 120; i++ { // Poll for up to 10 minutes
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		fmt.Printf("⏳ Polling for video completion (%d/120)...\n", i+1)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue // Retry on network error
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Try parsing with different response formats
		var opStatus struct {
			Done     bool `json:"done"`
			Response struct {
				// Veo 3.1 format
				GenerateVideoResponse struct {
					GeneratedSamples []struct {
						Video struct {
							URI string `json:"uri"`
						} `json:"video"`
					} `json:"generatedSamples"`
				} `json:"generateVideoResponse"`
				// Alternative format
				GeneratedVideos []struct {
					Video struct {
						URI string `json:"uri"`
					} `json:"video"`
				} `json:"generatedVideos"`
			} `json:"response"`
			Error *apiError `json:"error,omitempty"`
		}

		if err := json.Unmarshal(body, &opStatus); err != nil {
			fmt.Printf("Parse error: %v, body: %s\n", err, string(body)[:min(200, len(body))])
			continue
		}

		if opStatus.Error != nil {
			return "", fmt.Errorf("video generation failed: %s", opStatus.Error.Message)
		}

		if opStatus.Done {
			// Try generateVideoResponse format first
			if len(opStatus.Response.GenerateVideoResponse.GeneratedSamples) > 0 {
				uri := opStatus.Response.GenerateVideoResponse.GeneratedSamples[0].Video.URI
				if uri != "" {
					return uri, nil
				}
			}
			// Try generatedVideos format
			if len(opStatus.Response.GeneratedVideos) > 0 {
				uri := opStatus.Response.GeneratedVideos[0].Video.URI
				if uri != "" {
					return uri, nil
				}
			}
			return "", fmt.Errorf("video generation completed but no video URI found. Response: %s", string(body))
		}
	}

	return "", fmt.Errorf("video generation timed out after 10 minutes")
}

// GenerateVideoFromImages generates a video from multiple images
func (c *GeminiClient) GenerateVideoFromImages(ctx context.Context, imagePaths []string, prompt string) (string, error) {
	// Use Veo 3.1 with first frame image
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/veo-3.1-generate-preview:predictLongRunning?key=%s", c.apiKey)

	// Use first image as starting frame
	if len(imagePaths) == 0 {
		return "", fmt.Errorf("no images provided")
	}

	imageData, err := os.ReadFile(imagePaths[0])
	if err != nil {
		return "", fmt.Errorf("failed to read image %s: %w", imagePaths[0], err)
	}
	mimeType := getMimeType(imagePaths[0])
	encoded := base64.StdEncoding.EncodeToString(imageData)

	reqBody := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": prompt,
				"image": map[string]interface{}{
					"bytesBase64Encoded": encoded,
					"mimeType":           mimeType,
				},
			},
		},
		"parameters": map[string]interface{}{
			"aspectRatio": "16:9",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != 200 {
		var errResp struct {
			Error *apiError `json:"error,omitempty"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != nil {
			return "", fmt.Errorf("API error: %s (code %d)", errResp.Error.Message, errResp.Error.Code)
		}
		return "", fmt.Errorf("API error: status %d - %s", resp.StatusCode, string(body))
	}

	var opResp struct {
		Name  string    `json:"name"`
		Error *apiError `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &opResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if opResp.Error != nil {
		return "", fmt.Errorf("API error: %s (code %d)", opResp.Error.Message, opResp.Error.Code)
	}

	if opResp.Name == "" {
		return "", fmt.Errorf("no operation name returned. Response: %s", string(body))
	}

	return c.pollVideoOperation(ctx, opResp.Name)
}

// DownloadFile downloads a file from the Gemini API and saves it locally
func (c *GeminiClient) DownloadFile(ctx context.Context, fileURI string, outputPath string) (string, error) {
	// Add API key to the URI
	downloadURL := fileURI
	if strings.Contains(downloadURL, "?") {
		downloadURL += "&key=" + c.apiKey
	} else {
		downloadURL += "?key=" + c.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Copy response body to file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return outputPath, nil
}
