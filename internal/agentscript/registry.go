package agentscript

// registry.go is the single place that knows about all plugins.
// It imports every pkg/* plugin and wires them into the Registry.
//
// Adding a new package = one new import + one new Register() call here.
// Nothing else changes — not runtime.go, not the switch, not the grammar.
//
// This file is the composition root for the plugin system.

import (
	"os"

	"github.com/vinodhalaharvi/agentscript/pkg/openai"
	"github.com/vinodhalaharvi/agentscript/pkg/plugin"
	"github.com/vinodhalaharvi/agentscript/plugins/agent"
	"github.com/vinodhalaharvi/agentscript/plugins/cache"
	"github.com/vinodhalaharvi/agentscript/plugins/cloudrun"
	agcrypto "github.com/vinodhalaharvi/agentscript/plugins/crypto"
	"github.com/vinodhalaharvi/agentscript/plugins/datatable"
	agexec "github.com/vinodhalaharvi/agentscript/plugins/exec"
	aggithub "github.com/vinodhalaharvi/agentscript/plugins/github"
	"github.com/vinodhalaharvi/agentscript/plugins/huggingface"
	"github.com/vinodhalaharvi/agentscript/plugins/jobsearch"
	"github.com/vinodhalaharvi/agentscript/plugins/kg"
	"github.com/vinodhalaharvi/agentscript/plugins/mcp"
	"github.com/vinodhalaharvi/agentscript/plugins/mcpagent"
	"github.com/vinodhalaharvi/agentscript/plugins/mcpsearch"
	"github.com/vinodhalaharvi/agentscript/plugins/network"
	"github.com/vinodhalaharvi/agentscript/plugins/news"
	"github.com/vinodhalaharvi/agentscript/plugins/notify"
	"github.com/vinodhalaharvi/agentscript/plugins/ollama"
	"github.com/vinodhalaharvi/agentscript/plugins/pdffill"
	"github.com/vinodhalaharvi/agentscript/plugins/perplexity"
	"github.com/vinodhalaharvi/agentscript/plugins/plugagent"
	"github.com/vinodhalaharvi/agentscript/plugins/rag"
	"github.com/vinodhalaharvi/agentscript/plugins/reddit"
	"github.com/vinodhalaharvi/agentscript/plugins/review"
	"github.com/vinodhalaharvi/agentscript/plugins/rss"
	agstock "github.com/vinodhalaharvi/agentscript/plugins/stock"
	"github.com/vinodhalaharvi/agentscript/plugins/twitter"
	"github.com/vinodhalaharvi/agentscript/plugins/weather"
	"github.com/vinodhalaharvi/agentscript/plugins/whatsapp"
)

