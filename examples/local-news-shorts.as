// ============================================
// Local News YouTube Shorts Pipeline
// ============================================
// Uses Veo 3.1's NATIVE synchronized audio
// NO separate TTS needed - Veo generates speech!
// ============================================

search "local news San Francisco today"
-> summarize "Extract top 2 headlines in 2 short sentences"
-> video_script "news anchor"
-> video_generate "vertical shorts"
-> save "sf_news.mp4"
-> confirm "Upload to YouTube Shorts?"
-> youtube_shorts "SF Local News Update"

// ============================================
// HOW IT WORKS:
// ============================================
//
// 1. search: Gets news articles
//    Output: "Tech layoffs hit Bay Area, housing prices surge..."
//
// 2. summarize: Condenses to 2 sentences
//    Output: "Tech companies announce layoffs. Housing costs rise 15%."
//
// 3. video_script "news anchor": Converts to Veo prompt with dialogue
//    Output: 'News studio, portrait 9:16. Anchor speaking: 
//            "Tech layoffs hit the Bay Area today as housing costs 
//            continue rising." SFX: news jingle. Ambient: studio hum.'
//
// 4. video_generate: Veo 3.1 creates video WITH synchronized audio
//    - Generates news anchor visuals
//    - Generates SPEECH matching the quoted dialogue  
//    - Lip-syncs the anchor's mouth to the words
//    - Adds sound effects and ambient audio
//    Output: Video file with perfectly synced audio!
//
// 5. save/upload: Save and publish to YouTube Shorts
//
// ============================================
// KEY INSIGHT: Veo 3.1 generates audio NATIVELY
// The quoted dialogue becomes synchronized speech!
// ============================================
