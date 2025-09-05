package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/time/rate"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

type Config struct {
	URL           string
	Mutation      string
	AuthHeader    string
	AuthValue     string
	BaseAuthValue string
	Headers       map[string]string
	Concurrency   int
	TotalReqs     int
	TargetRPS     int
	DurationSec   int
	BaseVariables map[string]interface{}
	ShowProgress  bool
	SaveResults   bool
	OutputDir     string
	LogRequests   bool
	LogFile       string
}

type EnvConfig struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Auth    struct {
		Header string `yaml:"header"`
		Value  string `yaml:"value"`
	} `yaml:"auth"`
}

type MutationConfig struct {
	Name         string                    `yaml:"name"`
	Description  string                    `yaml:"description"`
	Environments map[string]EnvConfig      `yaml:"environments"`
	Query        string                    `yaml:"query"`
	Variables    map[string]interface{}    `yaml:"variables"`
	Headers      map[string]string         `yaml:"headers"`
	Load         struct {
		Concurrency   int `yaml:"concurrency"`
		Requests      int `yaml:"requests"`
		RPS           int `yaml:"rps"`
		DurationSec   int `yaml:"duration_seconds"`
	} `yaml:"load"`
	Logging struct {
		Enabled bool   `yaml:"enabled"`
		LogFile string `yaml:"file"`
	} `yaml:"logging"`
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type RequestResult struct {
	Duration     time.Duration
	StatusCode   int
	Error        error
	Success      bool
	RequestBody  string
	ResponseBody string
}

type TestResults struct {
	Metrics        *tachymeter.Metrics
	TotalRequests  int
	SuccessfulReqs int
	FailedReqs     int
	StatusCodes    map[int]int
}

type ResultsOutput struct {
	Timestamp   string            `json:"timestamp"`
	TestConfig  TestConfigSummary `json:"test_config"`
	Summary     ResultsSummary    `json:"summary"`
	Latency     LatencyMetrics    `json:"latency"`
	Percentiles PercentileMetrics `json:"percentiles"`
	StatusCodes map[int]int       `json:"status_codes"`
}

type TestConfigSummary struct {
	URL         string `json:"url"`
	Concurrency int    `json:"concurrency"`
	TotalReqs   int    `json:"total_requests"`
	TargetRPS   int    `json:"target_rps"`
	DurationSec int    `json:"duration_seconds"`
}

type ResultsSummary struct {
	TotalRequests  int     `json:"total_requests"`
	SuccessfulReqs int     `json:"successful_requests"`
	FailedReqs     int     `json:"failed_requests"`
	SuccessRate    float64 `json:"success_rate_percent"`
	RequestsPerSec float64 `json:"requests_per_second"`
}

type LatencyMetrics struct {
	Average      string `json:"average"`
	HarmonicMean string `json:"harmonic_mean"`
	Minimum      string `json:"minimum"`
	Maximum      string `json:"maximum"`
	Range        string `json:"range"`
	StandardDev  string `json:"standard_deviation"`
}

type PercentileMetrics struct {
	P50  string `json:"p50"`
	P75  string `json:"p75"`
	P95  string `json:"p95"`
	P99  string `json:"p99"`
	P999 string `json:"p999"`
}

var (
	gracefulShutdown int32
	testResults      *TestResults
	testConfig       *Config
)

func main() {
	var configFile string
	var environment string
	flag.StringVar(&configFile, "config", "", "Path to YAML configuration file (required)")
	flag.StringVar(&environment, "env", "", "Environment to use from config file (required)")
	flag.Parse()

	if configFile == "" {
		fmt.Printf("%sError: Config file is required%s\n", ColorRed, ColorReset)
		flag.Usage()
		os.Exit(1)
	}

	if environment == "" {
		fmt.Printf("%sError: Environment is required%s\n", ColorRed, ColorReset)
		flag.Usage()
		os.Exit(1)
	}

	config, err := loadConfigFromFile(configFile, environment)
	if err != nil {
		fmt.Printf("%sError loading config file: %v%s\n", ColorRed, err, ColorReset)
		os.Exit(1)
	}

	testConfig = config

	setupSignalHandling()

	fmt.Printf("%s%s=== configuration ===%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%sconcurrency:%s %d\n", ColorBlue, ColorReset, config.Concurrency)
	fmt.Printf("%stotal requests:%s %d\n", ColorBlue, ColorReset, config.TotalReqs)
	if config.TargetRPS > 0 {
		fmt.Printf("%starget rps:%s %d\n", ColorBlue, ColorReset, config.TargetRPS)
	}
	if config.DurationSec > 0 {
		fmt.Printf("%sduration (s):%s %d\n", ColorBlue, ColorReset, config.DurationSec)
	}
	if config.SaveResults {
		fmt.Printf("%soutput directory:%s %s\n", ColorBlue, ColorReset, config.OutputDir)
	}
	if config.LogRequests {
		fmt.Printf("%srequest logging:%s %senabled%s -> %s\n", ColorBlue, ColorReset, ColorGreen, ColorReset, config.LogFile)
	}
	fmt.Println()

	results := runLoadTest(config)
	testResults = results
	printResults(results)

	if config.SaveResults {
		if err := saveResults(results, config); err != nil {
			fmt.Printf("%sError saving results: %v%s\n", ColorRed, err, ColorReset)
		}
	}
}

func runLoadTest(config *Config) *TestResults {
	windowSize := 10000
	if config.TotalReqs < windowSize {
		windowSize = config.TotalReqs
	}

	if config.TargetRPS > 0 && config.Concurrency <= 0 {
		estP95 := 300 * time.Millisecond // adjust if you know better; or do a quick warm-up probe
		config.Concurrency = int(math.Ceil(float64(config.TargetRPS) * estP95.Seconds()))
		if config.Concurrency < 1 {
			config.Concurrency = 1
		}
		fmt.Printf("%sderived concurrency:%s %d (from %d rps @ ~%s p95)\n",
			ColorBlue, ColorReset, config.Concurrency, config.TargetRPS, estP95)
	}

	t := tachymeter.New(&tachymeter.Config{Size: windowSize})

	var mu sync.Mutex
	var wg sync.WaitGroup

	successCount := 0
	failedCount := 0
	statusCodes := make(map[int]int)

	var bar *progressbar.ProgressBar
	var progressMu sync.Mutex
	var completedRequests int64

	transport := &http.Transport{
		MaxIdleConns:        10000,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: config.Concurrency * 2,
		MaxConnsPerHost:     config.Concurrency * 2,
		ForceAttemptHTTP2:   true,
		DisableCompression:  true,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	wallTimeStart := time.Now()

	semaphore := make(chan struct{}, config.Concurrency)

	if config.ShowProgress {
		bar = progressbar.NewOptions(config.TotalReqs,
			progressbar.OptionUseANSICodes(false),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowIts(),
			progressbar.OptionShowCount(),
			progressbar.OptionSetDescription("load testing"),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "|",
				BarEnd:        "|",
			}),
			progressbar.OptionShowElapsedTimeOnFinish(),
		)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer cancel()

	var logger *AsyncLogger

	if config.LogRequests {
		// Ensure output directory exists
		if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
			fmt.Printf("%sWarning: Failed to create output directory for logs: %v%s\n", ColorYellow, err, ColorReset)
		} else {
			var err error
			logger, err = NewAsyncLogger(config.LogRequests, config.LogFile)
			if err != nil {
				fmt.Printf("%sWarning: Failed to create logger: %v%s\n", ColorYellow, err, ColorReset)
				config.LogRequests = false
			} else {
				if err := logger.Start(); err != nil {
					fmt.Printf("%sWarning: Failed to start logger: %v%s\n", ColorYellow, err, ColorReset)
					config.LogRequests = false
				} else {
					defer logger.Stop()
				}
			}
		}
	}

	var limiter *rate.Limiter
	if config.TargetRPS > 0 {
		limiter = rate.NewLimiter(rate.Limit(config.TargetRPS), config.TargetRPS)
	}

	for requestCount := 0; requestCount < config.TotalReqs; requestCount++ {
		if atomic.LoadInt32(&gracefulShutdown) == 1 {
			fmt.Printf("\n%sshutting down gracefully...%s\n", ColorYellow, ColorReset)
			break
		}

		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				break
			}
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
			}
			defer func() { <-semaphore }()

			if atomic.LoadInt32(&gracefulShutdown) == 1 {
				return
			}

			var variables map[string]interface{}
			if config.BaseVariables != nil {
				variables = generateRandomVariables(config.BaseVariables)
			}

			authValue := config.BaseAuthValue
			if authValue != "" {
				processed := replaceRandomPlaceholders(authValue)
				if str, ok := processed.(string); ok {
					authValue = str
				} else {
					authValue = fmt.Sprintf("%v", processed)
				}
			}

			payload := GraphQLRequest{
				Query:     config.Mutation,
				Variables: variables,
			}

			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				fmt.Printf("%sError marshaling payload: %v%s\n", ColorRed, err, ColorReset)
				return
			}

			result := makeRequest(client, config.URL, payloadBytes, config.AuthHeader, authValue, config.Headers, config.LogRequests)

			if logger != nil && logger.IsEnabled() {
				logger.LogRequest(result.StatusCode, result.RequestBody, result.ResponseBody)
			}

			t.AddTime(result.Duration)

			mu.Lock()
			if result.Success {
				successCount++
			} else {
				failedCount++
			}
			statusCodes[result.StatusCode]++

			testResults = &TestResults{
				Metrics:        t.Calc(),
				TotalRequests:  successCount + failedCount,
				SuccessfulReqs: successCount,
				FailedReqs:     failedCount,
				StatusCodes:    statusCodes,
			}
			mu.Unlock()

			if config.ShowProgress {
				progressMu.Lock()
				completedRequests++
				bar.Set64(completedRequests)
				progressMu.Unlock()
			}
		}()
	}

	wg.Wait()

	if config.ShowProgress {
		bar.Finish()
		fmt.Println()
	}

	wallTime := time.Since(wallTimeStart)
	t.SetWallTime(wallTime)

	fmt.Printf("%stest completed in %s%v%s\n", ColorGreen, ColorBold, wallTime, ColorReset)
	fmt.Printf("%stotal requests made: %s%d%s\n\n", ColorBlue, ColorBold, successCount+failedCount, ColorReset)

	return &TestResults{
		Metrics:        t.Calc(),
		TotalRequests:  successCount + failedCount,
		SuccessfulReqs: successCount,
		FailedReqs:     failedCount,
		StatusCodes:    statusCodes,
	}
}

