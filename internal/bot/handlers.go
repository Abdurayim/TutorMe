package bot

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"tg-bot/internal/database"
	"tg-bot/internal/roles"
	"tg-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

// State constants for our multi-step interactions
const (
	StateIdle                 = ""
	StateAwaitingSubjectName  = "awaiting_subject_name"
	StateAwaitingSubjectEdit  = "awaiting_subject_edit"
	StateAwaitingCategoryName = "awaiting_category_name"
	StateAwaitingCategoryEdit = "awaiting_category_edit"
	StateAwaitingTeacherPhone = "awaiting_teacher_phone"
	StateAwaitingResourceType = "awaiting_resource_type"
	StateAwaitingResourceURL  = "awaiting_resource_url"
	StateAwaitingResourceFile = "awaiting_resource_file"
)

// Handler holds the bot, database, and user state information.
type Handler struct {
	Bot              *tgbotapi.BotAPI
	DB               *gorm.DB
	userStates       map[int64]string // In-memory state: map[userID]state
	stateContext     map[int64]uint   // Stores context for a state, e.g., the subjectID to edit
	resourcePageInfo map[int64]int    // Stores current page number for resource pagination: map[userID]pageNumber
	VideoJobProcessor *VideoJobProcessor // Handles async video processing
}

// NewHandler initializes the bot handler with the state map.
func NewHandler(bot *tgbotapi.BotAPI, db *gorm.DB) *Handler {
	handler := &Handler{
		Bot:              bot,
		DB:               db,
		userStates:       make(map[int64]string),
		stateContext:     make(map[int64]uint),
		resourcePageInfo: make(map[int64]int),
	}

	// Initialize video job processor
	handler.VideoJobProcessor = NewVideoJobProcessor(bot, db)

	return handler
}

// HandleUpdate is the main entry point that routes updates.
func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		h.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		h.handleCallbackQuery(update.CallbackQuery)
	}
}

// handleMessage routes incoming messages based on user state or message type.
func (h *Handler) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	// If user is in a state, route to the stateful handler.
	if state := h.userStates[userID]; state != StateIdle {
		h.handleStatefulMessage(message)
		return
	}

	// Otherwise, handle as a regular message.
	if message.IsCommand() && message.Command() == "start" {
		h.handleStartCommand(message)
	} else if message.Contact != nil {
		h.handleContact(message)
	}
}

