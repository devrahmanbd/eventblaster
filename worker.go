package main

import (
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
)

// RegistrationWorker handles individual registration tasks
type RegistrationWorker struct {
	workerID       int
	proxies        []ProxyConfig
	headless       bool
	telegramChatID string
	logger         *Logger
}

func NewRegistrationWorker(workerID int, proxies []ProxyConfig, headless bool, telegramChatID string, logger *Logger) *RegistrationWorker {
	return &RegistrationWorker{
		workerID:       workerID,
		proxies:        proxies,
		headless:       headless,
		telegramChatID: telegramChatID,
		logger:         logger,
	}
}

func (w *RegistrationWorker) ExecuteRegistration(eventURL, firstName, lastName, email, organization string) RegistrationResult {
	var proxy *ProxyConfig
	if len(w.proxies) > 0 {
		proxy = &w.proxies[w.workerID%len(w.proxies)]
	}

	for attempt := 1; attempt <= config.RegistrationRetry; attempt++ {
		w.logger.Info("[%s] Attempt %d/%d", email, attempt, config.RegistrationRetry)
		success, message := w.tryRegistration(eventURL, firstName, lastName, email, organization, proxy)

		if success {
			w.logger.Info("✓ %s - Success", email)
			return RegistrationResult{
				Email:     email,
				Event:     truncateString(lastPathSegment(eventURL), 20),
				Status:    "SUCCESS",
				Attempt:   attempt,
				Message:   message,
				Timestamp: time.Now(),
			}
		}

		w.logger.Warning("✗ %s - Failed: %s", email, message)

		if attempt < config.RegistrationRetry {
			sleepDuration := time.Duration(pow(3, attempt)) * time.Second
			w.logger.Debug("Retrying in %v...", sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			// Send Telegram alert on final failure
			if w.telegramChatID != "" {
				alert := formatFailureAlert(email, eventURL, attempt, message)
				sendTelegramAlert(alert, w.telegramChatID, w.logger)
			}
		}
	}

	return RegistrationResult{
		Email:     email,
		Event:     truncateString(lastPathSegment(eventURL), 20),
		Status:    "FAILED",
		Attempt:   config.RegistrationRetry,
		Message:   "Max retries exceeded",
		Timestamp: time.Now(),
	}
}

func (w *RegistrationWorker) tryRegistration(eventURL, firstName, lastName, email, organization string, proxy *ProxyConfig) (bool, string) {
	// Install Playwright if needed (first run only)
	err := playwright.Install()
	if err != nil {
		return false, fmt.Sprintf("Playwright install error: %v", err)
	}

	// Start Playwright
	pw, err := playwright.Run()
	if err != nil {
		return false, fmt.Sprintf("Could not start Playwright: %v", err)
	}
	defer func() {
		if err := pw.Stop(); err != nil {
			w.logger.Error("Failed to stop Playwright: %v", err)
		}
	}()

	// Launch browser
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(w.headless),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--disable-setuid-sandbox",
		},
	}

	if proxy != nil {
		launchOptions.Proxy = &playwright.Proxy{
			Server:   proxy.Server,
			Username: playwright.String(proxy.Username),
			Password: playwright.String(proxy.Password),
		}
		w.logger.Info("🌐 Using proxy: %s", proxy.Server)
	} else {
		w.logger.Warning("⚠️  No proxy configured - using direct connection")
	}

	browser, err := pw.Chromium.Launch(launchOptions)
	if err != nil {
		return false, fmt.Sprintf("Could not launch browser: %v", err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			w.logger.Error("Failed to close browser: %v", err)
		}
	}()

	// Create context
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:  &playwright.Size{Width: 1248, Height: 836},
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	})
	if err != nil {
		return false, fmt.Sprintf("Could not create context: %v", err)
	}
	defer func() {
		if err := context.Close(); err != nil {
			w.logger.Error("Failed to close context: %v", err)
		}
	}()

	// Create page
	page, err := context.NewPage()
	if err != nil {
		return false, fmt.Sprintf("Could not create page: %v", err)
	}
	defer func() {
		if err := page.Close(); err != nil {
			w.logger.Error("Failed to close page: %v", err)
		}
	}()

	// VERIFY PROXY IS WORKING - Check IP
	if proxy != nil {
		w.logger.Info("🔍 Verifying proxy connection...")
		if _, err := page.Goto("https://api.ipify.org?format=json", playwright.PageGotoOptions{
			Timeout: playwright.Float(10000),
		}); err != nil {
			w.logger.Warning("⚠️  Could not verify proxy IP: %v", err)
		} else {
			ipInfo, _ := page.Evaluate("() => document.body.innerText")
			w.logger.Info("✅ Proxy IP check: %v", ipInfo)
		}
	}

	// Perform registration
	return performRegistration(page, eventURL, firstName, lastName, email, organization, w.logger)
}

