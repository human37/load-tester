package main

import (
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// generateRandomVariables creates a new map with random values based on placeholders
func generateRandomVariables(baseVars map[string]interface{}) map[string]interface{} {
	if baseVars == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range baseVars {
		result[k] = processRandomValue(v)
	}
	return result
}

// processRandomValue recursively processes values to replace random placeholders
func processRandomValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return replaceRandomPlaceholders(v)
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = processRandomValue(val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = processRandomValue(val)
		}
		return result
	default:
		return v
	}
}

// replaceRandomPlaceholders replaces {{random.*}} placeholders with actual random values
func replaceRandomPlaceholders(text string) interface{} {
	// Define regex patterns for different random types
	patterns := map[string]func([]string) interface{}{
		`\{\{random\.string(?:\((\d+)\))?\}\}`:                  generateRandomString,
		`\{\{random\.number(?:\((\d+),(\d+)\))?\}\}`:            generateRandomNumber,
		`\{\{random\.int(?:\((\d+),(\d+)\))?\}\}`:               generateRandomInt,
		`\{\{random\.float(?:\((\d+\.?\d*),(\d+\.?\d*)\))?\}\}`: generateRandomFloat,
		`\{\{random\.uuid\}\}`:                                  generateRandomUUID,
		`\{\{random\.email\}\}`:                                 generateRandomEmail,
		`\{\{random\.name\}\}`:                                  generateRandomName,
		`\{\{random\.timestamp\}\}`:                             generateRandomTimestamp,
		`\{\{random\.choice\(([^)]+)\)\}\}`:                     generateRandomChoice,
	}

	result := text
	for pattern, generator := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(result, -1)

		for _, match := range matches {
			replacement := generator(match)
			result = strings.Replace(result, match[0], fmt.Sprintf("%v", replacement), 1)
		}
	}

	// If the entire string was a placeholder and got replaced with a non-string, return the actual type
	if result != text {
		// Try to parse as number if it looks like one
		if val, err := strconv.ParseFloat(result, 64); err == nil {
			if val == math.Trunc(val) {
				return int(val)
			}
			return val
		}
	}

	return result
}

// generateRandomString generates a random string of specified length
func generateRandomString(match []string) interface{} {
	length := 10 // default length
	if len(match) > 1 && match[1] != "" {
		if l, err := strconv.Atoi(match[1]); err == nil {
			length = l
		}
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[mathrand.Intn(len(charset))]
	}
	return string(b)
}

// generateRandomNumber generates a random integer within specified range
func generateRandomNumber(match []string) interface{} {
	min, max := 1, 1000 // defaults
	if len(match) > 2 && match[1] != "" && match[2] != "" {
		if minVal, err := strconv.Atoi(match[1]); err == nil {
			min = minVal
		}
		if maxVal, err := strconv.Atoi(match[2]); err == nil {
			max = maxVal
		}
	}
	return mathrand.Intn(max-min+1) + min
}

// generateRandomInt is an alias for generateRandomNumber
func generateRandomInt(match []string) interface{} {
	return generateRandomNumber(match)
}

// generateRandomFloat generates a random float within specified range
func generateRandomFloat(match []string) interface{} {
	min, max := 0.0, 100.0 // defaults
	if len(match) > 2 && match[1] != "" && match[2] != "" {
		if minVal, err := strconv.ParseFloat(match[1], 64); err == nil {
			min = minVal
		}
		if maxVal, err := strconv.ParseFloat(match[2], 64); err == nil {
			max = maxVal
		}
	}
	return min + mathrand.Float64()*(max-min)
}