// handleStatefulMessage handles messages for users in a multi-step process.
func (h *Handler) handleStatefulMessage(message *tgbotapi.Message) {
	userID := message.From.ID
	chatID := message.Chat.ID
	state := h.userStates[userID]

	// Reset state after handling
	defer func() {
		delete(h.userStates, userID)
		delete(h.stateContext, userID)
	}()

	switch state {
	case StateAwaitingSubjectName:
		subject := database.Subject{Name: message.Text}
		if res := h.DB.Create(&subject); res.Error != nil {
			h.sendErrorMessage(chatID, "A subject with this name already exists.")
		} else {
			h.sendErrorMessage(chatID, fmt.Sprintf("✅ Subject '%s' created successfully!", subject.Name))
		}
		// After creating, show the updated subject list
		h.showManageSubjects(chatID, 0)

	case StateAwaitingSubjectEdit:
		subjectID := h.stateContext[userID]
		if res := h.DB.Model(&database.Subject{}).Where("id = ?", subjectID).Update("name", message.Text); res.Error != nil {
			h.sendErrorMessage(chatID, "Failed to update subject name. It may already exist.")
		} else {
			h.sendErrorMessage(chatID, "✅ Subject name updated successfully!")
		}
		h.showSubjectManagement(chatID, 0, subjectID)

	case StateAwaitingCategoryName:
		subjectID := h.stateContext[userID]
		category := database.Category{Name: message.Text, SubjectID: subjectID}
		if res := h.DB.Create(&category); res.Error != nil {
			h.sendErrorMessage(chatID, "Failed to create category.")
		} else {
			h.sendErrorMessage(chatID, fmt.Sprintf("✅ Category '%s' created successfully!", category.Name))
		}
		h.showManageCategories(chatID, 0, subjectID)

	case StateAwaitingCategoryEdit:
		categoryID := h.stateContext[userID]
		var category database.Category
		h.DB.First(&category, categoryID)
		if res := h.DB.Model(&database.Category{}).Where("id = ?", categoryID).Update("name", message.Text); res.Error != nil {
			h.sendErrorMessage(chatID, "Failed to update category name.")
		} else {
			h.sendErrorMessage(chatID, "✅ Category name updated successfully!")
		}
		h.showCategoryManagement(chatID, 0, categoryID, category.SubjectID)

	case StateAwaitingTeacherPhone:
		phone := message.Text
		if !strings.HasPrefix(phone, "+") {
			phone = "+" + phone
		}
		formattedPhone, err := utils.ValidateAndFormatPhoneNumber(phone)
		if err != nil {
			h.sendErrorMessage(chatID, "Invalid phone number format. Please try again.")
			return
		}

		// Check if user exists and update their role
		var user database.User
		if res := h.DB.First(&user, "phone_number = ?", formattedPhone); res.Error == gorm.ErrRecordNotFound {
			h.sendErrorMessage(chatID, "No user found with this phone number. The user must register first as a student.")
		} else {
			if user.Role == roles.Teacher {
				h.sendErrorMessage(chatID, "This user is already a teacher.")
			} else if user.Role == roles.Admin {
				h.sendErrorMessage(chatID, "Cannot change admin role.")
			} else {
				h.DB.Model(&user).Update("role", roles.Teacher)
				h.sendErrorMessage(chatID, fmt.Sprintf("✅ User %s has been promoted to Teacher!", user.Username))
			}
		}
		h.showManageTeachers(chatID, 0)

	case StateAwaitingResourceURL:
		categoryID := h.stateContext[userID]
		videoURL := strings.TrimSpace(message.Text)

		// Check if this is a video URL
		if utils.IsVideoURL(videoURL) {
			// This is a video URL - process it asynchronously
			source := utils.GetVideoSource(videoURL)

			// Create resource with pending status
			resource := database.Resource{
				Type:             "link", // Initially link, will become "video" after processing
				URL:              videoURL,
				CategoryID:       categoryID,
				ProcessingStatus: "pending",
				Progress:         0,
			}

			if res := h.DB.Create(&resource); res.Error != nil {
				log.Printf("Error creating resource: %v", res.Error)
				h.sendErrorMessage(chatID, "Resurs qo'shishda xatolik yuz berdi.")
			} else {
				log.Printf("Video resource created: ID=%d, URL=%s, Source=%s", resource.ID, videoURL, source)

				// Send initial processing message
				processingText := fmt.Sprintf("⏳ *Video qayta ishlanmoqda...*\n\n"+
					"Manba: %s\n"+
					"URL: %s\n\n"+
					"Bu bir necha daqiqa vaqt olishi mumkin. Progress yangilanishlarini ko'rib turasiz.",
					source, videoURL)

				progressMsg := tgbotapi.NewMessage(chatID, processingText)
				progressMsg.ParseMode = "Markdown"
				sentMsg, _ := h.Bot.Send(progressMsg)

				// Enqueue video processing job
				job := VideoJob{
					ResourceID: resource.ID,
					VideoURL:   videoURL,
					CategoryID: categoryID,
					ChatID:     chatID,
					MessageID:  sentMsg.MessageID,
				}
				h.VideoJobProcessor.EnqueueJob(job)

				log.Printf("Video job enqueued: ResourceID=%d", resource.ID)
			}
		} else {
			// Regular link (not a video)
			resource := database.Resource{
				Type:       "link",
				URL:        videoURL,
				CategoryID: categoryID,
			}

			if res := h.DB.Create(&resource); res.Error != nil {
				log.Printf("Error creating resource: %v", res.Error)
				h.sendErrorMessage(chatID, "Resurs qo'shishda xatolik yuz berdi.")
			} else {
				log.Printf("Link resource added: ID=%d, URL=%s", resource.ID, videoURL)
				h.sendErrorMessage(chatID, "✅ Havola muvaffaqiyatli qo'shildi!")
				// Notify subscribed users
				h.notifySubscribedUsers(categoryID, fmt.Sprintf("Yangi havola: %s", videoURL))
			}
		}

		h.showCategoryResources(chatID, 0, categoryID, userID)

	case StateAwaitingResourceFile:
		// Handle file upload (this will be triggered when user sends a document)
		if message.Document != nil {
			// Check file size (Telegram bot API limit is 50MB for downloads)
			if message.Document.FileSize > 50*1024*1024 { // 50MB in bytes
				h.sendErrorMessage(chatID, "❗️ Fayl hajmi juda katta (>50MB).\n\nIltimos:\n• Video uchun: YouTube'ga yuklang va havolani yuboring\n• Hujjat uchun: faylni siqing yoki Google Drive havolasini yuboring")
				return
			}

			categoryID := h.stateContext[userID]
			resource := database.Resource{
				Type:       "document",
				FileID:     message.Document.FileID,
				CategoryID: categoryID,
			}
			if res := h.DB.Create(&resource); res.Error != nil {
				h.sendErrorMessage(chatID, "Failed to add document.")
			} else {
				h.sendErrorMessage(chatID, "✅ Hujjat muvaffaqiyatli qo'shildi!")
				h.notifySubscribedUsers(categoryID, "Yangi hujjat qo'shildi")
			}
			h.showCategoryResources(chatID, 0, categoryID, userID)
		} else if message.Video != nil {
			// Check video size
			if message.Video.FileSize > 50*1024*1024 { // 50MB in bytes
				h.sendErrorMessage(chatID, "❗️ Video hajmi juda katta (>50MB).\n\nIltimos YouTube'ga yuklang va havolani yuboring.")
				return
			}

			categoryID := h.stateContext[userID]
			resource := database.Resource{
				Type:       "video",
				FileID:     message.Video.FileID,
				CategoryID: categoryID,
			}
			if res := h.DB.Create(&resource); res.Error != nil {
				h.sendErrorMessage(chatID, "Failed to add video.")
			} else {
				h.sendErrorMessage(chatID, "✅ Video muvaffaqiyatli qo'shildi!")
				h.notifySubscribedUsers(categoryID, "Yangi video qo'shildi")
			}
			h.showCategoryResources(chatID, 0, categoryID, userID)
		} else {
			h.sendErrorMessage(chatID, "Iltimos hujjat yoki video fayl yuboring.")
		}
	}
}

