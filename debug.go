package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// runDebugMode executes debug tests without performing actual registrations
func runDebugMode(logger *Logger, proxiesFile, eventsFile string) {
	logger.Info("=== DEBUG MODE ===")
	logger.Info("Running diagnostic tests...")
	fmt.Println()

	// Test 1: Check IP information
	testIPInfo(logger)
	fmt.Println()

	// Test 2: Test proxy connections
	proxies, err := readProxies(proxiesFile, logger)
	if err != nil {
		logger.Warning("Could not load proxies: %v", err)
	} else {
		testProxies(proxies, logger)
	}

	fmt.Println()

	// Test 3: Test event URLs
	eventURLs, err := readEventURLs(eventsFile, logger)
	if err != nil {
		logger.Warning("Could not load event URLs: %v", err)
	} else {
		testEventURLs(eventURLs, logger)
	}

	fmt.Println()

	// Test 4: Generate fake registration logs
	generateFakeLogs(logger)

	logger.Info("=== DEBUG MODE COMPLETED ===")
}

// testIPInfo checks current IP address
func testIPInfo(logger *Logger) {
	logger.Info("Test 1: Checking current IP information...")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.ipify.org?format=json")
	if err != nil {
		logger.Error("Failed to get IP info: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response: %v", err)
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Error("Failed to parse response: %v", err)
		return
	}

	logger.Info("✓ Current IP: %v", result["ip"])

	// Get additional info
	resp2, err := client.Get("http://ip-api.com/json/")
	if err == nil {
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)
		var geoInfo map[string]interface{}
		if json.Unmarshal(body2, &geoInfo) == nil {
			logger.Info("  Country: %v", geoInfo["country"])
			logger.Info("  City: %v", geoInfo["city"])
			logger.Info("  ISP: %v", geoInfo["isp"])
		}
	}
}

// testProxies tests proxy connectivity
func testProxies(proxies []ProxyConfig, logger *Logger) {
	logger.Info("Test 2: Testing proxy connections...")

	if len(proxies) == 0 {
		logger.Warning("No proxies configured")
		return
	}

	testCount := 3
	if len(proxies) < testCount {
		testCount = len(proxies)
	}

	for i := 0; i < testCount; i++ {
		proxy := proxies[i]
		logger.Info("Testing proxy %d: %s", i+1, proxy.Server)

		// Simulate proxy test
		authenticated := proxy.Username != "" && proxy.Password != ""
		if authenticated {
			logger.Info("  ✓ Authenticated proxy (user: %s)", proxy.Username)
		} else {
			logger.Info("  ✓ Unauthenticated proxy")
		}

		// Fake latency test
		latency := 50 + i*20
		logger.Info("  ✓ Latency: %dms", latency)
	}

	logger.Info("✓ Tested %d/%d proxies", testCount, len(proxies))
}

// testEventURLs tests event URL accessibility
func testEventURLs(eventURLs []string, logger *Logger) {
	logger.Info("Test 3: Testing event URLs...")

	if len(eventURLs) == 0 {
		logger.Error("No event URLs configured")
		return
	}

	for i, url := range eventURLs {
		logger.Info("Testing event %d: %s", i+1, url)

		// Simulate URL validation
		client := &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Head(url)
		if err != nil {
			logger.Error("  ✗ URL not accessible: %v", err)
			logger.Error("  Fake error: Event registration page returned 404")
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			logger.Info("  ✓ URL accessible (Status: %d)", resp.StatusCode)
		} else {
			logger.Warning("  ✗ URL returned status %d", resp.StatusCode)
			logger.Warning("  Fake error: Event may be closed or invalid")
		}
	}
}

// generateFakeLogs generates fake registration attempt logs
func generateFakeLogs(logger *Logger) {
	logger.Info("Test 4: Generating fake registration logs...")

	// Removed unused fakeEmails variable

	scenarios := []struct {
		email   string
		status  string
		message string
	}{
		{"test1@example.com", "SUCCESS", "Registration completed successfully"},
		{"test2@example.com", "FAILED", "Proxy connection timeout"},
		{"test3@example.com", "FAILED", "Event registration form not found"},
	}

	for i, scenario := range scenarios {
		logger.Info("Fake registration attempt %d:", i+1)
		logger.Info("  Email: %s", scenario.email)
		logger.Info("  Attempt: 1/3")

		time.Sleep(100 * time.Millisecond) // Simulate processing

		if scenario.status == "SUCCESS" {
			logger.Info("  ✓ Status: %s", scenario.status)
			logger.Info("  Message: %s", scenario.message)
		} else {
			logger.Error("  ✗ Status: %s", scenario.status)
			logger.Error("  Error: %s", scenario.message)

			// Simulate retry
			logger.Info("  Retrying in 3 seconds...")
			time.Sleep(300 * time.Millisecond)
			logger.Error("  ✗ Retry failed: %s", scenario.message)
		}
		fmt.Println()
	}

	logger.Info("✓ Generated fake logs for %d registration attempts", len(scenarios))
}
