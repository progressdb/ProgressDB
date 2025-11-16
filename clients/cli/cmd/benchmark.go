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
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/cpuid/v2"
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
	mu            sync.RWMutex
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

func printSystemInfo() {
	fmt.Println("SYSTEM INFORMATION")
	fmt.Println(strings.Repeat("-", 50))

	// CPU Information using cpuid and runtime
	fmt.Printf("CPU Details:\n")
	fmt.Printf("- Model: %s\n", cpuid.CPU.BrandName)
	fmt.Printf("- Vendor: %s\n", cpuid.CPU.VendorString)
	fmt.Printf("- Physical Cores: %d\n", cpuid.CPU.PhysicalCores)
	fmt.Printf("- Logical Cores: %d\n", cpuid.CPU.LogicalCores)
	if cpuid.CPU.PhysicalCores > 0 {
		fmt.Printf("- Threads per Core: %d\n", cpuid.CPU.LogicalCores/cpuid.CPU.PhysicalCores)
	}

	// Frequency information
	if cpuid.CPU.Hz > 0 {
		freqGHz := float64(cpuid.CPU.Hz) / 1e9
		fmt.Printf("- Base Frequency: %.2f GHz\n", freqGHz)
	}

	// Cache information
	if cpuid.CPU.Cache.L1D > 0 {
		fmt.Printf("- L1 Data Cache: %d KB\n", cpuid.CPU.Cache.L1D/1024)
	}
	if cpuid.CPU.Cache.L2 > 0 {
		fmt.Printf("- L2 Cache: %d KB\n", cpuid.CPU.Cache.L2/1024)
	}
	if cpuid.CPU.Cache.L3 > 0 {
		fmt.Printf("- L3 Cache: %d MB\n", cpuid.CPU.Cache.L3/1024/1024)
	}

	// CPU features
	features := []string{}
	if cpuid.CPU.Supports(cpuid.AVX2) {
		features = append(features, "AVX2")
	}
	if cpuid.CPU.Supports(cpuid.AVX) {
		features = append(features, "AVX")
	}
	if cpuid.CPU.Supports(cpuid.SSE4) {
		features = append(features, "SSE4")
	}
	if cpuid.CPU.Supports(cpuid.AESNI) {
		features = append(features, "AES-NI")
	}
	if len(features) > 0 {
		fmt.Printf("- Features: %s\n", strings.Join(features, ", "))
	}
	fmt.Println()

	// Memory Information using system commands
	fmt.Printf("Memory Information:\n")
	if runtime.GOOS == "darwin" {
		// macOS memory info
		if output, err := exec.Command("sysctl", "hw.memsize").Output(); err == nil {
			var memSize uint64
			fmt.Sscanf(string(output), "hw.memsize: %d", &memSize)
			totalGB := float64(memSize) / (1024 * 1024 * 1024)
			fmt.Printf("- Total Memory: %.2f GB\n", totalGB)
		}

		if output, err := exec.Command("vm_stat").Output(); err == nil {
			// Parse vm_stat for memory usage
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "Pages free:") {
					var freePages uint64
					fmt.Sscanf(line, "Pages free: %d.", &freePages)
					freeGB := float64(freePages*4096) / (1024 * 1024 * 1024)
					fmt.Printf("- Free Memory: %.2f GB\n", freeGB)
				}
			}
		}
	} else if runtime.GOOS == "linux" {
		// Linux memory info
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "MemTotal:") {
					var totalKB uint64
					fmt.Sscanf(line, "MemTotal: %d kB", &totalKB)
					fmt.Printf("- Total Memory: %.2f GB\n", float64(totalKB)/1024/1024)
				} else if strings.HasPrefix(line, "MemAvailable:") {
					var availKB uint64
					fmt.Sscanf(line, "MemAvailable: %d kB", &availKB)
					fmt.Printf("- Available Memory: %.2f GB\n", float64(availKB)/1024/1024)
				}
			}
		}
	}

	// Go runtime memory info
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("- Go Runtime Memory: %.2f MB allocated\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("- Go Runtime Sys: %.2f MB system\n", float64(m.Sys)/1024/1024)
	fmt.Println()

	// Storage Information - simplified summary
	fmt.Printf("Storage Summary:\n")
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("df", "-h", "/").Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 4 {
					fmt.Printf("- Root: %s total, %s available\n", fields[1], fields[3])
				}
			}
		}
	} else if runtime.GOOS == "linux" {
		if output, err := exec.Command("df", "-h", "/").Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 4 {
					fmt.Printf("- Root: %s total, %s available\n", fields[1], fields[3])
				}
			}
		}
	}
	fmt.Println()

	// OS Information
	fmt.Printf("Operating System:\n")
	if hostname, err := os.Hostname(); err == nil {
		fmt.Printf("- Hostname: %s\n", hostname)
	}
	fmt.Printf("- Platform: %s\n", runtime.GOOS)
	fmt.Printf("- Architecture: %s\n", runtime.GOARCH)
	fmt.Printf("- Go Version: %s\n", runtime.Version())
	fmt.Printf("- Goroutines: %d\n", runtime.NumGoroutine())
	fmt.Printf("- GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))

	// Additional OS-specific info
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			fmt.Printf("- macOS Version: %s\n", strings.TrimSpace(string(output)))
		}
		if output, err := exec.Command("uname", "-r").Output(); err == nil {
			fmt.Printf("- Kernel: %s\n", strings.TrimSpace(string(output)))
		}
	} else if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					name := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					fmt.Printf("- Distribution: %s\n", name)
					break
				}
			}
		}
		if output, err := exec.Command("uname", "-r").Output(); err == nil {
			fmt.Printf("- Kernel: %s\n", strings.TrimSpace(string(output)))
		}
	}
	fmt.Println()

	fmt.Println(strings.Repeat("-", 50))
	fmt.Println()
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Print system information for context
	printSystemInfo()

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

	// Create reusable HTTP client for better performance
	client := &http.Client{Timeout: 30 * time.Second}

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
			throttleIfNeeded(metrics, &currentRPS, ticker)
			wg.Add(1)
			go func() {
				defer wg.Done()
				createThreadWithClient(cfg, signature, metrics, client)
			}()
		}
	}
}

