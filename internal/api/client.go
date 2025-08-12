package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/pkg/types"
)

type Client struct {
	baseURL     string
	httpClient  *retryablehttp.Client
	limiter     *rate.Limiter
	token       string
	userAgent   string
	debug       bool
	isAnonymous bool

	requestCount  int64
	errorCount    int64
	lastRequestAt time.Time

	cfg *config.Config
}

type SortOption string

const (
	SortDefault       SortOption = ""
	SortPlayed        SortOption = "played"
	SortLikes         SortOption = "likes"
	SortLikesReversed SortOption = "-likes"
	SortLength        SortOption = "length"
	SortUploaded      SortOption = "uploaded"
)

func NewClient(cfg *config.Config) *Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = cfg.API.Retries
	retryClient.HTTPClient.Timeout = time.Duration(cfg.API.Timeout) * time.Second
	retryClient.Logger = nil

	if cfg.Debug {
		retryClient.Logger = &debugLogger{}
	}

	limiter := rate.NewLimiter(
		rate.Limit(cfg.API.RateLimit.RequestsPerSecond),
		cfg.API.RateLimit.BurstSize,
	)

	client := &Client{
		baseURL:     cfg.API.BaseURL,
		httpClient:  retryClient,
		limiter:     limiter,
		token:       cfg.API.Token,
		userAgent:   cfg.API.UserAgent,
		debug:       cfg.Debug,
		isAnonymous: cfg.User.IsAnonymous,
		cfg:         cfg,
	}

	client.debugLog("API Client initialized - Base URL: %s, Debug: %v, Anonymous: %v",
		cfg.API.BaseURL, cfg.Debug, cfg.User.IsAnonymous)

	return client
}

type debugLogger struct{}

func (d *debugLogger) Printf(format string, args ...interface{}) {
	log.Printf("[HTTP] "+format, args...)
}

func (c *Client) debugLog(format string, args ...interface{}) {
	if !c.debug {
		return
	}
	log.Printf("[API] "+format, args...)
}

func (c *Client) debugRequest(method, url string, body interface{}) {
	if !c.debug {
		return
	}

	c.requestCount++
	c.lastRequestAt = time.Now()

	authType := "AUTHENTICATED"
	if c.isAnonymous {
		authType = "ANONYMOUS"
	}

	var bodyStr string
	if body != nil {
		if bodyBytes, err := json.Marshal(body); err == nil {
			bodyStr = string(bodyBytes)
			if len(bodyStr) > 200 {
				bodyStr = bodyStr[:200] + "..."
			}
		}
	}

	c.debugLog("REQUEST #%d [%s] %s %s - Body: %s",
		c.requestCount, authType, method, url, bodyStr)
}

func (c *Client) debugResponse(method, url string, statusCode int, duration time.Duration, err error) {
	if !c.debug {
		return
	}

	if err != nil {
		c.errorCount++
		log.Printf("[API] ERROR %s %s - Status: %d - Duration: %v - Error: %v",
			method, url, statusCode, duration, err)
	}

	if c.requestCount%50 == 0 {
		log.Printf("[API] STATS - Total Requests: %d, Errors: %d, Error Rate: %.2f%%",
			c.requestCount, c.errorCount, float64(c.errorCount)/float64(max(c.requestCount, 1))*100)
	}
}

