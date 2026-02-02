package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// lastPathSegment returns the last segment of a URL path
func lastPathSegment(url string) string {
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}

// pow calculates base^exp for integers
func pow(base, exp int) int {
	return int(math.Pow(float64(base), float64(exp)))
}

// sendTelegramAlert sends an alert message to Telegram with improved error handling
func sendTelegramAlert(message, chatID string, logger *Logger) bool {
	if chatID == "" {
		logger.Debug("Telegram alert skipped: no chat ID provided")
		return false
	}

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal Telegram payload: %v", err)
		return false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(config.TelegramAPI, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("Failed to send Telegram alert: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Read response body for debugging
	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		logger.Error("Telegram API error (HTTP %d): %s", resp.StatusCode, string(body))
		return false
	}

	logger.Debug("Telegram alert sent successfully to chat ID: %s", chatID)
	return true
}

// formatFailureAlert formats a failure message for Telegram
func formatFailureAlert(email, eventURL string, attempt int, reason string) string {
	event := truncateString(lastPathSegment(eventURL), 20)
	return fmt.Sprintf(
		"❌ <b>Registration Failed</b>\n"+
		"━━━━━━━━━━━━━━━━━━━━\n"+
		"📧 Email: <code>%s</code>\n"+
		"🎫 Event: <code>%s...</code>\n"+
		"🔄 Attempt: %d/%d\n"+
		"❗️ Reason: %s\n"+
		"⏰ Time: %s",
		email, event, attempt, config.RegistrationRetry, reason,
		time.Now().Format("2006-01-02 15:04:05"),
	)
}

// testTelegramConnection tests the Telegram bot connection
func testTelegramConnection(chatID string, logger *Logger) bool {
	if chatID == "" {
		logger.Warning("No Telegram chat ID provided - notifications disabled")
		return false
	}

	logger.Info("Testing Telegram connection...")
	
	testMessage := fmt.Sprintf(
		"🤖 <b>Bot Connection Test</b>\n"+
		"━━━━━━━━━━━━━━━━━━━━\n"+
		"✅ Bot is online and ready\n"+
		"🆔 Chat ID: <code>%s</code>\n"+
		"⏰ Time: %s",
		chatID,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if sendTelegramAlert(testMessage, chatID, logger) {
		logger.Info("✓ Telegram connection successful")
		return true
	}
	
	logger.Error("✗ Telegram connection failed")
	logger.Error("  Check:")
	logger.Error("  1. Bot token is correct")
	logger.Error("  2. Chat ID is correct")
	logger.Error("  3. You've started a conversation with the bot")
	logger.Error("  4. Network connectivity")
	return false
}
