package synology

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// Client is a Synology API client
type Client struct {
	baseURL        string
	username       string
	password       string
	httpClient     *http.Client
	downloadClient *http.Client
	sid            string
	sidMu          sync.RWMutex
	apiInfo        map[string]APIEndpoint
	apiInfoMu      sync.RWMutex
}

// Ensure Client implements port.SynologyClient
var _ port.SynologyClient = (*Client)(nil)

// ClientConfig contains optional client configuration
type ClientConfig struct {
	BufferSizeMB int // Read/Write buffer size in MB (default: 8)
}

// NewClient creates a new Synology API client
func NewClient(baseURL, username, password string, skipTLSVerify bool) *Client {
	return NewClientWithConfig(baseURL, username, password, skipTLSVerify, nil)
}

// NewClientWithConfig creates a new Synology API client with custom configuration
func NewClientWithConfig(baseURL, username, password string, skipTLSVerify bool, cfg *ClientConfig) *Client {
	bufferSize := 8 * 1024 * 1024 // 8MB default
	if cfg != nil && cfg.BufferSizeMB > 0 {
		bufferSize = cfg.BufferSizeMB * 1024 * 1024
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}

	downloadTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
		// Connection pooling
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     120 * time.Second,

		// Buffer sizes for high-speed transfers
		WriteBufferSize: bufferSize,
		ReadBufferSize:  bufferSize,

		// HTTP/2 support
		ForceAttemptHTTP2: true,

		// Disable compression for binary files (saves CPU)
		DisableCompression: true,

		// Response header timeout (not total download timeout)
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		downloadClient: &http.Client{
			Transport: downloadTransport,
			Timeout:   0, // No timeout for downloads
		},
		apiInfo: make(map[string]APIEndpoint),
	}
}

// GetSID returns the current session ID
func (c *Client) GetSID() string {
	c.sidMu.RLock()
	defer c.sidMu.RUnlock()
	return c.sid
}

func (c *Client) setSID(sid string) {
	c.sidMu.Lock()
	defer c.sidMu.Unlock()
	c.sid = sid
}

func (c *Client) clearSID() {
	c.sidMu.Lock()
	defer c.sidMu.Unlock()
	c.sid = ""
}

// IsLoggedIn returns true if the client has a valid session
func (c *Client) IsLoggedIn() bool {
	return c.GetSID() != ""
}

// getAPIPath returns the API path for the given API name
func (c *Client) getAPIPath(apiName string) (string, int, error) {
	c.apiInfoMu.RLock()
	info, ok := c.apiInfo[apiName]
	c.apiInfoMu.RUnlock()

	if ok {
		return info.Path, info.MaxVersion, nil
	}

	// Fetch API info if not cached
	if err := c.QueryAPIInfo(apiName); err != nil {
		return "", 0, err
	}

	c.apiInfoMu.RLock()
	info, ok = c.apiInfo[apiName]
	c.apiInfoMu.RUnlock()

	if !ok {
		return "", 0, &APIError{Code: ErrAPINotExists, Message: fmt.Sprintf("api %s not found", apiName)}
	}

	return info.Path, info.MaxVersion, nil
}

// buildURL builds the full URL for an API request
func (c *Client) buildURL(path string, params url.Values) string {
	if sid := c.GetSID(); sid != "" {
		params.Set("_sid", sid)
	}
	return fmt.Sprintf("%s/webapi/%s?%s", c.baseURL, path, params.Encode())
}