func (c *Client) makeRequest(ctx context.Context, method, path string, params url.Values, body interface{}) (*http.Response, []byte, error) {
	startTime := time.Now()

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, nil, fmt.Errorf("rate limit wait: %w", err)
	}

	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	c.debugRequest(method, fullURL, body)

	var reqBody io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			c.debugResponse(method, fullURL, 0, time.Since(startTime), err)
			return nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		c.debugResponse(method, fullURL, 0, time.Since(startTime), err)
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.token != "" && !c.isAnonymous {
		req.Header.Set("Authorization", "Bearer "+c.token)
		c.debugLog("Using token: %s...", c.token[:min(len(c.token), 10)])
	} else if c.isAnonymous {
		c.debugLog("Anonymous mode: not sending Authorization header")
	} else {
		c.debugLog("No authentication token provided")
	}

	c.debugLog("Headers: " + fmt.Sprintf("%v", req.Header))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.debugResponse(method, fullURL, 0, time.Since(startTime), err)
		return nil, nil, fmt.Errorf("do request: %w", err)
	}

	responseBody, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		c.debugLog("Failed to close response body: %v", closeErr)
	}

	if readErr != nil {
		c.debugResponse(method, fullURL, resp.StatusCode, time.Since(startTime), readErr)
		return resp, nil, fmt.Errorf("read response body: %w", readErr)
	}

	c.debugResponse(method, fullURL, resp.StatusCode, time.Since(startTime), nil)

	if resp.StatusCode >= 400 {
		var apiError struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		}

		if json.Unmarshal(responseBody, &apiError) == nil {
			errorMsg := apiError.Error
			if errorMsg == "" {
				errorMsg = apiError.Message
			}
			if errorMsg == "" {
				errorMsg = apiError.Detail
			}
			if errorMsg == "" {
				errorMsg = resp.Status
			}

			err := fmt.Errorf("API error %d: %s", resp.StatusCode, errorMsg)
			c.debugResponse(method, fullURL, resp.StatusCode, time.Since(startTime), err)
			return resp, responseBody, err
		}

		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		c.debugResponse(method, fullURL, resp.StatusCode, time.Since(startTime), err)
		return resp, responseBody, err
	}

	return resp, responseBody, nil
}

func (c *Client) EnsureAnonymousToken(ctx context.Context) (string, error) {
	if c.token != "" && c.isAnonymous {
		c.debugLog("Using persisted anonymous token: %s...", c.token[:min(len(c.token), 10)])
		return c.token, nil
	}

	if c.cfg != nil && c.cfg.API.Token != "" && c.cfg.User.IsAnonymous {
		c.debugLog("Adopting anonymous token from config: %s...", c.cfg.API.Token[:min(len(c.cfg.API.Token), 10)])
		c.SetToken(c.cfg.API.Token)
		return c.token, nil
	}

	c.debugLog("Requesting new anonymous token...")
	_, responseBody, err := c.makeRequest(ctx, "POST", "/music/anon/create/", nil, map[string]any{})
	if err != nil {
		c.isAnonymous = true
		c.token = ""
		return "", fmt.Errorf("get anonymous token: %w", err)
	}

	var authResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &authResp); err != nil || authResp.ID == "" {
		c.isAnonymous = true
		c.token = ""
		if err == nil {
			err = fmt.Errorf("empty token in response")
		}
		return "", fmt.Errorf("parse anonymous token response: %w", err)
	}

	// Persist the new anon token
	c.isAnonymous = true
	c.SetToken(authResp.ID)
	c.debugLog("Anonymous token obtained and saved: %s...", authResp.ID[:min(len(authResp.ID), 10)])
	return authResp.ID, nil
}

func (c *Client) Authenticate(ctx context.Context, token string) error {
	c.debugLog("Authenticating with token: %s...", token[:min(len(token), 10)])

	oldToken := c.token
	oldAnonymous := c.isAnonymous

	c.token = token
	c.isAnonymous = false

	_, _, err := c.makeRequest(ctx, "GET", "/users/self/", nil, nil)
	if err != nil {
		c.token = oldToken
		c.isAnonymous = oldAnonymous
		return fmt.Errorf("authenticate: %w", err)
	}

	c.SetToken(token)
	c.debugLog("Authentication successful")
	return nil
}

func (c *Client) GetCurrentUser(ctx context.Context) (*types.User, error) {
	c.debugLog("Getting current user info...")

	_, responseBody, err := c.makeRequest(ctx, "GET", "/users/self/", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	var user types.User
	if err := json.Unmarshal(responseBody, &user); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}

	c.debugLog("Current user: %s (%s)", user.Username, user.Email)
	return &user, nil
}

func (c *Client) GetSongs(ctx context.Context, page int, search string) (*types.SongListResponse, error) {
	return c.GetSongsWithSort(ctx, page, search, SortDefault)
}

func (c *Client) GetSongsWithSort(ctx context.Context, page int, search string, sortOption SortOption) (*types.SongListResponse, error) {
	c.debugLog("Getting songs - page: %d, search: '%s', sort: '%s'", page, search, sortOption)

	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if search != "" {
		params.Set("search", search)
	}
	if sortOption != SortDefault {
		params.Set("sort", string(sortOption))
	}

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/song/", params, nil)
	if err != nil {
		return nil, fmt.Errorf("get songs: %w", err)
	}

	var result types.SongListResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode songs response: %w", err)
	}

	c.debugLog("Retrieved %d songs (page %d)", len(result.Results), page)
	return &result, nil
}

