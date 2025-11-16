package cmd

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	vegeta "github.com/tsenart/vegeta/lib"
)

type BenchmarkConfig struct {
	Host              string
	BackendKey        string
	FrontendKey       string
	UserID            string
	RPS               int
	Duration          time.Duration
	PayloadSize       int
	Pattern           string
	ThreadsCount      int
	MessagesPerThread int
	UsePagination     bool
}

type BenchmarkMetrics struct {
	TotalRequests int64
	SuccessCount  int64
	FailCount     int64
	TotalDuration int64 // in nanoseconds
	MinDuration   int64 // in nanoseconds
	MaxDuration   int64 // in nanoseconds
	Durations     []time.Duration
	mu            sync.Mutex
	StatusCodes   map[int]int
	BytesSent     int64
	StartTime     time.Time
	EndTime       time.Time
}

// Helper function to get environment variable with fallback
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// benchmarkCmd represents the benchmark command
var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run performance benchmarks against ProgressDB service",
	Long: `Run performance benchmarks to test ProgressDB service throughput and latency.
Supports different patterns like creating threads or sending messages to existing threads.`,
	RunE: runBenchmark,
}

var (
	auto              bool
	benchHost         string
	benchKey          string
	frontKey          string
	benchUser         string
	benchRPS          int
	benchDur          time.Duration
	benchSize         int
	benchPat          string
	threadsCount      int
	messagesPerThread int
	usePagination     bool
)

func init() {
	rootCmd.AddCommand(benchmarkCmd)

	benchmarkCmd.Flags().BoolVar(&auto, "auto", false, "Use default values and only prompt for pattern")
	benchmarkCmd.Flags().StringVar(&benchHost, "host", "http://localhost:8080", "ProgressDB service host")
	benchmarkCmd.Flags().StringVar(&benchKey, "backend-key", "sk_example", "Backend API key")
	benchmarkCmd.Flags().StringVar(&frontKey, "frontend-key", "pk_example", "Frontend API key")
	benchmarkCmd.Flags().StringVar(&benchUser, "user", "user1", "User ID for benchmark")
	benchmarkCmd.Flags().IntVar(&benchRPS, "rps", 1000, "Requests per second")
	benchmarkCmd.Flags().DurationVar(&benchDur, "duration", time.Minute, "Benchmark duration")
	benchmarkCmd.Flags().IntVar(&benchSize, "payload-size", 30, "Payload size in KB")
	benchmarkCmd.Flags().StringVar(&benchPat, "pattern", "thread_with_messages", "Benchmark pattern (create_threads, thread_with_messages, read_threads, read_messages, read_mixed)")
	benchmarkCmd.Flags().IntVar(&threadsCount, "threads-count", 100, "Number of threads to create for read benchmarks")
	benchmarkCmd.Flags().IntVar(&messagesPerThread, "messages-per-thread", 20, "Number of messages per thread for read benchmarks")
	benchmarkCmd.Flags().BoolVar(&usePagination, "use-pagination", true, "Use pagination in read benchmarks")
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Load configuration from environment variables with fallback to flag defaults
	host := getEnvOrDefault("PROGRESSDB_HOST", benchHost)
	backendKey := getEnvOrDefault("PROGRESSDB_BACKEND_KEY", benchKey)
	frontendKey := getEnvOrDefault("PROGRESSDB_FRONTEND_KEY", frontKey)
	userID := getEnvOrDefault("PROGRESSDB_USER", benchUser)

	if rpsStr := os.Getenv("PROGRESSDB_RPS"); rpsStr != "" {
		if rps, err := strconv.Atoi(rpsStr); err == nil {
			benchRPS = rps
		}
	}

	if durStr := os.Getenv("PROGRESSDB_DURATION"); durStr != "" {
		if dur, err := time.ParseDuration(durStr); err == nil {
			benchDur = dur
		}
	}

	if sizeStr := os.Getenv("PROGRESSDB_PAYLOAD_SIZE"); sizeStr != "" {
		if size, err := strconv.Atoi(sizeStr); err == nil {
			benchSize = size
		}
	}

	if pattern := os.Getenv("PROGRESSDB_PATTERN"); pattern != "" {
		benchPat = pattern
	}

	cfg := BenchmarkConfig{
		Host:              host,
		BackendKey:        backendKey,
		FrontendKey:       frontendKey,
		UserID:            userID,
		RPS:               benchRPS,
		Duration:          benchDur,
		PayloadSize:       benchSize,
		ThreadsCount:      threadsCount,
		MessagesPerThread: messagesPerThread,
		UsePagination:     usePagination,
	}

	// Use pattern from environment if set, otherwise use default behavior
	if cfg.Pattern == "" {
		if !auto {
			cfg = promptBenchmarkConfig(cfg)
		} else {
			cfg.Pattern = promptBenchmarkPattern()
		}
	}

	if verbose {
		fmt.Printf("Starting benchmark with config:\n")
		fmt.Printf("  Host: %s\n", cfg.Host)
		fmt.Printf("  User: %s\n", cfg.UserID)
		fmt.Printf("  RPS: %d\n", cfg.RPS)
		fmt.Printf("  Duration: %v\n", cfg.Duration)
		fmt.Printf("  Pattern: %s\n", cfg.Pattern)
		fmt.Printf("  Payload Size: %d KB\n", cfg.PayloadSize)
		if cfg.Pattern == "read_threads" || cfg.Pattern == "read_messages" || cfg.Pattern == "read_mixed" {
			fmt.Printf("  Threads Count: %d\n", cfg.ThreadsCount)
			fmt.Printf("  Messages Per Thread: %d\n", cfg.MessagesPerThread)
			fmt.Printf("  Use Pagination: %t\n", cfg.UsePagination)
		}
		fmt.Printf("  Workers: %d (CPU cores)\n", runtime.NumCPU())
		fmt.Println()
	}

	signature := fetchSignature(cfg)
	var metrics *BenchmarkMetrics
	switch cfg.Pattern {
	case "create_threads":
		metrics = runCreateThreadsBenchmark(cfg, signature)
	case "thread_with_messages":
		metrics = runThreadWithMessagesBenchmark(cfg, signature)
	case "read_threads", "read_messages", "read_mixed":
		metrics = runReadBenchmark(cfg, signature)
	default:
		log.Fatalf("Unknown benchmark pattern: %s", cfg.Pattern)
	}
	outputBenchmarkMetrics(metrics)

	return nil
}