func printResults(results *TestResults) {
	if results == nil {
		return
	}

	fmt.Printf("%s%s=== results ===%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%stotal requests:%s     %d\n", ColorBlue, ColorReset, results.TotalRequests)

	successRate := float64(results.SuccessfulReqs) / float64(results.TotalRequests) * 100
	successColor := ColorGreen
	if successRate < 95 {
		successColor = ColorYellow
	}
	if successRate < 80 {
		successColor = ColorRed
	}

	fmt.Printf("%ssuccessful:%s         %s%d (%.2f%%)%s\n", ColorBlue, ColorReset, successColor, results.SuccessfulReqs, successRate, ColorReset)

	failColor := ColorGreen
	if results.FailedReqs > 0 {
		failColor = ColorRed
	}
	fmt.Printf("%sfailed:%s             %s%d (%.2f%%)%s\n", ColorBlue, ColorReset, failColor, results.FailedReqs, float64(results.FailedReqs)/float64(results.TotalRequests)*100, ColorReset)
	fmt.Printf("%srequests/sec:%s       %s%.2f%s\n", ColorBlue, ColorReset, ColorBold, results.Metrics.Rate.Second, ColorReset)
	fmt.Println()

	fmt.Printf("%s%s=== latency ===%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%saverage:%s            %s\n", ColorBlue, ColorReset, results.Metrics.Time.Avg)
	fmt.Printf("%sharmonic mean:%s      %s\n", ColorBlue, ColorReset, results.Metrics.Time.HMean)
	fmt.Printf("%sminimum:%s            %s%s%s\n", ColorBlue, ColorReset, ColorGreen, results.Metrics.Time.Min, ColorReset)
	fmt.Printf("%smaximum:%s            %s%s%s\n", ColorBlue, ColorReset, ColorRed, results.Metrics.Time.Max, ColorReset)
	fmt.Printf("%srange:%s              %s\n", ColorBlue, ColorReset, results.Metrics.Time.Range)
	fmt.Printf("%sstandard deviation:%s %s\n", ColorBlue, ColorReset, results.Metrics.Time.StdDev)
	fmt.Println()

	fmt.Printf("%s%s=== percentiles ===%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%s50th percentile:%s    %s\n", ColorBlue, ColorReset, results.Metrics.Time.P50)
	fmt.Printf("%s75th percentile:%s    %s\n", ColorBlue, ColorReset, results.Metrics.Time.P75)
	fmt.Printf("%s95th percentile:%s    %s%s%s\n", ColorBlue, ColorReset, ColorYellow, results.Metrics.Time.P95, ColorReset)
	fmt.Printf("%s99th percentile:%s    %s%s%s\n", ColorBlue, ColorReset, ColorRed, results.Metrics.Time.P99, ColorReset)
	fmt.Printf("%s99.9th percentile:%s  %s%s%s\n", ColorBlue, ColorReset, ColorRed, results.Metrics.Time.P999, ColorReset)
	fmt.Println()

	fmt.Printf("%s%s=== status codes ===%s\n", ColorBold, ColorCyan, ColorReset)
	for code, count := range results.StatusCodes {
		percentage := float64(count) / float64(results.TotalRequests) * 100

		var codeColor string
		if code >= 200 && code < 300 {
			codeColor = ColorGreen
		} else if code >= 400 && code < 500 {
			codeColor = ColorYellow
		} else if code >= 500 {
			codeColor = ColorRed
		} else {
			codeColor = ColorWhite
		}

		fmt.Printf("%s%d:%s %s%d (%.2f%%)%s\n", codeColor, code, ColorReset, codeColor, count, percentage, ColorReset)
	}
	fmt.Println()
}

