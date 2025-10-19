package bot

import (
	"fmt"
	"strconv"                  // Use strconv for converting integers to strings, it's more standard
	"tg-bot/internal/database" // <-- **FIX**: IMPORTED THE DATABASE PACKAGE

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- Reply Keyboards ---

// GetContactKeyboard creates a reply keyboard asking the user to share their contact.
func GetContactKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonContact("📱 Telefon raqamni yuborish"),
		),
	)
}

// --- Inline Keyboards ---

// AdminDashboardKeyboard creates the main dashboard keyboard for admins.
func AdminDashboardKeyboard(teacherCount, studentCount int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Fanlarni boshqarish", "manage_subjects"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👨‍🏫 O'qituvchilarni boshqarish", "manage_teachers"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎓 Talabalarni boshqarish", "manage_students"),
		),
	)
}

// TeacherDashboardKeyboard creates the main dashboard for teachers.
func TeacherDashboardKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Fanlarni boshqarish", "manage_subjects"),
		),
	)
}

// StudentDashboardKeyboard creates the main dashboard for students.
func StudentDashboardKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Barcha fanlar", "student_browse_subjects"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔔 Obunalarim", "student_my_subscriptions"),
		),
	)
}

// ManageSubjectsKeyboard creates a dynamic keyboard with a list of subjects and an add button.
func ManageSubjectsKeyboard(subjects []database.Subject) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	// Create a button for each subject
	for _, subject := range subjects {
		// Callback data will be unique, e.g., "view_subject_1"
		callbackData := fmt.Sprintf("view_subject_%d", subject.ID)
		subjectButton := tgbotapi.NewInlineKeyboardButtonData("📖 "+subject.Name, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(subjectButton)
		keyboardRows = append(keyboardRows, row)
	}

	// Add the static "Add New Subject" button at the end
	addSubjectButton := tgbotapi.NewInlineKeyboardButtonData("➕ Yangi fan qo'shish", "add_subject")
	addRow := tgbotapi.NewInlineKeyboardRow(addSubjectButton)
	keyboardRows = append(keyboardRows, addRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// SubjectManagementKeyboard creates a keyboard with edit and delete options for a subject.
func SubjectManagementKeyboard(subjectID uint) tgbotapi.InlineKeyboardMarkup {
	// Use strconv.Itoa for better performance and type safety.
	idStr := strconv.Itoa(int(subjectID))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Nomini o'zgartirish", "edit_subject_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("🗑️ O'chirish", "delete_subject_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗂️ Kategoriyalarni boshqarish", "manage_categories_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Fanlarga qaytish", "manage_subjects"),
		),
	)
}

// ConfirmDeleteSubjectKeyboard creates a confirmation keyboard for deleting a subject.
func ConfirmDeleteSubjectKeyboard(subjectID uint) tgbotapi.InlineKeyboardMarkup {
	idStr := strconv.Itoa(int(subjectID))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Ha, o'chirish", "confirm_delete_subject_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("❌ Bekor qilish", "view_subject_"+idStr),
		),
	)
}

// BrowseSubjectsKeyboard creates a keyboard for students to browse and subscribe to subjects.
func BrowseSubjectsKeyboard(subjects []database.Subject, subscribedIDs map[uint]bool) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for _, subject := range subjects {
		buttonText := subject.Name
		callbackData := fmt.Sprintf("subscribe_subject_%d", subject.ID)

		if subscribedIDs[subject.ID] {
			buttonText = "✅ " + buttonText
			callbackData = fmt.Sprintf("unsubscribe_subject_%d", subject.ID)
		}

		subjectButton := tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(subjectButton)
		keyboardRows = append(keyboardRows, row)
	}

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// --- Teacher Management Keyboards ---

// ManageTeachersKeyboard creates the main teacher management keyboard.
func ManageTeachersKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ O'qituvchi qo'shish", "add_teacher"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 O'qituvchilar ro'yxati", "view_teachers_list"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Bosh sahifaga qaytish", "back_to_dashboard"),
		),
	)
}

// TeachersListKeyboard creates a keyboard showing all teachers.
func TeachersListKeyboard(teachers []database.User) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for _, teacher := range teachers {
		buttonText := fmt.Sprintf("🗑️ O'chirish: @%s", teacher.Username)
		if teacher.Username == "" {
			buttonText = fmt.Sprintf("🗑️ O'chirish: %s", teacher.PhoneNumber)
		}
		callbackData := fmt.Sprintf("demote_teacher_%d", teacher.ID)
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData),
		)
		keyboardRows = append(keyboardRows, row)
	}

	// Add back button
	backRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Orqaga", "manage_teachers"),
	)
	keyboardRows = append(keyboardRows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// --- Student Management Keyboards ---

// ManageStudentsKeyboard creates the main student management keyboard.
func ManageStudentsKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Talabalar ro'yxati", "view_students_list"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Bosh sahifaga qaytish", "back_to_dashboard"),
		),
	)
}

// StudentsListKeyboard creates a keyboard for the students list view.
func StudentsListKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Orqaga", "manage_students"),
		),
	)
}

// --- Category Management Keyboards ---

