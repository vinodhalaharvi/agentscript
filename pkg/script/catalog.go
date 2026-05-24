// Package script — catalog.go defines the COMPLETE builtin catalog: every
// verb the AgentScript grammar has ever supported, so the vocabulary is
// complete and backwards-compatible. Resolve recognizes all of them.
//
// Availability is per-backend, not per-existence:
//   - Every verb here is resolvable (no more "unknown builtin" for a real
//     verb just because we're on the temporal path).
//   - Each verb declares which backends it runs on. The full historical
//     set runs on BackendMemory (the in-process interpreter implements
//     them). Only verbs with a registered Sibyl activity also list
//     BackendTemporal — today, just echo.
//   - A known verb targeted at a backend it doesn't support yields a
//     distinct NotImplementedOnBackendError (see resolve), NOT an
//     UnknownBuiltinError. Genuinely unknown names (typos, hallucinations)
//     still fail as unknown — the safety net is preserved.
//
// Arg schemas here are intentionally permissive (variadic strings): the
// registry's job for memory verbs is vocabulary recognition, while the
// in-process interpreter does its own argument handling. echo keeps a
// precise schema because it is the temporal-path builtin.
//
// Porting a verb to temporal later = add BackendTemporal to its spec and
// register the matching Sibyl activity. Until then it is a known,
// memory-only verb that reports "not implemented on temporal".
package script

import "github.com/vinodhalaharvi/agentscript/pkg/script/registry"

// memoryVerbs is the complete historical verb set, all memory-backed.
// Sourced from the grammar's Action enum (the authoritative list).
var memoryVerbs = []string{
	"agent",
	"analyze",
	"ask",
	"audio_video_merge",
	"calendar",
	"claude",
	"codereview",
	"codereview_focus",
	"confirm",
	"contact_find",
	"crypto",
	"deploy",
	"dns_lookup",
	"doc_create",
	"drive_save",
	"email",
	"emoji_style",
	"exec",
	"fmap",
	"foreach",
	"form_create",
	"form_responses",
	"gcp_check",
	"github_pages_html",
	"hf_classify",
	"hf_embeddings",
	"hf_fill_mask",
	"hf_generate",
	"hf_image_classify",
	"hf_image_generate",
	"hf_ner",
	"hf_qa",
	"hf_similarity",
	"hf_speech_to_text",
	"hf_summarize",
	"hf_translate",
	"hf_zero_shot",
	"http_check",
	"if",
	"image_analyze",
	"image_audio_merge",
	"image_generate",
	"images_to_video",
	"job_search",
	"kg_connect",
	"kg_cypher",
	"kg_extract",
	"kg_hybrid",
	"kg_ingest",
	"kg_path",
	"kg_query",
	"kg_status",
	"list",
	"maps_trip",
	"match",
	"mcp",
	"mcp_agent",
	"mcp_connect",
	"mcp_list",
	"mcp_search",
	"mcp_search_install",
	"meet",
	"merge",
	"news",
	"news_headlines",
	"notify",
	"ollama",
	"pdf_fields",
	"pdf_fill",
	"perplexity",
	"perplexity_domain",
	"perplexity_pro",
	"perplexity_recent",
	"pfmap",
	"ping",
	"places_search",
	"plug_agent",
	"port_check",
	"rag_connect",
	"rag_index",
	"rag_query",
	"rag_schema",
	"rag_status",
	"read",
	"reddit",
	"render",
	"rss",
	"save",
	"schedule",
	"search",
	"sheet_append",
	"sheet_create",
	"ssl_check",
	"stdin",
	"stock",
	"summarize",
	"table_render",
	"task",
	"text_to_speech",
	"translate",
	"twitter",
	"undeploy",
	"video_analyze",
	"video_generate",
	"video_script",
	"weather",
	"whatsapp",
	"whois",
	"youtube_search",
	"youtube_shorts",
	"youtube_upload",
}

// memorySpec builds a permissive, memory-only spec for a historical verb:
// any number of string args, recognized by Resolve, run by the in-process
// interpreter.
func memorySpec(name string) registry.BuiltinSpec {
	return registry.BuiltinSpec{
		Name:    name,
		AgentID: "agentscript/" + name,
		ArgSchema: registry.ArgSchema{
			Variadic:     true,
			VariadicType: registry.StringT,
		},
		Backends: []registry.Backend{registry.BackendMemory},
	}
}

// CompleteRegistry returns a registry containing the entire historical
// verb vocabulary (all memory-backed) plus echo (memory + temporal). This
// is the backwards-compatible registry: every real verb resolves; the
// backend decides availability. Front ends that want the full grammar
// recognized use this; DefaultRegistry (echo only) remains for the
// minimal temporal-only case.
func CompleteRegistry() *registry.Registry {
	r := registry.New()
	// echo: the one verb available on BOTH backends.
	echo := EchoSpec()
	echo.Backends = []registry.Backend{registry.BackendMemory, registry.BackendTemporal}
	r.MustRegister(echo)
	// every historical verb, memory-backed.
	for _, v := range memoryVerbs {
		r.MustRegister(memorySpec(v))
	}
	return r
}
