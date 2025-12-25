package synoapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// API names
const (
	APIFileStationInfo     = "SYNO.FileStation.Info"
	APIFileStationList     = "SYNO.FileStation.List"
	APIFileStationDownload = "SYNO.FileStation.Download"
	APIFileStationSharing  = "SYNO.FileStation.Sharing"
	APIFileStationFavorite = "SYNO.FileStation.Favorite"
)

// FileInfo represents file information from the API
type FileInfo struct {
	Path       string         `json:"path"`
	Name       string         `json:"name"`
	IsDir      bool           `json:"isdir"`
	Additional *FileAdditional `json:"additional,omitempty"`
}

// FileAdditional contains additional file information
type FileAdditional struct {
	RealPath   string     `json:"real_path,omitempty"`
	Size       int64      `json:"size,omitempty"`
	Owner      *OwnerInfo `json:"owner,omitempty"`
	Time       *TimeInfo  `json:"time,omitempty"`
	Perm       *PermInfo  `json:"perm,omitempty"`
	Type       string     `json:"type,omitempty"`
	MountPoint string     `json:"mount_point_type,omitempty"`
}

// OwnerInfo contains owner information
type OwnerInfo struct {
	User  string `json:"user"`
	Group string `json:"group"`
	UID   int    `json:"uid"`
	GID   int    `json:"gid"`
}

// TimeInfo contains time information (Unix timestamps in seconds)
type TimeInfo struct {
	ATime  int64 `json:"atime"`  // Last access time
	MTime  int64 `json:"mtime"`  // Last modification time
	CTime  int64 `json:"ctime"`  // Last status change time
	CRTime int64 `json:"crtime"` // Creation time
}

// PermInfo contains permission information
type PermInfo struct {
	Posix    int  `json:"posix"`
	IsACLSet bool `json:"is_acl_set"`
	ACL      *ACL `json:"acl,omitempty"`
}

// ACL contains access control list information
type ACL struct {
	Append bool `json:"append"`
	Del    bool `json:"del"`
	Exec   bool `json:"exec"`
	Read   bool `json:"read"`
	Write  bool `json:"write"`
}

// ShareFolder represents a shared folder
type ShareFolder struct {
	Path       string         `json:"path"`
	Name       string         `json:"name"`
	Additional *FileAdditional `json:"additional,omitempty"`
}

// ListSharesResponse is the response from list_share method
type ListSharesResponse struct {
	Offset int           `json:"offset"`
	Total  int           `json:"total"`
	Shares []ShareFolder `json:"shares"`
}

// ListFilesResponse is the response from list method
type ListFilesResponse struct {
	Offset int        `json:"offset"`
	Total  int        `json:"total"`
	Files  []FileInfo `json:"files"`
}

// ListSharesOptions contains options for listing shared folders
type ListSharesOptions struct {
	Offset     int
	Limit      int
	SortBy     string // name, size, user, mtime, type
	SortDir    string // asc, desc
	Additional []string // real_path, owner, time, perm, volume_status
}

// ListFilesOptions contains options for listing files
type ListFilesOptions struct {
	FolderPath string
	Offset     int
	Limit      int
	SortBy     string   // name, size, user, mtime, type
	SortDir    string   // asc, desc
	Pattern    string   // glob pattern for filtering
	FileType   string   // file, dir, all
	Additional []string // real_path, size, owner, time, perm, type
}

// ListShares lists all shared folders
func (c *Client) ListShares(opts *ListSharesOptions) (*ListSharesResponse, error) {
	path, version, err := c.getAPIPath(APIFileStationList)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIFileStationList},
		"version": {strconv.Itoa(version)},
		"method":  {"list_share"},
	}

	if opts != nil {
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.SortBy != "" {
			params.Set("sort_by", opts.SortBy)
		}
		if opts.SortDir != "" {
			params.Set("sort_direction", opts.SortDir)
		}
		if len(opts.Additional) > 0 {
			params.Set("additional", strings.Join(opts.Additional, ","))
		}
	}

	resp, err := c.doAPIRequestWithRetry(path, params)
	if err != nil {
		return nil, err
	}

	var result ListSharesResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse list_share response: %w", err)
	}

	return &result, nil
}

// ListFiles lists files in a folder
func (c *Client) ListFiles(opts *ListFilesOptions) (*ListFilesResponse, error) {
	if opts == nil || opts.FolderPath == "" {
		return nil, fmt.Errorf("folder_path is required")
	}

	path, version, err := c.getAPIPath(APIFileStationList)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":         {APIFileStationList},
		"version":     {strconv.Itoa(version)},
		"method":      {"list"},
		"folder_path": {opts.FolderPath},
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
	if opts.SortDir != "" {
		params.Set("sort_direction", opts.SortDir)
	}
	if opts.Pattern != "" {
		params.Set("pattern", opts.Pattern)
	}
	if opts.FileType != "" {
		params.Set("filetype", opts.FileType)
	}
	if len(opts.Additional) > 0 {
		params.Set("additional", strings.Join(opts.Additional, ","))
	}

	resp, err := c.doAPIRequestWithRetry(path, params)
	if err != nil {
		return nil, err
	}

	var result ListFilesResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse list response: %w", err)
	}

	return &result, nil
}