func (c *Client) GetSong(ctx context.Context, slug string) (*types.Song, error) {
	c.debugLog("Getting song: %s", slug)

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/song/"+slug, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get song: %w", err)
	}

	var song types.Song
	if err := json.Unmarshal(responseBody, &song); err != nil {
		return nil, fmt.Errorf("decode song response: %w", err)
	}

	c.debugLog("Retrieved song: %s", song.Name)
	return &song, nil
}

func (c *Client) GetAlbums(ctx context.Context, page int, search string) (*types.AlbumListResponse, error) {
	c.debugLog("Getting albums - page: %d, search: '%s'", page, search)

	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if search != "" {
		params.Set("search", search)
	}

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/albums/", params, nil)
	if err != nil {
		return nil, fmt.Errorf("get albums: %w", err)
	}

	var result types.AlbumListResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode albums response: %w", err)
	}

	c.debugLog("Retrieved %d albums (page %d)", len(result.Results), page)
	return &result, nil
}

func (c *Client) GetAlbum(ctx context.Context, slug string) (*types.Album, error) {
	c.debugLog("Getting album: %s", slug)

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/albums/"+slug, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get album: %w", err)
	}

	var album types.Album
	if err := json.Unmarshal(responseBody, &album); err != nil {
		return nil, fmt.Errorf("decode album response: %w", err)
	}

	c.debugLog("Retrieved album: %s", album.Name)
	return &album, nil
}

func (c *Client) GetAuthors(ctx context.Context, page int, search string) (*types.AuthorListResponse, error) {
	c.debugLog("Getting authors - page: %d, search: '%s'", page, search)

	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if search != "" {
		params.Set("search", search)
	}

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/authors/", params, nil)
	if err != nil {
		return nil, fmt.Errorf("get authors: %w", err)
	}

	var result types.AuthorListResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode authors response: %w", err)
	}

	c.debugLog("Retrieved %d authors (page %d)", len(result.Results), page)
	return &result, nil
}

func (c *Client) GetAuthor(ctx context.Context, slug string) (*types.Author, error) {
	c.debugLog("Getting author: %s", slug)

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/authors/"+slug, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get author: %w", err)
	}

	var author types.Author
	if err := json.Unmarshal(responseBody, &author); err != nil {
		return nil, fmt.Errorf("decode author response: %w", err)
	}

	c.debugLog("Retrieved author: %s", author.Name)
	return &author, nil
}

func (c *Client) GetPlaylists(ctx context.Context) ([]*types.Playlist, error) {
	c.debugLog("Getting playlists...")

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/playlists/", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get playlists: %w", err)
	}

	var playlists []*types.Playlist
	if err := json.Unmarshal(responseBody, &playlists); err != nil {
		return nil, fmt.Errorf("decode playlists response: %w", err)
	}

	c.debugLog("Retrieved %d playlists", len(playlists))
	return playlists, nil
}

func (c *Client) GetPlaylist(ctx context.Context, slug string) (*types.Playlist, error) {
	c.debugLog("Getting playlist: %s", slug)

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/playlists/"+slug, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get playlist: %w", err)
	}

	var playlist types.Playlist
	if err := json.Unmarshal(responseBody, &playlist); err != nil {
		return nil, fmt.Errorf("decode playlist response: %w", err)
	}

	c.debugLog("Retrieved playlist: %s (%d songs)", playlist.Name, len(playlist.Songs))
	return &playlist, nil
}

func (c *Client) CreatePlaylist(ctx context.Context, playlist *types.Playlist) error {
	c.debugLog("Creating playlist: %s", playlist.Name)

	_, responseBody, err := c.makeRequest(ctx, "POST", "/music/playlists/", nil, playlist)
	if err != nil {
		return fmt.Errorf("create playlist: %w", err)
	}

	if err := json.Unmarshal(responseBody, playlist); err != nil {
		return fmt.Errorf("decode create playlist response: %w", err)
	}

	c.debugLog("Playlist created successfully: %s", playlist.Name)
	return nil
}

func (c *Client) UpdatePlaylist(ctx context.Context, playlist *types.Playlist) error {
	c.debugLog("Updating playlist: %s", playlist.Name)

	_, responseBody, err := c.makeRequest(ctx, "PUT", "/music/playlists/"+playlist.Slug, nil, playlist)
	if err != nil {
		return fmt.Errorf("update playlist: %w", err)
	}

	if err := json.Unmarshal(responseBody, playlist); err != nil {
		return fmt.Errorf("decode update playlist response: %w", err)
	}

	c.debugLog("Playlist updated successfully: %s", playlist.Name)
	return nil
}

