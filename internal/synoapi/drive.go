package synoapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Drive API names
const (
	APIDriveFiles           = "SYNO.SynologyDrive.Files"
	APIDriveNode            = "SYNO.SynologyDrive.Node"
	APIDriveDownload        = "SYNO.SynologyDrive.Node.Download"
	APIDriveSharing         = "SYNO.SynologyDrive.Sharing"
	APIDriveShare           = "SYNO.SynologyDrive.Share"
	APIDriveAdvanceSharing  = "SYNO.SynologyDrive.AdvanceSharing"
	APIDriveLabels          = "SYNO.SynologyDrive.Labels"
	APIDriveTeamFolder      = "SYNO.SynologyDrive.TeamFolders"
)

// DriveFile represents a file in Synology Drive
type DriveFile struct {
	ID            json.Number     `json:"file_id"`
	Name          string          `json:"name"`
	Path          string          `json:"display_path"`
	ContentType   string          `json:"content_type"` // "dir" or "file"
	Size          int64           `json:"size"`
	MTime         int64           `json:"content_mtime"` // Content modification time (Unix timestamp)
	ATime         int64           `json:"access_time"`   // Access time
	CTime         int64           `json:"created_time"`  // Create time
	ChangeTime    int64           `json:"change_time"`   // Change time
	Starred       bool            `json:"starred"`
	Shared        bool            `json:"adv_shared"`
	PermanentLink string          `json:"permanent_link"` // Share token for adv_shared files
	Labels        []DriveLabel    `json:"labels,omitempty"`
	Owner         *DriveOwner     `json:"owner,omitempty"`
	Perm          *DrivePerm      `json:"perm,omitempty"`
}

// GetID returns the file ID as int64
func (f *DriveFile) GetID() int64 {
	id, _ := f.ID.Int64()
	return id
}

// IsDir returns true if the file is a directory
func (f *DriveFile) IsDir() bool {
	return f.ContentType == "dir"
}

// DriveLabel represents a label in Drive
type DriveLabel struct {
	ID         string `json:"label_id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Position   int    `json:"position"`
	Type       string `json:"type"`
	UpdateTime int64  `json:"update_time"`
}

// DriveOwner represents owner info
type DriveOwner struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
}

// DrivePerm represents permissions
type DrivePerm struct {
	Readable  bool `json:"readable"`
	Writable  bool `json:"writable"`
	Shareable bool `json:"shareable"`
}

// DriveListOptions contains options for listing files in Drive
type DriveListOptions struct {
	Path          string   // Folder path (e.g., "/mydrive", "/team-folder/xxx")
	FileID        int64    // Alternative: use file ID instead of path
	Offset        int
	Limit         int
	SortBy        string   // name, time, size, type
	SortDirection string   // asc, desc
	FileType      string   // dir, file, all
	Additional    []string // Additional info to fetch
}

// DriveListResponse is the response from listing Drive files
type DriveListResponse struct {
	Offset int          `json:"offset"`
	Total  int          `json:"total"`
	Items  []DriveFile  `json:"items"`
}

// DriveListFiles lists files in a Drive folder
func (c *Client) DriveListFiles(opts *DriveListOptions) (*DriveListResponse, error) {
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
		if len(opts.Additional) > 0 {
			params.Set("additional", strings.Join(opts.Additional, ","))
		}
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse drive list response: %w", err)
	}

	return &result, nil
}

// DriveGetStarred gets starred files using list_starred method
func (c *Client) DriveGetStarred(offset, limit int) (*DriveListResponse, error) {
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

	var result DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse list_starred response: %w", err)
	}

	return &result, nil
}

// DriveGetSharedWithOthers gets files shared with others
func (c *Client) DriveGetSharedWithOthers(offset, limit int) (*DriveListResponse, error) {
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

	var result DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse shared_with_others response: %w", err)
	}

	return &result, nil
}

// DriveGetRecent gets recently accessed/modified files
func (c *Client) DriveGetRecent(offset, limit int) (*DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	// Try "recent" method first
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

	var result DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse recent files response: %w", err)
	}

	return &result, nil
}

// DriveDownloadFile downloads a file from Drive
// Uses SYNO.SynologyDrive.Files API with method=download and files parameter
func (c *Client) DriveDownloadFile(fileID int64, path string) (io.ReadCloser, string, int64, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, "", 0, err
	}

	// Build the files parameter as JSON array
	var filesJSON string
	if path != "" {
		filesJSON = fmt.Sprintf(`["%s"]`, path)
	} else if fileID > 0 {
		// Try with file_id - might work in some versions
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

	resp, err := c.doRequest("GET", urlStr, nil)
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
		return nil, "", 0, &APIError{Code: code, Message: getErrorMessage(code)}
	}

	// Check for bad request (empty response)
	if resp.StatusCode != 200 {
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

// DriveShareLink represents a share link in Drive
type DriveShareLink struct {
	ID            string `json:"sharing_id"`
	URL           string `json:"sharing_link"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	FileID        int64  `json:"file_id"`
	IsFolder      bool   `json:"isdir"`
	DateExpired   string `json:"date_expired,omitempty"`
	DateAvailable string `json:"date_available,omitempty"`
	Status        string `json:"status"`
	HasPassword   bool   `json:"has_password"`
	Permission    string `json:"perm"` // view, edit
}