// GetFileInfo gets information for specific files
func (c *Client) GetFileInfo(paths []string, additional []string) ([]FileInfo, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}

	apiPath, version, err := c.getAPIPath(APIFileStationList)
	if err != nil {
		return nil, err
	}

	// Encode paths as JSON array
	pathsJSON, err := json.Marshal(paths)
	if err != nil {
		return nil, fmt.Errorf("failed to encode paths: %w", err)
	}

	params := url.Values{
		"api":     {APIFileStationList},
		"version": {strconv.Itoa(version)},
		"method":  {"getinfo"},
		"path":    {string(pathsJSON)},
	}

	if len(additional) > 0 {
		params.Set("additional", strings.Join(additional, ","))
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Files []FileInfo `json:"files"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse getinfo response: %w", err)
	}

	return result.Files, nil
}

// DownloadFile downloads a file and returns the response body
// The caller is responsible for closing the returned ReadCloser
func (c *Client) DownloadFile(filePath string, mode string) (io.ReadCloser, string, int64, error) {
	apiPath, version, err := c.getAPIPath(APIFileStationDownload)
	if err != nil {
		return nil, "", 0, err
	}

	if mode == "" {
		mode = "download"
	}

	params := url.Values{
		"api":     {APIFileStationDownload},
		"version": {strconv.Itoa(version)},
		"method":  {"download"},
		"path":    {filePath},
		"mode":    {mode},
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

	// Get filename from Content-Disposition header
	filename := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			filename = strings.Trim(cd[idx+9:], "\"")
		}
	}

	return resp.Body, filename, resp.ContentLength, nil
}

// ShareLink represents a sharing link
type ShareLink struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	QRCode    string `json:"qrcode,omitempty"`
	Path      string `json:"path"`
	IsFolder  bool   `json:"isFolder"`
}

// CreateShareOptions contains options for creating a share link
type CreateShareOptions struct {
	Path          string // File or folder path
	Password      string // Optional password
	DateExpired   string // Expiration date (YYYY-MM-DD), "0" for never
	DateAvailable string // Start date (YYYY-MM-DD)
}

// CreateShareLink creates a sharing link for a file or folder
func (c *Client) CreateShareLink(opts *CreateShareOptions) (*ShareLink, error) {
	if opts == nil || opts.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	apiPath, version, err := c.getAPIPath(APIFileStationSharing)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIFileStationSharing},
		"version": {strconv.Itoa(version)},
		"method":  {"create"},
		"path":    {opts.Path},
	}

	if opts.Password != "" {
		params.Set("password", opts.Password)
	}
	if opts.DateExpired != "" {
		params.Set("date_expired", opts.DateExpired)
	}
	if opts.DateAvailable != "" {
		params.Set("date_available", opts.DateAvailable)
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Links []ShareLink `json:"links"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse create share response: %w", err)
	}

	if len(result.Links) == 0 {
		return nil, fmt.Errorf("no share link created")
	}

	return &result.Links[0], nil
}

// ListShareLinksOptions contains options for listing share links
type ListShareLinksOptions struct {
	Offset    int
	Limit     int
	SortBy    string // id, name, isFolder, path, date_expired, date_available, status, has_password, url, link_owner
	SortDir   string // asc, desc
	ForceClean bool  // If true, remove expired and broken links
}

// ShareLinkInfo represents detailed share link information
type ShareLinkInfo struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Link          string `json:"link,omitempty"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	IsFolder      bool   `json:"isFolder"`
	DateExpired   string `json:"date_expired,omitempty"`
	DateAvailable string `json:"date_available,omitempty"`
	Status        string `json:"status"` // valid, invalid, expired, broken
	HasPassword   bool   `json:"has_password"`
	LinkOwner     string `json:"link_owner"`
}

// ListShareLinksResponse is the response from listing share links
type ListShareLinksResponse struct {
	Offset int             `json:"offset"`
	Total  int             `json:"total"`
	Links  []ShareLinkInfo `json:"links"`
}

// ListShareLinks lists all share links created by the current user
func (c *Client) ListShareLinks(opts *ListShareLinksOptions) (*ListShareLinksResponse, error) {
	apiPath, version, err := c.getAPIPath(APIFileStationSharing)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIFileStationSharing},
		"version": {strconv.Itoa(version)},
		"method":  {"list"},
	}

	if opts != nil {
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.SortBy != "" {
			params.Set("sort_by", opts.SortBy)
		}
		if opts.SortDir != "" {
			params.Set("sort_direction", opts.SortDir)
		}
		if opts.ForceClean {
			params.Set("force_clean", "true")
		}
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result ListShareLinksResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse share links response: %w", err)
	}

	return &result, nil
}

// Favorite represents a favorite item
type Favorite struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Status string `json:"status"` // valid, broken
}

// ListFavoritesResponse is the response from listing favorites
type ListFavoritesResponse struct {
	Offset    int        `json:"offset"`
	Total     int        `json:"total"`
	Favorites []Favorite `json:"favorites"`
}

// ListFavorites lists all favorites (starred files/folders)
func (c *Client) ListFavorites(offset, limit int) (*ListFavoritesResponse, error) {
	apiPath, version, err := c.getAPIPath(APIFileStationFavorite)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIFileStationFavorite},
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

	var result ListFavoritesResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse favorites response: %w", err)
	}

	return &result, nil
}

// FileStationInfo contains FileStation information
type FileStationInfo struct {
	IsManager        bool   `json:"is_manager"`
	SupportSharing   bool   `json:"support_sharing"`
	Hostname         string `json:"hostname"`
	SupportVFS       bool   `json:"support_vfs"`
}

// GetFileStationInfo gets FileStation information
func (c *Client) GetFileStationInfo() (*FileStationInfo, error) {
	apiPath, version, err := c.getAPIPath(APIFileStationInfo)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"api":     {APIFileStationInfo},
		"version": {strconv.Itoa(version)},
		"method":  {"get"},
	}

	resp, err := c.doAPIRequestWithRetry(apiPath, params)
	if err != nil {
		return nil, err
	}

	var result FileStationInfo
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse info response: %w", err)
	}

	return &result, nil
}
