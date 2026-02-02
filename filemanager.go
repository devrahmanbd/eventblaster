package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// readEmails reads and validates email addresses from file
func readEmails(filename string, logger *Logger) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("email file not found: %s", filename)
	}
	defer file.Close()

	var emails []string
	scanner := bufio.NewScanner(file)
	
	// Regex to extract email addresses
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Check if it's a markdown link format: [text](mailto:email@example.com)
		if strings.Contains(line, "](mailto:") {
			// Extract email from mailto: link
			start := strings.Index(line, "mailto:")
			if start != -1 {
				end := strings.Index(line[start:], ")")
				if end != -1 {
					email := line[start+7 : start+end] // Skip "mailto:"
					if emailRegex.MatchString(email) {
						emails = append(emails, email)
						logger.Debug("Loaded email: %s", email)
						continue
					}
				}
			}
		}
		
		// Check if it's a markdown link format: [email@example.com](url)
		if strings.Contains(line, "[") && strings.Contains(line, "](") {
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start != -1 && end != -1 && end > start {
				text := line[start+1 : end]
				// Check if the text inside brackets is an email
				if emailRegex.MatchString(text) {
					emails = append(emails, text)
					logger.Debug("Loaded email: %s", text)
					continue
				}
			}
		}
		
		// Try to extract any email from the line using regex
		if found := emailRegex.FindString(line); found != "" {
			emails = append(emails, found)
			logger.Debug("Loaded email: %s", found)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading emails: %v", err)
	}

	logger.Info("Loaded %d emails from %s", len(emails), filename)
	return emails, nil
}

// readEventURLs reads event URLs from file
func readEventURLs(filename string, logger *Logger) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("event list file not found: %s", filename)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && (strings.HasPrefix(line, "http") || strings.Contains(line, "event")) {
			urls = append(urls, line)
			logger.Debug("Loaded event URL: %s", line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading event URLs: %v", err)
	}

	logger.Info("Loaded %d event URLs from %s", len(urls), filename)
	return urls, nil
}

// readProxies reads and parses proxy configurations from file
func readProxies(filename string, logger *Logger) ([]ProxyConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		logger.Warning("Proxy file not found: %s. Running without proxies.", filename)
		return []ProxyConfig{}, nil
	}
	defer file.Close()

	var proxies []ProxyConfig
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		proxy := parseProxyLine(line)
		if proxy != nil {
			proxies = append(proxies, *proxy)
			logger.Debug("Loaded proxy: %s", proxy.Server)
		} else if line != "" && !strings.HasPrefix(line, "#") {
			logger.Warning("Skipping invalid proxy line: %s", truncateString(line, 120))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading proxies: %v", err)
	}

	logger.Info("Loaded %d proxies from %s", len(proxies), filename)
	return proxies, nil
}

// parseProxyLine parses various proxy formats
func parseProxyLine(line string) *ProxyConfig {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return nil
	}

	// Unwrap markdown links
	if strings.Contains(s, "[") && strings.Contains(s, "](") {
		start := strings.Index(s, "[")
		end := strings.Index(s, "]")
		if start != -1 && end != -1 && end > start {
			s = s[start+1 : end]
		}
	}

	// Parse URL format (http://user:pass@host:port)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" || u.Port() == "" {
			return nil
		}

		proxy := &ProxyConfig{
			Server: fmt.Sprintf("%s://%s:%s", u.Scheme, u.Hostname(), u.Port()),
		}

		if u.User != nil {
			proxy.Username = u.User.Username()
			if password, ok := u.User.Password(); ok {
				proxy.Password = password
			}
		}
		return proxy
	}

	// Handle USER:PASS@HOST:PORT
	if strings.Contains(s, "@") {
		parts := strings.Split(s, "@")
		if len(parts) == 2 {
			userPass := strings.Split(parts[0], ":")
			hostPort := strings.Split(parts[1], ":")
			if len(userPass) == 2 && len(hostPort) == 2 {
				if _, err := strconv.Atoi(hostPort[1]); err == nil {
					return &ProxyConfig{
						Server:   fmt.Sprintf("http://%s:%s", hostPort[0], hostPort[1]),
						Username: userPass[0],
						Password: userPass[1],
					}
				}
			}
		}
	}

	// Handle HOST:PORT:USER:PASS or USER:PASS:HOST:PORT
	parts := strings.Split(s, ":")
	if len(parts) == 4 {
		// Check if second part is a port (HOST:PORT:USER:PASS)
		if _, err := strconv.Atoi(parts[1]); err == nil {
			return &ProxyConfig{
				Server:   fmt.Sprintf("http://%s:%s", parts[0], parts[1]),
				Username: parts[2],
				Password: parts[3],
			}
		}
		// Check if fourth part is a port (USER:PASS:HOST:PORT)
		if _, err := strconv.Atoi(parts[3]); err == nil {
			return &ProxyConfig{
				Server:   fmt.Sprintf("http://%s:%s", parts[2], parts[3]),
				Username: parts[0],
				Password: parts[1],
			}
		}
	}

	// Handle bare HOST:PORT (no auth)
	if len(parts) == 2 {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			return &ProxyConfig{
				Server: fmt.Sprintf("http://%s:%s", parts[0], parts[1]),
			}
		}
	}

	return nil
}
