package synoapi

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a Synology File Station API client
type Client struct {
	baseURL       string
	username      string
	password      string
	httpClient    *http.Client
	sid           string
	sidMu         sync.RWMutex
	apiInfo       map[string]APIEndpoint
	apiInfoMu     sync.RWMutex
}

// APIEndpoint contains API path and version information
type APIEndpoint struct {
	Path       string `json:"path"`
	MinVersion int    `json:"minVersion"`
	MaxVersion int    `json:"maxVersion"`
}

// Response is the base response structure from Synology API
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *ErrorInfo      `json:"error,omitempty"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code   int             `json:"code"`
	Errors json.RawMessage `json:"errors,omitempty"` // Can be array or object
}

// Common error codes
const (
	ErrUnknown           = 100
	ErrInvalidParam      = 101
	ErrAPINotExists      = 102
	ErrMethodNotExists   = 103
	ErrVersionNotSupport = 104
	ErrNoPermission      = 105
	ErrSessionTimeout    = 106
	ErrDuplicateLogin    = 107
	ErrSIDNotFound       = 119
)

// FileStation specific error codes
const (
	ErrFSUnknown          = 400
	ErrFSNoSuchFile       = 408
	ErrFSFileExists       = 414
	ErrFSInvalidPath      = 418
	ErrFSAccessDenied     = 419
)

// APIError represents an error from the Synology API
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("synology api error %d: %s", e.Code, e.Message)
}

// IsSessionError returns true if the error indicates session issues
func (e *APIError) IsSessionError() bool {
	return e.Code == ErrNoPermission || e.Code == ErrSessionTimeout || e.Code == ErrSIDNotFound
}

// errorMessages maps error codes to human-readable messages
var errorMessages = map[int]string{
	ErrUnknown:           "unknown error",
	ErrInvalidParam:      "invalid parameter",
	ErrAPINotExists:      "api does not exist",
	ErrMethodNotExists:   "method does not exist",
	ErrVersionNotSupport: "version not supported",
	ErrNoPermission:      "no permission",
	ErrSessionTimeout:    "session timeout",
	ErrDuplicateLogin:    "duplicate login",
	ErrSIDNotFound:       "sid not found",
	ErrFSUnknown:         "file station unknown error",
	ErrFSNoSuchFile:      "no such file or directory",
	ErrFSFileExists:      "file already exists",
	ErrFSInvalidPath:     "invalid path",
	ErrFSAccessDenied:    "access denied",
}

func getErrorMessage(code int) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}
	return fmt.Sprintf("error code %d", code)
}

// NewClient creates a new Synology API client
func NewClient(baseURL, username, password string, skipTLSVerify bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
	}

	return &Client{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		apiInfo: make(map[string]APIEndpoint),
	}
}

// SetTimeout sets the HTTP client timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}

// GetSID returns the current session ID
func (c *Client) GetSID() string {
	c.sidMu.RLock()
	defer c.sidMu.RUnlock()
	return c.sid
}

// setSID sets the session ID
func (c *Client) setSID(sid string) {
	c.sidMu.Lock()
	defer c.sidMu.Unlock()
	c.sid = sid
}

// clearSID clears the session ID
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
	// Add session ID if available
	if sid := c.GetSID(); sid != "" {
		params.Set("_sid", sid)
	}

	return fmt.Sprintf("%s/webapi/%s?%s", c.baseURL, path, params.Encode())
}

// doRequest performs an HTTP request and returns the response
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
		return nil, &APIError{Code: code, Message: getErrorMessage(code)}
	}

	return &apiResp, nil
}

// TestRequest makes a raw HTTP request and returns the body (for debugging)
func (c *Client) TestRequest(method, urlStr string) ([]byte, error) {
	resp, err := c.doRequest(method, urlStr, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
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
