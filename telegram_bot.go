package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TelegramBot manages the bot interface
type TelegramBot struct {
	token        string
	apiURL       string
	lastUpdateID int64
	logger       *Logger
	campaign     *CampaignManager
	userConfigs  map[int64]*UserConfig
	mu           sync.Mutex
}

// UserConfig stores per-user configuration
type UserConfig struct {
	FirstName    string
	LastName     string
	Organization string
	EmailsFile   string
	EventsFile   string
	ProxiesFile  string
	MaxWorkers   int
	State        string
	mu           sync.Mutex
}

// CampaignManager manages ongoing campaigns
type CampaignManager struct {
	running       bool
	orchestrator  *RegistrationOrchestrator
	results       []RegistrationResult
	startTime     time.Time
	mu            sync.Mutex
}

// TelegramUpdate represents a Telegram API update
type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID int64             `json:"message_id"`
	From      *TelegramUser     `json:"from"`
	Chat      *TelegramChat     `json:"chat"`
	Text      string            `json:"text"`
	Document  *TelegramDocument `json:"document"`
}

// TelegramDocument represents a file attachment
type TelegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// TelegramFile represents file info from getFile
type TelegramFile struct {
	Ok     bool `json:"ok"`
	Result struct {
		FileID   string `json:"file_id"`
		FilePath string `json:"file_path"`
	} `json:"result"`
}

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(token string, logger *Logger) *TelegramBot {
	return &TelegramBot{
		token:       token,
		apiURL:      fmt.Sprintf("https://api.telegram.org/bot%s", token),
		logger:      logger,
		campaign:    &CampaignManager{},
		userConfigs: make(map[int64]*UserConfig),
	}
}

// getUserConfig gets or creates user config
func (b *TelegramBot) getUserConfig(chatID int64) *UserConfig {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.userConfigs[chatID]; !exists {
		b.userConfigs[chatID] = &UserConfig{
			EmailsFile:  fmt.Sprintf("emails_%d.txt", chatID),
			EventsFile:  fmt.Sprintf("events_%d.txt", chatID),
			ProxiesFile: "proxies.txt",
			MaxWorkers:  20, // Default
			State:       "idle",
		}
	}
	return b.userConfigs[chatID]
}

// Start begins polling for Telegram updates
func (b *TelegramBot) Start() {
	b.logger.Info("🤖 Telegram Bot started - waiting for commands...")
	b.logger.Info("Send /help to see available commands")

	for {
		updates, err := b.getUpdates()
		if err != nil {
			b.logger.Error("Failed to get updates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.Message != nil {
				b.handleMessage(update.Message)
			}
			b.lastUpdateID = update.UpdateID + 1
		}

		time.Sleep(1 * time.Second)
	}
}

