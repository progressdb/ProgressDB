package cmd

import (
	"bufio"
	"context"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
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

	cfg := BenchmarkConfig{
		Host:              benchHost,
		BackendKey:        benchKey,
		FrontendKey:       frontKey,
		UserID:            benchUser,
		RPS:               benchRPS,
		Duration:          benchDur,
		PayloadSize:       benchSize,
		ThreadsCount:      threadsCount,
		MessagesPerThread: messagesPerThread,
		UsePagination:     usePagination,
	}

	if !auto {
		cfg = promptBenchmarkConfig(cfg)
	} else {
		cfg.Pattern = promptBenchmarkPattern()
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
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	currentRPS := cfg.RPS
	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint)

	for {
		select {
		case <-ctx.Done():
			close(stopPrint)
			metrics.EndTime = time.Now()
			wg.Wait()
			return metrics
		case <-ticker.C:
			// Check failure rate and throttle if needed
			totalReqs := atomic.LoadInt64(&metrics.TotalRequests)
			failCount := atomic.LoadInt64(&metrics.FailCount)
			if totalReqs > 10 && failCount*10 > totalReqs {
				newRPS := currentRPS / 2
				if newRPS > 0 && newRPS != currentRPS {
					currentRPS = newRPS
					ticker.Reset(time.Second / time.Duration(currentRPS))
					fmt.Printf("\nThrottling down to %d RPS due to high failure rate\n", currentRPS)
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				createThread(cfg, signature, metrics)
			}()
		}
	}
}

func runThreadWithMessagesBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	metrics := &BenchmarkMetrics{StartTime: time.Now()}

	// First, create a single thread
	var threadID string
	createThreadSync(cfg, signature, &threadID)
	if threadID == "" {
		log.Fatal("Failed to create initial thread")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	currentRPS := cfg.RPS
	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint)

	for {
		select {
		case <-ctx.Done():
			close(stopPrint)
			metrics.EndTime = time.Now()
			wg.Wait()
			return metrics
		case <-ticker.C:
			// Check failure rate and throttle if needed
			totalReqs := atomic.LoadInt64(&metrics.TotalRequests)
			failCount := atomic.LoadInt64(&metrics.FailCount)
			if totalReqs > 10 && failCount*10 > totalReqs {
				newRPS := currentRPS / 2
				if newRPS > 0 && newRPS != currentRPS {
					currentRPS = newRPS
					ticker.Reset(time.Second / time.Duration(currentRPS))
					fmt.Printf("\nThrottling down to %d RPS due to high failure rate\n", currentRPS)
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Create message in the single thread
				createMessage(cfg, signature, metrics, threadID)
			}()
		}
	}
}

func createThread(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics) {
	url := cfg.Host + "/frontend/v1/threads"
	title := fmt.Sprintf("bench-thread-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"title":"%s"}`, title)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		metrics.record(0, duration, int64(len(payload)), false)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	success := resp.StatusCode == 200 || resp.StatusCode == 202
	if !success {
		fmt.Printf("Error: status %d, body: %s\n", resp.StatusCode, string(body))
	}
	metrics.record(resp.StatusCode, duration, int64(len(payload)), success)
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

func createMessage(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, threadID string) {
	url := fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID)

	content, checksum := generatePayload(cfg.PayloadSize)
	messageID := fmt.Sprintf("msg-%d-%s", time.Now().UnixNano(), randomString(9))
	payload := fmt.Sprintf(`{"id":"%s","content":"%s","checksum":"%s","body":{}}`, messageID, content, checksum)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		metrics.record(0, duration, int64(len(payload)), false)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	success := resp.StatusCode == 200 || resp.StatusCode == 202
	if !success {
		fmt.Printf("Error: status %d, body: %s\n", resp.StatusCode, string(body))
	}
	metrics.record(resp.StatusCode, duration, int64(len(payload)), success)
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

func printLiveBenchmarkStats(metrics *BenchmarkMetrics, totalDuration time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	start := metrics.StartTime
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
			totalReqs := atomic.LoadInt64(&metrics.TotalRequests)
			totalDur := atomic.LoadInt64(&metrics.TotalDuration)
			minDur := atomic.LoadInt64(&metrics.MinDuration)
			maxDur := atomic.LoadInt64(&metrics.MaxDuration)
			rps := float64(totalReqs) / elapsed.Seconds()
			var avgResp time.Duration
			if totalReqs > 0 {
				avgResp = time.Duration(totalDur / totalReqs)
			}
			minResp := time.Duration(minDur)
			maxResp := time.Duration(maxDur)
			fmt.Printf("\rRequests: %d | RPS: %.1f | Avg: %v | Min: %v | Max: %v | Remaining: %v", totalReqs, rps, avgResp, minResp, maxResp, remaining.Round(time.Second))
		}
	}
}

func (m *BenchmarkMetrics) record(status int, duration time.Duration, bytes int64, success bool) {
	durNs := int64(duration)
	atomic.AddInt64(&m.TotalRequests, 1)
	atomic.AddInt64(&m.TotalDuration, durNs)
	atomic.AddInt64(&m.BytesSent, bytes)
	if success {
		atomic.AddInt64(&m.SuccessCount, 1)
	} else {
		atomic.AddInt64(&m.FailCount, 1)
	}
	// Update min/max
	for {
		min := atomic.LoadInt64(&m.MinDuration)
		if min == 0 || durNs < min {
			if atomic.CompareAndSwapInt64(&m.MinDuration, min, durNs) {
				break
			}
		} else {
			break
		}
	}
	for {
		max := atomic.LoadInt64(&m.MaxDuration)
		if durNs > max {
			if atomic.CompareAndSwapInt64(&m.MaxDuration, max, durNs) {
				break
			}
		} else {
			break
		}
	}
	// Append duration for percentiles
	m.mu.Lock()
	m.Durations = append(m.Durations, duration)
	m.mu.Unlock()
	// StatusCodes not atomic, but since it's map, need mutex for it
	// For simplicity, skip live status codes, only final
}

func runReadBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	fmt.Printf("Loading test data: creating %d threads with %d messages each...\n", cfg.ThreadsCount, cfg.MessagesPerThread)

	// Load test data
	threadIDs := loadTestData(cfg, signature)
	if len(threadIDs) == 0 {
		log.Fatal("Failed to load test data")
	}
	fmt.Printf("Created %d threads with messages for benchmarking\n", len(threadIDs))

	metrics := &BenchmarkMetrics{StartTime: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	currentRPS := cfg.RPS
	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	stopPrint := make(chan struct{})
	go printLiveBenchmarkStats(metrics, cfg.Duration, stopPrint)

	for {
		select {
		case <-ctx.Done():
			close(stopPrint)
			metrics.EndTime = time.Now()
			wg.Wait()
			return metrics
		case <-ticker.C:
			// Check failure rate and throttle if needed
			totalReqs := atomic.LoadInt64(&metrics.TotalRequests)
			failCount := atomic.LoadInt64(&metrics.FailCount)
			if totalReqs > 10 && failCount*10 > totalReqs {
				newRPS := currentRPS / 2
				if newRPS > 0 && newRPS != currentRPS {
					currentRPS = newRPS
					ticker.Reset(time.Second / time.Duration(currentRPS))
					fmt.Printf("\nThrottling down to %d RPS due to high failure rate\n", currentRPS)
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch cfg.Pattern {
				case "read_threads":
					performReadThreads(cfg, signature, metrics)
				case "read_messages":
					performReadMessages(cfg, signature, metrics, threadIDs)
				case "read_mixed":
					if time.Now().UnixNano()%2 == 0 {
						performReadThreads(cfg, signature, metrics)
					} else {
						performReadMessages(cfg, signature, metrics, threadIDs)
					}
				}
			}()
		}
	}
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

func performReadThreads(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics) {
	url := cfg.Host + "/frontend/v1/threads"

	// Add pagination parameters if enabled
	if cfg.UsePagination {
		url += "?limit=50"
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		metrics.record(0, duration, 0, false)
		return
	}
	defer resp.Body.Close()

	success := resp.StatusCode == 200
	metrics.record(resp.StatusCode, duration, 0, success)
}

func performReadMessages(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, threadIDs []string) {
	// Pick a random thread
	if len(threadIDs) == 0 {
		metrics.record(0, 0, 0, false)
		return
	}

	threadIndex := int(time.Now().UnixNano()) % len(threadIDs)
	threadID := threadIDs[threadIndex]

	url := fmt.Sprintf("%s/frontend/v1/threads/%s/messages", cfg.Host, threadID)

	// Add pagination parameters if enabled
	if cfg.UsePagination {
		url += "?limit=20"
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		metrics.record(0, duration, 0, false)
		return
	}
	defer resp.Body.Close()

	success := resp.StatusCode == 200
	metrics.record(resp.StatusCode, duration, 0, success)
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

	// Calculate percentiles
	metrics.mu.Lock()
	durations := make([]time.Duration, len(metrics.Durations))
	copy(durations, metrics.Durations)
	metrics.mu.Unlock()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	n := len(durations)
	p90 := durations[int(float64(n)*0.9)]
	p95 := durations[int(float64(n)*0.95)]
	p99 := durations[int(float64(n)*0.99)]

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