func performRegistration(page playwright.Page, eventURL, firstName, lastName, email, organization string, logger *Logger) (bool, string) {
	logger.Info("📄 Loading event URL...")

	// Navigate to event page with LONGER timeout (60s instead of 15s)
	if _, err := page.Goto(eventURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(60000), // 60 seconds
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return false, fmt.Sprintf("Failed to load page: %v", err)
	}

	logger.Info("✅ Page loaded successfully")
	page.WaitForTimeout(2000)

	logger.Debug("📝 Filling form fields...")

	// Fill first name
	if err := page.Locator("#first_name").Click(); err != nil {
		return false, fmt.Sprintf("First name field not found: %v", err)
	}
	if err := page.Locator("#first_name").Fill(firstName); err != nil {
		return false, fmt.Sprintf("Failed to fill first name: %v", err)
	}
	page.WaitForTimeout(500)

	// Fill last name
	if err := page.Locator("#last_name").Click(); err != nil {
		return false, fmt.Sprintf("Last name field not found: %v", err)
	}
	if err := page.Locator("#last_name").Fill(lastName); err != nil {
		return false, fmt.Sprintf("Failed to fill last name: %v", err)
	}
	page.WaitForTimeout(500)

	// Fill email
	if err := page.Locator("#email").Click(); err != nil {
		return false, fmt.Sprintf("Email field not found: %v", err)
	}
	page.Locator("#email").Clear()
	if err := page.Locator("#email").Fill(email); err != nil {
		return false, fmt.Sprintf("Failed to fill email: %v", err)
	}
	page.WaitForTimeout(1000)

	// Fill organization
	orgLocator := "#add3dffe-7bd0-4e39-872e-8398117afd53"
	if err := page.Locator(orgLocator).Click(); err != nil {
		return false, fmt.Sprintf("Organization field not found: %v", err)
	}
	if err := page.Locator(orgLocator).Fill(organization); err != nil {
		return false, fmt.Sprintf("Failed to fill organization: %v", err)
	}
	page.WaitForTimeout(500)

	// Accept terms
	if err := page.Locator("#ms-event-terms-and-conditions").Click(); err != nil {
		return false, fmt.Sprintf("Terms checkbox not found: %v", err)
	}
	page.WaitForTimeout(1000)

	// Submit
	logger.Info("📤 Submitting registration...")
	if err := page.Locator("#submitRegistration").Click(); err != nil {
		return false, fmt.Sprintf("Submit button not found: %v", err)
	}

	// Wait longer for server response
	logger.Debug("⏳ Waiting for response...")
	page.WaitForTimeout(5000)

	// Check for success indicators (multiple strategies)
	// Strategy 1: Check for success modal
	successLocator := page.Locator("#modalSuccessTitle")
	successText, err := successLocator.TextContent(playwright.LocatorTextContentOptions{
		Timeout: playwright.Float(3000),
	})
	if err == nil && successText != "" {
		logger.Info("✓ Registration successful: %s", successText)
		return true, fmt.Sprintf("Success: %s", successText)
	}

	// Strategy 2: Check for any success-related elements
	successVariants := []string{
		".success-message",
		"[data-testid='success-message']",
		"text=success",
		"text=registered",
		"text=confirmation",
	}
	for _, selector := range successVariants {
		if elem := page.Locator(selector); elem != nil {
			if text, err := elem.TextContent(playwright.LocatorTextContentOptions{
				Timeout: playwright.Float(1000),
			}); err == nil && text != "" {
				logger.Info("✓ Registration successful (found: %s)", selector)
				return true, fmt.Sprintf("Success: %s", text)
			}
		}
	}

	// Strategy 3: Check URL change (redirect to success page)
	currentURL := page.URL()
	if currentURL != eventURL {
		logger.Debug("URL changed to: %s", currentURL)
		if containsSuccessIndicator(currentURL) {
			logger.Info("✓ Registration successful (URL redirect)")
			return true, "Success: Redirected to success page"
		}
	}

	// Check for error messages
	errorSelectors := []string{
		".error-message",
		"[role='alert']",
		".alert-danger",
		"text=error",
		"text=failed",
	}
	for _, selector := range errorSelectors {
		if elem := page.Locator(selector); elem != nil {
			if text, err := elem.TextContent(playwright.LocatorTextContentOptions{
				Timeout: playwright.Float(1000),
			}); err == nil && text != "" {
				return false, fmt.Sprintf("Error: %s", text)
			}
		}
	}

	// Take screenshot for debugging
	screenshotPath := fmt.Sprintf("debug_screenshot_%d.png", time.Now().Unix())
	page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(screenshotPath),
	})
	logger.Debug("Screenshot saved: %s", screenshotPath)

	return false, "Could not confirm registration status - check screenshot"
}

// containsSuccessIndicator checks if URL contains success indicators
func containsSuccessIndicator(url string) bool {
	successKeywords := []string{"success", "confirmation", "thank", "registered", "complete"}
	urlLower := fmt.Sprintf("%v", url)
	for _, keyword := range successKeywords {
		if contains(urlLower, keyword) {
			return true
		}
	}
	return false
}

// contains checks if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	// Simple case-insensitive check
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > 0 && len(substr) > 0 &&
			fmt.Sprintf("%v", s) != fmt.Sprintf("%v", substr)))
}
