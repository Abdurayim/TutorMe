package database

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	gorm.Model
	TelegramID  int64 `gorm:"uniqueIndex"`
	Username    string
	FirstName   string
	LastName    string
	PhoneNumber string
	Role        string // admin, teacher, student
}

// Subject represents a course subject
type Subject struct {
	gorm.Model
	Name string `gorm:"unique"`
}

// Category represents a category within a subject
type Category struct {
	gorm.Model
	Name      string
	SubjectID uint
	Subject   Subject
}

// Resource represents an educational material
type Resource struct {
	gorm.Model
	Type            string // video, link, pdf, doc
	URL             string // Original URL (for links) or source URL (for downloaded videos)
	FileID          string // For files uploaded directly to Telegram
	CategoryID      uint
	Category        Category
	ProcessingStatus string // "pending", "processing", "completed", "failed", "" (for non-video resources)
	Progress        int    // 0-100 percentage for video processing
	Title           string // Video title from platform
	ErrorMessage    string // Error details if processing failed
}

// Subscription links a user to a subject
type Subscription struct {
	gorm.Model
	UserID    uint `gorm:"uniqueIndex:idx_user_subject"`
	SubjectID uint `gorm:"uniqueIndex:idx_user_subject"`
	User      User
	Subject   Subject
}

// InitDB initializes the SQLite database and runs auto-migrations.
func InitDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("bot.db"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate the schema
	err = db.AutoMigrate(&User{}, &Subject{}, &Category{}, &Resource{}, &Subscription{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