// doRequest performs an HTTP request
func (c *Client) doRequest(method, urlStr string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// doDownloadRequest performs an HTTP request for file downloads
func (c *Client) doDownloadRequest(method, urlStr string, body io.Reader) (*http.Response, error) {
	return c.doDownloadRequestWithRange(method, urlStr, body, -1)
}

// doDownloadRequestWithRange performs an HTTP request with optional Range header
func (c *Client) doDownloadRequestWithRange(method, urlStr string, body io.Reader, rangeStart int64) (*http.Response, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if rangeStart >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// doAPIRequest performs an API request and parses the JSON response
func (c *Client) doAPIRequest(path string, params url.Values) (*Response, error) {
	urlStr := c.buildURL(path, params)

	resp, err := c.doRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp Response
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !apiResp.Success {
		code := 0
		if apiResp.Error != nil {
			code = apiResp.Error.Code
		}
		return nil, &APIError{Code: code, Message: GetErrorMessage(code)}
	}

	return &apiResp, nil
}

// doAPIRequestWithRetry performs an API request with automatic re-login on session errors
func (c *Client) doAPIRequestWithRetry(path string, params url.Values) (*Response, error) {
	resp, err := c.doAPIRequest(path, params)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsSessionError() {
			// Try to re-login
			if loginErr := c.Login(); loginErr != nil {
				return nil, fmt.Errorf("session expired and re-login failed: %w", loginErr)
			}
			// Retry the request
			return c.doAPIRequest(path, params)
		}
		return nil, err
	}
	return resp, nil
}

// QueryAPIInfo queries available API information
func (c *Client) QueryAPIInfo(apis ...string) error {
	query := "all"
	if len(apis) > 0 {
		query = ""
		for i, api := range apis {
			if i > 0 {
				query += ","
			}
			query += api
		}
	}

	params := url.Values{
		"api":     {"SYNO.API.Info"},
		"version": {"1"},
		"method":  {"query"},
		"query":   {query},
	}

	urlStr := fmt.Sprintf("%s/webapi/%s?%s", c.baseURL, apiInfoPath, params.Encode())

	resp, err := c.doRequest("GET", urlStr, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp struct {
		Success bool                   `json:"success"`
		Data    map[string]APIEndpoint `json:"data"`
		Error   *ErrorInfo             `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to decode api info response: %w", err)
	}

	if !apiResp.Success {
		code := 0
		if apiResp.Error != nil {
			code = apiResp.Error.Code
		}
		return &APIError{Code: code, Message: GetErrorMessage(code)}
	}

	// Cache the API info
	c.apiInfoMu.Lock()
	for name, info := range apiResp.Data {
		c.apiInfo[name] = info
	}
	c.apiInfoMu.Unlock()

	return nil
}

// Login authenticates with the Synology NAS
func (c *Client) Login() error {
	params := url.Values{
		"api":     {"SYNO.API.Auth"},
		"version": {"3"},
		"method":  {"login"},
		"account": {c.username},
		"passwd":  {c.password},
		"session": {sessionName},
		"format":  {"sid"},
	}

	urlStr := fmt.Sprintf("%s/webapi/%s?%s", c.baseURL, authPath, params.Encode())

	resp, err := c.doRequest("GET", urlStr, nil)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	var loginResp struct {
		Success bool `json:"success"`
		Data    struct {
			SID string `json:"sid"`
		} `json:"data"`
		Error *ErrorInfo `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	if !loginResp.Success {
		code := 0
		if loginResp.Error != nil {
			code = loginResp.Error.Code
		}
		return &APIError{Code: code, Message: GetErrorMessage(code)}
	}

	c.setSID(loginResp.Data.SID)
	return nil
}

// Logout terminates the current session
func (c *Client) Logout() error {
	if !c.IsLoggedIn() {
		return nil
	}

	params := url.Values{
		"api":     {"SYNO.API.Auth"},
		"version": {"1"},
		"method":  {"logout"},
		"session": {sessionName},
	}

	urlStr := fmt.Sprintf("%s/webapi/%s?%s", c.baseURL, authPath, params.Encode())

	resp, err := c.doRequest("GET", urlStr, nil)
	if err != nil {
		return fmt.Errorf("logout request failed: %w", err)
	}
	defer resp.Body.Close()

	var logoutResp Response
	if err := json.NewDecoder(resp.Body).Decode(&logoutResp); err != nil {
		return fmt.Errorf("failed to decode logout response: %w", err)
	}

	c.clearSID()

	if !logoutResp.Success {
		code := 0
		if logoutResp.Error != nil {
			code = logoutResp.Error.Code
		}
		return &APIError{Code: code, Message: GetErrorMessage(code)}
	}

	return nil
}