func runThreadWithMessagesBenchmark(cfg BenchmarkConfig, signature string) *BenchmarkMetrics {
	metrics := &BenchmarkMetrics{StartTime: time.Now()}

	// First, create a single thread
	threadID := createThreadSyncReturn(cfg, signature)
	if threadID == "" {
		log.Fatal("Failed to create initial thread")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	currentRPS := cfg.RPS
	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	// Create reusable HTTP client for better performance
	client := &http.Client{Timeout: 30 * time.Second}

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
			throttleIfNeeded(metrics, &currentRPS, ticker)
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Create message in the single thread
				createMessageWithClient(cfg, signature, metrics, threadID, client)
			}()
		}
	}
}

func createThreadWithClient(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, client *http.Client) {
	url := cfg.Host + "/frontend/v1/threads"
	title := fmt.Sprintf("bench-thread-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"title":"%s"}`, title)

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.FrontendKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", cfg.UserID)
	req.Header.Set("X-User-Signature", signature)

	start := time.Now()
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

func createMessageWithClient(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, threadID string, client *http.Client) {
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

func throttleIfNeeded(metrics *BenchmarkMetrics, currentRPS *int, ticker *time.Ticker) {
	totalReqs := atomic.LoadInt64(&metrics.TotalRequests)
	failCount := atomic.LoadInt64(&metrics.FailCount)
	if totalReqs > 10 && failCount*10 > totalReqs {
		newRPS := *currentRPS / 2
		if newRPS > 0 && newRPS != *currentRPS {
			*currentRPS = newRPS
			ticker.Reset(time.Second / time.Duration(newRPS))
			fmt.Printf("\nThrottling down to %d RPS due to high failure rate\n", newRPS)
		}
	}
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
	// Track status codes properly
	if m.StatusCodes == nil {
		m.StatusCodes = make(map[int]int)
	}
	m.StatusCodes[status]++
	m.mu.Unlock()
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

	// Create reusable HTTP clients for better performance
	threadsClient := &http.Client{Timeout: 30 * time.Second}
	messagesClient := &http.Client{Timeout: 30 * time.Second}

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
			throttleIfNeeded(metrics, &currentRPS, ticker)
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch cfg.Pattern {
				case "read_threads":
					performReadThreadsWithClient(cfg, signature, metrics, threadsClient)
				case "read_messages":
					performReadMessagesWithClient(cfg, signature, metrics, threadIDs, messagesClient)
				case "read_mixed":
					if time.Now().UnixNano()%2 == 0 {
						performReadThreadsWithClient(cfg, signature, metrics, threadsClient)
					} else {
						performReadMessagesWithClient(cfg, signature, metrics, threadIDs, messagesClient)
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

func performReadThreadsWithClient(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, client *http.Client) {
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

func performReadMessagesWithClient(cfg BenchmarkConfig, signature string, metrics *BenchmarkMetrics, threadIDs []string, client *http.Client) {
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

	// Calculate stats - ensure all atomic operations are completed
	totalRequests := atomic.LoadInt64(&metrics.TotalRequests)
	totalDur := atomic.LoadInt64(&metrics.TotalDuration)
	minDur := atomic.LoadInt64(&metrics.MinDuration)
	maxDur := atomic.LoadInt64(&metrics.MaxDuration)
	successCount := atomic.LoadInt64(&metrics.SuccessCount)
	failCount := atomic.LoadInt64(&metrics.FailCount)
	bytesSent := atomic.LoadInt64(&metrics.BytesSent)

	var avgDuration, minDuration, maxDuration time.Duration
	if totalRequests > 0 {
		avgDuration = time.Duration(totalDur / totalRequests)
		minDuration = time.Duration(minDur)
		maxDuration = time.Duration(maxDur)
	}

	// Calculate percentiles with proper locking to avoid race conditions
	metrics.mu.RLock()
	durations := make([]time.Duration, len(metrics.Durations))
	copy(durations, metrics.Durations)
	statusCodes := make(map[int]int)
	for k, v := range metrics.StatusCodes {
		statusCodes[k] = v
	}
	metrics.mu.RUnlock()

	// Calculate percentiles safely
	var p90, p95, p99 time.Duration
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		n := len(durations)
		p90 = durations[int(float64(n)*0.9)]
		p95 = durations[int(float64(n)*0.95)]
		p99 = durations[int(float64(n)*0.99)]
	}

	// Calculate actual RPS
	actualDuration := metrics.EndTime.Sub(metrics.StartTime).Seconds()
	var actualRPS float64
	if actualDuration > 0 {
		actualRPS = float64(totalRequests) / actualDuration
	}

	result := map[string]interface{}{
		"total_requests":  totalRequests,
		"success_count":   successCount,
		"fail_count":      failCount,
		"avg_duration_ms": avgDuration.Milliseconds(),
		"min_duration_ms": minDuration.Milliseconds(),
		"max_duration_ms": maxDuration.Milliseconds(),
		"p90_duration_ms": p90.Milliseconds(),
		"p95_duration_ms": p95.Milliseconds(),
		"p99_duration_ms": p99.Milliseconds(),
		"status_codes":    statusCodes,
		"bytes_sent":      bytesSent,
		"start_time":      metrics.StartTime,
		"end_time":        metrics.EndTime,
		"duration":        metrics.EndTime.Sub(metrics.StartTime),
		"actual_rps":      actualRPS,
	}

	json.NewEncoder(file).Encode(result)

	// Print summary
	fmt.Println()
	fmt.Println("BENCHMARK RESULTS SUMMARY")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("- Target vs Actual RPS: %.1f → %.1f\n", actualRPS, actualRPS)
	fmt.Printf("- Total Requests: %d (%d success, %d failed)\n", totalRequests, successCount, failCount)
	fmt.Printf("- Latency: Avg %v | P90 %v | P95 %v | P99 %v\n", avgDuration, p90, p95, p99)
	fmt.Printf("- Data Sent: %.2f MB\n", float64(bytesSent)/(1024*1024))
	fmt.Printf("- Duration: %v\n", metrics.EndTime.Sub(metrics.StartTime))

	// Performance rating
	var rating string
	var ratingSymbol string
	successRate := float64(successCount) / float64(totalRequests) * 100
	switch {
	case successRate >= 99 && actualRPS > 100:
		rating = "Excellent"
		ratingSymbol = "[A+]"
	case successRate >= 95 && actualRPS > 50:
		rating = "Good"
		ratingSymbol = "[B+]"
	case successRate >= 90:
		rating = "Fair"
		ratingSymbol = "[C]"
	default:
		rating = "Poor"
		ratingSymbol = "[D]"
	}

	fmt.Printf("- Performance Rating: %s %s (%.1f%% success rate)\n", ratingSymbol, rating, successRate)
	fmt.Printf("- Detailed logs: %s\n", outFile)
	fmt.Println(strings.Repeat("-", 50))
}
