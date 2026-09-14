; Fan-out. The branches of (par ...) run concurrently on the same input;
; merge collapses their outputs back into one value for the next stage.
(pipe
  (par
    (rss "hn")
    (rss "lobsters")
    (reddit "r/golang"))
  merge
  summarize
  (save "digest.md"))
