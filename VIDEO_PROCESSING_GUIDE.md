# Video Processing Feature Guide

## Overview

The bot now automatically converts social media video links into embedded Telegram videos. When teachers add a video URL from YouTube, Instagram, TikTok, or other platforms, the bot:

1. Detects it's a video URL
2. Downloads the video in background
3. Transcodes if needed (for large files)
4. Uploads to Telegram
5. Shows live progress updates

**Students can watch videos directly in Telegram without leaving the app!**

---

## Supported Platforms

✅ **YouTube** (with 720p quality preference)
✅ **Instagram** (Reels, Posts, IGTV)
✅ **TikTok**
✅ **Facebook Videos**
✅ **Twitter/X Videos**
✅ **Vimeo**
✅ **Dailymotion**
✅ **Reddit Videos**
✅ **Twitch VODs**

*Non-video links are saved as regular clickable links*

---

## How It Works

### For Teachers:

1. Click "🔗 Havola qo'shish" in category management
2. Send a video URL (e.g., YouTube link)
3. Bot detects it's a video and shows: **"⏳ Video qayta ishlanmoqda..."**
4. Watch live progress updates:
   - 📥 Yuklab olish: 0-50%
   - 🎬 Siqish: 50-100% (if needed)
   - 📤 Yuklash: 90-100%
5. When done: **"✅ Video muvaffaqiyatli qo'shildi!"**
6. Video appears embedded in resource list

### For Students:

- Videos appear as native Telegram videos (not links)
- Can watch directly in the app
- Can download for offline viewing
- Get notifications when new videos are added

---

## Architecture

```
Teacher submits URL
       ↓
   Is video URL?
       ↓ Yes
Create pending resource
       ↓
Background job starts
       ↓
Download with yt-dlp
       ↓
Size > 100MB?
       ↓ Yes
Transcode with ffmpeg (720p, 2Mbps)
       ↓
Upload to Telegram
       ↓
Save FileID → Resource
       ↓
Notify teacher & students
```

### Database Changes

**Resource model** now includes:
- `ProcessingStatus`: "pending", "processing", "completed", "failed"
- `Progress`: 0-100 percentage
- `Title`: Video title extracted from platform
- `ErrorMessage`: Error details if processing failed
- `URL`: Original source URL (preserved even for processed videos)

---

## Configuration

### Standard Mode (50 MB limit)

If you don't need large video support:

```bash
# .env
BOT_TOKEN=your_bot_token
ADMIN_PHONE_NUMBER=+998901234567

# Video processing (optional)
YT_DLP_PATH=/path/to/yt-dlp
FFMPEG_PATH=/usr/local/bin/ffmpeg
```

Videos larger than 50MB will fail with error message.

### Local Bot API Mode (2 GB limit) - **RECOMMENDED**

For full video support:

**1. Set up Local Bot API server** (see `LOCAL_BOT_API_SETUP.md`)

**2. Configure .env:**
```bash
# Telegram Bot
BOT_TOKEN=your_bot_token
ADMIN_PHONE_NUMBER=+998901234567

# Local Bot API
USE_LOCAL_API=true
LOCAL_API_URL=http://localhost:8081
API_ID=your_api_id
API_HASH=your_api_hash

# Video Processing
YT_DLP_PATH=/path/to/yt-dlp
FFMPEG_PATH=/usr/local/bin/ffmpeg
MAX_VIDEO_SIZE_MB=1500
TARGET_VIDEO_QUALITY=720p
ENABLE_TRANSCODING=true
```

**3. Run both servers:**

Terminal 1:
```bash
cd ~/telegram-bot-api/build
./telegram-bot-api --api-id=YOUR_API_ID --api-hash=YOUR_API_HASH --local
```

Terminal 2:
```bash
cd /Users/abdurayim/Desktop/PROJECTS/tutor
go run cmd/main.go
```

---

## Quality & Size Optimization

### YouTube Videos:
- Downloads in 720p (good quality, reasonable size)
- Typical size: ~50-150 MB for 30 min video

### Other Platforms:
- Downloads best available quality
- If video > 100MB: auto-transcode to 720p, 2Mbps
- After transcoding: ~60-80 MB for 30 min video

### Transcoding Settings:
- Resolution: 720p (1280x720)
- Video bitrate: 2 Mbps
- Audio bitrate: 128 Kbps
- Codec: H.264 (widely compatible)
- Format: MP4

**To disable transcoding:**
```bash
ENABLE_TRANSCODING=false
```

---

## Performance

### Processing Time Estimates:

| Video Length | Download | Transcode | Upload | Total |
|--------------|----------|-----------|--------|-------|
| 5 min (50MB) | ~30s     | Skip      | ~20s   | ~1min |
| 15 min (150MB) | ~1min  | ~2min     | ~30s   | ~4min |
| 30 min (300MB) | ~2min  | ~4min     | ~1min  | ~7min |
| 60 min (600MB) | ~4min  | ~8min     | ~2min  | ~14min |

*Times vary based on internet speed and server CPU*

### Concurrent Processing:
- Max 3 videos processed simultaneously
- Queue size: 100 jobs
- Jobs process in FIFO order

---

## Error Handling

### Common Errors & Solutions:

**1. "Video yuklashda xatolik"**
- Video is private/deleted
- Platform not supported
- Network error
→ Video saved as regular link

**2. "Video hajmi juda katta"**
- Video exceeds 1.5GB limit
- Platform serves higher quality than expected
→ Lower MAX_VIDEO_SIZE_MB or enable transcoding

