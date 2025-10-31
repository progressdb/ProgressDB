package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
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
)

type Config struct {
	Host        string
	BackendKey  string
	FrontendKey string
	UserID      string
	RPS         int
	Duration    time.Duration
	PayloadSize int
	Pattern     string
}

type Metrics struct {
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

func (m *Metrics) record(status int, duration time.Duration, bytes int64, success bool) {
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

func main() {
	auto := flag.Bool("auto", false, "Use default values and only prompt for pattern")
	flag.Parse()

	cfg := Config{
		Host:        "http://localhost:8080",
		BackendKey:  "sk_example",
		FrontendKey: "pk_example",
		UserID:      "user1",
		RPS:         1000,
		Duration:    time.Minute,
		PayloadSize: 30,
	}

	if !*auto {
		cfg = promptConfig()
	} else {
		cfg.Pattern = promptPattern()
	}

	signature := fetchSignature(cfg)
	var metrics *Metrics
	if cfg.Pattern == "create_threads" {
		metrics = runCreateThreadsBenchmark(cfg, signature)
	} else {
		metrics = runThreadWithMessagesBenchmark(cfg, signature)
	}
	outputMetrics(metrics)
}

func promptConfig() Config {
	scanner := bufio.NewScanner(os.Stdin)
	cfg := Config{
		Host:        "http://localhost:8080",
		BackendKey:  "sk_example",
		FrontendKey: "pk_example",
		UserID:      "user1",
		RPS:         1000,
		Duration:    time.Minute,
		PayloadSize: 30,
	}

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

	fmt.Printf("Payload size (KB) [%d]: ", cfg.PayloadSize)
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			if size, err := strconv.Atoi(input); err == nil {
				cfg.PayloadSize = size
			}
		}
	}

	fmt.Printf("Pattern (create_threads or thread_with_messages) [thread_with_messages]: ")
	cfg.Pattern = "thread_with_messages"
	if scanner.Scan() {
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			if input == "create_threads" || input == "thread_with_messages" {
				cfg.Pattern = input
			}
		}
	}

	return cfg
}

func promptPattern() string {
	fmt.Println("Choose pattern:")
	fmt.Println("1. create_threads")
	fmt.Println("2. thread_with_messages")
	fmt.Print("Enter 1 or 2: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "1" {
			return "create_threads"
		} else if input == "2" {
			return "thread_with_messages"
		}
	}
	// Default
	return "thread_with_messages"
}

func fetchSignature(cfg Config) string {
	url := cfg.Host + "/backend/sign"
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
		log.Fatal("Signature request failed with status:", resp.StatusCode)
	}

	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal("Failed to decode signature response:", err)
	}

	return result.Signature
}

func runCreateThreadsBenchmark(cfg Config, signature string) *Metrics {
	metrics := &Metrics{StartTime: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	currentRPS := cfg.RPS
	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	stopPrint := make(chan struct{})
	go printLiveStats(metrics, cfg.Duration, stopPrint)

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

func runThreadWithMessagesBenchmark(cfg Config, signature string) *Metrics {
	metrics := &Metrics{StartTime: time.Now()}

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
	go printLiveStats(metrics, cfg.Duration, stopPrint)

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

func createThread(cfg Config, signature string, metrics *Metrics) {
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

func createThreadSync(cfg Config, signature string, threadID *string) {
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

func createMessage(cfg Config, signature string, metrics *Metrics, threadID string) {
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

func printLiveStats(metrics *Metrics, totalDuration time.Duration, stop <-chan struct{}) {
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
			avgResp := time.Duration(totalDur / totalReqs)
			minResp := time.Duration(minDur)
			maxResp := time.Duration(maxDur)
			fmt.Printf("\rRequests: %d | RPS: %.1f | Avg: %v | Min: %v | Max: %v | Remaining: %v", totalReqs, rps, avgResp, minResp, maxResp, remaining.Round(time.Second))
		}
	}
}

func outputMetrics(metrics *Metrics) {
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
