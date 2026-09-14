; The smallest program. No (block ...) wrapper, so this is the memory
; backend in static mode — the default.
;
; Note `exec` rather than `echo`: echo is a temporal activity (see
; durable-echo.as), and the in-process interpreter does not implement
; it. Resolve accepts it on either backend, so the mismatch only shows
; up at run time.
(exec "echo hello from AgentScript")
