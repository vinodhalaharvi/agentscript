# Multimodal workflow - image and video generation/analysis

# Generate an image based on research
search "modern office interior design trends 2024" -> summarize -> image_generate "modern minimalist office interior with natural lighting"

# Analyze images in parallel
parallel {
    image_analyze "design1.jpg" -> ask "what style is this?"
    image_analyze "design2.jpg" -> ask "what style is this?"
} -> merge -> ask "compare these design styles" -> save "design-comparison.md"

# Video generation from text prompt
video_generate "a serene sunrise over mountains with birds flying, cinematic"

# Generate video from multiple images (space separated)
images_to_video "product1.jpg product2.jpg product3.jpg"

# Video analysis workflow
video_analyze "presentation.mp4" -> summarize -> task "Review presentation feedback"

# Full creative pipeline: research -> generate images -> create video
search "nature documentary style" -> image_generate "majestic eagle soaring over canyon"
