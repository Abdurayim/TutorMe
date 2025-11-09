package utils

import (
	"regexp"
	"strings"
)

// VideoSource represents a video hosting platform
type VideoSource struct {
	Name    string
	Pattern *regexp.Regexp
}

// Common video hosting platforms
var videoSources = []VideoSource{
	{
		Name:    "YouTube",
		Pattern: regexp.MustCompile(`(?i)(youtube\.com/watch\?v=|youtu\.be/|youtube\.com/shorts/)`),
	},
	{
		Name:    "Instagram",
		Pattern: regexp.MustCompile(`(?i)instagram\.com/(p|reel|tv)/`),
	},
	{
		Name:    "TikTok",
		Pattern: regexp.MustCompile(`(?i)tiktok\.com/@.*/(video|photo)/`),
	},
	{
		Name:    "Facebook",
		Pattern: regexp.MustCompile(`(?i)facebook\.com/.*/videos/`),
	},
	{
		Name:    "Twitter/X",
		Pattern: regexp.MustCompile(`(?i)(twitter\.com|x\.com)/.*/status/`),
	},
	{
		Name:    "Vimeo",
		Pattern: regexp.MustCompile(`(?i)vimeo\.com/`),
	},
	{
		Name:    "Dailymotion",
		Pattern: regexp.MustCompile(`(?i)dailymotion\.com/video/`),
	},
	{
		Name:    "Reddit",
		Pattern: regexp.MustCompile(`(?i)reddit\.com/r/.*/comments/`),
	},
	{
		Name:    "Twitch",
		Pattern: regexp.MustCompile(`(?i)twitch\.tv/videos/`),
	},
}

// IsVideoURL checks if a URL is from a known video hosting platform
func IsVideoURL(url string) bool {
	url = strings.TrimSpace(url)
	for _, source := range videoSources {
		if source.Pattern.MatchString(url) {
			return true
		}
	}
	return false
}

// GetVideoSource returns the name of the video hosting platform
func GetVideoSource(url string) string {
	url = strings.TrimSpace(url)
	for _, source := range videoSources {
		if source.Pattern.MatchString(url) {
			return source.Name
		}
	}
	return "Unknown"
}

// IsYouTubeURL checks if URL is specifically from YouTube
func IsYouTubeURL(url string) bool {
	return videoSources[0].Pattern.MatchString(url)
}