func promptBenchmarkConfig(cfg BenchmarkConfig) BenchmarkConfig {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("Host endpoint [%s]: ", cfg.Host)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			cfg.Host = input
		}
	}

	fmt.Printf("Backend API key [%s]: ", cfg.BackendKey)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			cfg.BackendKey = input
		}
	}

	fmt.Printf("Frontend API key [%s]: ", cfg.FrontendKey)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			cfg.FrontendKey = input
		}
	}

	fmt.Printf("User ID [%s]: ", cfg.UserID)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			cfg.UserID = input
		}
	}

	fmt.Printf("Requests per second [%d]: ", cfg.RPS)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			if rps, err := strconv.Atoi(input); err == nil {
				cfg.RPS = rps
			}
		}
	}

	fmt.Printf("Duration (e.g. 1m, 30s) [%s]: ", cfg.Duration)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			if dur, err := time.ParseDuration(input); err == nil {
				cfg.Duration = dur
			}
		}
	}

	fmt.Printf("Pattern (create_threads, thread_with_messages, read_threads, read_messages, read_mixed) [thread_with_messages]: ")
	cfg.Pattern = "thread_with_messages"
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			validPatterns := []string{"create_threads", "thread_with_messages", "read_threads", "read_messages", "read_mixed"}
			for _, pattern := range validPatterns {
				if input == pattern {
					cfg.Pattern = input
					break
				}
			}
		}
	}

	fmt.Printf("Payload size (KB) [%d]: ", cfg.PayloadSize)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			if size, err := strconv.Atoi(input); err == nil {
				cfg.PayloadSize = size
			}
		}
	}

	// Only show read-specific options for read patterns
	if cfg.Pattern == "read_threads" || cfg.Pattern == "read_messages" || cfg.Pattern == "read_mixed" {
		fmt.Printf("Threads count for read benchmarks [%d]: ", cfg.ThreadsCount)
		if scanner.Scan() {
			if input := strings.TrimSpace(scanner.Text()); input != "" {
				if count, err := strconv.Atoi(input); err == nil {
					cfg.ThreadsCount = count
				}
			}
		}

		fmt.Printf("Messages per thread for read benchmarks [%d]: ", cfg.MessagesPerThread)
		if scanner.Scan() {
			if input := strings.TrimSpace(scanner.Text()); input != "" {
				if count, err := strconv.Atoi(input); err == nil {
					cfg.MessagesPerThread = count
				}
			}
		}

		fmt.Printf("Use pagination in reads [%t]: ", cfg.UsePagination)
		if scanner.Scan() {
			if input := strings.TrimSpace(scanner.Text()); input != "" {
				if usePag, err := strconv.ParseBool(input); err == nil {
					cfg.UsePagination = usePag
				}
			}
		}
	}

	return cfg
}