// handleCallbackQuery is the main router for all button presses.
func (h *Handler) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// Acknowledge the callback to remove the loading icon on the button
	h.Bot.Request(tgbotapi.NewCallback(query.ID, ""))

	// Check for nil message (can happen with inline queries)
	if query.Message == nil {
		log.Printf("Received callback with nil message from user %d", query.From.ID)
		return
	}

	data := query.Data
	userID := query.From.ID
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	log.Printf("Received callback data: '%s' from userID: %d", data, userID)

	// --- ADMIN & TEACHER ACTIONS ---
	if data == "manage_subjects" {
		// Check if user is admin or teacher
		var user database.User
		if h.DB.First(&user, "telegram_id = ?", userID).Error != nil {
			h.sendErrorMessage(chatID, "User not found.")
			return
		}
		if user.Role != roles.Admin && user.Role != roles.Teacher {
			h.sendErrorMessage(chatID, "You don't have permission to manage subjects.")
			return
		}
		h.showManageSubjects(chatID, messageID)
	} else if data == "add_subject" {
		// Check if user is admin or teacher
		var user database.User
		if h.DB.First(&user, "telegram_id = ?", userID).Error != nil {
			h.sendErrorMessage(chatID, "User not found.")
			return
		}
		if user.Role != roles.Admin && user.Role != roles.Teacher {
			h.sendErrorMessage(chatID, "You don't have permission to add subjects.")
			return
		}
		h.userStates[userID] = StateAwaitingSubjectName
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos yangi fan nomini yuboring."))
	} else if strings.HasPrefix(data, "view_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "view_subject_"))
		h.showSubjectManagement(chatID, messageID, uint(subjectID))
	} else if strings.HasPrefix(data, "edit_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "edit_subject_"))
		h.userStates[userID] = StateAwaitingSubjectEdit
		h.stateContext[userID] = uint(subjectID)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos fan uchun yangi nom yuboring."))
	} else if strings.HasPrefix(data, "delete_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "delete_subject_"))
		text := "⚠️ Are you sure you want to delete this subject and all its categories/resources? This action cannot be undone."
		msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		markup := ConfirmDeleteSubjectKeyboard(uint(subjectID))
		msg.ReplyMarkup = &markup
		h.Bot.Send(msg)
	} else if strings.HasPrefix(data, "confirm_delete_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "confirm_delete_subject_"))
		// In a real app, you'd delete associated categories and resources here in a transaction.
		if res := h.DB.Delete(&database.Subject{}, subjectID); res.Error != nil {
			h.sendErrorMessage(chatID, "Error deleting subject.")
		} else {
			h.sendErrorMessage(chatID, "🗑️ Subject deleted successfully.")
		}
		h.showManageSubjects(chatID, messageID)

		// --- STUDENT ACTIONS ---
	} else if data == "browse_subjects" {
		h.showBrowseSubjects(chatID, messageID, userID)
	} else if strings.HasPrefix(data, "subscribe_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "subscribe_subject_"))
		// Use a temporary user object to get the correct User ID from the database
		var user database.User
		h.DB.First(&user, "telegram_id = ?", userID)
		subscription := database.Subscription{UserID: user.ID, SubjectID: uint(subjectID)}
		h.DB.FirstOrCreate(&subscription, subscription)
		h.showBrowseSubjects(chatID, messageID, userID) // Refresh the list
	} else if strings.HasPrefix(data, "unsubscribe_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "unsubscribe_subject_"))
		var user database.User
		h.DB.First(&user, "telegram_id = ?", userID)
		h.DB.Where("user_id = ? AND subject_id = ?", user.ID, subjectID).Delete(&database.Subscription{})
		h.showBrowseSubjects(chatID, messageID, userID) // Refresh the list

		// --- TEACHER MANAGEMENT ---
	} else if data == "manage_teachers" {
		h.showManageTeachers(chatID, messageID)
	} else if data == "add_teacher" {
		h.userStates[userID] = StateAwaitingTeacherPhone
		h.Bot.Send(tgbotapi.NewMessage(chatID, "O'qituvchi qilmoqchi bo'lgan foydalanuvchining telefon raqamini yuboring (format: +998901234567)."))
	} else if data == "view_teachers_list" {
		h.showTeachersList(chatID, messageID)
	} else if strings.HasPrefix(data, "demote_teacher_") {
		teacherID, _ := strconv.Atoi(strings.TrimPrefix(data, "demote_teacher_"))
		h.DB.Model(&database.User{}).Where("id = ?", teacherID).Update("role", roles.Student)
		h.sendErrorMessage(chatID, "✅ Teacher has been demoted to Student.")
		h.showManageTeachers(chatID, messageID)

		// --- STUDENT MANAGEMENT ---
	} else if data == "manage_students" {
		h.showManageStudents(chatID, messageID)
	} else if data == "view_students_list" {
		h.showStudentsList(chatID, messageID)

		// --- CATEGORY MANAGEMENT ---
	} else if strings.HasPrefix(data, "manage_categories_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "manage_categories_"))
		h.showManageCategories(chatID, messageID, uint(subjectID))
	} else if strings.HasPrefix(data, "add_category_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "add_category_"))
		h.userStates[userID] = StateAwaitingCategoryName
		h.stateContext[userID] = uint(subjectID)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos yangi kategoriya nomini yuboring."))
	} else if strings.HasPrefix(data, "view_category_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "view_category_"))
		var category database.Category
		h.DB.First(&category, categoryID)
		h.showCategoryManagement(chatID, messageID, uint(categoryID), category.SubjectID)
	} else if strings.HasPrefix(data, "edit_category_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "edit_category_"))
		h.userStates[userID] = StateAwaitingCategoryEdit
		h.stateContext[userID] = uint(categoryID)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos kategoriya uchun yangi nom yuboring."))
	} else if strings.HasPrefix(data, "delete_category_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "delete_category_"))
		var category database.Category
		h.DB.First(&category, categoryID)
		text := "⚠️ Are you sure you want to delete this category and all its resources?"
		msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		markup := ConfirmDeleteCategoryKeyboard(uint(categoryID), category.SubjectID)
		msg.ReplyMarkup = &markup
		h.Bot.Send(msg)
	} else if strings.HasPrefix(data, "confirm_delete_category_") {
		parts := strings.Split(strings.TrimPrefix(data, "confirm_delete_category_"), "_")
		categoryID, _ := strconv.Atoi(parts[0])
		subjectID, _ := strconv.Atoi(parts[1])
		h.DB.Delete(&database.Category{}, categoryID)
		h.sendErrorMessage(chatID, "🗑️ Category deleted successfully.")
		h.showManageCategories(chatID, messageID, uint(subjectID))

		// --- RESOURCE MANAGEMENT ---
	} else if strings.HasPrefix(data, "manage_resources_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "manage_resources_"))
		h.resourcePageInfo[userID] = 0 // Reset to first page
		h.showCategoryResources(chatID, messageID, uint(categoryID), userID)
	} else if strings.HasPrefix(data, "resources_page_") {
		// Format: resources_page_CATEGORYID_PAGENUMBER
		parts := strings.Split(strings.TrimPrefix(data, "resources_page_"), "_")
		categoryID, _ := strconv.Atoi(parts[0])
		page, _ := strconv.Atoi(parts[1])
		h.resourcePageInfo[userID] = page
		h.showCategoryResources(chatID, messageID, uint(categoryID), userID)
	} else if strings.HasPrefix(data, "add_resource_link_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "add_resource_link_"))
		h.userStates[userID] = StateAwaitingResourceURL
		h.stateContext[userID] = uint(categoryID)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos resurs havolasini yuboring (YouTube, ijtimoiy tarmoqlar va boshqalar)."))
	} else if strings.HasPrefix(data, "add_resource_file_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "add_resource_file_"))
		h.userStates[userID] = StateAwaitingResourceFile
		h.stateContext[userID] = uint(categoryID)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "Iltimos fayl yuboring (PDF, DOC, video va boshqalar)."))
	} else if strings.HasPrefix(data, "delete_resource_") {
		parts := strings.Split(strings.TrimPrefix(data, "delete_resource_"), "_")
		resourceID, _ := strconv.Atoi(parts[0])
		categoryID, _ := strconv.Atoi(parts[1])
		h.DB.Delete(&database.Resource{}, resourceID)
		h.sendErrorMessage(chatID, "🗑️ Resource deleted successfully.")
		h.showCategoryResources(chatID, messageID, uint(categoryID), userID)

		// --- STUDENT BROWSING ACTIONS ---
	} else if data == "student_browse_subjects" {
		h.showStudentBrowseSubjects(chatID, messageID)
	} else if data == "student_my_subscriptions" {
		h.showMySubscriptions(chatID, messageID, userID)
	} else if strings.HasPrefix(data, "student_view_subject_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "student_view_subject_"))
		h.showStudentViewSubject(chatID, messageID, userID, uint(subjectID))
	} else if strings.HasPrefix(data, "student_subscribe_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "student_subscribe_"))
		var user database.User
		h.DB.First(&user, "telegram_id = ?", userID)
		subscription := database.Subscription{UserID: user.ID, SubjectID: uint(subjectID)}
		h.DB.FirstOrCreate(&subscription, subscription)
		h.Bot.Send(tgbotapi.NewMessage(chatID, "🔔 You will now receive notifications for new resources!"))
		h.showStudentViewSubject(chatID, messageID, userID, uint(subjectID))
	} else if strings.HasPrefix(data, "student_unsubscribe_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "student_unsubscribe_"))
		var user database.User
		h.DB.First(&user, "telegram_id = ?", userID)
		h.DB.Where("user_id = ? AND subject_id = ?", user.ID, subjectID).Delete(&database.Subscription{})
		h.Bot.Send(tgbotapi.NewMessage(chatID, "🔕 You will no longer receive notifications for this subject."))
		h.showStudentViewSubject(chatID, messageID, userID, uint(subjectID))
	} else if strings.HasPrefix(data, "student_view_categories_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "student_view_categories_"))
		h.showStudentViewCategories(chatID, messageID, uint(subjectID))
	} else if strings.HasPrefix(data, "student_view_category_") {
		categoryID, _ := strconv.Atoi(strings.TrimPrefix(data, "student_view_category_"))
		h.resourcePageInfo[userID] = 0 // Reset to first page when viewing a category
		h.showStudentViewResources(chatID, messageID, uint(categoryID), userID)
	} else if strings.HasPrefix(data, "student_resources_page_") {
		parts := strings.Split(strings.TrimPrefix(data, "student_resources_page_"), "_")
		categoryID, _ := strconv.Atoi(parts[0])
		page, _ := strconv.Atoi(parts[1])
		h.resourcePageInfo[userID] = page
		h.showStudentViewResources(chatID, messageID, uint(categoryID), userID)

		// --- BACK BUTTONS ---
	} else if strings.HasPrefix(data, "back_to_categories_") {
		subjectID, _ := strconv.Atoi(strings.TrimPrefix(data, "back_to_categories_"))
		h.showManageCategories(chatID, messageID, uint(subjectID))
	} else if data == "back_to_dashboard" {
		var user database.User
		h.DB.First(&user, "telegram_id = ?", userID)
		h.showDashboard(&user)
	} else if data == "noop" {
		// No operation - used for non-clickable pagination indicators
		// Just answer the callback to remove loading state
		callback := tgbotapi.NewCallback(query.ID, "")
		h.Bot.Request(callback)
		return
	}
} // <-- THIS IS THE CORRECT END OF THE handleCallbackQuery FUNCTION

