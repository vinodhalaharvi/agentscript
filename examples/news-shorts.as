parallel {
   search "top US news headlines today site:twitter.com OR site:x.com"
    -> ask "Extract the top 10 news headlines from these results. For each headline, give just the clean headline text without any special characters, emojis, hashtags, or URLs. Format as a numbered list."
    -> ask "Write a 45-second news anchor script summarizing these 10 headlines. Make it sound professional but engaging, like a quick news briefing. Start with 'Here are today's top US news headlines' and end with 'Stay informed, stay engaged.'"
    -> text_to_speech "Charon"
    -> save "news_narration.wav"

    parallel {
        image_generate "professional news studio background, modern broadcast desk, blue lighting, cinematic" -> save "news1.png"
        image_generate "digital news graphics with world map, breaking news style, professional broadcast look" -> save "news2.png"
    } -> merge -> images_to_video "news1.png news2.png" -> save "news_video.mp4"
}
-> merge
-> audio_video_merge "news_shorts.mp4"
-> youtube_shorts "Top 10 US News Headlines Today - AI News Brief"
