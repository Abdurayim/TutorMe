package utils

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// VideoProcessor handles video downloading and transcoding
type VideoProcessor struct {
	YtDlpPath     string
	FfmpegPath    string
	WorkDir       string
	MaxSizeMB     int
	TargetQuality string // "720p", "480p", etc.
}

// ProcessingProgress tracks video processing status
type ProcessingProgress struct {
	Stage      string // "downloading", "transcoding", "completed", "failed"
	Percentage int    // 0-100
	Message    string
}

// VideoInfo contains metadata about the downloaded video
type VideoInfo struct {
	Title    string
	FilePath string
	SizeMB   float64
	Duration int // in seconds
}

// NewVideoProcessor creates a new video processor with configuration from environment
func NewVideoProcessor() *VideoProcessor {
	ytDlpPath := os.Getenv("YT_DLP_PATH")
	if ytDlpPath == "" {
		ytDlpPath = "yt-dlp" // Try PATH
	}

	ffmpegPath := os.Getenv("FFMPEG_PATH")
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg" // Try PATH
	}

	maxSize := 1500 // Default 1.5GB
	if maxSizeStr := os.Getenv("MAX_VIDEO_SIZE_MB"); maxSizeStr != "" {
		if parsed, err := strconv.Atoi(maxSizeStr); err == nil {
			maxSize = parsed
		}
	}

	quality := os.Getenv("TARGET_VIDEO_QUALITY")
	if quality == "" {
		quality = "720p"
	}

	// Create work directory for temporary files
	workDir := filepath.Join(os.TempDir(), "bot_videos")
	os.MkdirAll(workDir, 0755)

	return &VideoProcessor{
		YtDlpPath:     ytDlpPath,
		FfmpegPath:    ffmpegPath,
		WorkDir:       workDir,
		MaxSizeMB:     maxSize,
		TargetQuality: quality,
	}
}

// DownloadVideo downloads a video from URL with progress tracking
func (vp *VideoProcessor) DownloadVideo(url string, progressCallback func(ProcessingProgress)) (*VideoInfo, error) {
	// Create unique subdirectory for this download to avoid conflicts with concurrent downloads
	downloadDir, err := os.MkdirTemp(vp.WorkDir, "download_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Generate output filename
	outputTemplate := filepath.Join(downloadDir, "%(id)s.%(ext)s")

	// Build yt-dlp command
	args := []string{
		url,
		"-o", outputTemplate,
		"--no-playlist",
		"--newline", // Each progress line on new line
	}

	// For YouTube, prefer 720p. For others, get best available
	if IsYouTubeURL(url) {
		// YouTube: prefer 720p, fallback to lower
		args = append(args,
			"-f", "bestvideo[height<=720]+bestaudio/best[height<=720]/best",
			"--merge-output-format", "mp4",
		)
	} else {
		// Other platforms: get best available format
		args = append(args,
			"-f", "best",
		)
	}

	// Add filesize limit
	maxBytes := vp.MaxSizeMB * 1024 * 1024
	args = append(args, "--max-filesize", fmt.Sprintf("%d", maxBytes))

	log.Printf("Executing: %s %s", vp.YtDlpPath, strings.Join(args, " "))

	cmd := exec.Command(vp.YtDlpPath, args...)

	// Capture stdout for progress tracking
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Capture stderr for errors
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	// Progress regex patterns
	progressRegex := regexp.MustCompile(`(\d+\.?\d*)%`)

	// Read progress from stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("yt-dlp: %s", line)

			// Extract percentage
			if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
				if percentage, err := strconv.ParseFloat(matches[1], 64); err == nil {
					if progressCallback != nil {
						progressCallback(ProcessingProgress{
							Stage:      "downloading",
							Percentage: int(percentage / 2), // Download is 0-50% of total process
							Message:    fmt.Sprintf("Videoni yuklab olish: %.0f%%", percentage),
						})
					}
				}
			}
		}
	}()

	// Read errors from stderr
	errorOutput := ""
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("yt-dlp error: %s", line)
			errorOutput += line + "\n"
		}
	}()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("yt-dlp failed: %w\nOutput: %s", err, errorOutput)
	}

	// Find the downloaded file in the unique download directory
	files, err := filepath.Glob(filepath.Join(downloadDir, "*.*"))
	if err != nil || len(files) == 0 {
		os.RemoveAll(downloadDir) // Cleanup empty directory
		return nil, fmt.Errorf("no video file found after download")
	}

	// Should only be one file, but take the first one
	downloadedFile := files[0]

	// Get file info
	fileInfo, err := os.Stat(downloadedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	sizeMB := float64(fileInfo.Size()) / 1024 / 1024

	// Extract video title using yt-dlp
	title, _ := vp.getVideoTitle(url)

	info := &VideoInfo{
		Title:    title,
		FilePath: downloadedFile,
		SizeMB:   sizeMB,
	}

	if progressCallback != nil {
		progressCallback(ProcessingProgress{
			Stage:      "downloading",
			Percentage: 50,
			Message:    "Yuklash tugallandi",
		})
	}

	return info, nil
}