// buildRegistry constructs the plugin registry from the runtime's clients.
// Called once from NewRuntime — the result is stored on r.registry.
func (r *Runtime) buildRegistry(c *cache.Cache) *plugin.Registry {
	reg := plugin.NewRegistry()

	// --- Data plugins ---
	reg.Register(weather.NewPlugin(c, r.verbose))
	reg.Register(agstock.NewPlugin(r.searchKey, c, r.verbose))
	reg.Register(agcrypto.NewPlugin(c, r.verbose))
	reg.Register(news.NewPlugin(r.searchKey, c, r.verbose))
	reg.Register(reddit.NewPlugin(c, r.verbose))
	reg.Register(rss.NewPlugin(c, r.verbose))
	reg.Register(twitter.NewPlugin(r.verbose))
	reg.Register(jobsearch.NewPlugin(r.searchKey, c, r.verbose))

	// --- Notification plugins ---
	reg.Register(notify.NewPlugin(r.verbose))
	reg.Register(whatsapp.NewPlugin(r.verbose))

	// --- HuggingFace ---
	reg.Register(huggingface.NewPlugin(r.verbose))

	// --- Perplexity AI Search ---
	// API key from PERPLEXITY_API_KEY. Falls back gracefully if not set.
	if pplxKey := os.Getenv("PERPLEXITY_API_KEY"); pplxKey != "" {
		reg.Register(perplexity.NewPlugin(pplxKey, "", r.verbose))
	}

	// --- MCP — stateful, shares the same client as the runtime ---
	reg.Register(mcp.NewPlugin(r.mcp))

	// --- MCP Agent — AI-driven tool selection over connected MCP servers
	// Reasoner is Claude if available, Gemini as fallback.
	// Same MCPClient as above — shares already-connected servers.
	var mcpReasoner mcpagent.Reasoner
	if r.claude != nil {
		mcpReasoner = r.claude.Chat
	} else if r.gemini != nil {
		mcpReasoner = r.gemini.GenerateContent
	}
	if mcpReasoner != nil {
		reg.Register(mcpagent.NewPlugin(r.mcp, mcpReasoner, r.verbose))
	}

	// --- Agent — natural language to DSL via Claude
	// r.RunDSL is the Executor seam — a functional field that lets the
	// agent plugin run DSL strings without knowing about the Runtime.
	if r.claude != nil {
		reg.Register(agent.NewPlugin(r.claude, r.RunDSL, r.verbose))
	}

	// --- Network diagnostics — pure Go, no external deps ---
	reg.Register(network.NewPlugin(r.verbose))

	// --- Shell exec — top-level shell primitive for pipelines ---
	reg.Register(agexec.NewPlugin(r.verbose))

	// --- PDF Form Fill — AI-powered PDF form filling ---
	// Reasoner is Claude if available, Gemini as fallback.
	var pdfReasoner pdffill.Reasoner
	if r.claude != nil {
		pdfReasoner = r.claude.Chat
	} else if r.gemini != nil {
		pdfReasoner = r.gemini.GenerateContent
	}
	reg.Register(pdffill.NewPlugin(pdfReasoner, r.verbose))

	// --- Cloud Run — deploy + schedule DSL scripts as Cloud Run Jobs ---
	reg.Register(cloudrun.NewPlugin(r.verbose))

	// --- DataTable — render .table DSL into self-contained HTML ---
	reg.Register(datatable.NewPlugin(r.verbose))

	// --- RAG — Postgres + LLM vector search pipeline ---
	reg.Register(rag.NewPlugin())

	// --- Knowledge Graph — Apache AGE + pgvector GraphRAG pipeline ---
	reg.Register(kg.NewPlugin())

	// --- AI Table Render — JSON in, interactive dashboard HTML out ---
	var tableReasoner datatable.Reasoner
	if r.claude != nil {
		tableReasoner = r.claude.Chat
	} else if r.gemini != nil {
		tableReasoner = r.gemini.GenerateContent
	}
	reg.Register(datatable.NewAIPlugin(tableReasoner, r.verbose))

	// --- MCP Search — searches the official MCP registry
	// No API key needed — the registry is public
	reg.Register(mcpsearch.NewPlugin(r.verbose))

	// --- Ollama — local LLM, no data leaves the machine ---
	// Connects to local Ollama server (default localhost:11434)
	ollamaClient := ollama.NewClient("", "") // reads OLLAMA_URL and OLLAMA_MODEL from env
	reg.Register(ollama.NewPlugin(ollamaClient, r.verbose))

	// --- Plug Agent — generates new plugins from English descriptions
	// Generator is Claude.Chat — injected as functional field seam.
	// repoRoot is detected from the binary's working directory.
	if r.claude != nil {
		repoRoot, _ := os.Getwd()
		reg.Register(plugagent.NewPlugin(r.claude.Chat, repoRoot, r.verbose))
	}

	// --- Code Review Forum — Claude + Gemini + GPT-4 debate
	// GeminiReviewer is the functional field seam for Gemini injection.
	// Gracefully degrades — works with any subset of the three models.
	var geminiReviewer review.GeminiReviewer
	if r.gemini != nil {
		geminiReviewer = r.gemini.GenerateContent
	}
	var openaiClient *openai.Client
	if oaiKey := os.Getenv("OPENAI_API_KEY"); oaiKey != "" {
		openaiClient = openai.NewClient(oaiKey, "")
	}
	if r.claude != nil || geminiReviewer != nil || openaiClient != nil {
		reg.Register(review.NewPlugin(r.claude, geminiReviewer, openaiClient, r.verbose))
	}

	// --- GitHub — deploys plain HTML to GitHub Pages.
	reg.Register(aggithub.NewPlugin(r.github))

	return reg
}