func promptBenchmarkPattern() string {
	fmt.Println("Choose pattern:")
	fmt.Println("1. create_threads")
	fmt.Println("2. thread_with_messages")
	fmt.Println("3. read_threads")
	fmt.Println("4. read_messages")
	fmt.Println("5. read_mixed")
	fmt.Print("Enter 1-5: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			return "create_threads"
		case "2":
			return "thread_with_messages"
		case "3":
			return "read_threads"
		case "4":
			return "read_messages"
		case "5":
			return "read_mixed"
		}
	}
	// Default
	return "thread_with_messages"
}

func fetchSignature(cfg BenchmarkConfig) string {
	url := cfg.Host + "/backend/v1/sign"
	payload := fmt.Sprintf(`{"userId":"%s"}`, cfg.UserID)
	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		log.Fatal("Failed to create signature request:", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.BackendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Failed to fetch signature:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatal("Signature request failed with status:", resp.StatusCode, "body:", string(body))
	}

	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal("Failed to decode signature response:", err)
	}

	return result.Signature
}

func runCreateThreadsBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	metrics := &BenchmarkMetrics{StartTime: time.Now()}

	// Calculate total requests
	totalRequests := cfg.RPS * int(cfg.Duration.Seconds())
	targets := make([]vegeta.Target, 0, totalRequests)

	// Pre-generate targets
	for i := 0; i < totalRequests; i++ {
		title := fmt.Sprintf("bench-thread-%d", time.Now().UnixNano()+int64(i))
		payload := fmt.Sprintf(`{"title":"%s"}`, title)

		target := vegeta.Target{
			Method: "POST",
			URL:    cfg.Host + "/frontend/v1/threads",
			Body:   []byte(payload),
			Header: map[string][]string{
				"Authorization":    {"Bearer " + cfg.FrontendKey},
				"Content-Type":     {"application/json"},
				"X-User-ID":        {cfg.UserID},
				"X-User-Signature": {signature},
			},
		}
		targets = append(targets, target)
	}

	// Setup Vegeta attacker
	targeter := vegeta.NewStaticTargeter(targets...)
	rate := vegeta.Rate{Freq: cfg.RPS, Per: time.Second}
	attacker := vegeta.NewAttacker(vegeta.Workers(uint64(runtime.NumCPU())))

	// Run attack with live stats
	results := &vegeta.Metrics{}
	resChan := attacker.Attack(targeter, rate, cfg.Duration, "create_threads")

	// Live stats
	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint, resChan)

	// Collect results
	for res := range resChan {
		results.Add(res)
	}
	close(stopPrint)
	results.Close()

	metrics.EndTime = time.Now()
	return convertVegetaMetrics(results, metrics)
}

func runThreadWithMessagesBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	metrics := &BenchmarkMetrics{StartTime: time.Now()}

	// First, create a single thread
	var threadID string
	createThreadSync(cfg, signature, &threadID)
	if threadID == "" {
		log.Fatal("Failed to create initial thread")
	}

	// Calculate total requests
	totalRequests := cfg.RPS * int(cfg.Duration.Seconds())
	targets := make([]vegeta.Target, 0, totalRequests)

	// Pre-generate targets for messages
	for i := 0; i < totalRequests; i++ {
		content, checksum := generatePayload(cfg.PayloadSize)
		messageID := fmt.Sprintf("msg-%d-%s", time.Now().UnixNano()+int64(i), randomString(9))
		payload := fmt.Sprintf(`{"id":"%s","content":"%s","checksum":"%s","body":{}}`, messageID, content, checksum)

		target := vegeta.Target{
			Method: "POST",
			URL:    fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID),
			Body:   []byte(payload),
			Header: map[string][]string{
				"Authorization":    {"Bearer " + cfg.FrontendKey},
				"Content-Type":     {"application/json"},
				"X-User-ID":        {cfg.UserID},
				"X-User-Signature": {signature},
			},
		}
		targets = append(targets, target)
	}

	// Setup Vegeta attacker
	targeter := vegeta.NewStaticTargeter(targets...)
	rate := vegeta.Rate{Freq: cfg.RPS, Per: time.Second}
	attacker := vegeta.NewAttacker(vegeta.Workers(uint64(runtime.NumCPU())))

	// Run attack with live stats
	results := &vegeta.Metrics{}
	resChan := attacker.Attack(targeter, rate, cfg.Duration, "thread_with_messages")

	// Live stats
	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint, resChan)

	// Collect results
	for res := range resChan {
		results.Add(res)
	}
	close(stopPrint)
	results.Close()

	metrics.EndTime = time.Now()
	return convertVegetaMetrics(results, metrics)
}

func runReadBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	fmt.Printf("Loading test data: creating %d threads with %d messages each...\n", cfg.ThreadsCount, cfg.MessagesPerThread)

	// Load test data using existing functions
	threadIDs := loadTestData(cfg, signature)
	if len(threadIDs) == 0 {
		log.Fatal("Failed to load test data")
	}
	fmt.Printf("Created %d threads with messages for benchmarking\n", len(threadIDs))

	metrics := &BenchmarkMetrics{StartTime: time.Now()}

	// Calculate total requests
	totalRequests := cfg.RPS * int(cfg.Duration.Seconds())
	targets := make([]vegeta.Target, 0, totalRequests)

	// Pre-generate targets for reads
	for i := 0; i < totalRequests; i++ {
		var target vegeta.Target
		switch cfg.Pattern {
		case "read_threads":
			url := cfg.Host + "/frontend/v1/threads"
			if cfg.UsePagination {
				url += "?limit=50"
			}
			target = vegeta.Target{
				Method: "GET",
				URL:    url,
				Header: map[string][]string{
					"Authorization":    {"Bearer " + cfg.FrontendKey},
					"X-User-ID":        {cfg.UserID},
					"X-User-Signature": {signature},
				},
			}
		case "read_messages":
			threadIndex := i % len(threadIDs)
			threadID := threadIDs[threadIndex]
			url := fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID)
			if cfg.UsePagination {
				url += "?limit=20"
			}
			target = vegeta.Target{
				Method: "GET",
				URL:    url,
				Header: map[string][]string{
					"Authorization":    {"Bearer " + cfg.FrontendKey},
					"X-User-ID":        {cfg.UserID},
					"X-User-Signature": {signature},
				},
			}
		case "read_mixed":
			if i%2 == 0 {
				url := cfg.Host + "/frontend/v1/threads"
				if cfg.UsePagination {
					url += "?limit=50"
				}
				target = vegeta.Target{
					Method: "GET",
					URL:    url,
					Header: map[string][]string{
						"Authorization":    {"Bearer " + cfg.FrontendKey},
						"X-User-ID":        {cfg.UserID},
						"X-User-Signature": {signature},
					},
				}
			} else {
				threadIndex := i % len(threadIDs)
				threadID := threadIDs[threadIndex]
				url := fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID)
				if cfg.UsePagination {
					url += "?limit=20"
				}
				target = vegeta.Target{
					Method: "GET",
					URL:    url,
					Header: map[string][]string{
						"Authorization":    {"Bearer " + cfg.FrontendKey},
						"X-User-ID":        {cfg.UserID},
						"X-User-Signature": {signature},
					},
				}
			}
		}
		targets = append(targets, target)
	}

	// Setup Vegeta attacker
	targeter := vegeta.NewStaticTargeter(targets...)
	rate := vegeta.Rate{Freq: cfg.RPS, Per: time.Second}
	attacker := vegeta.NewAttacker(vegeta.Workers(uint64(runtime.NumCPU())))

	// Run attack with live stats
	results := &vegeta.Metrics{}
	resChan := attacker.Attack(targeter, rate, cfg.Duration, "read_benchmark")

	// Live stats
	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint, resChan)

	// Collect results
	for res := range resChan {
		results.Add(res)
	}
	close(stopPrint)
	results.Close()

	metrics.EndTime = time.Now()
	return convertVegetaMetrics(results, metrics)
}