// TranscodeVideo transcodes video to reduce size with target size limit
func (vp *VideoProcessor) TranscodeVideo(inputPath string, maxSizeMB float64, progressCallback func(ProcessingProgress)) (string, error) {
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + "_transcoded.mp4"

	// Get video duration for progress calculation
	duration, _ := vp.getVideoDuration(inputPath)

	// Determine compression level based on target size
	// For smaller targets, use more aggressive compression
	var crf string
	var videoBitrate string
	var resolution string

	if maxSizeMB <= 50 {
		// Standard API - very aggressive compression
		crf = "28"              // Higher CRF = more compression
		videoBitrate = "800k"   // Low bitrate
		resolution = "scale=-2:480" // 480p
	} else if maxSizeMB <= 500 {
		// Local API - moderate compression
		crf = "26"
		videoBitrate = "1500k"
		resolution = "scale=-2:720" // 720p
	} else {
		// Large file support - light compression
		crf = "23"
		videoBitrate = "2500k"
		resolution = "scale=-2:720"
	}

	// Build ffmpeg command
	// Target: Optimized resolution, H.264 baseline profile, AAC audio
	args := []string{
		"-i", inputPath,
		"-c:v", "libx264",
		"-profile:v", "baseline", // Maximum compatibility
		"-level", "3.0",
		"-preset", "medium",
		"-crf", crf,
		"-maxrate", videoBitrate,
		"-bufsize", fmt.Sprintf("%dk", 2*mustParseInt(videoBitrate)),
		"-vf", resolution, // Scale based on target size
		"-c:a", "aac",
		"-b:a", "96k",     // Reduced audio bitrate
		"-ar", "44100",    // Audio sample rate
		"-ac", "2",        // Stereo audio
		"-movflags", "+faststart", // Enable streaming
		"-pix_fmt", "yuv420p",     // Pixel format for compatibility
		"-y",                       // Overwrite output file
		"-progress", "pipe:1",      // Output progress to stdout
		outputPath,
	}

	log.Printf("Transcoding with target size: %.0fMB, CRF: %s, Bitrate: %s, Resolution: %s", maxSizeMB, crf, videoBitrate, resolution)

	log.Printf("Executing: %s %s", vp.FfmpegPath, strings.Join(args, " "))

	cmd := exec.Command(vp.FfmpegPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Progress tracking
	timeRegex := regexp.MustCompile(`out_time_ms=(\d+)`)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			// Extract current time in microseconds
			if matches := timeRegex.FindStringSubmatch(line); len(matches) > 1 {
				if timeMicro, err := strconv.ParseInt(matches[1], 10, 64); err == nil && duration > 0 {
					currentSec := timeMicro / 1000000
					percentage := int((float64(currentSec) / float64(duration)) * 100)
					if percentage > 100 {
						percentage = 100
					}

					if progressCallback != nil {
						// Transcoding is 50-100% of total process
						totalPercentage := 50 + (percentage / 2)
						progressCallback(ProcessingProgress{
							Stage:      "transcoding",
							Percentage: totalPercentage,
							Message:    fmt.Sprintf("Videoni siqish: %d%%", percentage),
						})
					}
				}
			}
		}
	}()

	// Read errors
	errorOutput := ""
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("ffmpeg: %s", line)
			errorOutput += line + "\n"
		}
	}()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\nOutput: %s", err, errorOutput)
	}

	if progressCallback != nil {
		progressCallback(ProcessingProgress{
			Stage:      "transcoding",
			Percentage: 100,
			Message:    "Siqish tugallandi",
		})
	}

	return outputPath, nil
}

// getVideoTitle extracts video title using yt-dlp
func (vp *VideoProcessor) getVideoTitle(url string) (string, error) {
	cmd := exec.Command(vp.YtDlpPath, "--get-title", url)
	output, err := cmd.Output()
	if err != nil {
		return "Video", nil // Default title if extraction fails
	}
	return strings.TrimSpace(string(output)), nil
}

// getVideoDuration gets video duration in seconds
func (vp *VideoProcessor) getVideoDuration(filePath string) (int, error) {
	cmd := exec.Command(vp.FfmpegPath,
		"-i", filePath,
		"-f", "null",
		"-",
	)

	output, _ := cmd.CombinedOutput()
	durationRegex := regexp.MustCompile(`Duration: (\d{2}):(\d{2}):(\d{2})`)

	if matches := durationRegex.FindStringSubmatch(string(output)); len(matches) == 4 {
		hours, _ := strconv.Atoi(matches[1])
		minutes, _ := strconv.Atoi(matches[2])
		seconds, _ := strconv.Atoi(matches[3])
		return hours*3600 + minutes*60 + seconds, nil
	}

	return 0, fmt.Errorf("could not extract duration")
}

// Cleanup removes temporary files and their parent directories if they're in WorkDir
func (vp *VideoProcessor) Cleanup(filePaths ...string) {
	for _, path := range filePaths {
		if path != "" {
			// Remove the file
			os.Remove(path)
			log.Printf("Cleaned up temporary file: %s", path)

			// Check if the parent directory is a temp download directory and remove it
			parentDir := filepath.Dir(path)
			if strings.HasPrefix(filepath.Base(parentDir), "download_") &&
				strings.HasPrefix(parentDir, vp.WorkDir) {
				os.RemoveAll(parentDir)
				log.Printf("Cleaned up temporary directory: %s", parentDir)
			}
		}
	}
}

// mustParseInt parses integer from string like "800k"
func mustParseInt(s string) int {
	s = strings.TrimSuffix(s, "k")
	s = strings.TrimSuffix(s, "K")
	val, _ := strconv.Atoi(s)
	return val
}
