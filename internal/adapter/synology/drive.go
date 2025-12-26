package synology

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/vertextoedge/synology-file-cache/internal/port"
)

// DriveClient wraps Client to implement port.DriveClient
type DriveClient struct {
	*Client
}

// Ensure DriveClient implements port.DriveClient
var _ port.DriveClient = (*DriveClient)(nil)

// NewDriveClient creates a new Drive API client
func NewDriveClient(client *Client) *DriveClient {
	return &DriveClient{Client: client}
}

// GetSharedFiles returns files shared with others
func (c *DriveClient) GetSharedFiles(offset, limit int) (*port.DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":            {APIDriveFiles},
		"version":        {strconv.Itoa(version)},
		"method":         {"shared_with_others"},
		"sort_by":        {`"owner"`},
		"sort_direction": {`"asc"`},
		"filter":         {`{"include_transient":true}`},
	}

	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	return c.parseListResponse(resp)
}

// GetStarredFiles returns starred files
func (c *DriveClient) GetStarredFiles(offset, limit int) (*port.DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":            {APIDriveFiles},
		"version":        {strconv.Itoa(version)},
		"method":         {"list_starred"},
		"sort_by":        {`"owner"`},
		"sort_direction": {`"asc"`},
		"filter":         {`{"include_transient":true}`},
	}

	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	return c.parseListResponse(resp)
}

// GetLabeledFiles returns files with a specific label
func (c *DriveClient) GetLabeledFiles(labelID string, offset, limit int) (*port.DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":            {APIDriveFiles},
		"version":        {strconv.Itoa(version)},
		"method":         {"list_labelled"},
		"label_id":       {fmt.Sprintf(`"%s"`, labelID)},
		"sort_by":        {`"owner"`},
		"sort_direction": {`"asc"`},
		"filter":         {`{"include_transient":true}`},
	}

	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	return c.parseListResponse(resp)
}

// GetRecentFiles returns recently accessed/modified files
func (c *DriveClient) GetRecentFiles(offset, limit int) (*port.DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveFiles},
		"version": {strconv.Itoa(version)},
		"method":  {"recent"},
	}

	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	return c.parseListResponse(resp)
}

// GetLabels returns all labels
func (c *DriveClient) GetLabels() ([]port.DriveLabel, error) {
	apiPath, version, err := c.getAPIPath(APIDriveLabels)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveLabels},
		"version": {strconv.Itoa(version)},
		"method":  {"list"},
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []port.DriveLabel `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse labels response: %w", err)
	}

	return result.Items, nil
}

// ListFiles lists files in a folder
func (c *DriveClient) ListFiles(opts *port.DriveListOptions) (*port.DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveFiles},
		"version": {strconv.Itoa(version)},
		"method":  {"list"},
	}

	if opts != nil {
		if opts.Path != "" {
			params.Set("path", opts.Path)
		}
		if opts.FileID > 0 {
			params.Set("file_id", strconv.FormatInt(opts.FileID, 10))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.SortBy != "" {
			params.Set("sort_by", opts.SortBy)
		}
		if opts.SortDirection != "" {
			params.Set("sort_direction", opts.SortDirection)
		}
		if opts.FileType != "" {
			params.Set("type", opts.FileType)
		}
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	return c.parseListResponse(resp)
}

// DownloadFile downloads a file
func (c *DriveClient) DownloadFile(fileID int64, path string) (io.ReadCloser, string, int64, error) {
	return c.DownloadFileWithRange(fileID, path, -1)
}

// DownloadFileWithRange downloads a file with optional byte range support
func (c *DriveClient) DownloadFileWithRange(fileID int64, path string, rangeStart int64) (io.ReadCloser, string, int64, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, "", 0, err
	}

	var filesJSON string
	if path != "" {
		filesJSON = fmt.Sprintf(`["%s"]`, path)
	} else if fileID > 0 {
		filesJSON = fmt.Sprintf(`["%d"]`, fileID)
	} else {
		return nil, "", 0, fmt.Errorf("either file_id or path is required")
	}

	params := url.Values{
		"api":     {APIDriveFiles},
		"version": {strconv.Itoa(version)},
		"method":  {"download"},
		"files":   {filesJSON},
	}

	urlStr := c.buildURL(apiPath, params)

	resp, err := c.doDownloadRequestWithRange("GET", urlStr, nil, rangeStart)
	if err != nil {
		return nil, "", 0, err
	}

	// Check if response is an error (JSON)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		defer resp.Body.Close()

		var apiResp Response
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return nil, "", 0, fmt.Errorf("failed to decode error response: %w", err)
		}

		code := 0
		if apiResp.Error != nil {
			code = apiResp.Error.Code
		}
		return nil, "", 0, &APIError{Code: code, Message: GetErrorMessage(code)}
	}

	// Accept both 200 OK and 206 Partial Content
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Get filename from Content-Disposition header
	filename := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			filename = strings.Trim(cd[idx+9:], "\"")
		}
	}

	return resp.Body, filename, resp.ContentLength, nil
}

// GetAdvanceSharing gets advanced sharing info for a file
func (c *DriveClient) GetAdvanceSharing(fileID int64, path string) (*port.AdvanceSharingInfo, error) {
	apiPath, version, err := c.getAPIPath(APIDriveAdvanceSharing)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveAdvanceSharing},
		"version": {strconv.Itoa(version)},
		"method":  {"get"},
	}

	if fileID > 0 {
		params.Set("path", fmt.Sprintf(`"id:%d"`, fileID))
	} else if path != "" {
		params.Set("path", fmt.Sprintf(`"%s"`, path))
	} else {
		return nil, fmt.Errorf("either file_id or path is required")
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		SharingLink     string `json:"sharing_link"`
		URL             string `json:"url"`
		ProtectPassword string `json:"protect_password"`
		DueDate         int64  `json:"due_date"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse advance sharing response: %w", err)
	}

	return &port.AdvanceSharingInfo{
		SharingLink:     result.SharingLink,
		URL:             result.URL,
		ProtectPassword: result.ProtectPassword,
		DueDate:         result.DueDate,
	}, nil
}

// parseListResponse parses a drive list response
func (c *DriveClient) parseListResponse(resp *Response) (*port.DriveListResponse, error) {
	var result port.DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse list response: %w", err)
	}
	return &result, nil
}
