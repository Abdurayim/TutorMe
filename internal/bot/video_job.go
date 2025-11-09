package bot

import (
	"fmt"
	"log"
	"os"
	"tg-bot/internal/database"
	"tg-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

// VideoJob represents a video processing job
type VideoJob struct {
	ResourceID uint
	VideoURL   string
	CategoryID uint
	ChatID     int64
	MessageID  int // Progress message ID
}

// VideoJobProcessor handles background video processing
type VideoJobProcessor struct {
	Bot             *tgbotapi.BotAPI
	DB              *gorm.DB
	VideoProcessor  *utils.VideoProcessor
	JobQueue        chan VideoJob
	UseLocalAPI     bool
	MaxWorkers      int
}

// NewVideoJobProcessor creates a new video job processor
func NewVideoJobProcessor(bot *tgbotapi.BotAPI, db *gorm.DB) *VideoJobProcessor {
	useLocalAPI := os.Getenv("USE_LOCAL_API") == "true"

	processor := &VideoJobProcessor{
		Bot:            bot,
		DB:             db,
		VideoProcessor: utils.NewVideoProcessor(),
		JobQueue:       make(chan VideoJob, 100), // Buffer up to 100 jobs
		UseLocalAPI:    useLocalAPI,
		MaxWorkers:     3, // Process 3 videos concurrently
	}

	// Start worker goroutines
	for i := 0; i < processor.MaxWorkers; i++ {
		go processor.worker(i)
	}

	log.Printf("Video job processor started with %d workers (Local API: %v)", processor.MaxWorkers, useLocalAPI)

	return processor
}

// EnqueueJob adds a video processing job to the queue
func (vjp *VideoJobProcessor) EnqueueJob(job VideoJob) {
	vjp.JobQueue <- job
	log.Printf("Video job enqueued: ResourceID=%d, URL=%s", job.ResourceID, job.VideoURL)
}

// worker processes video jobs from the queue
func (vjp *VideoJobProcessor) worker(workerID int) {
	log.Printf("Worker %d started", workerID)

	for job := range vjp.JobQueue {
		log.Printf("Worker %d processing job: ResourceID=%d", workerID, job.ResourceID)
		vjp.processVideo(job)
	}
}

// processVideo handles the complete video processing workflow
func (vjp *VideoJobProcessor) processVideo(job VideoJob) {
	// Update resource status to "processing"
	vjp.DB.Model(&database.Resource{}).Where("id = ?", job.ResourceID).Updates(map[string]interface{}{
		"processing_status": "processing",
		"progress":          0,
	})

	// Progress callback to update database and send messages
	progressCallback := func(progress utils.ProcessingProgress) {
		// Update database
		vjp.DB.Model(&database.Resource{}).Where("id = ?", job.ResourceID).Updates(map[string]interface{}{
			"progress": progress.Percentage,
		})

		// Send progress update to teacher
		if job.MessageID != 0 {
			progressText := fmt.Sprintf("⏳ *Video qayta ishlanmoqda...*\n\n"+
				"Bosqich: %s\n"+
				"Progress: %d%%\n"+
				"%s",
				vjp.getStageEmoji(progress.Stage), progress.Percentage, progress.Message)

			editMsg := tgbotapi.NewEditMessageText(job.ChatID, job.MessageID, progressText)
			editMsg.ParseMode = "Markdown"
			vjp.Bot.Send(editMsg)
		}

		log.Printf("Resource %d progress: %d%% (%s)", job.ResourceID, progress.Percentage, progress.Stage)
	}

	// Step 1: Download video
	progressCallback(utils.ProcessingProgress{
		Stage:      "downloading",
		Percentage: 0,
		Message:    "Videoni yuklab olish boshlanmoqda...",
	})

	videoInfo, err := vjp.VideoProcessor.DownloadVideo(job.VideoURL, progressCallback)
	if err != nil {
		vjp.handleError(job, fmt.Sprintf("Video yuklashda xatolik: %v", err))
		return
	}

	defer vjp.VideoProcessor.Cleanup(videoInfo.FilePath)

	// Step 2: Determine upload size limit
	maxUploadSizeMB := float64(50) // Standard API limit
	if vjp.UseLocalAPI {
		maxUploadSizeMB = 2000 // 2GB for Local Bot API
	}

	// Step 3: Check if transcoding is needed
	var finalVideoPath string
	enableTranscoding := os.Getenv("ENABLE_TRANSCODING") != "false" // Default true

	// Transcode if video is too large OR if it's close to the limit
	needsTranscoding := enableTranscoding && (videoInfo.SizeMB > (maxUploadSizeMB * 0.8))

	if needsTranscoding {
		// Video is large, transcode it
		progressCallback(utils.ProcessingProgress{
			Stage:      "transcoding",
			Percentage: 50,
			Message:    "Video hajmi katta, siqish boshlanmoqda...",
		})

		transcodedPath, err := vjp.VideoProcessor.TranscodeVideo(videoInfo.FilePath, maxUploadSizeMB, progressCallback)
		if err != nil {
			vjp.handleError(job, fmt.Sprintf("Video siqishda xatolik: %v", err))
			return
		}

		finalVideoPath = transcodedPath
		defer vjp.VideoProcessor.Cleanup(transcodedPath)

		// Verify transcoded file is within limits
		finalInfo, err := os.Stat(finalVideoPath)
		if err == nil {
			finalSizeMB := float64(finalInfo.Size()) / 1024 / 1024
			if finalSizeMB > maxUploadSizeMB {
				vjp.handleError(job, fmt.Sprintf("Video hajmi juda katta (%.1fMB). Maksimal hajm: %.0fMB", finalSizeMB, maxUploadSizeMB))
				return
			}
			log.Printf("Transcoded video size: %.2f MB (limit: %.0f MB)", finalSizeMB, maxUploadSizeMB)
		}
	} else {
		finalVideoPath = videoInfo.FilePath

		// Verify original file is within limits
		if videoInfo.SizeMB > maxUploadSizeMB {
			vjp.handleError(job, fmt.Sprintf("Video hajmi juda katta (%.1fMB). Maksimal hajm: %.0fMB", videoInfo.SizeMB, maxUploadSizeMB))
			return
		}
	}

	// Step 4: Upload to Telegram
	progressCallback(utils.ProcessingProgress{
		Stage:      "uploading",
		Percentage: 90,
		Message:    "Telegram serveriga yuklash boshlanmoqda...",
	})

	// Get file size for upload estimate
	finalInfo, err := os.Stat(finalVideoPath)
	if err != nil {
		vjp.handleError(job, fmt.Sprintf("Fayl ma'lumotlarini olishda xatolik: %v", err))
		return
	}
	finalSizeMB := float64(finalInfo.Size()) / 1024 / 1024

	// Send informative message about upload
	uploadNotice := fmt.Sprintf("📤 *Telegram serveriga yuklash boshlanmoqda...*\n\n"+
		"Fayl hajmi: %.1f MB\n"+
		"Bu bir necha daqiqa vaqt olishi mumkin.\n"+
		"Iltimos, kuting...",
		finalSizeMB)

	noticeMsg := tgbotapi.NewMessage(job.ChatID, uploadNotice)
	noticeMsg.ParseMode = "Markdown"
	uploadNoticeMessage, _ := vjp.Bot.Send(noticeMsg)

	fileBytes, err := os.ReadFile(finalVideoPath)
	if err != nil {
		vjp.handleError(job, fmt.Sprintf("Faylni o'qishda xatolik: %v", err))
		return
	}

	videoMsg := tgbotapi.NewVideo(job.ChatID, tgbotapi.FileBytes{
		Name:  "video.mp4",
		Bytes: fileBytes,
	})
	videoMsg.Caption = videoInfo.Title

	sentMsg, err := vjp.Bot.Send(videoMsg)

	// Delete the upload notice message
	if uploadNoticeMessage.MessageID != 0 {
		deleteNotice := tgbotapi.NewDeleteMessage(job.ChatID, uploadNoticeMessage.MessageID)
		vjp.Bot.Send(deleteNotice)
	}
	if err != nil {
		vjp.handleError(job, fmt.Sprintf("Telegram'ga yuklashda xatolik: %v", err))
		return
	}

	// Step 5: Update resource with FileID
	fileID := sentMsg.Video.FileID
	vjp.DB.Model(&database.Resource{}).Where("id = ?", job.ResourceID).Updates(map[string]interface{}{
		"file_id":            fileID,
		"processing_status":  "completed",
		"progress":           100,
		"title":              videoInfo.Title,
		"type":               "video", // Change from "link" to "video"
	})

	// Delete the progress message
	if job.MessageID != 0 {
		deleteMsg := tgbotapi.NewDeleteMessage(job.ChatID, job.MessageID)
		vjp.Bot.Send(deleteMsg)
	}

	// Get category and subject info for navigation
	var category database.Category
	if err := vjp.DB.Preload("Subject").First(&category, job.CategoryID).Error; err != nil {
		log.Printf("Warning: Could not load category %d: %v", job.CategoryID, err)
		// Continue anyway with basic success message
		basicMsg := tgbotapi.NewMessage(job.ChatID, "✅ *Video muvaffaqiyatli qo'shildi!*")
		basicMsg.ParseMode = "Markdown"
		vjp.Bot.Send(basicMsg)
		return
	}

	// Send completion message with navigation buttons
	completionText := fmt.Sprintf("✅ *Video muvaffaqiyatli qo'shildi!*\n\n"+
		"*Sarlavha:* %s\n"+
		"*Manba:* %s\n"+
		"*Fan:* %s\n"+
		"*Kategoriya:* %s",
		videoInfo.Title, utils.GetVideoSource(job.VideoURL), category.Subject.Name, category.Name)

	// Create keyboard with navigation
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyaga qaytish", fmt.Sprintf("view_category_%d", job.CategoryID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Resurslarni boshqarish", fmt.Sprintf("manage_resources_%d", job.CategoryID)),
		),
	)

	successMsg := tgbotapi.NewMessage(job.ChatID, completionText)
	successMsg.ParseMode = "Markdown"
	successMsg.ReplyMarkup = keyboard
	vjp.Bot.Send(successMsg)

	// Notify subscribed users
	vjp.notifySubscribedUsers(job.CategoryID, videoInfo.Title)

	log.Printf("Video processing completed successfully: ResourceID=%d, FileID=%s", job.ResourceID, fileID)
}

