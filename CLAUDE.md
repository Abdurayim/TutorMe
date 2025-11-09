# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Telegram bot built in Go that provides an educational platform with role-based access control. The bot manages subjects, categories, and educational resources with three user roles: Admin, Teacher, and Student.

**Key Feature**: Automatic video processing - when teachers share YouTube, Instagram, TikTok, or other video platform links, the bot automatically downloads, optimizes, and embeds videos directly in Telegram so students can watch without leaving the app.

## Technology Stack

- **Language**: Go 1.24.5
- **Bot Framework**: go-telegram-bot-api/v5
- **Update Method**: Long Polling
- **Database**: SQLite with GORM ORM
- **Phone Validation**: nyaruka/phonenumbers

## Running the Application

### Prerequisites
1. Create a `.env` file with required environment variables:
   ```
   BOT_TOKEN=your_telegram_bot_token
   ADMIN_PHONE_NUMBER=+1234567890
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

### Run the bot
```bash
go run cmd/main.go
```

The bot will:
- Load environment variables from `.env`
- Initialize SQLite database (`bot.db`)
- Delete any existing webhook configuration
- Start polling for updates with 60-second timeout

## Architecture

### Package Structure
```
cmd/main.go              - Entry point, initializes bot and polling
internal/
  bot/
    handlers.go          - Main update router and business logic
    keyboards.go         - Telegram keyboard UI builders
    video_job.go         - Background video processing with goroutines
  database/
    models.go           - GORM models and DB initialization
  roles/
    roles.go            - Role constants (admin, teacher, student)
  utils/
    validation.go       - Phone number validation utilities
    video.go            - Video URL detection for social platforms
    video_processor.go  - Video download/transcode with yt-dlp/ffmpeg
```

### State Management
The bot uses an **in-memory state machine** for multi-step interactions:
- `Handler.userStates`: Maps user IDs to current state strings
- `Handler.stateContext`: Stores additional context (e.g., subject ID being edited)
- States are cleared after completion using defer statements

Current states:
- `StateIdle`: Default state
- `StateAwaitingSubjectName`: Waiting for new subject name
- `StateAwaitingSubjectEdit`: Waiting for updated subject name

### Database Models
All models use GORM conventions:
- `User`: Stores Telegram users with role, phone number, and Telegram ID
- `Subject`: Educational subjects (e.g., Math, Physics)
- `Category`: Subcategories within subjects
- `Resource`: Educational materials with video processing support:
  - `Type`: "video", "link", "document"
  - `URL`: Original source URL (preserved for videos)
  - `FileID`: Telegram file identifier
  - `ProcessingStatus`: "", "pending", "processing", "completed", "failed"
  - `Progress`: 0-100 percentage during processing
  - `Title`: Video title from platform
  - `ErrorMessage`: Processing error details
- `Subscription`: Many-to-many relationship between Users and Subjects

### Request Flow
1. Bot continuously polls Telegram servers for new updates (60-second timeout)
2. `Handler.HandleUpdate()` receives updates and routes to:
   - `handleMessage()` for text messages and commands
   - `handleCallbackQuery()` for inline button presses
3. Stateful messages are routed through `handleStatefulMessage()`
4. UI updates use message editing for callback queries, new messages for commands

### Role-Based Access
- **Admin**: Set by matching phone number with `ADMIN_PHONE_NUMBER` env var
  - Can manage all subjects, teachers, and students
  - Gets admin dashboard on `/start`
- **Teacher**: Can manage their assigned subjects (partial implementation)
- **Student**: Default role, can browse and subscribe to subjects

### Callback Data Patterns
Inline keyboard buttons use prefixed callback data:
- `manage_subjects`: Show subject management UI
- `view_subject_{id}`: View specific subject details
- `edit_subject_{id}`: Enter edit mode for subject name
- `delete_subject_{id}`: Show delete confirmation
- `confirm_delete_subject_{id}`: Execute deletion
- `subscribe_subject_{id}` / `unsubscribe_subject_{id}`: Toggle subscription
- `browse_subjects`: Show subject browse UI for students

## Development Notes

### Phone Number Handling
- All phone numbers are validated and stored in E.164 format (+country_code...)
- Telegram contact sharing automatically provides formatted numbers
- Admin phone must match exactly in E.164 format

### Testing the Bot
1. Make sure `BOT_TOKEN` is set in `.env` file
2. Start the bot: `go run cmd/main.go`
3. Test with Telegram client using `/start` command
4. No need for ngrok or public URL - polling works from localhost

### Video Processing System

The bot includes a sophisticated video processing pipeline:

**Supported Platforms**: YouTube, Instagram, TikTok, Facebook, Twitter/X, Vimeo, Dailymotion, Reddit, Twitch

**Processing Flow**:
1. Teacher submits video URL
2. Bot detects platform using regex patterns
3. Creates pending resource in database
4. Enqueues background job (non-blocking)
5. Worker goroutine processes:
   - Downloads with yt-dlp (720p for YouTube)
   - Transcodes with ffmpeg if >100MB
   - Uploads to Telegram via Local Bot API
   - Updates database with FileID
6. Sends live progress updates (0-100%)
7. Notifies teacher and subscribed students

**Configuration**:
- Standard mode: 50 MB file limit
- Local Bot API mode: 2 GB file limit (recommended)
- 3 concurrent worker goroutines
- Automatic cleanup of temporary files

See `VIDEO_PROCESSING_GUIDE.md` and `LOCAL_BOT_API_SETUP.md` for details.

### Known Issues
- Teacher role functionality is partially implemented
- Deletion operations don't cascade properly (missing transaction handling)

### Extending the Bot
- **Adding new states**: Define constant in `handlers.go`, add case in `handleStatefulMessage()`
- **Adding new callbacks**: Add string matching logic in `handleCallbackQuery()`
- **Adding new keyboards**: Create builder function in `keyboards.go` returning `InlineKeyboardMarkup`
- **Database changes**: Update models in `database/models.go` and restart (auto-migration runs on startup)