func saveResults(results *TestResults, config *Config) error {
	if results == nil {
		return fmt.Errorf("no results to save")
	}

	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s/loadtest_results_%s.json", config.OutputDir, timestamp)

	output := ResultsOutput{
		Timestamp: time.Now().Format(time.RFC3339),
		TestConfig: TestConfigSummary{
			URL:         config.URL,
			Concurrency: config.Concurrency,
			TotalReqs:   config.TotalReqs,
			TargetRPS:   config.TargetRPS,
			DurationSec: config.DurationSec,
		},
		Summary: ResultsSummary{
			TotalRequests:  results.TotalRequests,
			SuccessfulReqs: results.SuccessfulReqs,
			FailedReqs:     results.FailedReqs,
			SuccessRate:    float64(results.SuccessfulReqs) / float64(results.TotalRequests) * 100,
			RequestsPerSec: results.Metrics.Rate.Second,
		},
		Latency: LatencyMetrics{
			Average:      results.Metrics.Time.Avg.String(),
			HarmonicMean: results.Metrics.Time.HMean.String(),
			Minimum:      results.Metrics.Time.Min.String(),
			Maximum:      results.Metrics.Time.Max.String(),
			Range:        results.Metrics.Time.Range.String(),
			StandardDev:  results.Metrics.Time.StdDev.String(),
		},
		Percentiles: PercentileMetrics{
			P50:  results.Metrics.Time.P50.String(),
			P75:  results.Metrics.Time.P75.String(),
			P95:  results.Metrics.Time.P95.String(),
			P99:  results.Metrics.Time.P99.String(),
			P999: results.Metrics.Time.P999.String(),
		},
		StatusCodes: results.StatusCodes,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results to JSON: %w", err)
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write results to file: %w", err)
	}

	fmt.Printf("%sresults saved to: %s%s%s\n", ColorGreen, ColorBold, filename, ColorReset)
	return nil
}
