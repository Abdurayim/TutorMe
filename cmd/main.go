package main

import (
	"log"
	"os"
	"tg-bot/internal/bot"
	"tg-bot/internal/database"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	// --- THIS IS THE FIX ---
	// Load .env file. This should be the first thing in your main function.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}
	// --- END OF FIX ---

	// Read configuration from environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable not set")
	}

	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("WEBHOOK_URL environment variable not set")
	}

	// Initialize the database
	db, err := database.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Initialize the bot using the variables
	botAPI, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	botAPI.Debug = true
	log.Printf("Authorized on account %s", botAPI.Self.UserName)

	// Set up the webhook using the variables
	wh, _ := tgbotapi.NewWebhook(webhookURL + "/" + botAPI.Token)
	_, err = botAPI.Request(wh)
	if err != nil {
		log.Fatal(err)
	}

	info, err := botAPI.GetWebhookInfo()
	if err != nil {
		log.Fatal(err)
	}
	if info.URL == "" {
		log.Fatal("Webhook not set")
	}

	// Initialize the bot handler
	handler := bot.NewHandler(botAPI, db)

	// Set up the Gin router
	router := gin.Default()
	router.POST("/"+botAPI.Token, handler.HandleUpdate)

	// Run the server
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