// ManageCategoriesKeyboard creates a dynamic keyboard with a list of categories and an add button.
func ManageCategoriesKeyboard(categories []database.Category, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	// Create a button for each category
	for _, category := range categories {
		callbackData := fmt.Sprintf("view_category_%d", category.ID)
		categoryButton := tgbotapi.NewInlineKeyboardButtonData("📁 "+category.Name, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(categoryButton)
		keyboardRows = append(keyboardRows, row)
	}

	// Add the static "Add New Category" button
	addCategoryButton := tgbotapi.NewInlineKeyboardButtonData("➕ Yangi kategoriya qo'shish", fmt.Sprintf("add_category_%d", subjectID))
	addRow := tgbotapi.NewInlineKeyboardRow(addCategoryButton)
	keyboardRows = append(keyboardRows, addRow)

	// Add back button
	backButton := tgbotapi.NewInlineKeyboardButtonData("⬅️ Fanga qaytish", fmt.Sprintf("view_subject_%d", subjectID))
	backRow := tgbotapi.NewInlineKeyboardRow(backButton)
	keyboardRows = append(keyboardRows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// CategoryManagementKeyboard creates a keyboard with edit and delete options for a category.
func CategoryManagementKeyboard(categoryID uint, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	idStr := strconv.Itoa(int(categoryID))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Nomini o'zgartirish", "edit_category_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("🗑️ O'chirish", "delete_category_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Resurslarni boshqarish", "manage_resources_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyalarga qaytish", fmt.Sprintf("back_to_categories_%d", subjectID)),
		),
	)
}

// ConfirmDeleteCategoryKeyboard creates a confirmation keyboard for deleting a category.
func ConfirmDeleteCategoryKeyboard(categoryID uint, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Ha, o'chirish", fmt.Sprintf("confirm_delete_category_%d_%d", categoryID, subjectID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Bekor qilish", fmt.Sprintf("view_category_%d", categoryID)),
		),
	)
}

// --- Resource Management Keyboards ---

// CategoryResourcesKeyboard creates a clean keyboard for managing resources within a category.
// Note: Delete buttons are now attached to individual resource messages
func CategoryResourcesKeyboard(resources []database.Resource, categoryID uint, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 Havola qo'shish", fmt.Sprintf("add_resource_link_%d", categoryID)),
			tgbotapi.NewInlineKeyboardButtonData("📎 Fayl qo'shish", fmt.Sprintf("add_resource_file_%d", categoryID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyaga qaytish", fmt.Sprintf("view_category_%d", categoryID)),
		),
	)
}

// --- Student Browsing Keyboards ---

// StudentBrowseSubjectsKeyboard creates a keyboard for students to browse all subjects
func StudentBrowseSubjectsKeyboard(subjects []database.Subject) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for _, subject := range subjects {
		callbackData := fmt.Sprintf("student_view_subject_%d", subject.ID)
		subjectButton := tgbotapi.NewInlineKeyboardButtonData("📖 "+subject.Name, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(subjectButton)
		keyboardRows = append(keyboardRows, row)
	}

	// Add back to dashboard button
	backRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Bosh sahifaga qaytish", "back_to_dashboard"),
	)
	keyboardRows = append(keyboardRows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// StudentViewSubjectKeyboard shows subject details with subscription toggle
func StudentViewSubjectKeyboard(subjectID uint, isSubscribed bool) tgbotapi.InlineKeyboardMarkup {
	idStr := strconv.Itoa(int(subjectID))

	var subscribeButton tgbotapi.InlineKeyboardButton
	if isSubscribed {
		subscribeButton = tgbotapi.NewInlineKeyboardButtonData("🔕 Obunani bekor qilish", "student_unsubscribe_"+idStr)
	} else {
		subscribeButton = tgbotapi.NewInlineKeyboardButtonData("🔔 Obuna bo'lish", "student_subscribe_"+idStr)
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(subscribeButton),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📁 Kategoriyalarni ko'rish", "student_view_categories_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Fanlarga qaytish", "student_browse_subjects"),
		),
	)
}

// StudentViewCategoriesKeyboard shows all categories in a subject for students
func StudentViewCategoriesKeyboard(categories []database.Category, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for _, category := range categories {
		callbackData := fmt.Sprintf("student_view_category_%d", category.ID)
		categoryButton := tgbotapi.NewInlineKeyboardButtonData("📁 "+category.Name, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(categoryButton)
		keyboardRows = append(keyboardRows, row)
	}

	// Add back button
	backButton := tgbotapi.NewInlineKeyboardButtonData("⬅️ Fanga qaytish", fmt.Sprintf("student_view_subject_%d", subjectID))
	backRow := tgbotapi.NewInlineKeyboardRow(backButton)
	keyboardRows = append(keyboardRows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}

// StudentViewResourcesKeyboard shows all resources in a category for students (read-only)
func StudentViewResourcesKeyboard(categoryID uint, subjectID uint) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kategoriyalarga qaytish", fmt.Sprintf("student_view_categories_%d", subjectID)),
		),
	)
}

// MySubscriptionsKeyboard shows subscribed subjects with option to manage
func MySubscriptionsKeyboard(subjects []database.Subject) tgbotapi.InlineKeyboardMarkup {
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for _, subject := range subjects {
		callbackData := fmt.Sprintf("student_view_subject_%d", subject.ID)
		subjectButton := tgbotapi.NewInlineKeyboardButtonData("✅ "+subject.Name, callbackData)
		row := tgbotapi.NewInlineKeyboardRow(subjectButton)
		keyboardRows = append(keyboardRows, row)
	}

	// Add back to dashboard button
	backRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Bosh sahifaga qaytish", "back_to_dashboard"),
	)
	keyboardRows = append(keyboardRows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
}
