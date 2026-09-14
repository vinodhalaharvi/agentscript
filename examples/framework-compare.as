; Nested fan-out. In the operator dialect this needed knowledge of how
; >=> and <*> bind against each other; here the parentheses say it
; outright, which is the strongest argument for the syntax.
(pipe
  (par
    (pipe
      (par
        (pipe (search "React pros cons") analyze)
        (pipe (search "Vue pros cons") analyze))
      merge
      (ask "summarize frontend frameworks"))
    (pipe
      (par
        (pipe (search "Node.js backend") analyze)
        (pipe (search "Go backend") analyze))
      merge
      (ask "summarize backend options")))
  merge
  (ask "full-stack recommendation")
  (save "recommendation.md"))
