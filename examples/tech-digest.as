// Tech Digest from multiple feeds
parallel {
  rss "hn"
  rss "lobsters"
  reddit "r/golang" "top"
  news_headlines "technology"
}
-> merge
-> summarize
-> save "tech-digest.md"
