# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Telegram bot built in Go that provides an educational platform with role-based access control. The bot manages subjects, categories, and educational resources with three user roles: Admin, Teacher, and Student.

## Technology Stack

- **Language**: Go 1.24.5
- **Bot Framework**: go-telegram-bot-api/v5
- **Web Framework**: Gin (for webhook handling)
- **Database**: SQLite with GORM ORM
- **Phone Validation**: nyaruka/phonenumbers

## Running the Application

### Prerequisites
1. Create a `.env` file with required environment variables:
   ```
   BOT_TOKEN=your_telegram_bot_token
   WEBHOOK_URL=your_webhook_url
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
- Set up webhook at `WEBHOOK_URL/{BOT_TOKEN}`
- Start Gin server on port 8080

## Architecture

### Package Structure
```
cmd/main.go              - Entry point, initializes bot and webhook
internal/
  bot/
    handlers.go          - Main update router and business logic
    keyboards.go         - Telegram keyboard UI builders
  database/
    models.go           - GORM models and DB initialization
  roles/
    roles.go            - Role constants (admin, teacher, student)
  utils/
    validation.go       - Phone number validation utilities
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
- `Resource`: Educational materials (video, link, pdf, doc) with FileID for Telegram files
- `Subscription`: Many-to-many relationship between Users and Subjects

### Request Flow
1. Telegram sends webhook POST to `/{BOT_TOKEN}`
2. `Handler.HandleUpdate()` parses JSON and routes to:
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
1. Set up ngrok or similar for webhook URL: `ngrok http 8080`
2. Update `WEBHOOK_URL` in `.env` to ngrok URL
3. Start the bot: `go run cmd/main.go`
4. Test with Telegram client using `/start` command

### Known Issues
- Line 313 in `handlers.go` has syntax error: `if len(allSubjects) == ==` (double equals)
- Teacher role functionality is partially implemented
- Category and Resource management not yet implemented
- Deletion operations don't cascade properly (missing transaction handling)

### Extending the Bot
- **Adding new states**: Define constant in `handlers.go`, add case in `handleStatefulMessage()`
- **Adding new callbacks**: Add string matching logic in `handleCallbackQuery()`
- **Adding new keyboards**: Create builder function in `keyboards.go` returning `InlineKeyboardMarkup`
- **Database changes**: Update models in `database/models.go` and restart (auto-migration runs on startup)