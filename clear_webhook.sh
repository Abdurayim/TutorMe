#!/bin/bash
# Script to clear webhook and reset it
# Run this when you restart ngrok with a new URL

if [ -z "$BOT_TOKEN" ]; then
    echo "Error: BOT_TOKEN not set"
    echo "Run: export BOT_TOKEN=your_token_here"
    exit 1
fi

if [ -z "$WEBHOOK_URL" ]; then
    echo "Error: WEBHOOK_URL not set"
    echo "Run: export WEBHOOK_URL=your_ngrok_url"
    exit 1
fi

echo "Clearing webhook..."
curl -X POST "https://api.telegram.org/bot${BOT_TOKEN}/deleteWebhook"

echo -e "\n\nSetting new webhook..."
curl -X POST "https://api.telegram.org/bot${BOT_TOKEN}/setWebhook?url=${WEBHOOK_URL}/${BOT_TOKEN}"

echo -e "\n\nChecking webhook status..."
curl -X POST "https://api.telegram.org/bot${BOT_TOKEN}/getWebhookInfo"

echo -e "\n\nDone!"