// --- Handler Logic Functions (Helpers) ---

func (h *Handler) handleStartCommand(message *tgbotapi.Message) {
	var user database.User
	if res := h.DB.First(&user, "telegram_id = ?", message.From.ID); res.Error == gorm.ErrRecordNotFound {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Xush kelibsiz! Boshlash uchun telefon raqamingizni ulashing.")
		msg.ReplyMarkup = GetContactKeyboard()
		h.Bot.Send(msg)
	} else {
		h.showDashboard(&user)
	}
}

func (h *Handler) handleContact(message *tgbotapi.Message) {
	phone := message.Contact.PhoneNumber
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}
	formattedPhone, err := utils.ValidateAndFormatPhoneNumber(phone)
	if err != nil {
		h.sendErrorMessage(message.Chat.ID, "Kiritilgan telefon raqam noto'g'ri.")
		return
	}

	userRole := roles.Student
	if adminPhone := os.Getenv("ADMIN_PHONE_NUMBER"); adminPhone != "" && formattedPhone == adminPhone {
		userRole = roles.Admin
	}

	newUser := database.User{
		TelegramID:  message.From.ID,
		Username:    message.From.UserName,
		FirstName:   message.From.FirstName,
		LastName:    message.From.LastName,
		PhoneNumber: formattedPhone,
		Role:        userRole,
	}

	if res := h.DB.Create(&newUser); res.Error != nil {
		h.sendErrorMessage(message.Chat.ID, "Bu telefon raqam allaqachon ro'yxatdan o'tgan.")
		return
	}

	welcomeText := "✅ Ro'yxatdan o'tish muvaffaqiyatli! Siz endi Talabasiz."
	if userRole == roles.Admin {
		welcomeText = "👑 Admin sifatida ro'yxatdan o'tdingiz! Xush kelibsiz."
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	h.Bot.Send(msg)
	h.showDashboard(&newUser)
}

