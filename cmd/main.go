package main

import (
	"log"
	"os"
	"tg-bot/internal/bot"
	"tg-bot/internal/database"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file. This should be the first thing in your main function.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Read configuration from environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable not set")
	}

	// Initialize the database
	db, err := database.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Initialize the bot using the variables
	var botAPI *tgbotapi.BotAPI

	// Check if Local Bot API server is configured
	useLocalAPI := os.Getenv("USE_LOCAL_API") == "true"
	localAPIURL := os.Getenv("LOCAL_API_URL")

	if useLocalAPI && localAPIURL != "" {
		// Use Local Bot API server for large file support (up to 2GB)
		botAPI, err = tgbotapi.NewBotAPIWithAPIEndpoint(botToken, localAPIURL+"/bot%s/%s")
		if err != nil {
			log.Panic(err)
		}
		log.Printf("Using Local Bot API server: %s", localAPIURL)
	} else {
		// Use standard Telegram Bot API (50 MB limit)
		botAPI, err = tgbotapi.NewBotAPI(botToken)
		if err != nil {
			log.Panic(err)
		}
		log.Printf("Using standard Telegram Bot API")
	}

	botAPI.Debug = true
	log.Printf("Authorized on account %s", botAPI.Self.UserName)

	// Remove webhook (in case it was previously set)
	_, err = botAPI.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("Failed to delete webhook: %v", err)
	}

	// Initialize the bot handler
	handler := bot.NewHandler(botAPI, db)

	// Set up polling with UpdateConfig
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := botAPI.GetUpdatesChan(u)

	log.Println("Bot started with polling. Waiting for updates...")

	// Process updates
	for update := range updates {
		handler.HandleUpdate(update)
	}
}