// getUpdates fetches new updates from Telegram
func (b *TelegramBot) getUpdates() ([]TelegramUpdate, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", b.apiURL, b.lastUpdateID)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Ok     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// handleMessage processes incoming messages
func (b *TelegramBot) handleMessage(msg *TelegramMessage) {
	chatID := msg.Chat.ID
	userConfig := b.getUserConfig(chatID)

	// Handle file uploads
	if msg.Document != nil {
		b.handleFileUpload(chatID, msg.Document, userConfig)
		return
	}

	text := strings.TrimSpace(msg.Text)
	b.logger.Info("📨 Message from @%s: %s", msg.From.Username, text)

	// Handle state-based input
	userConfig.mu.Lock()
	state := userConfig.State
	userConfig.mu.Unlock()

	if state != "idle" {
		b.handleStateInput(chatID, text, userConfig)
		return
	}

	// Handle commands
	switch {
	case text == "/start":
		b.sendWelcome(chatID)
	case text == "/help":
		b.sendHelp(chatID)
	case text == "/setup":
		b.handleSetup(chatID, userConfig)
	case strings.HasPrefix(text, "/workers"):
		b.handleWorkers(chatID, text, userConfig)
	case text == "/status":
		b.sendStatus(chatID)
	case text == "/register":
		b.handleRegister(chatID, userConfig)
	case text == "/stop":
		b.handleStop(chatID)
	case text == "/results":
		b.sendResults(chatID)
	case text == "/stats":
		b.sendStats(chatID)
	case text == "/config":
		b.handleConfig(chatID, userConfig)
	default:
		b.sendMessage(chatID, "❌ Unknown command. Send /help for available commands.")
	}
}

// handleWorkers sets max worker count
func (b *TelegramBot) handleWorkers(chatID int64, text string, userConfig *UserConfig) {
	parts := strings.Fields(text)

	if len(parts) == 1 {
		// Just /workers - show current
		userConfig.mu.Lock()
		current := userConfig.MaxWorkers
		userConfig.mu.Unlock()

		msg := fmt.Sprintf(
			"<b>⚙️ Worker Configuration</b>\n\n"+
				"Current: <b>%d workers</b>\n\n"+
				"<b>Usage:</b> /workers &lt;number&gt;\n"+
				"Example: <code>/workers 50</code>\n\n"+
				"<b>Recommended Settings:</b>\n"+
				"• Low: 10-20 workers (safe, slower)\n"+
				"• Medium: 20-50 workers (balanced)\n"+
				"• High: 50-100 workers (fast, requires good server)\n"+
				"• Extreme: 100+ workers (Hetzner dedicated only)\n\n"+
				"<b>Note:</b> Each worker runs 1 browser instance",
			current,
		)
		b.sendMessage(chatID, msg)
		return
	}

	if len(parts) != 2 {
		b.sendMessage(chatID, "❌ Usage: /workers &lt;number&gt;\nExample: <code>/workers 50</code>")
		return
	}

	workers, err := strconv.Atoi(parts[1])
	if err != nil || workers < 1 || workers > 200 {
		b.sendMessage(chatID, "❌ Please provide a number between 1 and 200")
		return
	}

	userConfig.mu.Lock()
	userConfig.MaxWorkers = workers
	userConfig.mu.Unlock()

	var recommendation string
	if workers <= 20 {
		recommendation = "✅ Safe for most servers"
	} else if workers <= 50 {
		recommendation = "⚠️ Requires decent server (2+ cores, 4GB+ RAM)"
	} else if workers <= 100 {
		recommendation = "⚠️ Requires powerful server (4+ cores, 8GB+ RAM)"
	} else {
		recommendation = "🔥 Extreme! Requires dedicated server (8+ cores, 16GB+ RAM)"
	}

	msg := fmt.Sprintf(
		"✅ <b>Workers updated!</b>\n\n"+
			"Max Workers: <b>%d</b>\n"+
			"Status: %s\n\n"+
			"<b>Performance Estimate:</b>\n"+
			"• ~%d simultaneous registrations\n"+
			"• ~%.1f GB RAM usage\n"+
			"• ~%d Mbps bandwidth needed",
		workers, recommendation, workers, float64(workers)*0.15, workers*2,
	)
	b.sendMessage(chatID, msg)
}

// handleFileUpload processes file uploads
func (b *TelegramBot) handleFileUpload(chatID int64, doc *TelegramDocument, userConfig *UserConfig) {
	fileName := strings.ToLower(doc.FileName)

	var targetFile string
	var fileType string
	if strings.Contains(fileName, "email") {
		targetFile = userConfig.EmailsFile
		fileType = "emails"
	} else if strings.Contains(fileName, "event") || strings.Contains(fileName, "list") {
		targetFile = userConfig.EventsFile
		fileType = "events"
	} else if strings.Contains(fileName, "proxy") || strings.Contains(fileName, "proxies") {
		targetFile = userConfig.ProxiesFile
		fileType = "proxies"
	} else {
		b.sendMessage(chatID, "❌ Unknown file type. Please name your file:\n• emails.txt\n• events.txt or list.txt\n• proxies.txt")
		return
	}

	if err := b.downloadFile(doc.FileID, targetFile); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Failed to download file: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ %s file uploaded successfully!\n\nFile: <code>%s</code>", strings.Title(fileType), targetFile))
	b.logger.Info("File uploaded for chat %d: %s -> %s", chatID, doc.FileName, targetFile)
}

// downloadFile downloads a file from Telegram
func (b *TelegramBot) downloadFile(fileID, targetPath string) error {
	fileInfoURL := fmt.Sprintf("%s/getFile?file_id=%s", b.apiURL, fileID)
	resp, err := http.Get(fileInfoURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var fileInfo TelegramFile
	if err := json.NewDecoder(resp.Body).Decode(&fileInfo); err != nil {
		return err
	}

	if !fileInfo.Ok {
		return fmt.Errorf("failed to get file info")
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, fileInfo.Result.FilePath)
	fileResp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer fileResp.Body.Close()

	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, fileResp.Body)
	return err
}

// handleSetup starts the setup wizard
func (b *TelegramBot) handleSetup(chatID int64, userConfig *UserConfig) {
	userConfig.mu.Lock()
	userConfig.State = "awaiting_firstname"
	userConfig.mu.Unlock()

	msg := "<b>⚙️ Setup Wizard</b>\n\n" +
		"Let's configure your registration campaign.\n\n" +
		"<b>Step 1/3:</b> Please enter your <b>First Name</b>:"
	b.sendMessage(chatID, msg)
}

// handleStateInput processes state-based user input
func (b *TelegramBot) handleStateInput(chatID int64, text string, userConfig *UserConfig) {
	userConfig.mu.Lock()
	defer userConfig.mu.Unlock()

	switch userConfig.State {
	case "awaiting_firstname":
		userConfig.FirstName = text
		userConfig.State = "awaiting_lastname"
		b.sendMessage(chatID, fmt.Sprintf("✅ First Name: <b>%s</b>\n\n<b>Step 2/3:</b> Please enter your <b>Last Name</b>:", text))

	case "awaiting_lastname":
		userConfig.LastName = text
		userConfig.State = "awaiting_org"
		b.sendMessage(chatID, fmt.Sprintf("✅ Last Name: <b>%s</b>\n\n<b>Step 3/3:</b> Please enter your <b>Organization Name</b>:", text))

	case "awaiting_org":
		userConfig.Organization = text
		userConfig.State = "idle"
		msg := fmt.Sprintf(
			"✅ Organization: <b>%s</b>\n\n"+
				"<b>🎉 Setup Complete!</b>\n\n"+
				"<b>Your Configuration:</b>\n"+
				"• First Name: <b>%s</b>\n"+
				"• Last Name: <b>%s</b>\n"+
				"• Organization: <b>%s</b>\n"+
				"• Workers: <b>%d</b>\n\n"+
				"<b>Next Steps:</b>\n"+
				"1. Upload files (emails.txt, events.txt)\n"+
				"2. Optional: /workers &lt;number&gt; to change workers\n"+
				"3. /register to start campaign",
			text, userConfig.FirstName, userConfig.LastName, userConfig.Organization, userConfig.MaxWorkers,
		)
		b.sendMessage(chatID, msg)
	}
}

// sendWelcome sends welcome message
func (b *TelegramBot) sendWelcome(chatID int64) {
	msg := fmt.Sprintf(
		"👋 <b>Welcome to EventBlast Bot!</b>\n\n"+
			"🎫 Automated event registration system\n"+
			"🤖 Your Chat ID: <code>%d</code>\n\n"+
			"<b>Quick Start:</b>\n"+
			"1. /setup - Configure your details\n"+
			"2. Upload files (emails.txt, events.txt)\n"+
			"3. /workers 20 - Set worker count (optional)\n"+
			"4. /register - Start campaign\n\n"+
			"Send /help for all commands",
		chatID,
	)
	b.sendMessage(chatID, msg)
}

// sendHelp sends help message
func (b *TelegramBot) sendHelp(chatID int64) {
	msg := "<b>📋 Available Commands</b>\n\n" +
		"<b>Setup:</b>\n" +
		"/setup - Configure first name, last name, organization\n" +
		"/workers [number] - Set max concurrent workers\n" +
		"/config - View current configuration\n\n" +
		"<b>Campaign Control:</b>\n" +
		"/register - Start registration campaign\n" +
		"/stop - Stop running campaign\n" +
		"/status - Check campaign status\n\n" +
		"<b>Information:</b>\n" +
		"/results - View campaign results\n" +
		"/stats - Show statistics\n\n" +
		"<b>File Upload:</b>\n" +
		"Send files named:\n" +
		"• <code>emails.txt</code> - Email list\n" +
		"• <code>events.txt</code> or <code>list.txt</code> - Event URLs\n" +
		"• <code>proxies.txt</code> - Proxies (optional)\n\n" +
		"<b>System:</b>\n" +
		"/help - Show this help\n" +
		"/start - Welcome message"
	b.sendMessage(chatID, msg)
}


// sendStatus sends campaign status
func (b *TelegramBot) sendStatus(chatID int64) {
	b.campaign.mu.Lock()
	defer b.campaign.mu.Unlock()

	if !b.campaign.running {
		b.sendMessage(chatID, "⏸️ No campaign running\n\nSend /register to start")
		return
	}

	elapsed := time.Since(b.campaign.startTime)
	successful := 0
	for _, r := range b.campaign.results {
		if r.Status == "SUCCESS" {
			successful++
		}
	}

	msg := fmt.Sprintf(
		"🚀 <b>Campaign Running</b>\n\n"+
			"⏱️ Duration: %s\n"+
			"📊 Completed: %d\n"+
			"✅ Successful: %d\n"+
			"❌ Failed: %d",
		elapsed.Round(time.Second),
		len(b.campaign.results),
		successful,
		len(b.campaign.results)-successful,
	)
	b.sendMessage(chatID, msg)
}

// handleRegister starts a registration campaign
func (b *TelegramBot) handleRegister(chatID int64, userConfig *UserConfig) {
	userConfig.mu.Lock()
	firstName := userConfig.FirstName
	lastName := userConfig.LastName
	organization := userConfig.Organization
	emailsFile := userConfig.EmailsFile
	eventsFile := userConfig.EventsFile
	proxiesFile := userConfig.ProxiesFile
	maxWorkers := userConfig.MaxWorkers
	userConfig.mu.Unlock()

	// Validate configuration
	if firstName == "" || lastName == "" || organization == "" {
		b.sendMessage(chatID, "❌ Please run /setup first to configure your details")
		return
	}

	b.campaign.mu.Lock()
	if b.campaign.running {
		b.campaign.mu.Unlock()
		b.sendMessage(chatID, "⚠️ Campaign already running!\n\nSend /stop first")
		return
	}
	b.campaign.running = true
	b.campaign.startTime = time.Now()
	b.campaign.results = []RegistrationResult{}
	b.campaign.mu.Unlock()

	emails, err := readEmails(emailsFile, b.logger)
	if err != nil {
		b.campaign.running = false
		b.sendMessage(chatID, fmt.Sprintf("❌ Failed to load emails from <code>%s</code>\n\nPlease upload emails.txt", emailsFile))
		return
	}

	eventURLs, err := readEventURLs(eventsFile, b.logger)
	if err != nil {
		b.campaign.running = false
		b.sendMessage(chatID, fmt.Sprintf("❌ Failed to load events from <code>%s</code>\n\nPlease upload events.txt", eventsFile))
		return
	}

	proxies, _ := readProxies(proxiesFile, b.logger)

	totalTasks := len(emails) * len(eventURLs)

	msg := fmt.Sprintf(
		"🚀 <b>Campaign Started!</b>\n\n"+
			"👤 Name: <b>%s %s</b>\n"+
			"🏢 Organization: <b>%s</b>\n"+
			"⚙️ Workers: <b>%d</b>\n\n"+
			"📧 Emails: %d\n"+
			"🎫 Events: %d\n"+
			"🔄 Total tasks: %d\n"+
			"🌐 Proxies: %d\n\n"+
			"Use /status to check progress",
		firstName, lastName, organization, maxWorkers,
		len(emails), len(eventURLs), totalTasks, len(proxies),
	)
	b.sendMessage(chatID, msg)

	go b.runCampaign(chatID, firstName, lastName, organization, maxWorkers, emails, eventURLs, proxies)
}

// runCampaign executes the registration campaign
func (b *TelegramBot) runCampaign(chatID int64, firstName, lastName, organization string, maxWorkers int, emails, eventURLs []string, proxies []ProxyConfig) {
	orchestrator := NewRegistrationOrchestrator(
		firstName,
		lastName,
		organization,
		true,
		maxWorkers,
		strconv.FormatInt(chatID, 10),
		b.logger,
	)

	results := orchestrator.Run(eventURLs, emails, proxies)

	b.campaign.mu.Lock()
	b.campaign.results = results
	b.campaign.running = false
	b.campaign.mu.Unlock()

	successful := 0
	failed := 0
	for _, r := range results {
		if r.Status == "SUCCESS" {
			successful++
		} else {
			failed++
		}
	}

	successRate := 0.0
	if len(results) > 0 {
		successRate = float64(successful) / float64(len(results)) * 100
	}

	duration := time.Since(b.campaign.startTime)

	msg := fmt.Sprintf(
		"✅ <b>Campaign Completed!</b>\n\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📊 Total: %d\n"+
			"✅ Successful: %d\n"+
			"❌ Failed: %d\n"+
			"📈 Success Rate: %.1f%%\n"+
			"⏱️ Duration: %s\n"+
			"⚡ Rate: %.1f tasks/sec\n"+
			"━━━━━━━━━━━━━━━━━━━━\n\n"+
			"Send /results for details",
		len(results), successful, failed, successRate,
		duration.Round(time.Second),
		float64(len(results))/duration.Seconds(),
	)
	b.sendMessage(chatID, msg)
}

// handleStop stops the running campaign
func (b *TelegramBot) handleStop(chatID int64) {
	b.campaign.mu.Lock()
	defer b.campaign.mu.Unlock()

	if !b.campaign.running {
		b.sendMessage(chatID, "⏸️ No campaign running")
		return
	}

	b.campaign.running = false
	b.sendMessage(chatID, "⏹️ Campaign stop requested\n\nWaiting for current tasks...")
}

// sendResults sends campaign results
func (b *TelegramBot) sendResults(chatID int64) {
	b.campaign.mu.Lock()
	results := b.campaign.results
	b.campaign.mu.Unlock()

	if len(results) == 0 {
		b.sendMessage(chatID, "📭 No results yet\n\nRun /register first")
		return
	}

	count := len(results)
	if count > 10 {
		count = 10
	}

	msg := fmt.Sprintf("<b>📊 Last %d Results</b>\n\n", count)
	for i := len(results) - count; i < len(results); i++ {
		r := results[i]
		status := "✅"
		if r.Status == "FAILED" {
			status = "❌"
		}
		msg += fmt.Sprintf(
			"%s <code>%s</code>\n   Event: %s\n   %s\n\n",
			status, truncateString(r.Email, 30), truncateString(r.Event, 40), r.Message,
		)
	}

	b.sendMessage(chatID, msg)
}

// sendStats sends statistics
func (b *TelegramBot) sendStats(chatID int64) {
	userConfig := b.getUserConfig(chatID)

	userConfig.mu.Lock()
	emailsFile := userConfig.EmailsFile
	eventsFile := userConfig.EventsFile
	proxiesFile := userConfig.ProxiesFile
	maxWorkers := userConfig.MaxWorkers
	userConfig.mu.Unlock()

	emails, _ := readEmails(emailsFile, b.logger)
	events, _ := readEventURLs(eventsFile, b.logger)
	proxies, _ := readProxies(proxiesFile, b.logger)

	b.campaign.mu.Lock()
	running := b.campaign.running
	resultsCount := len(b.campaign.results)
	b.campaign.mu.Unlock()

	status := "⏸️ Idle"
	if running {
		status = "🚀 Running"
	}

	estimatedBandwidth := maxWorkers * 2
	estimatedRAM := float64(maxWorkers) * 0.15

	msg := fmt.Sprintf(
		"<b>📊 System Statistics</b>\n\n"+
			"<b>Status:</b> %s\n\n"+
			"<b>Configuration:</b>\n"+
			"📧 Emails loaded: %d\n"+
			"🎫 Events loaded: %d\n"+
			"🌐 Proxies loaded: %d\n"+
			"👥 Max workers: %d\n\n"+
			"<b>Estimated Resources:</b>\n"+
			"📡 Bandwidth: ~%d Mbps\n"+
			"💾 RAM: ~%.1f GB\n\n"+
			"<b>Campaign:</b>\n"+
			"📝 Results cached: %d\n"+
			"🆔 Your Chat ID: <code>%d</code>",
		status, len(emails), len(events), len(proxies),
		maxWorkers, estimatedBandwidth, estimatedRAM,
		resultsCount, chatID,
	)
	b.sendMessage(chatID, msg)
}

// handleConfig shows configuration
func (b *TelegramBot) handleConfig(chatID int64, userConfig *UserConfig) {
	userConfig.mu.Lock()
	defer userConfig.mu.Unlock()

	msg := fmt.Sprintf(
		"<b>⚙️ Your Configuration</b>\n\n"+
			"<b>Personal Details:</b>\n"+
			"• First Name: <b>%s</b>\n"+
			"• Last Name: <b>%s</b>\n"+
			"• Organization: <b>%s</b>\n\n"+
			"<b>Files:</b>\n"+
			"• Emails: <code>%s</code>\n"+
			"• Events: <code>%s</code>\n"+
			"• Proxies: <code>%s</code>\n\n"+
			"<b>Performance:</b>\n"+
			"• Max Workers: <b>%d</b>\n"+
			"• Retry Attempts: %d\n\n"+
			"Send /setup or /workers to change",
		userConfig.FirstName, userConfig.LastName, userConfig.Organization,
		userConfig.EmailsFile, userConfig.EventsFile, userConfig.ProxiesFile,
		userConfig.MaxWorkers, config.RegistrationRetry,
	)
	b.sendMessage(chatID, msg)
}

// sendMessage sends a message to a chat
func (b *TelegramBot) sendMessage(chatID int64, text string) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(
		fmt.Sprintf("%s/sendMessage", b.apiURL),
		"application/json",
		strings.NewReader(string(jsonData)),
	)

	if err != nil {
		b.logger.Error("Failed to send message: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		b.logger.Error("Telegram API error: %s", string(body))
	}
}

// RunBotMode starts the application in bot mode
func RunBotMode(logger *Logger) {
	logger.Info(strings.Repeat("=", 70))
	logger.Info("TELEGRAM BOT MODE")
	logger.Info(strings.Repeat("=", 70))

	bot := NewTelegramBot(config.TelegramToken, logger)
	bot.Start()
}