func (h *Handler) showDashboard(user *database.User) {
	var msg tgbotapi.MessageConfig
	switch user.Role {
	case roles.Admin:
		var teacherCount, studentCount int64
		h.DB.Model(&database.User{}).Where("role = ?", roles.Teacher).Count(&teacherCount)
		h.DB.Model(&database.User{}).Where("role = ?", roles.Student).Count(&studentCount)
		text := fmt.Sprintf("👑 *Admin Dashboard*\n\n*Teachers:* %d\n*Students:* %d", teacherCount, studentCount)
		msg = tgbotapi.NewMessage(user.TelegramID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = AdminDashboardKeyboard(teacherCount, studentCount)
	case roles.Teacher:
		msg = tgbotapi.NewMessage(user.TelegramID, "👨‍🏫 *Teacher Dashboard*")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = TeacherDashboardKeyboard()
	case roles.Student:
		msg = tgbotapi.NewMessage(user.TelegramID, "🎓 *Student Dashboard*")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = StudentDashboardKeyboard()
	}
	h.Bot.Send(msg)
}

func (h *Handler) sendErrorMessage(chatID int64, text string) {
	h.Bot.Send(tgbotapi.NewMessage(chatID, "❗️ "+text))
}

// --- UI Flow Functions (Helpers) ---

func (h *Handler) showManageSubjects(chatID int64, messageID int) {
	var subjects []database.Subject
	h.DB.Order("name asc").Find(&subjects)
	text := "📚 *Manage Subjects*\n\nSelect a subject to manage or add a new one."
	if len(subjects) == 0 {
		text = "No subjects found. Please add the first one."
	}
	markup := ManageSubjectsKeyboard(subjects)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = markup

	if messageID != 0 { // If it's from a callback, edit the message
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msg.Text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = &markup
		h.Bot.Send(editMsg)
	} else {
		h.Bot.Send(msg)
	}
}

func (h *Handler) showSubjectManagement(chatID int64, messageID int, subjectID uint) {
	var subject database.Subject
	if h.DB.First(&subject, subjectID).Error != nil {
		h.sendErrorMessage(chatID, "Subject not found.")
		return
	}
	text := fmt.Sprintf("📖 Managing Subject: *%s*", subject.Name)
	markup := SubjectManagementKeyboard(subjectID)
	// We always edit here since this view only comes from a button press
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = &markup
	h.Bot.Send(msg)
}

func (h *Handler) showBrowseSubjects(chatID int64, messageID int, telegramUserID int64) {
	var user database.User
	if res := h.DB.First(&user, "telegram_id = ?", telegramUserID); res.Error != nil {
		h.sendErrorMessage(chatID, "Could not find your user record.")
		return
	}

	var allSubjects []database.Subject
	h.DB.Order("name asc").Find(&allSubjects)

	var subscriptions []database.Subscription
	h.DB.Where("user_id = ?", user.ID).Find(&subscriptions)
	subscribedIDs := make(map[uint]bool)
	for _, sub := range subscriptions {
		subscribedIDs[sub.SubjectID] = true
	}

	text := "📖 *Browse Subjects*\n\nSelect a subject to subscribe or unsubscribe. A ✅ indicates you are already subscribed."
	if len(allSubjects) == 0 {
		text = "There are no subjects available to subscribe to yet."
	}
	markup := BrowseSubjectsKeyboard(allSubjects, subscribedIDs)
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = &markup
	h.Bot.Send(msg)
}

// --- Teacher Management Functions ---

func (h *Handler) showManageTeachers(chatID int64, messageID int) {
	var teacherCount int64
	h.DB.Model(&database.User{}).Where("role = ?", roles.Teacher).Count(&teacherCount)
	text := fmt.Sprintf("👨‍🏫 *Manage Teachers*\n\n*Total Teachers:* %d", teacherCount)
	markup := ManageTeachersKeyboard()

	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = &markup
		h.Bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = markup
		h.Bot.Send(msg)
	}
}

func (h *Handler) showTeachersList(chatID int64, messageID int) {
	var teachers []database.User
	h.DB.Where("role = ?", roles.Teacher).Find(&teachers)

	text := "👨‍🏫 *O'qituvchilar Ro'yxati*\n\n"
	if len(teachers) == 0 {
		text += "Hech qanday o'qituvchi ro'yxatdan o'tmagan."
	} else {
		for i, teacher := range teachers {
			// Build full name
			fullName := teacher.FirstName
			if teacher.LastName != "" {
				fullName += " " + teacher.LastName
			}
			if fullName == "" {
				fullName = "Noma'lum"
			}

			// Username display - escape underscores for Markdown
			username := teacher.Username
			if username == "" {
				username = "Yo'q"
			} else {
				// Escape underscores for Markdown
				username = strings.ReplaceAll(username, "_", "\\_")
			}

			text += fmt.Sprintf("%d. *Ism:* %s\n   *Username:* %s\n   *Telefon:* %s\n   *Telegram ID:* %d\n\n",
				i+1, fullName, username, teacher.PhoneNumber, teacher.TelegramID)
		}
	}

	markup := TeachersListKeyboard(teachers)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}

// --- Student Management Functions ---

func (h *Handler) showManageStudents(chatID int64, messageID int) {
	var studentCount int64
	h.DB.Model(&database.User{}).Where("role = ?", roles.Student).Count(&studentCount)
	text := fmt.Sprintf("🎓 *Manage Students*\n\n*Total Students:* %d", studentCount)
	markup := ManageStudentsKeyboard()

	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = &markup
		h.Bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = markup
		h.Bot.Send(msg)
	}
}