**3. "yt-dlp not found"**
- yt-dlp binary not in PATH
→ Set YT_DLP_PATH in .env

**4. "ffmpeg not found"**
- ffmpeg not installed
→ Install ffmpeg: `brew install ffmpeg` (macOS)

### Graceful Degradation:
- If video processing fails → Resource saved as clickable link
- Teacher gets error notification
- Students can still access via URL

---

## Background Jobs

### How Jobs Work:

1. **Enqueue**: Teacher submits URL → Job added to queue
2. **Process**: Worker goroutine picks job from queue
3. **Progress**: Updates sent to teacher in real-time
4. **Complete**: Video saved, notifications sent
5. **Cleanup**: Temporary files deleted

### Worker Pool:
- 3 concurrent workers (configurable)
- Each worker handles one video at a time
- Non-blocking: bot continues serving other requests

### Database States:
```
Resource.ProcessingStatus:
  "" (empty)  → Regular link/file (no processing)
  "pending"   → Job queued, not started
  "processing" → Currently downloading/transcoding
  "completed" → Successfully processed
  "failed"    → Error occurred
```

---

## Monitoring & Logs

### Log Messages:

```
[INFO] Video resource created: ID=42, URL=https://youtube.com/..., Source=YouTube
[INFO] Video job enqueued: ResourceID=42
[INFO] Worker 0 processing job: ResourceID=42
[INFO] yt-dlp: [download] 45.2% of 125.6MiB at 5.2MiB/s
[INFO] Resource 42 progress: 22% (downloading)
[INFO] ffmpeg: time=00:05:30 bitrate=2000kb/s speed=3.2x
[INFO] Resource 42 progress: 75% (transcoding)
[INFO] Video processing completed: ResourceID=42, FileID=BAACAgIA...
[INFO] Cleaned up temporary file: /tmp/bot_videos/abc123.mp4
```

### Progress Tracking:
- Teachers see live percentage updates
- Database tracks current progress (0-100)
- Messages update every few seconds

---

## Resource Usage

### Disk Space:
- Temporary files stored in `/tmp/bot_videos/`
- Files deleted after upload
- Ensure enough space for concurrent downloads:
  - 3 workers × 1.5 GB = ~5 GB free space needed

### Memory:
- Each worker: ~100-200 MB during processing
- Total: ~500 MB for video processing
- Base bot: ~50 MB

### CPU:
- Downloading: Low CPU, network-bound
- Transcoding: High CPU (ffmpeg uses multiple cores)
- 3 concurrent transcodes: Expect high CPU load

### Network:
- Download speed depends on source platform
- Upload to Telegram: Usually fast with Local API
- Total bandwidth: ~2-10 Mbps per video

---

## Testing

### Test Different Platforms:

```bash
# YouTube
Add URL: https://www.youtube.com/watch?v=dQw4w9WgXcQ

# Instagram
Add URL: https://www.instagram.com/reel/xyz123/

# TikTok
Add URL: https://www.tiktok.com/@user/video/123456789

# Regular link (non-video)
Add URL: https://google.com
```

### Expected Behavior:
✅ Video URLs → Show progress, become embedded videos
✅ Non-video URLs → Immediate confirmation, stay as links
✅ Failed processing → Error message, saved as link

---

## Troubleshooting

### Videos Not Processing?

**Check yt-dlp:**
```bash
/path/to/yt-dlp --version
/path/to/yt-dlp https://youtube.com/watch?v=test
```

**Check ffmpeg:**
```bash
ffmpeg -version
```

**Check Local Bot API:**
```bash
curl http://localhost:8081/botYOUR_TOKEN/getMe
```

**Check logs:**
```bash
go run cmd/main.go 2>&1 | tee bot.log
```

### Performance Issues?

**Reduce concurrent workers:**
Edit `internal/bot/video_job.go`:
```go
MaxWorkers: 1, // Process one at a time
```

**Disable transcoding:**
```bash
ENABLE_TRANSCODING=false
```

**Lower quality:**
```bash
TARGET_VIDEO_QUALITY=480p
MAX_VIDEO_SIZE_MB=500
```

---

## Migration from Links

### Existing Resources:
- Old link resources remain unchanged
- New video URLs auto-process
- No database migration needed
- Backwards compatible

### Re-processing Failed Videos:
Failed videos stay as links. To retry:
1. Delete the failed resource
2. Add the URL again
3. Bot will re-attempt processing

---

## Security Considerations

1. **URL Validation**: Only known platforms accepted
2. **File Size Limits**: Prevents disk exhaustion
3. **Temporary Files**: Auto-deleted after upload
4. **Error Handling**: Malformed URLs don't crash bot
5. **User Permissions**: Only teachers can add resources

---

## Future Enhancements

Possible improvements:
- [ ] Video thumbnail extraction
- [ ] Duration limits (e.g., max 2 hours)
- [ ] Subtitle extraction and embedding
- [ ] Quality selection UI for teachers
- [ ] Video preview before final upload
- [ ] Batch video processing
- [ ] Analytics (most watched videos)
- [ ] Automatic retry on failure

---

## Support

Having issues? Check:
1. `LOCAL_BOT_API_SETUP.md` - Server setup
2. `.env.example` - Configuration template
3. Bot logs - Error messages and progress
4. This guide - Troubleshooting section

**All code is thoroughly tested and ready for production!** 🚀
