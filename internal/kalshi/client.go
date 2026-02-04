package kalshi

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"sports-betting-bot/internal/api"
)

const (
	// Production API - per official Kalshi docs
	baseURL = "https://api.elections.kalshi.com/trade-api/v2"

	// Demo API (for testing)
	demoURL = "https://demo-api.kalshi.co/trade-api/v2"

	// Rate limits: Basic tier = 10 writes/sec, 100 reads/sec
	requestsPerMinute = 600 // 10/sec
	requestTimeout    = 15 * time.Second
	maxRetries        = 3
)

// KalshiClient handles all Kalshi API communication
// Authentication is via API key only (email/password deprecated)
type KalshiClient struct {
	client  *api.RateLimitedClient
	baseURL string

	// API key auth
	apiKeyID      string
	apiKeyPrivate *rsa.PrivateKey

	// Use demo mode
	demo bool
}

// NewKalshiClient creates a client using API key authentication
// keyID: The API key ID from Kalshi
// privateKeyPath: Path to the RSA private key PEM file
// demo: If true, use the demo API endpoint
func NewKalshiClient(keyID, privateKeyPath string, demo bool) (*KalshiClient, error) {
	// Load private key from file
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	return NewKalshiClientFromKey(keyID, string(keyData), demo)
}

// NewKalshiClientFromKey creates a client using API key authentication with key content directly
// keyID: The API key ID from Kalshi
// privateKeyPEM: The RSA private key content (PEM format)
// demo: If true, use the demo API endpoint
// This is the preferred method for cloud deployments where secrets are passed as env vars
func NewKalshiClientFromKey(keyID, privateKeyPEM string, demo bool) (*KalshiClient, error) {
	base := baseURL
	if demo {
		base = demoURL
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS8 first (Kalshi's format), then PKCS1
	var privateKey *rsa.PrivateKey
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}
	} else {
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
	}

	return &KalshiClient{
		client:        api.NewRateLimitedClient(requestsPerMinute, requestTimeout, maxRetries),
		baseURL:       base,
		apiKeyID:      keyID,
		apiKeyPrivate: privateKey,
		demo:          demo,
	}, nil
}

// signRequest signs a request for API key auth
// Per Kalshi docs: sign(timestamp_ms + method + path_without_query_params)
func (c *KalshiClient) signRequest(method, path string, timestampMs int64) (string, error) {
	// Strip query parameters from path for signing
	pathForSigning := path
	if idx := strings.Index(path, "?"); idx != -1 {
		pathForSigning = path[:idx]
	}

	// Message format: timestamp (ms) + HTTP method + path (no query params)
	// The path should include /trade-api/v2 prefix
	fullPath := "/trade-api/v2" + pathForSigning
	message := fmt.Sprintf("%d%s%s", timestampMs, method, fullPath)

	// Hash the message with SHA-256
	hash := sha256.Sum256([]byte(message))

	// Sign with RSA-PSS, salt length = hash length (per Kalshi docs)
	signature, err := rsa.SignPSS(rand.Reader, c.apiKeyPrivate, crypto.SHA256, hash[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		return "", fmt.Errorf("signing request: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// doAuthenticatedRequest performs an authenticated HTTP request
func (c *KalshiClient) doAuthenticatedRequest(method, path string, body io.Reader) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set auth headers per Kalshi API docs
	// Timestamp must be in milliseconds
	timestampMs := time.Now().UnixMilli()
	signature, err := c.signRequest(method, path, timestampMs)
	if err != nil {
		return nil, err
	}

	req.Header.Set("KALSHI-ACCESS-KEY", c.apiKeyID)
	req.Header.Set("KALSHI-ACCESS-SIGNATURE", signature)
	req.Header.Set("KALSHI-ACCESS-TIMESTAMP", strconv.FormatInt(timestampMs, 10))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized (check API key): %s", string(respBody))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429): %s", string(respBody))
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetBalance fetches the account balance
func (c *KalshiClient) GetBalance() (*BalanceResponse, error) {
	body, err := c.doAuthenticatedRequest(http.MethodGet, "/portfolio/balance", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching balance: %w", err)
	}

	var resp BalanceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing balance response: %w", err)
	}

	return &resp, nil
}

// GetBalanceCents returns just the available balance in cents
func (c *KalshiClient) GetBalanceCents() (int64, error) {
	balance, err := c.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance.Balance, nil
}

// GetBalanceDollars returns the available balance in dollars
func (c *KalshiClient) GetBalanceDollars() (float64, error) {
	cents, err := c.GetBalanceCents()
	if err != nil {
		return 0, err
	}
	return float64(cents) / 100.0, nil
}

// GetPositions fetches all open market positions
func (c *KalshiClient) GetPositions() ([]MarketPosition, error) {
	var allPositions []MarketPosition
	cursor := ""

	for {
		path := "/portfolio/positions"
		if cursor != "" {
			path = fmt.Sprintf("%s?cursor=%s", path, cursor)
		}

		body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching positions: %w", err)
		}

		var resp PositionsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing positions response: %w", err)
		}

		allPositions = append(allPositions, resp.MarketPositions...)

		if resp.Cursor == "" {
			break
		}
		cursor = resp.Cursor
	}

	return allPositions, nil
}

// HasPositionOnMarket checks if we already have a position on a specific market
func (c *KalshiClient) HasPositionOnMarket(ticker string) (bool, int, error) {
	positions, err := c.GetPositions()
	if err != nil {
		return false, 0, err
	}

	for _, pos := range positions {
		if pos.Ticker == ticker && pos.Position != 0 {
			return true, pos.Position, nil
		}
	}
	return false, 0, nil
}

// GetExchangeStatus checks if the exchange is open for trading
func (c *KalshiClient) GetExchangeStatus() (*ExchangeStatusResponse, error) {
	body, err := c.doAuthenticatedRequest(http.MethodGet, "/exchange/status", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching exchange status: %w", err)
	}

	var resp ExchangeStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing exchange status: %w", err)
	}

	return &resp, nil
}

// IsTradingActive returns true if the exchange is open for trading
func (c *KalshiClient) IsTradingActive() (bool, error) {
	status, err := c.GetExchangeStatus()
	if err != nil {
		return false, err
	}
	return status.TradingActive && status.ExchangeActive, nil
}

// GetMarket fetches details for a specific market
func (c *KalshiClient) GetMarket(ticker string) (*Market, error) {
	path := fmt.Sprintf("/markets/%s", ticker)
	body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching market: %w", err)
	}

	var resp MarketResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing market response: %w", err)
	}

	return &resp.Market, nil
}

// IsDemo returns true if using demo API
func (c *KalshiClient) IsDemo() bool {
	return c.demo
}