func (h *Handler) showStudentsList(chatID int64, messageID int) {
	var students []database.User
	h.DB.Where("role = ?", roles.Student).Find(&students)

	log.Printf("Found %d students in database", len(students))

	text := "🎓 *Talabalar Ro'yxati*\n\n"
	if len(students) == 0 {
		text += "Hech qanday talaba ro'yxatdan o'tmagan."
	} else {
		for i, student := range students {
			// Build full name
			fullName := student.FirstName
			if student.LastName != "" {
				fullName += " " + student.LastName
			}
			if fullName == "" {
				fullName = "Noma'lum"
			}

			// Username display - escape underscores for Markdown
			username := student.Username
			if username == "" {
				username = "Yo'q"
			} else {
				// Escape underscores for Markdown
				username = strings.ReplaceAll(username, "_", "\\_")
			}

			text += fmt.Sprintf("%d. *Ism:* %s\n   *Username:* %s\n   *Telefon:* %s\n   *Telegram ID:* %d\n\n",
				i+1, fullName, username, student.PhoneNumber, student.TelegramID)

			log.Printf("Student %d: Name=%s, Username=%s, Phone=%s, TelegramID=%d", i+1, fullName, student.Username, student.PhoneNumber, student.TelegramID)
		}
	}

	markup := StudentsListKeyboard()
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}

// --- Category Management Functions ---

func (h *Handler) showManageCategories(chatID int64, messageID int, subjectID uint) {
	var subject database.Subject
	if h.DB.First(&subject, subjectID).Error != nil {
		h.sendErrorMessage(chatID, "Subject not found.")
		return
	}

	var categories []database.Category
	h.DB.Where("subject_id = ?", subjectID).Order("name asc").Find(&categories)

	text := fmt.Sprintf("🗂️ *Manage Categories*\n*Subject:* %s\n\n", subject.Name)
	if len(categories) == 0 {
		text += "No categories found. Please add one."
	} else {
		text += "Select a category to manage:"
	}

	markup := ManageCategoriesKeyboard(categories, subjectID)

	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = &markup
		h.Bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = markup
		h.Bot.Send(msg)
	}
}

func (h *Handler) showCategoryManagement(chatID int64, messageID int, categoryID uint, subjectID uint) {
	var category database.Category
	if h.DB.First(&category, categoryID).Error != nil {
		h.sendErrorMessage(chatID, "Category not found.")
		return
	}

	text := fmt.Sprintf("📁 *Managing Category:* %s", category.Name)
	markup := CategoryManagementKeyboard(categoryID, subjectID)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}

// --- Resource Management Functions ---

const ResourcesPerPage = 10

func (h *Handler) showCategoryResources(chatID int64, messageID int, categoryID uint, userID int64) {
	var category database.Category
	if h.DB.Preload("Subject").First(&category, categoryID).Error != nil {
		h.sendErrorMessage(chatID, "Category not found.")
		return
	}

	var resources []database.Resource
	// Exclude resources that are still processing (only show completed or non-video resources)
	h.DB.Where("category_id = ? AND (processing_status = '' OR processing_status = 'completed' OR processing_status = 'failed')", categoryID).Order("created_at asc").Find(&resources)

	log.Printf("Showing resources for category %d: Found %d resources", categoryID, len(resources))
	for _, res := range resources {
		log.Printf("  Resource ID=%d, Type=%s, URL=%s, FileID=%s", res.ID, res.Type, res.URL, res.FileID)
	}

	// Get current page for this user
	currentPage, exists := h.resourcePageInfo[userID]
	if !exists {
		currentPage = 0
		h.resourcePageInfo[userID] = 0
	}

	// Calculate pagination
	totalResources := len(resources)
	totalPages := (totalResources + ResourcesPerPage - 1) / ResourcesPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	// Ensure current page is valid
	if currentPage >= totalPages {
		currentPage = totalPages - 1
		h.resourcePageInfo[userID] = currentPage
	}
	if currentPage < 0 {
		currentPage = 0
		h.resourcePageInfo[userID] = 0
	}

	// Calculate slice indices for current page
	start := currentPage * ResourcesPerPage
	end := start + ResourcesPerPage
	if end > totalResources {
		end = totalResources
	}

	// Send header message
	headerText := fmt.Sprintf("📚 *%s dagi resurslar*\n*Fan:* %s\n\n", category.Name, category.Subject.Name)
	if totalResources == 0 {
		headerText += "Hech qanday resurs yo'q. Qo'shing!"
	} else {
		headerText += fmt.Sprintf("Jami %d ta resurs (Sahifa %d/%d):", totalResources, currentPage+1, totalPages)
	}

	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, headerText)
		editMsg.ParseMode = "Markdown"
		h.Bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, headerText)
		msg.ParseMode = "Markdown"
		h.Bot.Send(msg)
	}

	// Send only the resources for the current page (skip if no resources)
	if totalResources == 0 {
		// Skip resource iteration - show only keyboard
		// Build management keyboard
		var keyboardRows [][]tgbotapi.InlineKeyboardButton

		// Add resource buttons
		keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 Havola qo'shish", fmt.Sprintf("add_resource_link_%d", categoryID)),
			tgbotapi.NewInlineKeyboardButtonData("📎 Fayl qo'shish", fmt.Sprintf("add_resource_file_%d", categoryID)),
		))

		// Back button
		keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyaga qaytish", fmt.Sprintf("view_category_%d", categoryID)),
		))

		addButtonsMarkup := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
		keyboardMsg := tgbotapi.NewMessage(chatID, "⚙️ *Boshqarish:*")
		keyboardMsg.ParseMode = "Markdown"
		keyboardMsg.ReplyMarkup = addButtonsMarkup
		h.Bot.Send(keyboardMsg)
		return
	}

	pageResources := resources[start:end]
	for i, res := range pageResources {
		// Global index for numbering (not just page index)
		globalIndex := start + i + 1

		emoji := "🔗"
		if res.Type == "video" {
			emoji = "🎥"
		} else if res.Type == "document" {
			emoji = "📄"
		}

		// Create delete button for this resource
		deleteButton := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ O'chirish", fmt.Sprintf("delete_resource_%d_%d", res.ID, categoryID)),
			),
		)

		if res.Type == "link" {
			// Send link as a numbered message with delete button
			// Escape underscores in URL for Markdown
			escapedURL := strings.ReplaceAll(res.URL, "_", "\\_")
			linkText := fmt.Sprintf("%d. %s *Havola:*\n%s", globalIndex, emoji, escapedURL)
			linkMsg := tgbotapi.NewMessage(chatID, linkText)
			linkMsg.ParseMode = "Markdown"
			linkMsg.ReplyMarkup = deleteButton
			h.Bot.Send(linkMsg)
		} else if res.Type == "video" {
			// Send video with caption, title, and source URL link
			videoMsg := tgbotapi.NewVideo(chatID, tgbotapi.FileID(res.FileID))
			caption := fmt.Sprintf("%d. %s", globalIndex, emoji)
			if res.Title != "" {
				caption += fmt.Sprintf(" *%s*", res.Title)
			} else {
				caption += " *Video*"
			}
			// Add source URL link if available
			if res.URL != "" {
				escapedURL := strings.ReplaceAll(res.URL, "_", "\\_")
				caption += fmt.Sprintf("\n\n🔗 Manba: %s", escapedURL)
			}
			videoMsg.Caption = caption
			videoMsg.ParseMode = "Markdown"
			videoMsg.ReplyMarkup = &deleteButton
			h.Bot.Send(videoMsg)
		} else if res.Type == "document" {
			// Send document with caption and delete button
			docMsg := tgbotapi.NewDocument(chatID, tgbotapi.FileID(res.FileID))
			docMsg.Caption = fmt.Sprintf("%d. %s *Hujjat*", globalIndex, emoji)
			docMsg.ParseMode = "Markdown"
			docMsg.ReplyMarkup = &deleteButton
			h.Bot.Send(docMsg)
		}
	}

	// Build pagination and management keyboard
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	// Pagination row (only show if there are multiple pages)
	if totalPages > 1 {
		var paginationRow []tgbotapi.InlineKeyboardButton
		if currentPage > 0 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Oldingi", fmt.Sprintf("resources_page_%d_%d", categoryID, currentPage-1)))
		}
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Sahifa %d/%d", currentPage+1, totalPages), "noop"))
		if currentPage < totalPages-1 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("Keyingi ➡️", fmt.Sprintf("resources_page_%d_%d", categoryID, currentPage+1)))
		}
		keyboardRows = append(keyboardRows, paginationRow)
	}

	// Add resource buttons
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔗 Havola qo'shish", fmt.Sprintf("add_resource_link_%d", categoryID)),
		tgbotapi.NewInlineKeyboardButtonData("📎 Fayl qo'shish", fmt.Sprintf("add_resource_file_%d", categoryID)),
	))

	// Back button
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyaga qaytish", fmt.Sprintf("view_category_%d", categoryID)),
	))

	addButtonsMarkup := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	keyboardMsg := tgbotapi.NewMessage(chatID, "⚙️ *Boshqarish:*")
	keyboardMsg.ParseMode = "Markdown"
	keyboardMsg.ReplyMarkup = addButtonsMarkup
	h.Bot.Send(keyboardMsg)
}