// generateRandomUUID generates a random UUID v4
func generateRandomUUID(match []string) interface{} {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // Version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // Variant is 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

// generateRandomEmail generates a random email address
func generateRandomEmail(match []string) interface{} {
	names := []string{"john", "jane", "bob", "alice", "charlie", "diana", "eve", "frank"}
	domains := []string{"example.com", "test.com", "demo.org", "sample.net"}

	name := names[mathrand.Intn(len(names))]
	domain := domains[mathrand.Intn(len(domains))]
	suffix := mathrand.Intn(9999)

	return fmt.Sprintf("%s%d@%s", name, suffix, domain)
}

// generateRandomName generates a random full name
func generateRandomName(match []string) interface{} {
	firstNames := []string{"John", "Jane", "Bob", "Alice", "Charlie", "Diana", "Eve", "Frank", "Grace", "Henry"}
	lastNames := []string{"Smith", "Johnson", "Brown", "Davis", "Miller", "Wilson", "Moore", "Taylor", "Anderson", "Thomas"}

	first := firstNames[mathrand.Intn(len(firstNames))]
	last := lastNames[mathrand.Intn(len(lastNames))]

	return fmt.Sprintf("%s %s", first, last)
}

// generateRandomTimestamp generates a random timestamp within the last 30 days
func generateRandomTimestamp(match []string) interface{} {
	// Generate a timestamp within the last 30 days
	now := time.Now()
	pastTime := now.AddDate(0, 0, -30)

	randomTime := pastTime.Add(time.Duration(mathrand.Int63n(int64(now.Sub(pastTime)))))
	return randomTime.Format(time.RFC3339)
}

// generateRandomChoice selects a random option from comma-separated choices
func generateRandomChoice(match []string) interface{} {
	if len(match) < 2 {
		return "option1"
	}

	choices := strings.Split(match[1], ",")
	for i, choice := range choices {
		choices[i] = strings.TrimSpace(choice)
	}

	if len(choices) == 0 {
		return "option1"
	}

	return choices[mathrand.Intn(len(choices))]
}

type RequestLogEntry struct {
	Date     string
	Status   int
	Request  string
	Response string
}

type AsyncLogger struct {
	enabled    bool
	logFile    *os.File
	csvWriter  *csv.Writer
	logChannel chan RequestLogEntry
	waitGroup  sync.WaitGroup
	started    bool
}

// NewAsyncLogger creates a new async logger instance
func NewAsyncLogger(enabled bool, logFilePath string) (*AsyncLogger, error) {
	logger := &AsyncLogger{
		enabled: enabled,
	}

	if !enabled {
		return logger, nil
	}

	// Create the log file
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger.logFile = logFile
	logger.csvWriter = csv.NewWriter(logFile)
	logger.logChannel = make(chan RequestLogEntry, 1000) // Buffer 1000 entries

	return logger, nil
}

// Start begins the async logging process
func (al *AsyncLogger) Start() error {
	if !al.enabled || al.started {
		return nil
	}

	// Write CSV header
	if err := al.csvWriter.Write([]string{"Date", "Status", "Request", "Response"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Start the async logging goroutine
	al.waitGroup.Add(1)
	go al.logWorker()

	al.started = true
	return nil
}

// logWorker processes log entries asynchronously
func (al *AsyncLogger) logWorker() {
	defer al.waitGroup.Done()
	defer al.csvWriter.Flush()

	for entry := range al.logChannel {
		al.csvWriter.Write([]string{
			entry.Date,
			fmt.Sprintf("%d", entry.Status),
			entry.Request,
			entry.Response,
		})

		// Flush periodically to avoid losing data
		al.csvWriter.Flush()
	}
}

// Log queues a log entry for async processing
func (al *AsyncLogger) Log(entry RequestLogEntry) {
	if !al.enabled || !al.started {
		return
	}

	// Non-blocking send to avoid affecting latency
	select {
	case al.logChannel <- entry:
		// Successfully queued for logging
	default:
		// Channel is full, skip this log entry to avoid blocking
		// Could add a counter here to track dropped entries if needed
	}
}

// LogRequest is a convenience method to log a request/response
func (al *AsyncLogger) LogRequest(statusCode int, requestBody, responseBody string) {
	if !al.enabled {
		return
	}

	entry := RequestLogEntry{
		Date:     time.Now().Format("2006-01-02 15:04:05"),
		Status:   statusCode,
		Request:  requestBody,
		Response: responseBody,
	}

	al.Log(entry)
}

// Stop closes the logger and waits for all entries to be written
func (al *AsyncLogger) Stop() error {
	if !al.enabled || !al.started {
		return nil
	}

	// Close the channel and wait for all entries to be processed
	close(al.logChannel)
	al.waitGroup.Wait()

	// Close the file
	if al.logFile != nil {
		if err := al.logFile.Close(); err != nil {
			return fmt.Errorf("failed to close log file: %w", err)
		}
	}

	al.started = false
	return nil
}

// IsEnabled returns whether logging is enabled
func (al *AsyncLogger) IsEnabled() bool {
	return al.enabled
}



func loadConfigFromFile(filename, environment string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var yamlConfig MutationConfig
	if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Check if environment exists
	envConfig, exists := yamlConfig.Environments[environment]
	if !exists {
		return nil, fmt.Errorf("environment '%s' not found in config file", environment)
	}

	// Set defaults
	if envConfig.Auth.Header == "" {
		envConfig.Auth.Header = "Authorization"
	}
	if yamlConfig.Load.Concurrency == 0 {
		yamlConfig.Load.Concurrency = 10
	}
	if yamlConfig.Load.Requests == 0 {
		yamlConfig.Load.Requests = 100
	}

	// Validate required fields
	if envConfig.URL == "" {
		return nil, fmt.Errorf("URL is required for environment '%s' in config file", environment)
	}
	if yamlConfig.Query == "" {
		return nil, fmt.Errorf("Query/Mutation is required in config file")
	}
	if envConfig.Auth.Value == "" {
		return nil, fmt.Errorf("auth value is required for environment '%s' in config file", environment)
	}

	// Set default log file if logging is enabled but no file specified
	logFile := yamlConfig.Logging.LogFile
	if yamlConfig.Logging.Enabled && logFile == "" {
		timestamp := time.Now().Format("20060102_150405")
		logFile = fmt.Sprintf("results/%s/request_log_%s.jsonl", environment, timestamp)
	}

	config := &Config{
		URL:           envConfig.URL,
		Mutation:      yamlConfig.Query,
		AuthHeader:    envConfig.Auth.Header,
		AuthValue:     envConfig.Auth.Value,
		BaseAuthValue: envConfig.Auth.Value,
		Concurrency:   yamlConfig.Load.Concurrency,
		TotalReqs:     yamlConfig.Load.Requests,
		BaseVariables: yamlConfig.Variables,
		ShowProgress:  true,
		SaveResults:   true,
		OutputDir:     fmt.Sprintf("results/%s", environment),
		LogRequests:   yamlConfig.Logging.Enabled,
		LogFile:       logFile,
	}

	return config, nil
}

func setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\n%s%sReceived interrupt signal, finishing current requests...%s\n", ColorBold, ColorYellow, ColorReset)
		atomic.StoreInt32(&gracefulShutdown, 1)

		time.Sleep(2 * time.Second)

		if testResults != nil {
			fmt.Printf("%s%sTest interrupted - showing partial results:%s\n", ColorBold, ColorYellow, ColorReset)
			printResults(testResults)

			if testConfig != nil && testConfig.SaveResults {
				if err := saveResults(testResults, testConfig); err != nil {
					fmt.Printf("%sError saving results: %v%s\n", ColorRed, err, ColorReset)
				}
			}
		}

		os.Exit(0)
	}()
}

// makeRequest performs a single HTTP request and returns the result
func makeRequest(client *http.Client, url string, payload []byte, authHeader, authValue string, logRequests bool) RequestResult {
	start := time.Now()

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return RequestResult{
			Duration: time.Since(start),
			Error:    err,
			Success:  false,
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authHeader, authValue)

	resp, err := client.Do(req)
	if err != nil {
		return RequestResult{
			Duration: time.Since(start),
			Error:    err,
			Success:  false,
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	var gqlResp GraphQLResponse
	success := resp.StatusCode == 200

	if success {
		if err := json.Unmarshal(body, &gqlResp); err == nil {
			success = len(gqlResp.Errors) == 0
		}
	}

	result := RequestResult{
		Duration:   duration,
		StatusCode: resp.StatusCode,
		Success:    success,
	}

	// Include request/response bodies if logging is enabled
	if logRequests {
		result.RequestBody = string(payload)
		result.ResponseBody = string(body)
	}

	return result
}
