# Local Bot API Server Setup Guide

This guide will help you set up Telegram's Local Bot API Server to handle large video files (up to 2GB).

## Why Local Bot API Server?

- **Standard Bot API**: 50 MB file size limit
- **Local Bot API**: 2000 MB (2 GB) file size limit
- **Faster**: Files don't route through Telegram's servers
- **More control**: Direct file access

## Prerequisites

- Go 1.19+ (you already have Go 1.24.5 ✓)
- Git
- CMake 3.0.2+
- C++ compiler (gcc/g++ or clang)
- OpenSSL development libraries
- zlib development libraries

## Installation Steps

### 1. Install System Dependencies

**macOS (using Homebrew):**
```bash
brew install cmake openssl gperf zlib
```

**Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install -y git cmake g++ make openssl libssl-dev zlib1g-dev gperf
```

**CentOS/RHEL:**
```bash
sudo yum install -y git cmake gcc-c++ make openssl openssl-devel zlib-devel gperf
```

### 2. Download and Build Telegram Bot API Server

```bash
# Create a directory for the bot API server
mkdir -p ~/telegram-bot-api
cd ~/telegram-bot-api

# Clone the official Telegram Bot API repository
git clone --recursive https://github.com/tdlib/telegram-bot-api.git
cd telegram-bot-api

# Create build directory
mkdir build
cd build

# Configure build
cmake -DCMAKE_BUILD_TYPE=Release ..

# Build (this may take 10-30 minutes)
cmake --build . --target install

# The binary will be in the build directory
# File: telegram-bot-api
```

### 3. Download Required Binaries for Video Processing

**yt-dlp (for downloading videos):**

```bash
# macOS/Linux - download standalone binary
cd ~/telegram-bot-api
curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o yt-dlp
chmod +x yt-dlp

# Verify installation
./yt-dlp --version
```

**ffmpeg (for video transcoding):**

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt-get install -y ffmpeg

# CentOS/RHEL
sudo yum install -y ffmpeg

# Verify installation
ffmpeg -version
```

### 4. Get Your Bot API ID and Hash

You need API credentials from Telegram:

1. Go to https://my.telegram.org/auth
2. Log in with your phone number
3. Go to "API development tools"
4. Create a new application (if you haven't):
   - App title: "Educational Bot"
   - Short name: "edubot"
   - Platform: Other
5. You'll receive:
   - `api_id` (numeric, like 12345678)
   - `api_hash` (string, like "abcdef1234567890abcdef1234567890")

**⚠️ IMPORTANT**: These are different from your `BOT_TOKEN`!

### 5. Run Local Bot API Server

```bash
cd ~/telegram-bot-api/build

# Run the server
./telegram-bot-api --api-id=YOUR_API_ID --api-hash=YOUR_API_HASH --local
```

**The server will:**
- Start on `http://localhost:8081` by default
- Create a working directory for file storage
- Show logs of all requests

**Example output:**
```
[2025-11-09 10:00:00] [INFO] Telegram Bot API server started
[2025-11-09 10:00:00] [INFO] Local mode enabled
[2025-11-09 10:00:00] [INFO] Listening on http://localhost:8081
```

### 6. Update Your Bot Configuration

Add to your `.env` file:

```bash
# Existing variables
BOT_TOKEN=your_telegram_bot_token
ADMIN_PHONE_NUMBER=+1234567890

# New variables for Local Bot API
USE_LOCAL_API=true
LOCAL_API_URL=http://localhost:8081
API_ID=your_api_id
API_HASH=your_api_hash

# Video processing paths (adjust based on your installation)
YT_DLP_PATH=/Users/yourusername/telegram-bot-api/yt-dlp
FFMPEG_PATH=/usr/local/bin/ffmpeg

# Video processing settings
MAX_VIDEO_SIZE_MB=1500
TARGET_VIDEO_QUALITY=720p
ENABLE_TRANSCODING=true
```

### 7. Run Both Servers

You'll need to run two processes:

**Terminal 1 - Local Bot API Server:**
```bash
cd ~/telegram-bot-api/build
./telegram-bot-api --api-id=YOUR_API_ID --api-hash=YOUR_API_HASH --local
```

**Terminal 2 - Your Go Bot:**
```bash
cd /Users/abdurayim/Desktop/PROJECTS/tutor
go run cmd/main.go
```

### 8. Production Setup (Optional)

For production, run both as system services:

**Create systemd service for Local Bot API (Linux):**

```bash
sudo nano /etc/systemd/system/telegram-bot-api.service
```

```ini
[Unit]
Description=Telegram Bot API Server
After=network.target

[Service]
Type=simple
User=yourusername
WorkingDirectory=/home/yourusername/telegram-bot-api/build
ExecStart=/home/yourusername/telegram-bot-api/build/telegram-bot-api --api-id=YOUR_API_ID --api-hash=YOUR_API_HASH --local
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable telegram-bot-api
sudo systemctl start telegram-bot-api
```

**For macOS, create a LaunchAgent** (similar process).

## Verification

Test that everything works:

```bash
# Test Local Bot API is running
curl http://localhost:8081/botYOUR_BOT_TOKEN/getMe

# Should return your bot's information
```

## File Storage

The Local Bot API server stores files in:
```
~/telegram-bot-api/build/telegram-bot-api-data/
```

Make sure you have enough disk space for video files!

## Troubleshooting

**Problem**: Build fails with OpenSSL errors
```bash
# macOS - point to Homebrew OpenSSL
export OPENSSL_ROOT_DIR=/usr/local/opt/openssl
cmake -DCMAKE_BUILD_TYPE=Release -DOPENSSL_ROOT_DIR=$OPENSSL_ROOT_DIR ..
```

**Problem**: Server starts but can't connect
- Check firewall settings
- Verify the server is on localhost:8081
- Check logs for errors

**Problem**: Videos fail to upload
- Check disk space
- Verify file permissions in working directory
- Check server logs for specific errors

## Next Steps

Once the Local Bot API server is running, the Go bot will automatically:
1. Detect video URLs
2. Download using yt-dlp
3. Transcode if needed using ffmpeg
4. Upload via Local Bot API (no 50MB limit!)
5. Show progress to teachers

---

**Ready to proceed?** Let me know when you have the Local Bot API server running!