// --- Notification System ---

func (h *Handler) notifySubscribedUsers(categoryID uint, message string) {
	var category database.Category
	if h.DB.Preload("Subject").First(&category, categoryID).Error != nil {
		return
	}

	var subscriptions []database.Subscription
	h.DB.Where("subject_id = ?", category.SubjectID).Preload("User").Find(&subscriptions)

	notificationText := fmt.Sprintf("🔔 *New Content Alert!*\n\n*Subject:* %s\n*Category:* %s\n\n%s",
		category.Subject.Name, category.Name, message)

	for _, sub := range subscriptions {
		msg := tgbotapi.NewMessage(sub.User.TelegramID, notificationText)
		msg.ParseMode = "Markdown"
		h.Bot.Send(msg)
	}
}

// --- Student Browsing Functions ---

func (h *Handler) showStudentBrowseSubjects(chatID int64, messageID int) {
	var subjects []database.Subject
	h.DB.Order("name asc").Find(&subjects)

	text := "📚 *Browse All Subjects*\n\nSelect a subject to view its categories and resources."
	if len(subjects) == 0 {
		text = "No subjects available yet."
	}

	markup := StudentBrowseSubjectsKeyboard(subjects)

	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = &markup
		h.Bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = markup
		h.Bot.Send(msg)
	}
}

func (h *Handler) showStudentViewSubject(chatID int64, messageID int, userID int64, subjectID uint) {
	var subject database.Subject
	if h.DB.First(&subject, subjectID).Error != nil {
		h.sendErrorMessage(chatID, "Subject not found.")
		return
	}

	// Check if student is subscribed
	var user database.User
	h.DB.First(&user, "telegram_id = ?", userID)

	var subscription database.Subscription
	isSubscribed := h.DB.Where("user_id = ? AND subject_id = ?", user.ID, subjectID).First(&subscription).Error == nil

	text := fmt.Sprintf("📖 *Subject: %s*\n\n", subject.Name)
	if isSubscribed {
		text += "🔔 You are subscribed to notifications for this subject.\n\n"
	} else {
		text += "🔕 Subscribe to get notified when new resources are added.\n\n"
	}
	text += "Click 'View Categories' to explore the available materials."

	markup := StudentViewSubjectKeyboard(subjectID, isSubscribed)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}

