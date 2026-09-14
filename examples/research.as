; A flat pipeline. Each stage's output feeds the next, which is what
; (pipe ...) means — the s-expression spelling of the old >=> operator.
(pipe
  (search "golang 1.24 release notes")
  summarize
  (save "go-release-notes.md"))