// handleError handles processing errors
func (vjp *VideoJobProcessor) handleError(job VideoJob, errorMsg string) {
	log.Printf("Video processing error: ResourceID=%d, Error=%s", job.ResourceID, errorMsg)

	// Update resource status to "failed"
	vjp.DB.Model(&database.Resource{}).Where("id = ?", job.ResourceID).Updates(map[string]interface{}{
		"processing_status": "failed",
		"error_message":     errorMsg,
	})

	// Delete progress message
	if job.MessageID != 0 {
		deleteMsg := tgbotapi.NewDeleteMessage(job.ChatID, job.MessageID)
		vjp.Bot.Send(deleteMsg)
	}

	// Get category info for navigation
	var category database.Category
	var errorText string
	var keyboard tgbotapi.InlineKeyboardMarkup

	if err := vjp.DB.Preload("Subject").First(&category, job.CategoryID).Error; err != nil {
		log.Printf("Warning: Could not load category %d: %v", job.CategoryID, err)
		// Send basic error message without navigation
		errorText = fmt.Sprintf("❌ *Video qayta ishlashda xatolik yuz berdi*\n\n"+
			"*Xatolik:* %s\n\n"+
			"Video oddiy havola sifatida saqlandi.",
			errorMsg)
	} else {
		// Send error message with full context and navigation
		errorText = fmt.Sprintf("❌ *Video qayta ishlashda xatolik yuz berdi*\n\n"+
			"*Xatolik:* %s\n\n"+
			"Video oddiy havola sifatida saqlandi.\n"+
			"*Fan:* %s\n"+
			"*Kategoriya:* %s",
			errorMsg, category.Subject.Name, category.Name)

		// Create keyboard with navigation
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyaga qaytish", fmt.Sprintf("view_category_%d", job.CategoryID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📚 Resurslarni boshqarish", fmt.Sprintf("manage_resources_%d", job.CategoryID)),
			),
		)
	}

	errorMsgToSend := tgbotapi.NewMessage(job.ChatID, errorText)
	errorMsgToSend.ParseMode = "Markdown"
	if keyboard.InlineKeyboard != nil {
		errorMsgToSend.ReplyMarkup = keyboard
	}
	vjp.Bot.Send(errorMsgToSend)
}

