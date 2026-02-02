package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Config holds application configuration
type Config struct {
	TelegramToken     string
	TelegramAPI       string
	ElementWait       time.Duration
	PageLoadWait      time.Duration
	RegistrationRetry int
	MaxWorkers        int
}

var config = Config{
	TelegramToken:     "8144020899:AAFsc11elbxfhsYtzW-9vbStDZZ-TXhLxW0",
	ElementWait:       10 * time.Second,
	PageLoadWait:      15 * time.Second,
	RegistrationRetry: 3,
	MaxWorkers:        20,
}

func init() {
	config.TelegramAPI = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.TelegramToken)
}

// ProxyConfig represents a proxy configuration
type ProxyConfig struct {
	Server   string `json:"server"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// RegistrationResult represents the result of a registration attempt
type RegistrationResult struct {
	Email     string    `json:"email"`
	Event     string    `json:"event"`
	Status    string    `json:"status"`
	Attempt   int       `json:"attempt"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Logger provides structured logging
type Logger struct {
	verbose bool
}

func NewLogger(verbose bool) *Logger {
	return &Logger{verbose: verbose}
}

func (l *Logger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.verbose {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func (l *Logger) Warning(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func main() {
	// Command-line flags
	botMode := flag.Bool("bot", false, "Run in Telegram bot mode (interactive)")
	firstName := flag.String("first-name", "", "Registration first name (REQUIRED for CLI mode)")
	lastName := flag.String("last-name", "", "Registration last name (REQUIRED for CLI mode)")
	organization := flag.String("organization", "", "Organization name (REQUIRED for CLI mode)")
	emailsFile := flag.String("emails", "emails.txt", "Email file path")
	eventsFile := flag.String("events", "list.txt", "Event URLs file path")
	proxiesFile := flag.String("proxies", "proxies.txt", "Proxy file path")
	workers := flag.Int("workers", config.MaxWorkers, "Max concurrent workers")
	headless := flag.Bool("headless", true, "Run browser in headless mode")
	windowMode := flag.Bool("window", false, "Show browser window")
	verbose := flag.Bool("verbose", false, "Enable debug logging")
	telegram := flag.String("telegram", "", "Telegram chat ID for notifications")
	debug := flag.Bool("debug", false, "Run in debug mode (test IP info and fake logs)")

	flag.Parse()

	logger := NewLogger(*verbose)

	// Bot mode - interactive control via Telegram
	if *botMode {
		logger.Info("Starting in Telegram Bot mode...")
		logger.Info("Send /start to your bot to begin")
		RunBotMode(logger)
		return
	}

	// CLI mode - requires arguments
	if *firstName == "" || *lastName == "" || *organization == "" {
		fmt.Println("Error: --first-name, --last-name, and --organization are required")
		fmt.Println("")
		fmt.Println("Or use --bot flag to run in interactive Telegram bot mode:")
		fmt.Println("  go run . --bot")
		fmt.Println("")
		flag.Usage()
		os.Exit(1)
	}

	logger.Info("System: %s", getSystemInfo())
	logger.Info("Starting Event Registration Automation")

	// Test Telegram connection if chat ID provided
	if *telegram != "" {
		logger.Info("Telegram notifications enabled (Chat ID: %s)", *telegram)
		testTelegramConnection(*telegram, logger)
	} else {
		logger.Warning("Telegram notifications disabled (no --telegram flag)")
	}

	// Debug mode
	if *debug {
		runDebugMode(logger, *proxiesFile, *eventsFile)
		return
	}

	// Load configuration files
	emails, err := readEmails(*emailsFile, logger)
	if err != nil {
		logger.Error("Failed to read emails: %v", err)
		os.Exit(1)
	}

	eventURLs, err := readEventURLs(*eventsFile, logger)
	if err != nil {
		logger.Error("Failed to read event URLs: %v", err)
		os.Exit(1)
	}

	proxies, err := readProxies(*proxiesFile, logger)
	if err != nil {
		logger.Warning("Failed to read proxies: %v", err)
		proxies = []ProxyConfig{} // Continue without proxies
	}

	if len(emails) == 0 || len(eventURLs) == 0 {
		logger.Error("Missing emails or event URLs")
		os.Exit(1)
	}

	// Create orchestrator
	orchestrator := NewRegistrationOrchestrator(
		*firstName,
		*lastName,
		*organization,
		!*windowMode && *headless,
		*workers,
		*telegram,
		logger,
	)

	// Run registration campaign
	results := orchestrator.Run(eventURLs, emails, proxies)

	if len(results) > 0 {
		os.Exit(0)
	}
	os.Exit(1)
}

// RegistrationOrchestrator manages the registration campaign
type RegistrationOrchestrator struct {
	firstName      string
	lastName       string
	organization   string
	headless       bool
	maxWorkers     int
	telegramChatID string
	logger         *Logger
}

func NewRegistrationOrchestrator(firstName, lastName, organization string, headless bool, maxWorkers int, telegramChatID string, logger *Logger) *RegistrationOrchestrator {
	return &RegistrationOrchestrator{
		firstName:      firstName,
		lastName:       lastName,
		organization:   organization,
		headless:       headless,
		maxWorkers:     maxWorkers,
		telegramChatID: telegramChatID,
		logger:         logger,
	}
}

func (o *RegistrationOrchestrator) Run(eventURLs, emails []string, proxies []ProxyConfig) []RegistrationResult {
	totalTasks := len(eventURLs) * len(emails)

	o.logger.Info("Starting registration campaign:")
	o.logger.Info("  Events: %d", len(eventURLs))
	o.logger.Info("  Emails: %d", len(emails))
	o.logger.Info("  Total tasks: %d", totalTasks)
	o.logger.Info("  Workers: %d", o.maxWorkers)
	o.logger.Info("  Headless: %v", o.headless)
	o.logger.Info("  Proxies: %d", len(proxies))

	startTime := time.Now()

	// Create work queue
	type job struct {
		eventURL string
		email    string
		workerID int
	}

	jobs := make(chan job, totalTasks)
	results := make(chan RegistrationResult, totalTasks)

	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < o.maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker := NewRegistrationWorker(workerID, proxies, o.headless, o.telegramChatID, o.logger)

			for job := range jobs {
				result := worker.ExecuteRegistration(
					job.eventURL,
					o.firstName,
					o.lastName,
					job.email,
					o.organization,
				)
				results <- result
			}
		}(i)
	}

	// Queue jobs
	jobIndex := 0
	for _, eventURL := range eventURLs {
		for _, email := range emails {
			jobs <- job{
				eventURL: eventURL,
				email:    email,
				workerID: jobIndex % o.maxWorkers,
			}
			jobIndex++
		}
	}
	close(jobs)

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []RegistrationResult
	completed := 0
	successCount := 0

	for result := range results {
		allResults = append(allResults, result)
		completed++
		if result.Status == "SUCCESS" {
			successCount++
		}

		elapsed := time.Since(startTime).Seconds()
		o.logger.Info("Progress: %d/%d | Success: %d | Elapsed: %.0fs", completed, totalTasks, successCount, elapsed)
	}

	elapsed := time.Since(startTime)
	o.printSummary(allResults, elapsed)

	return allResults
}

func (o *RegistrationOrchestrator) printSummary(results []RegistrationResult, elapsed time.Duration) {
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

	rate := float64(len(results)) / elapsed.Seconds()

	o.logger.Info("\n" + strings.Repeat("=", 70))
	o.logger.Info("REGISTRATION CAMPAIGN SUMMARY")
	o.logger.Info(strings.Repeat("=", 70))
	o.logger.Info("Total: %d", len(results))
	o.logger.Info("✓ Successful: %d", successful)
	o.logger.Info("✗ Failed: %d", failed)
	o.logger.Info("Success Rate: %.1f%%", successRate)
	o.logger.Info("Duration: %.1fs", elapsed.Seconds())
	o.logger.Info("Rate: %.1f registrations/sec", rate)
	o.logger.Info(strings.Repeat("=", 70))

	o.saveResults(results)
}

func (o *RegistrationOrchestrator) saveResults(results []RegistrationResult) {
	outputFile := fmt.Sprintf("results_%s.json", time.Now().Format("20060102_150405"))

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		o.logger.Error("Failed to marshal results: %v", err)
		return
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		o.logger.Error("Failed to save results: %v", err)
		return
	}

	o.logger.Info("Results saved to %s", outputFile)
}

func getSystemInfo() string {
	return fmt.Sprintf("Go version: %s", strings.TrimPrefix(os.Getenv("GO_VERSION"), "go"))
}