func (h *Handler) showStudentViewCategories(chatID int64, messageID int, subjectID uint) {
	var subject database.Subject
	if h.DB.First(&subject, subjectID).Error != nil {
		h.sendErrorMessage(chatID, "Subject not found.")
		return
	}

	var categories []database.Category
	h.DB.Where("subject_id = ?", subjectID).Order("name asc").Find(&categories)

	text := fmt.Sprintf("📁 *Categories in %s*\n\n", subject.Name)
	if len(categories) == 0 {
		text += "No categories available yet."
	} else {
		text += "Select a category to view its resources:"
	}

	markup := StudentViewCategoriesKeyboard(categories, subjectID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}

func (h *Handler) showStudentViewResources(chatID int64, messageID int, categoryID uint, userID int64) {
	var category database.Category
	if h.DB.Preload("Subject").First(&category, categoryID).Error != nil {
		h.sendErrorMessage(chatID, "Kategoriya topilmadi.")
		return
	}

	var resources []database.Resource
	// Exclude resources that are still processing (only show completed or non-video resources)
	h.DB.Where("category_id = ? AND (processing_status = '' OR processing_status = 'completed' OR processing_status = 'failed')", categoryID).Order("created_at asc").Find(&resources)

	// Get current page for this user
	currentPage, exists := h.resourcePageInfo[userID]
	if !exists {
		currentPage = 0
		h.resourcePageInfo[userID] = 0
	}

	// Calculate pagination
	totalResources := len(resources)
	totalPages := (totalResources + ResourcesPerPage - 1) / ResourcesPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	// Ensure current page is valid
	if currentPage >= totalPages {
		currentPage = totalPages - 1
		h.resourcePageInfo[userID] = currentPage
	}
	if currentPage < 0 {
		currentPage = 0
		h.resourcePageInfo[userID] = 0
	}

	// Calculate slice indices for current page
	start := currentPage * ResourcesPerPage
	end := start + ResourcesPerPage
	if end > totalResources {
		end = totalResources
	}

	// Send header message
	headerText := fmt.Sprintf("📚 *%s dagi resurslar*\n*Fan:* %s\n\n", category.Name, category.Subject.Name)
	if totalResources == 0 {
		headerText += "Hech qanday resurs mavjud emas."
	} else {
		headerText += fmt.Sprintf("Jami %d ta resurs (Sahifa %d/%d):", totalResources, currentPage+1, totalPages)
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, headerText)
	editMsg.ParseMode = "Markdown"
	h.Bot.Send(editMsg)

	// Send only the resources for the current page (skip if no resources)
	if totalResources == 0 {
		// Skip resource iteration - show only back button
		markup := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyalarga qaytish", fmt.Sprintf("student_view_categories_%d", category.SubjectID)),
			),
		)
		keyboardMsg := tgbotapi.NewMessage(chatID, "⬅️ Orqaga qaytish:")
		keyboardMsg.ReplyMarkup = markup
		h.Bot.Send(keyboardMsg)
		return
	}

	pageResources := resources[start:end]
	for i, res := range pageResources {
		// Global index for numbering (not just page index)
		globalIndex := start + i + 1

		emoji := "🔗"
		if res.Type == "video" {
			emoji = "🎥"
		} else if res.Type == "document" {
			emoji = "📄"
		}

		if res.Type == "link" {
			// Send link as a numbered message
			// Escape underscores in URL for Markdown
			escapedURL := strings.ReplaceAll(res.URL, "_", "\\_")
			linkText := fmt.Sprintf("%d. %s *Havola:*\n%s", globalIndex, emoji, escapedURL)
			linkMsg := tgbotapi.NewMessage(chatID, linkText)
			linkMsg.ParseMode = "Markdown"
			h.Bot.Send(linkMsg)
		} else if res.Type == "video" {
			// Send video with caption, title, and source URL link
			videoMsg := tgbotapi.NewVideo(chatID, tgbotapi.FileID(res.FileID))
			caption := fmt.Sprintf("%d. %s", globalIndex, emoji)
			if res.Title != "" {
				caption += fmt.Sprintf(" *%s*", res.Title)
			} else {
				caption += " *Video*"
			}
			// Add source URL link if available
			if res.URL != "" {
				escapedURL := strings.ReplaceAll(res.URL, "_", "\\_")
				caption += fmt.Sprintf("\n\n🔗 Manba: %s", escapedURL)
			}
			videoMsg.Caption = caption
			videoMsg.ParseMode = "Markdown"
			h.Bot.Send(videoMsg)
		} else if res.Type == "document" {
			// Send document with caption
			docMsg := tgbotapi.NewDocument(chatID, tgbotapi.FileID(res.FileID))
			docMsg.Caption = fmt.Sprintf("%d. %s *Hujjat*", globalIndex, emoji)
			docMsg.ParseMode = "Markdown"
			h.Bot.Send(docMsg)
		}
	}

	// Build pagination and navigation keyboard
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	// Pagination row (only show if there are multiple pages)
	if totalPages > 1 {
		var paginationRow []tgbotapi.InlineKeyboardButton
		if currentPage > 0 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Oldingi", fmt.Sprintf("student_resources_page_%d_%d", categoryID, currentPage-1)))
		}
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Sahifa %d/%d", currentPage+1, totalPages), "noop"))
		if currentPage < totalPages-1 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("Keyingi ➡️", fmt.Sprintf("student_resources_page_%d_%d", categoryID, currentPage+1)))
		}
		keyboardRows = append(keyboardRows, paginationRow)
	}

	// Back button
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyalarga qaytish", fmt.Sprintf("student_view_categories_%d", category.SubjectID)),
	))

	markup := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	keyboardMsg := tgbotapi.NewMessage(chatID, "⬅️ Orqaga qaytish:")
	keyboardMsg.ReplyMarkup = markup
	h.Bot.Send(keyboardMsg)
}

func (h *Handler) showMySubscriptions(chatID int64, messageID int, userID int64) {
	var user database.User
	if h.DB.First(&user, "telegram_id = ?", userID).Error != nil {
		h.sendErrorMessage(chatID, "User not found.")
		return
	}

	var subscriptions []database.Subscription
	h.DB.Where("user_id = ?", user.ID).Preload("Subject").Find(&subscriptions)

	var subjects []database.Subject
	for _, sub := range subscriptions {
		subjects = append(subjects, sub.Subject)
	}

	text := "🔔 *My Subscriptions*\n\n"
	if len(subjects) == 0 {
		text += "You haven't subscribed to any subjects yet.\n\nSubscriptions allow you to receive notifications when new resources are added."
	} else {
		text += "You will receive notifications for these subjects when new resources are added:"
	}

	markup := MySubscriptionsKeyboard(subjects)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &markup
	h.Bot.Send(editMsg)
}
