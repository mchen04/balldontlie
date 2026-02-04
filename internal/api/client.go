package api

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// RateLimitedClient wraps http.Client with rate limiting
type RateLimitedClient struct {
	client      *http.Client
	rateLimiter *rateLimiter
	maxRetries  int
}

type rateLimiter struct {
	mu          sync.Mutex
	tokens      int
	maxTokens   int
	refillRate  time.Duration
	lastRefill  time.Time
}

func newRateLimiter(requestsPerMinute int) *rateLimiter {
	// Convert requests/min to token bucket
	// 600 req/min = 10 req/sec, refill 1 token every 100ms
	refillRate := time.Minute / time.Duration(requestsPerMinute)
	return &rateLimiter{
		tokens:     requestsPerMinute / 6, // Start with 10 seconds worth
		maxTokens:  requestsPerMinute / 6, // Max burst of 10 seconds
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) wait() {
	for {
		rl.mu.Lock()

		// Refill tokens based on time elapsed
		now := time.Now()
		elapsed := now.Sub(rl.lastRefill)
		tokensToAdd := int(elapsed / rl.refillRate)
		if tokensToAdd > 0 {
			rl.tokens = min(rl.tokens+tokensToAdd, rl.maxTokens)
			rl.lastRefill = now
		}

		// If tokens available, consume one and return
		if rl.tokens > 0 {
			rl.tokens--
			rl.mu.Unlock()
			return
		}

		// Calculate wait time and release lock before sleeping
		waitTime := rl.refillRate
		rl.mu.Unlock()
		time.Sleep(waitTime)
		// Loop back to re-check with lock
	}
}

// NewRateLimitedClient creates a client limited to requestsPerMinute
func NewRateLimitedClient(requestsPerMinute int, timeout time.Duration, maxRetries int) *RateLimitedClient {
	return &RateLimitedClient{
		client: &http.Client{
			Timeout: timeout,
		},
		rateLimiter: newRateLimiter(requestsPerMinute),
		maxRetries:  maxRetries,
	}
}

// Do executes an HTTP request with rate limiting and retries
func (c *RateLimitedClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		c.rateLimiter.wait()

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		// Handle rate limit responses (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("rate limited (429)")
			backoff := time.Duration(1<<attempt) * time.Second
			time.Sleep(backoff)
			continue
		}

		// Handle server errors with retry
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Get performs a rate-limited GET request
func (c *RateLimitedClient) Get(url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