func (c *Client) DeletePlaylist(ctx context.Context, slug string) error {
	c.debugLog("Deleting playlist: %s", slug)

	_, _, err := c.makeRequest(ctx, "DELETE", "/music/playlists/"+slug, nil, nil)
	if err != nil {
		return fmt.Errorf("delete playlist: %w", err)
	}

	c.debugLog("Playlist deleted successfully: %s", slug)
	return nil
}

func (c *Client) LikeSong(ctx context.Context, slug string) error {
	c.debugLog("Liking song: %s", slug)

	data := map[string]string{"song": slug}
	_, _, err := c.makeRequest(ctx, "POST", "/music/song/like/", nil, data)
	if err != nil {
		return fmt.Errorf("like song: %w", err)
	}

	c.debugLog("Song liked successfully: %s", slug)
	return nil
}

func (c *Client) DislikeSong(ctx context.Context, slug string) error {
	c.debugLog("Disliking song: %s", slug)

	data := map[string]string{"song": slug}
	_, _, err := c.makeRequest(ctx, "POST", "/music/song/dislike/", nil, data)
	if err != nil {
		return fmt.Errorf("dislike song: %w", err)
	}

	c.debugLog("Song disliked successfully: %s", slug)
	return nil
}

func (c *Client) ListenSong(ctx context.Context, slug string, userID string) error {
	c.debugLog("Recording listen for song: %s, user: %s", slug, userID)

	// user_id must be the anon token when anonymous, otherwise null
	payload := map[string]interface{}{
		"song":    slug,
		"user_id": nil,
	}
	if c.isAnonymous {
		if userID != "" {
			payload["user_id"] = userID
		} else if c.token != "" {
			payload["user_id"] = c.token // anon id
		}
	}

	_, _, err := c.makeRequest(ctx, "POST", "/music/song/listen/", nil, payload)
	if err != nil {
		return fmt.Errorf("listen song: %w", err)
	}
	c.debugLog("Listen recorded successfully for song: %s", slug)
	return nil
}

func (c *Client) SearchAll(ctx context.Context, query string) (*types.SearchResponse, error) {
	c.debugLog("Searching for: '%s'", query)

	params := url.Values{}
	params.Set("search", query)

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/search/", params, nil)
	if err != nil {
		return nil, fmt.Errorf("search all: %w", err)
	}

	var result types.SearchResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	c.debugLog("Search results - Songs: %d, Albums: %d, Authors: %d",
		len(result.Songs), len(result.Albums), len(result.Authors))
	return &result, nil
}

func (c *Client) IsAnonymous() bool {
	return c.isAnonymous
}

func (c *Client) SetDebug(debug bool) {
	c.debug = debug
	c.debugLog("Debug logging %s", map[bool]string{true: "enabled", false: "disabled"}[debug])
}

func (c *Client) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_requests":  c.requestCount,
		"total_errors":    c.errorCount,
		"error_rate":      float64(c.errorCount) / float64(max(c.requestCount, 1)) * 100,
		"last_request_at": c.lastRequestAt,
		"is_anonymous":    c.isAnonymous,
		"has_token":       c.token != "",
		"base_url":        c.baseURL,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (c *Client) GetLikedSongs(ctx context.Context) ([]*types.Song, error) {
	c.debugLog("Getting liked songs...")

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/song/liked/", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get liked songs: %w", err)
	}

	var songs []*types.Song
	if err := json.Unmarshal(responseBody, &songs); err != nil {
		return nil, fmt.Errorf("decode liked songs response: %w", err)
	}

	c.debugLog("Retrieved %d liked songs", len(songs))
	return songs, nil
}

func (c *Client) GetListenHistory(ctx context.Context) ([]*types.Song, error) {
	c.debugLog("Getting listen history...")

	_, responseBody, err := c.makeRequest(ctx, "GET", "/music/song/listened/", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get listen history: %w", err)
	}

	var songs []*types.Song
	if err := json.Unmarshal(responseBody, &songs); err != nil {
		return nil, fmt.Errorf("decode listen history response: %w", err)
	}

	c.debugLog("Retrieved %d songs from listen history", len(songs))
	return songs, nil
}
