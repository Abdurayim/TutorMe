#!/bin/bash
# Script to clear webhook
# This bot now uses polling instead of webhooks
# Run this script if you previously used webhooks and want to switch to polling

if [ -z "$BOT_TOKEN" ]; then
    echo "Error: BOT_TOKEN not set"
    echo "Run: export BOT_TOKEN=your_token_here"
    exit 1
fi

echo "Clearing webhook..."
curl -X POST "https://api.telegram.org/bot${BOT_TOKEN}/deleteWebhook"

echo -e "\n\nChecking webhook status..."
curl -X POST "https://api.telegram.org/bot${BOT_TOKEN}/getWebhookInfo"

echo -e "\n\nDone! The bot is now ready to use polling."
echo "Note: This bot no longer uses webhooks. It uses polling instead."
