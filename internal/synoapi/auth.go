package synoapi

import (
	"encoding/json"
	"fmt"
	"net/url"
)

const (
	apiInfoPath = "query.cgi"
	authPath    = "auth.cgi"
	sessionName = "FileStation"
)

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
		Success bool                    `json:"success"`
		Data    map[string]APIEndpoint  `json:"data"`
		Error   *ErrorInfo              `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to decode api info response: %w", err)
	}

	if !apiResp.Success {
		code := 0
		if apiResp.Error != nil {
			code = apiResp.Error.Code
		}
		return &APIError{Code: code, Message: getErrorMessage(code)}
	}

	// Cache the API info
	c.apiInfoMu.Lock()
	for name, info := range apiResp.Data {
		c.apiInfo[name] = info
	}
	c.apiInfoMu.Unlock()

	return nil
}

// Login authenticates with the Synology NAS and obtains a session ID
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
		Success bool       `json:"success"`
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
		return &APIError{Code: code, Message: getErrorMessage(code)}
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
		return &APIError{Code: code, Message: getErrorMessage(code)}
	}

	return nil
}