func createThreadSync(cfg BenchmarkConfig, signature string, threadID *string) {
	url := cfg.Host + "/frontend/v1/threads"
	title := fmt.Sprintf("bench-thread-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"title":"%s"}`, title)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Failed to create thread:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		log.Fatal("Thread creation failed with status:", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Key == "" {
		log.Fatal("Failed to parse thread creation response")
	}
	*threadID = result.Key
}

func generatePayload(sizeKB int) (string, string) {
	size := sizeKB * 1024
	data := make([]byte, size)
	rand.Read(data)
	content := hex.EncodeToString(data)
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])
	return content, checksum
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}
	return string(bytes)
}

func printLiveBenchmarkStats(metrics *BenchmarkMetrics, totalDuration time.Duration, stop <-chan struct{}, resChan <-chan *vegeta.Result) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	start := metrics.StartTime

	var totalReqs int64
	var totalDur time.Duration
	var minDur, maxDur time.Duration
	var successCount int64

	for {
		select {
		case <-stop:
			fmt.Println()
			return
		case <-ticker.C:
			elapsed := time.Since(start)
			remaining := totalDuration - elapsed
			if remaining < 0 {
				remaining = 0
			}

			// Calculate current RPS
			rps := float64(totalReqs) / elapsed.Seconds()
			var avgResp time.Duration
			if totalReqs > 0 {
				avgResp = totalDur / time.Duration(totalReqs)
			}

			fmt.Printf("\rRequests: %d | RPS: %.1f | Avg: %v | Min: %v | Max: %v | Success: %d | Remaining: %v",
				totalReqs, rps, avgResp, minDur, maxDur, successCount, remaining.Round(time.Second))
		case res, ok := <-resChan:
			if !ok {
				return
			}
			totalReqs++
			totalDur += res.Latency

			if minDur == 0 || res.Latency < minDur {
				minDur = res.Latency
			}
			if res.Latency > maxDur {
				maxDur = res.Latency
			}

			if res.Code >= 200 && res.Code < 400 {
				successCount++
			}
		}
	}
}

func convertVegetaMetrics(results *vegeta.Metrics, metrics *BenchmarkMetrics) *BenchmarkMetrics {
	metrics.TotalRequests = int64(results.Requests)
	metrics.SuccessCount = int64(results.Success)
	metrics.FailCount = int64(len(results.Errors))
	metrics.TotalDuration = int64(results.Latencies.Mean)
	metrics.MinDuration = int64(results.Latencies.P50) // Use P50 as min approximation
	metrics.MaxDuration = int64(results.Latencies.P99) // Use P99 as max approximation

	// Convert Vegeta latencies to durations for percentile calculation
	metrics.Durations = make([]time.Duration, 0, int(results.Requests))
	// Note: Vegeta doesn't expose individual latencies, so we'll use the histogram data
	// For now, we'll store mean, min, max and calculate percentiles from Vegeta's built-in data

	return metrics
}

