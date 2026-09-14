; Conditional. The surface verb is `when`, not `if`: in an s-expression
; syntax `if` reads as a special form with lazy branches, and this is a
; pipeline stage that takes a condition string. It resolves to the `if`
; builtin.
(pipe
  (stock "NVDA")
  (when "change > 5")
  (notify "slack"))
