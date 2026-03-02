# Troubleshooting

## Downloads Marked as Failed

**Symptom**: SABnzbd completes successfully but Zarr shows "failed"

**Cause**: Path mismatch between containers

**Solution**: Ensure both containers share the same volume mount for downloads:
```yaml
volumes:
  - ./data/usenet:/data/usenet  # Both containers
```

## "No video files found" Error

**Check**:
1. SABnzbd completed extraction successfully
2. Files have supported extensions (.mkv, .mp4, .avi, .m4v, .ts)
3. Download path is accessible to Zarr container

## SABnzbd Connection Issues

**Symptom**: Zarr can't connect to SABnzbd, or SABnzbd logs show "Refused connection"

**Solution**:
1. Check SABnzbd Settings > General > Security > Host Whitelist
2. Add: `sabnzbd, localhost, 127.0.0.1`
3. Restart SABnzbd
4. Verify URL in Zarr uses `http://sabnzbd:8080` (Docker service name, not localhost)

## Downloads Not Appearing in SABnzbd

**Symptom**: Zarr says "grabbed" but nothing appears in SABnzbd queue

**Solution**:
1. Check SABnzbd Settings > Categories
2. Ensure `mediaforge` category exists with `+Delete` enabled
3. Without this category, SABnzbd may reject downloads

## Search Returns No Results

**Common causes**:
1. Indexer API key invalid — test in Settings > Indexers
2. No releases match quality profile — check reject patterns
3. TVDB ID missing for series — try adding manually via library detail

## AI Assistant Not Responding

**Check**:
1. OpenRouter API key valid and has credits
2. Model selected in settings
3. Network connectivity to OpenRouter API
4. Browser console for error messages

## Performance Tips

- **Indexer Priority**: Set faster indexers to higher priority
- **Quality Profiles**: Be specific — fewer qualities = faster decisions
- **Episode Polling**: Default 30min is fine, reduce for faster new episode detection
- **Database Location**: SSD recommended for better query performance
- **Docker Resources**: Allocate at least 1GB RAM for smooth operation

## Security Notes

- **API Keys**: Stored encrypted in SQLite database
- **Network Exposure**: Zarr has no authentication — run behind reverse proxy or VPN
- **File Permissions**: Container runs as user:group 1000:1000 by default
- **HTTPS**: Use nginx/Caddy reverse proxy for HTTPS in production