func loadTestData(cfg BenchmarkConfig, signature string) []string {
	var threadIDs []string
	var mu sync.Mutex

	// Create threads with messages in parallel
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent creations

	for i := 0; i < cfg.ThreadsCount; i++ {
		wg.Add(1)
		go func(threadIndex int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			// Create thread
			threadID := createThreadSyncReturn(cfg, signature)
			if threadID == "" {
				return
			}

			// Create messages for this thread
			for j := 0; j < cfg.MessagesPerThread; j++ {
				createMessageSync(cfg, signature, threadID)
			}

			mu.Lock()
			threadIDs = append(threadIDs, threadID)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	return threadIDs
}

func createThreadSyncReturn(cfg BenchmarkConfig, signature string) string {
	url := cfg.Host + "/frontend/v1/threads"
	title := fmt.Sprintf("bench-thread-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"title":"%s"}`, title)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return ""
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Key == "" {
		return ""
	}
	return result.Key
}

func createMessageSync(cfg BenchmarkConfig, signature string, threadID string) {
	url := fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID)

	content, checksum := generatePayload(1) // Small payload for data loading
	messageID := fmt.Sprintf("msg-%d-%s", time.Now().UnixNano(), randomString(9))
	payload := fmt.Sprintf(`{"id":"%s","content":"%s","checksum":"%s","body":{}}`, messageID, content, checksum)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func outputBenchmarkMetrics(metrics *BenchmarkMetrics) {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatal("Failed to get executable path:", err)
	}
	execDir := filepath.Dir(execPath)
	logsDir := filepath.Join(execDir, "logs")
	os.MkdirAll(logsDir, 0755)

	testID := fmt.Sprintf("bench-%d", time.Now().Unix())
	outFile := filepath.Join(logsDir, testID+".json")

	file, err := os.Create(outFile)
	if err != nil {
		log.Fatal("Failed to create output file:", err)
	}
	defer file.Close()

	// Calculate stats
	totalRequests := atomic.LoadInt64(&metrics.TotalRequests)
	totalDur := atomic.LoadInt64(&metrics.TotalDuration)
	minDur := atomic.LoadInt64(&metrics.MinDuration)
	maxDur := atomic.LoadInt64(&metrics.MaxDuration)
	avgDuration := time.Duration(totalDur / totalRequests)
	minDuration := time.Duration(minDur)
	maxDuration := time.Duration(maxDur)

	// For percentiles, we'll use Vegeta's built-in data if available
	// For now, use existing duration array if populated
	var p90, p95, p99 time.Duration
	if len(metrics.Durations) > 0 {
		sort.Slice(metrics.Durations, func(i, j int) bool { return metrics.Durations[i] < metrics.Durations[j] })
		n := len(metrics.Durations)
		p90 = metrics.Durations[int(float64(n)*0.9)]
		p95 = metrics.Durations[int(float64(n)*0.95)]
		p99 = metrics.Durations[int(float64(n)*0.99)]
	} else {
		// Fallback to calculated values
		p90 = avgDuration
		p95 = avgDuration
		p99 = avgDuration
	}

	result := map[string]interface{}{
		"total_requests":  atomic.LoadInt64(&metrics.TotalRequests),
		"success_count":   atomic.LoadInt64(&metrics.SuccessCount),
		"fail_count":      atomic.LoadInt64(&metrics.FailCount),
		"avg_duration_ms": avgDuration.Milliseconds(),
		"min_duration_ms": minDuration.Milliseconds(),
		"max_duration_ms": maxDuration.Milliseconds(),
		"p90_duration_ms": p90.Milliseconds(),
		"p95_duration_ms": p95.Milliseconds(),
		"p99_duration_ms": p99.Milliseconds(),
		"status_codes":    metrics.StatusCodes,
		"bytes_sent":      atomic.LoadInt64(&metrics.BytesSent),
		"start_time":      metrics.StartTime,
		"end_time":        metrics.EndTime,
		"duration":        metrics.EndTime.Sub(metrics.StartTime),
	}

	json.NewEncoder(file).Encode(result)
	fmt.Printf("Output: %s\n", outFile)
}
