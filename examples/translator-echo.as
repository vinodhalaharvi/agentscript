// translator-echo.as — the translator MVP demo.
//
// Compiles to a two-node Sibyl Plan: echo "hello from agentscript"
// feeds its output to a second echo (passthrough). Run it with:
//
//   agentscript-run --dry-run examples/translator-echo.as   # print the Plan
//   agentscript-run examples/translator-echo.as             # submit to Sibyl
//
// The second form needs a Temporal cluster and a Sibyl worker running.
temporal static ( echo "hello from agentscript" >=> echo )
