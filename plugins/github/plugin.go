package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/agentscript/pkg/plugin"
)

// Plugin wraps GitHubClient as a Plugin.
type Plugin struct {
	client *GitHubClient
}

// NewPlugin creates a github plugin.
func NewPlugin(client *GitHubClient) *Plugin {
	return &Plugin{client: client}
}

func (p *Plugin) Name() string { return "github" }

func (p *Plugin) Commands() map[string]plugin.CommandFunc {
	return map[string]plugin.CommandFunc{
		"github_pages_html": p.githubPagesHTML,
	}
}

// githubPagesHTML deploys raw HTML directly to GitHub Pages — no AI needed.
func (p *Plugin) githubPagesHTML(ctx context.Context, args []string, input string) (string, error) {
	if p.client == nil {
		return "", githubNotConfiguredErr()
	}
	if input == "" {
		return "", fmt.Errorf("no content to deploy — pipe content into github_pages_html")
	}
	title := plugin.Arg(args, 0)
	if title == "" {
		title = "AgentScript Page"
	}
	repoName := sanitizeRepoName(title)
	fmt.Printf("🚀 Deploying HTML to GitHub Pages: %s...\n", title)
	pagesURL, err := p.client.DeployToPages(ctx, repoName, title, input)
	if err != nil {
		return "", fmt.Errorf("GitHub Pages deployment failed: %w", err)
	}
	fmt.Printf("✅ Deployed to: %s\n   (Note: May take 1-2 minutes to go live)\n", pagesURL)
	return pagesURL, nil
}

func sanitizeRepoName(title string) string {
	name := strings.ToLower(title)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "'", "")
	name = strings.ReplaceAll(name, "\"", "")
	return name
}

func githubNotConfiguredErr() error {
	return fmt.Errorf(
		"GitHub API not configured\n" +
			"Setup: https://github.com/settings/developers → New OAuth App\n" +
			"Then set GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET",
	)
}
