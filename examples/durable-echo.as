; The temporal backend, selected in the source rather than by a flag, so
; the same text always runs the same way. `sibyl` is accepted as a
; synonym for `temporal`.
;
; Compile and inspect without any infrastructure:
;   agentscript --dry-run examples/durable-echo.as
(block :backend temporal :mode static
  (pipe
    (echo "step one")
    (echo "step two")))