// getStageEmoji returns emoji for processing stage
func (vjp *VideoJobProcessor) getStageEmoji(stage string) string {
	switch stage {
	case "downloading":
		return "📥 Yuklab olish"
	case "transcoding":
		return "🎬 Siqish"
	case "uploading":
		return "📤 Yuklash"
	case "completed":
		return "✅ Tayyor"
	case "failed":
		return "❌ Xatolik"
	default:
		return "⏳ Qayta ishlash"
	}
}

// notifySubscribedUsers sends notification about new video
func (vjp *VideoJobProcessor) notifySubscribedUsers(categoryID uint, videoTitle string) {
	var category database.Category
	if vjp.DB.Preload("Subject").First(&category, categoryID).Error != nil {
		return
	}

	var subscriptions []database.Subscription
	vjp.DB.Where("subject_id = ?", category.SubjectID).Preload("User").Find(&subscriptions)

	notificationText := fmt.Sprintf("🔔 *Yangi video qo'shildi!*\n\n"+
		"*Fan:* %s\n"+
		"*Kategoriya:* %s\n"+
		"*Video:* %s",
		category.Subject.Name, category.Name, videoTitle)

	for _, sub := range subscriptions {
		msg := tgbotapi.NewMessage(sub.User.TelegramID, notificationText)
		msg.ParseMode = "Markdown"
		vjp.Bot.Send(msg)
	}
}