// DriveShareListResponse is the response from listing share links
type DriveShareListResponse struct {
	Offset int              `json:"offset"`
	Total  int              `json:"total"`
	Items  []DriveShareLink `json:"items"`
}

// DriveListShareLinks lists all share links in Drive
func (c *Client) DriveListShareLinks(offset, limit int) (*DriveShareListResponse, error) {
	// Try SYNO.SynologyDrive.Share first
	apiPath, version, err := c.getAPIPath(APIDriveShare)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveShare},
		"version": {strconv.Itoa(version)},
		"method":  {"list"},
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

	var result DriveShareListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse share links response: %w", err)
	}

	return &result, nil
}

// DriveLabel operations

// DriveGetLabels gets all labels
func (c *Client) DriveGetLabels() ([]DriveLabel, error) {
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
		Items []DriveLabel `json:"items"`
		Total int          `json:"total"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse labels response: %w", err)
	}

	return result.Items, nil
}

// DriveGetFilesByLabel gets files with a specific label
func (c *Client) DriveGetFilesByLabel(labelID string, offset, limit int) (*DriveListResponse, error) {
	apiPath, version, err := c.getAPIPath(APIDriveFiles)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":            {APIDriveFiles},
		"version":        {strconv.Itoa(version)},
		"method":         {"list_labelled"},
		"label_id":       {fmt.Sprintf(`"%s"`, labelID)}, // label_id needs to be quoted
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

	var result DriveListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse files by label response: %w", err)
	}

	return &result, nil
}

// DriveTeamFolder represents a team folder
type DriveTeamFolder struct {
	ID     int64  `json:"folder_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	FileID int64  `json:"file_id"`
}

// DriveListTeamFolders lists all team folders
func (c *Client) DriveListTeamFolders() ([]DriveTeamFolder, error) {
	apiPath, version, err := c.getAPIPath(APIDriveTeamFolder)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveTeamFolder},
		"version": {strconv.Itoa(version)},
		"method":  {"list"},
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		TeamFolders []DriveTeamFolder `json:"team_folders"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse team folders response: %w", err)
	}

	return result.TeamFolders, nil
}

// DriveGetFileInfo gets detailed info for a file
func (c *Client) DriveGetFileInfo(fileID int64, path string) (*DriveFile, error) {
	apiPath, version, err := c.getAPIPath(APIDriveNode)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveNode},
		"version": {strconv.Itoa(version)},
		"method":  {"get"},
	}

	if fileID > 0 {
		params.Set("file_id", strconv.FormatInt(fileID, 10))
	} else if path != "" {
		params.Set("path", path)
	} else {
		return nil, fmt.Errorf("either file_id or path is required")
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result DriveFile
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse file info response: %w", err)
	}

	return &result, nil
}

// AdvanceSharingInfo represents advanced sharing information for a file
type AdvanceSharingInfo struct {
	SharingLink     string `json:"sharing_link"`     // Extra segment (e.g., OXSIBppBv2Lxc0znhDKNs5k...)
	URL             string `json:"url"`              // Full URL (e.g., https://drive.verte.kr/d/s/{token}/{sharing_link})
	PermanentID     int64  `json:"permanent_id"`     // Permanent ID of the file
	Role            string `json:"role"`             // "viewer", "editor", etc.
	ProtectPassword string `json:"protect_password"` // Password if set
	DueDate         int64  `json:"due_date"`         // Expiration timestamp (0 if none)
	UID             int    `json:"uid"`              // User ID
}

// DriveGetAdvanceSharing gets the advanced sharing info for a file
// This returns the full sharing_link including the extra segment
func (c *Client) DriveGetAdvanceSharing(fileID int64, path string) (*AdvanceSharingInfo, error) {
	apiPath, version, err := c.getAPIPath(APIDriveAdvanceSharing)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIDriveAdvanceSharing},
		"version": {strconv.Itoa(version)},
		"method":  {"get"},
	}

	// Use id:fileID format or path
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

	var result AdvanceSharingInfo
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse advance sharing response: %w", err)
	}

	return &result, nil
}
