package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseProxyLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *ProxyConfig
	}{
		{
			name:  "URL format with auth",
			input: "http://user:pass@proxy.example.com:8080",
			expected: &ProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "user",
				Password: "pass",
			},
		},
		{
			name:  "USER:PASS@HOST:PORT",
			input: "user:pass@proxy.example.com:8080",
			expected: &ProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "user",
				Password: "pass",
			},
		},
		{
			name:  "HOST:PORT:USER:PASS",
			input: "proxy.example.com:8080:user:pass",
			expected: &ProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "user",
				Password: "pass",
			},
		},
		{
			name:  "USER:PASS:HOST:PORT",
			input: "user:pass:proxy.example.com:8080",
			expected: &ProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "user",
				Password: "pass",
			},
		},
		{
			name:  "HOST:PORT (no auth)",
			input: "proxy.example.com:8080",
			expected: &ProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "",
				Password: "",
			},
		},
		{
			name:     "Empty line",
			input:    "",
			expected: nil,
		},
		{
			name:     "Comment line",
			input:    "# This is a comment",
			expected: nil,
		},
		{
			name:     "Invalid format",
			input:    "invalid-proxy-format",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProxyLine(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("Expected %+v, got nil", tt.expected)
				return
			}

			if result.Server != tt.expected.Server {
				t.Errorf("Server mismatch: expected %s, got %s", tt.expected.Server, result.Server)
			}

			if result.Username != tt.expected.Username {
				t.Errorf("Username mismatch: expected %s, got %s", tt.expected.Username, result.Username)
			}

			if result.Password != tt.expected.Password {
				t.Errorf("Password mismatch: expected %s, got %s", tt.expected.Password, result.Password)
			}
		})
	}
}

func TestReadEmails(t *testing.T) {
	// Create temporary test file
	content := `test1@example.com
test2@gmail.com
[email](mailto:test3@company.com)
invalid-email
test4@domain.org
`
	tmpFile, err := os.CreateTemp("", "emails_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	logger := NewLogger(false)
	emails, err := readEmails(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("readEmails failed: %v", err)
	}

	expected := []string{
		"test1@example.com",
		"test2@gmail.com",
		"test3@company.com",
		"test4@domain.org",
	}

	if len(emails) != len(expected) {
		t.Errorf("Expected %d emails, got %d", len(expected), len(emails))
	}

	for i, email := range emails {
		if email != expected[i] {
			t.Errorf("Email %d: expected %s, got %s", i, expected[i], email)
		}
	}
}

func TestReadEventURLs(t *testing.T) {
	// Create temporary test file
	content := `https://events.example.com/event/12345
https://register.example.com/event/67890
# Comment line
not-a-url
https://example.com/event/abc123
`
	tmpFile, err := os.CreateTemp("", "events_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	logger := NewLogger(false)
	urls, err := readEventURLs(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("readEventURLs failed: %v", err)
	}

	if len(urls) != 3 {
		t.Errorf("Expected 3 URLs, got %d", len(urls))
	}
}

func TestReadProxies(t *testing.T) {
	// Create temporary test file
	content := `# Proxy list
http://user1:pass1@proxy1.example.com:8080
user2:pass2@proxy2.example.com:3128
proxy3.example.com:8080:user3:pass3
proxy4.example.com:8080
invalid-proxy-format
`
	tmpFile, err := os.CreateTemp("", "proxies_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	logger := NewLogger(false)
	proxies, err := readProxies(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("readProxies failed: %v", err)
	}

	if len(proxies) != 4 {
		t.Errorf("Expected 4 proxies, got %d", len(proxies))
	}

	// Test first proxy (URL format)
	if proxies[0].Server != "http://proxy1.example.com:8080" {
		t.Errorf("Proxy 0 server mismatch: %s", proxies[0].Server)
	}
	if proxies[0].Username != "user1" || proxies[0].Password != "pass1" {
		t.Errorf("Proxy 0 auth mismatch")
	}

	// Test last proxy (no auth)
	if proxies[3].Username != "" || proxies[3].Password != "" {
		t.Errorf("Proxy 3 should have no auth")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello"},
		{"test", 4, "test"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestLastPathSegment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/event/12345", "12345"},
		{"https://example.com/event/12345/", "12345"},
		{"https://example.com", "example.com"},
		{"event123", "event123"},
	}

	for _, tt := range tests {
		result := lastPathSegment(tt.input)
		if result != tt.expected {
			t.Errorf("lastPathSegment(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestPow(t *testing.T) {
	tests := []struct {
		base     int
		exp      int
		expected int
	}{
		{2, 3, 8},
		{3, 2, 9},
		{5, 0, 1},
		{10, 1, 10},
	}

	for _, tt := range tests {
		result := pow(tt.base, tt.exp)
		if result != tt.expected {
			t.Errorf("pow(%d, %d) = %d, expected %d", tt.base, tt.exp, result, tt.expected)
		}
	}
}

func TestFormatFailureAlert(t *testing.T) {
	email := "test@example.com"
	eventURL := "https://example.com/event/12345"
	attempt := 2
	reason := "Connection timeout"

	result := formatFailureAlert(email, eventURL, attempt, reason)

	if !strings.Contains(result, email) {
		t.Error("Alert should contain email")
	}

	if !strings.Contains(result, "12345") {
		t.Error("Alert should contain event ID")
	}

	if !strings.Contains(result, "2/3") {
		t.Error("Alert should contain attempt count")
	}

	if !strings.Contains(result, reason) {
		t.Error("Alert should contain reason")
	}
}

func TestRegistrationResult(t *testing.T) {
	result := RegistrationResult{
		Email:     "test@example.com",
		Event:     "event123",
		Status:    "SUCCESS",
		Attempt:   1,
		Message:   "Registration completed",
		Timestamp: time.Now(),
	}

	if result.Email != "test@example.com" {
		t.Error("Email mismatch")
	}

	if result.Status != "SUCCESS" {
		t.Error("Status mismatch")
	}

	if result.Attempt != 1 {
		t.Error("Attempt mismatch")
	}
}

func TestLogger(t *testing.T) {
	// Test verbose logger
	verboseLogger := NewLogger(true)
	if !verboseLogger.verbose {
		t.Error("Verbose logger should have verbose=true")
	}

	// Test non-verbose logger
	normalLogger := NewLogger(false)
	if normalLogger.verbose {
		t.Error("Normal logger should have verbose=false")
	}
}

func BenchmarkParseProxyLine(b *testing.B) {
	input := "user:pass@proxy.example.com:8080"
	for i := 0; i < b.N; i++ {
		parseProxyLine(input)
	}
}

func BenchmarkTruncateString(b *testing.B) {
	input := "This is a long string that needs to be truncated"
	for i := 0; i < b.N; i++ {
		truncateString(input, 20)
	}
}
