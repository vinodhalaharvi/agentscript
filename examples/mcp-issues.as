; MCP tools. mcp_connect registers a server, then mcp calls one of its
; tools as "server:tool".
(pipe
  (mcp_connect "github" "npx -y @modelcontextprotocol/server-github")
  (mcp "github:list_issues" "owner/repo")
  (ask "group these issues by theme and rank by urgency")
  (notify "slack"))
