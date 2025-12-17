package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juruen/rmapi/api"
	"github.com/juruen/rmapi/api/sync15"
	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/config"
	"github.com/juruen/rmapi/filetree"
	"github.com/juruen/rmapi/hwr"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/shell"
	"github.com/juruen/rmapi/transport"
	"github.com/juruen/rmapi/util"
	"github.com/juruen/rmapi/version"
	"github.com/juruen/rmapi/visualize"
)


type ApiServer struct {
	mu            sync.RWMutex
	ctx           api.ApiCtx
	userInfo      *api.UserInfo
	shellCtx      *shell.ShellCtxt
	refreshCancel context.CancelFunc
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func NewApiServer() (*ApiServer, error) {
	server := &ApiServer{}
	
	// Try to initialize with existing tokens, but don't fail if they don't exist
	err := server.initialize()
	if err != nil {
		log.Info.Println("Server starting without authentication. Use POST /api/auth to authenticate.")
		log.Trace.Println("Initialization error (expected if no token):", err)
	} else {
		// Start background token refresh goroutine if authentication succeeded
		server.startTokenRefreshGoroutine()
	}
	
	return server, nil
}

func (s *ApiServer) initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if tokens exist before attempting authentication
	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %v", err)
	}
	
	authTokens := config.LoadTokens(configPath)
	if authTokens.DeviceToken == "" {
		return fmt.Errorf("no device token found")
	}
	
	var ctx api.ApiCtx
	var userInfo *api.UserInfo
	var initErr error

	ni := true // Non-interactive for server mode
	const AUTH_RETRIES = 3

	for i := 0; i < AUTH_RETRIES; i++ {
		authCtx := api.AuthHttpCtx(i > 0, ni)

		userInfo, initErr = api.ParseToken(authCtx.Tokens.UserToken)
		if initErr != nil {
			log.Trace.Println(initErr)
			continue
		}

		ctx, initErr = api.CreateApiCtx(authCtx, userInfo.SyncVersion)
		if initErr != nil {
			log.Trace.Println(initErr)
		} else {
			break
		}
	}

	if initErr != nil {
		return fmt.Errorf("failed to build documents tree, last error: %v", initErr)
	}

	shellCtx := &shell.ShellCtxt{
		Node:           ctx.Filetree().Root(),
		Api:            ctx,
		Path:           ctx.Filetree().Root().Name(),
		UseHiddenFiles: shell.UseHiddenFiles(),
		UserInfo:       *userInfo,
		JSONOutput:     true,
	}

	s.ctx = ctx
	s.userInfo = userInfo
	s.shellCtx = shellCtx
	
	return nil
}

// refreshTokens refreshes only the user token without recreating the API context
func (s *ApiServer) refreshTokens() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.ctx == nil {
		return fmt.Errorf("API context not initialized")
	}
	
	err := s.ctx.RefreshToken()
	if err != nil {
		return err
	}
	
	// Update user info from refreshed token
	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %v", err)
	}
	authTokens := config.LoadTokens(configPath)
	userInfo, err := api.ParseToken(authTokens.UserToken)
	if err != nil {
		return fmt.Errorf("failed to parse refreshed token: %v", err)
	}
	
	s.userInfo = userInfo
	s.shellCtx.UserInfo = *userInfo
	
	log.Info.Println("Tokens refreshed successfully")
	return nil
}

// startTokenRefreshGoroutine starts a background goroutine that refreshes tokens every 2 hours
func (s *ApiServer) startTokenRefreshGoroutine() {
	ctx, cancel := context.WithCancel(context.Background())
	s.refreshCancel = cancel
	
	go func() {
		ticker := time.NewTicker(2 * time.Hour)
		defer ticker.Stop()
		
		log.Info.Println("Started background token refresh goroutine (refreshes every 2 hours)")
		
		// Refresh immediately on startup (in case token is close to expiring)
		if err := s.refreshTokens(); err != nil {
			log.Trace.Printf("Initial token refresh failed (may not be authenticated yet): %v", err)
		}
		
		for {
			select {
			case <-ctx.Done():
				log.Info.Println("Stopping token refresh goroutine")
				return
			case <-ticker.C:
				if err := s.refreshTokens(); err != nil {
					log.Trace.Printf("Background token refresh failed: %v", err)
					// Continue trying - don't stop the goroutine on error
				} else {
					log.Trace.Println("Background token refresh succeeded")
				}
			}
		}
	}()
}

// fetchDocumentWithRetry fetches a document and automatically refreshes tokens if needed
func (s *ApiServer) fetchDocumentWithRetry(docId, dstPath string) error {
	err := s.ctx.FetchDocument(docId, dstPath)
	if err != nil {
		// Check if error is due to expired token (401 Unauthorized)
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			log.Info.Println("Token expired, attempting to refresh...")
			if refreshErr := s.refreshTokens(); refreshErr != nil {
				return fmt.Errorf("failed to refresh tokens: %v", refreshErr)
			}
			// Retry the fetch after token refresh
			return s.ctx.FetchDocument(docId, dstPath)
		}
		return err
	}
	return nil
}

func (s *ApiServer) isAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx != nil && s.userInfo != nil
}

func (s *ApiServer) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	s.mu.RLock()
	authenticated := s.ctx != nil && s.userInfo != nil && s.shellCtx != nil
	s.mu.RUnlock()
	
	if !authenticated {
		s.writeError(w, http.StatusUnauthorized, fmt.Errorf("not authenticated. Please authenticate using POST /api/auth with your one-time code from https://my.remarkable.com/device/browser/connect"))
		return false
	}
	return true
}

func (s *ApiServer) writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
}

func (s *ApiServer) writeSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SuccessResponse{Data: data})
}

// POST /api/auth or GET /api/auth?code=XXXXXX - Authenticate with one-time code
func (s *ApiServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	var code string

	// Support both GET (query parameter) and POST (JSON body)
	if r.Method == http.MethodGet {
		code = r.URL.Query().Get("code")
		if code == "" {
			// For GET requests, show a helpful HTML page if no code provided
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title>rmapi Authentication</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 600px; margin: 50px auto; padding: 20px; }
		.code-input { padding: 10px; font-size: 16px; width: 200px; text-align: center; letter-spacing: 2px; }
		.submit-btn { padding: 10px 20px; font-size: 16px; background: #007bff; color: white; border: none; cursor: pointer; }
		.submit-btn:hover { background: #0056b3; }
		.info { background: #e7f3ff; padding: 15px; border-radius: 5px; margin: 20px 0; }
		.error { color: red; }
	</style>
</head>
<body>
	<h1>rmapi Authentication</h1>
	<div class="info">
		<p>To authenticate, get your one-time code from:</p>
		<p><strong><a href="https://my.remarkable.com/device/browser/connect" target="_blank">https://my.remarkable.com/device/browser/connect</a></strong></p>
	</div>
	<form method="GET" action="/api/auth">
		<label for="code">Enter your 8-digit code:</label><br><br>
		<input type="text" id="code" name="code" class="code-input" maxlength="8" pattern="[0-9]{8}" placeholder="12345678" required autofocus>
		<br><br>
		<button type="submit" class="submit-btn">Authenticate</button>
	</form>
	<p><small>Or use: <code>GET /api/auth?code=YOUR_CODE</code> or <code>POST /api/auth</code> with JSON body</small></p>
</body>
</html>
			`)
			return
		}
	} else if r.Method == http.MethodPost {
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
			return
		}
		code = req.Code
	} else {
		http.Error(w, "Method not allowed. Use GET or POST", http.StatusMethodNotAllowed)
		return
	}

	if code == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("code is required"))
		return
	}

	// Validate code length
	if len(code) != 8 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("code must be 8 characters"))
		return
	}

	// Get config path
	configPath, err := config.ConfigPath()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to get config path: %v", err))
		return
	}

	// Load existing tokens
	authTokens := config.LoadTokens(configPath)
	httpClientCtx := transport.CreateHttpClientCtx(authTokens)

	// Create device token from code
	deviceToken, err := api.NewDeviceToken(&httpClientCtx, code)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, fmt.Errorf("failed to create device token: %v", err))
		return
	}

	// Save device token
	authTokens.DeviceToken = deviceToken
	httpClientCtx.Tokens.DeviceToken = deviceToken
	config.SaveTokens(configPath, authTokens)

	// Create user token
	userToken, err := api.NewUserToken(&httpClientCtx)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, fmt.Errorf("failed to create user token: %v", err))
		return
	}

	// Save user token
	authTokens.UserToken = userToken
	config.SaveTokens(configPath, authTokens)

	// Reinitialize server with new tokens
	if err := s.initialize(); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to initialize server: %v", err))
		return
	}
	
	// Start token refresh goroutine if not already running
	if s.refreshCancel == nil {
		s.startTokenRefreshGoroutine()
	}

	s.mu.RLock()
	user := s.userInfo.User
	s.mu.RUnlock()
	
	// For GET requests, show a success page; for POST, return JSON
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title>Authentication Successful</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 600px; margin: 50px auto; padding: 20px; }
		.success { background: #d4edda; padding: 20px; border-radius: 5px; border: 1px solid #c3e6cb; }
		.user-info { margin-top: 15px; font-size: 18px; }
	</style>
</head>
<body>
	<div class="success">
		<h1>✓ Authentication Successful!</h1>
		<div class="user-info">
			<p>Authenticated as: <strong>%s</strong></p>
			<p>The server is now ready to use.</p>
		</div>
	</div>
	<p><a href="/">← Back to API</a></p>
</body>
</html>
		`, user)
	} else {
		s.writeSuccess(w, map[string]interface{}{
			"message": "Authentication successful",
			"user":    user,
		})
	}
}

// GET /api/auth/status - Check authentication status
func (s *ApiServer) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	authenticated := s.ctx != nil && s.userInfo != nil
	userInfo := s.userInfo
	s.mu.RUnlock()

	if authenticated {
		s.writeSuccess(w, map[string]interface{}{
			"authenticated": true,
			"user":          userInfo.User,
		})
	} else {
		s.writeSuccess(w, map[string]interface{}{
			"authenticated": false,
			"message":       "Not authenticated. Use POST /api/auth with your one-time code from https://my.remarkable.com/device/browser/connect",
		})
	}
}

// GET /api/ls?path=<path>&compact=<bool>&long=<bool>&reverse=<bool>&dirFirst=<bool>&byTime=<bool>&showTemplates=<bool>
func (s *ApiServer) handleLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	query := r.URL.Query()
	path := query.Get("path")
	if path == "" {
		path = s.shellCtx.Path
	}

	options := shell.LsOptions{
		Compact:       query.Get("compact") == "true",
		Long:          query.Get("long") == "true",
		Reverse:       query.Get("reverse") == "true",
		DirFirst:      query.Get("dirFirst") == "true",
		ByTime:        query.Get("byTime") == "true",
		ShowTemplates: query.Get("showTemplates") == "true",
	}

	var nodes []*model.Node
	var err error
	if path == "" || path == "." {
		nodes = s.shellCtx.Node.Nodes()
	} else {
		nodes, err = s.ctx.Filetree().NodesByPath(path, s.shellCtx.Node, true)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
	}

	sorted := shell.SortNodes(shell.FilterNodes(nodes, options), options)

	jsonNodes := make([]shell.NodeJSON, len(sorted))
	for i, node := range sorted {
		jsonNodes[i] = shell.NodeToJSON(node)
	}

	s.writeSuccess(w, jsonNodes)
}

// GET /api/pwd
func (s *ApiServer) handlePwd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	s.mu.RLock()
	path := s.shellCtx.Path
	s.mu.RUnlock()
	s.writeSuccess(w, map[string]string{"path": path})
}

// POST /api/cd
func (s *ApiServer) handleCd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.Path == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path is required"))
		return
	}

	node, err := s.ctx.Filetree().NodeByPath(req.Path, s.shellCtx.Node)
	if err != nil || node.IsFile() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("directory doesn't exist"))
		return
	}

	path, err := s.ctx.Filetree().NodeToPath(node)
	if err != nil || node.IsFile() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("directory doesn't exist"))
		return
	}

	s.shellCtx.Path = path
	s.shellCtx.Node = node

	s.writeSuccess(w, map[string]string{"path": path})
}

// GET /api/get?path=<path>
func (s *ApiServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	query := r.URL.Query()
	srcName := query.Get("path")
	if srcName == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path parameter is required"))
		return
	}

	node, err := s.ctx.Filetree().NodeByPath(srcName, s.shellCtx.Node)
	if err != nil || node.IsDirectory() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("file doesn't exist"))
		return
	}

	// Download the file to a temporary location
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("rmapi-*.%s", util.RMDOC))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %v", err))
		return
	}
	tmpFile.Close()
	rmdocPath := tmpFile.Name()
	defer os.Remove(rmdocPath)

	err = s.fetchDocumentWithRetry(node.Document.ID, rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to download file: %v", err))
		return
	}

	// Open the downloaded file
	file, err := os.Open(rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to open file: %v", err))
		return
	}
	defer file.Close()

	// Get file info for Content-Length header
	fileInfo, err := file.Stat()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to stat file: %v", err))
		return
	}

	// Set headers for file download
	filename := fmt.Sprintf("%s.%s", node.Name(), util.RMDOC)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	w.WriteHeader(http.StatusOK)

	// Stream the file content to the response
	_, err = io.Copy(w, file)
	if err != nil {
		log.Error.Printf("Failed to stream file: %v", err)
		// Can't write error response here as headers are already sent
		return
	}
}

// Helper function to generate PNG to memory buffer
func generatePNGToBuffer(zipArchive *archive.Zip, pageNumber int, baseName string) (*bytes.Buffer, error) {
	// Create temporary file
	tmpPNG, err := os.CreateTemp("", fmt.Sprintf("rmapi-png-*.png"))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp PNG file: %v", err)
	}
	tmpPNGPath := tmpPNG.Name()
	tmpPNG.Close()
	defer os.Remove(tmpPNGPath)

	// Generate PNG to temp file
	err = visualize.VisualizePage(zipArchive, pageNumber, tmpPNGPath)
	if err != nil {
		return nil, fmt.Errorf("failed to visualize page: %v", err)
	}

	// Read temp file into buffer
	pngData, err := os.ReadFile(tmpPNGPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PNG file: %v", err)
	}

	return bytes.NewBuffer(pngData), nil
}

// GET /api/convert?path=<path>&inline=<bool>
func (s *ApiServer) handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	srcName := query.Get("path")
	if srcName == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path parameter is required"))
		return
	}

	inline := query.Get("inline") == "true"

	node, err := s.ctx.Filetree().NodeByPath(srcName, s.shellCtx.Node)
	if err != nil || node.IsDirectory() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("file doesn't exist"))
		return
	}

	// Download the file to a temporary location
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("rmapi-*.%s", util.RMDOC))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %v", err))
		return
	}
	tmpFile.Close()
	rmdocPath := tmpFile.Name()
	defer os.Remove(rmdocPath)

	err = s.fetchDocumentWithRetry(node.Document.ID, rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to download file: %v", err))
		return
	}

	// Load the archive
	file, err := os.Open(rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to open file: %v", err))
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to stat file: %v", err))
		return
	}

	file.Seek(0, 0)
	zipArchive, err := shell.LoadArchive(file, fileInfo.Size())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to read archive: %v", err))
		return
	}

	baseNameWithoutExt := node.Name()

	if inline {
		// Return PNGs as binary data
		var pngBuffers []*bytes.Buffer
		for i := 0; i < len(zipArchive.Pages); i++ {
			buf, err := generatePNGToBuffer(zipArchive, i, baseNameWithoutExt)
			if err != nil {
				log.Trace.Printf("Failed to convert page %d: %v", i, err)
				continue
			}
			pngBuffers = append(pngBuffers, buf)
		}

		if len(pngBuffers) == 0 {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("no pages were converted"))
			return
		}

		// If single page, return PNG directly
		if len(pngBuffers) == 1 {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s.png\"", baseNameWithoutExt))
			w.WriteHeader(http.StatusOK)
			io.Copy(w, pngBuffers[0])
			return
		}

		// Multiple pages: return as ZIP file
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", baseNameWithoutExt))
		w.WriteHeader(http.StatusOK)

		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		for i, buf := range pngBuffers {
			filename := fmt.Sprintf("%s_page_%d.png", baseNameWithoutExt, i)
			fileWriter, err := zipWriter.Create(filename)
			if err != nil {
				log.Trace.Printf("Failed to create zip entry %s: %v", filename, err)
				continue
			}
			io.Copy(fileWriter, buf)
		}
		return
	}

	// Default behavior: write to disk
	outputDir := "/home/app/downloads"
	var convertedFiles []string

	for i := 0; i < len(zipArchive.Pages); i++ {
		outputPNG := filepath.Join(outputDir, fmt.Sprintf("%s_page_%d.png", baseNameWithoutExt, i))
		err := visualize.VisualizePage(zipArchive, i, outputPNG)
		if err != nil {
			continue
		}
		convertedFiles = append(convertedFiles, outputPNG)
	}

	if len(convertedFiles) == 0 {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("no pages were converted"))
		return
	}

	s.writeSuccess(w, map[string]interface{}{
		"message":        fmt.Sprintf("Converted %d page(s) to PNG", len(convertedFiles)),
		"converted_files": convertedFiles,
	})
}

// GET /api/hwr?path=<path>&type=<Text|Math|Diagram>&lang=<lang>&page=<N>&split=<bool>&inline=<bool>
func (s *ApiServer) handleHwr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	query := r.URL.Query()
	srcName := query.Get("path")
	if srcName == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path parameter is required"))
		return
	}

	inputType := query.Get("type")
	if inputType == "" {
		inputType = "Text"
	}

	lang := query.Get("lang")
	if lang == "" {
		lang = "en_US"
	}

	page := -1
	if pageStr := query.Get("page"); pageStr != "" {
		fmt.Sscanf(pageStr, "%d", &page)
	}

	splitPages := query.Get("split") == "true"
	inline := query.Get("inline") == "true"

	// Check for API credentials
	applicationKey := os.Getenv("RMAPI_HWR_APPLICATIONKEY")
	if applicationKey == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("RMAPI_HWR_APPLICATIONKEY environment variable is required"))
		return
	}
	hmacKey := os.Getenv("RMAPI_HWR_HMAC")
	if hmacKey == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("RMAPI_HWR_HMAC environment variable is required"))
		return
	}

	node, err := s.ctx.Filetree().NodeByPath(srcName, s.shellCtx.Node)
	if err != nil || node.IsDirectory() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("file doesn't exist"))
		return
	}

	// Download the file to a temporary location
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("rmapi-*.%s", util.RMDOC))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %v", err))
		return
	}
	tmpFile.Close()
	rmdocPath := tmpFile.Name()
	defer os.Remove(rmdocPath)

	err = s.fetchDocumentWithRetry(node.Document.ID, rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to download file: %v", err))
		return
	}

	// Load the archive
	file, err := os.Open(rmdocPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to open file: %v", err))
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to stat file: %v", err))
		return
	}

	file.Seek(0, 0)
	zipArchive, err := shell.LoadArchive(file, fileInfo.Size())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to read archive: %v", err))
		return
	}

	baseNameWithoutExt := node.Name()

	if inline {
		// Inline mode: return ZIP file with TXT files
		cfg := hwr.Config{
			Page:      page,
			Lang:      lang,
			InputType: inputType,
			BatchSize: 3,
		}

		textContent, err := hwr.HwrInline(zipArchive, cfg, applicationKey, hmacKey)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("HWR failed: %v", err))
			return
		}

		if len(textContent) == 0 {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("no pages were recognized"))
			return
		}

		// Always return as ZIP file, even for single page (consistent behavior)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", baseNameWithoutExt))
		w.WriteHeader(http.StatusOK)

		zipWriter := zip.NewWriter(w)
		
		// Write all files to ZIP
		writtenCount := 0
		for pageNum, text := range textContent {
			filename := fmt.Sprintf("%s_page_%d.txt", baseNameWithoutExt, pageNum)
			fileWriter, err := zipWriter.Create(filename)
			if err != nil {
				log.Trace.Printf("Failed to create zip entry %s: %v", filename, err)
				continue
			}
			if _, err := fileWriter.Write([]byte(text)); err != nil {
				log.Trace.Printf("Failed to write zip entry %s: %v", filename, err)
				continue
			}
			writtenCount++
		}
		
		// Close the ZIP writer to finalize the archive
		if err := zipWriter.Close(); err != nil {
			log.Trace.Printf("Failed to close zip writer: %v", err)
			// Can't write error response here as headers are already sent
			return
		}
		
		if writtenCount == 0 {
			log.Trace.Printf("Warning: No files written to ZIP")
		}
		return
	}

	// Default behavior: write to disk
	outputDir := "/home/app/downloads"
	outputFile := filepath.Join(outputDir, baseNameWithoutExt)

	cfg := hwr.Config{
		Page:       page,
		Lang:       lang,
		InputType:  inputType,
		OutputFile: outputFile,
		SplitPages: splitPages,
		BatchSize:  3,
	}

	outputFiles, err := hwr.Hwr(zipArchive, cfg, applicationKey, hmacKey)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("HWR failed: %v", err))
		return
	}

	if len(outputFiles) == 0 {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("no output files were created"))
		return
	}

	s.writeSuccess(w, map[string]interface{}{
		"message":      fmt.Sprintf("Recognized %d page(s)", len(outputFiles)),
		"output_files": outputFiles,
	})
}

// POST /api/mkdir
func (s *ApiServer) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var target string
	recursive := false

	// Try to get path from query parameter first
	target = r.URL.Query().Get("path")
	recursiveParam := r.URL.Query().Get("recursive")
	if recursiveParam == "true" || recursiveParam == "1" {
		recursive = true
	}

	if target == "" {
		// Fall back to JSON body
		var req struct {
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		target = req.Path
		recursive = req.Recursive
	}

	if target == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path is required"))
		return
	}

	// Check if target already exists
	existingNode, err := s.ctx.Filetree().NodeByPath(target, s.shellCtx.Node)
	if err == nil {
		if existingNode.IsDirectory() {
			s.writeSuccess(w, map[string]interface{}{
				"message": "Directory already exists",
				"node":    shell.NodeToJSON(existingNode),
			})
			return
		} else {
			s.writeError(w, http.StatusConflict, fmt.Errorf("file already exists at path"))
			return
		}
	}

	// Normalize path
	target = strings.Trim(target, "/")
	if target == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid path"))
		return
	}

	// Split path into components
	parts := strings.Split(target, "/")
	if len(parts) == 0 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid directory name"))
		return
	}

	// Find or create parent directories recursively
	currentPath := ""
	currentNode := s.shellCtx.Node
	parentId := ""

	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		currentPath = path.Join(currentPath, part)

		// Check if this directory exists
		checkNode, err := s.ctx.Filetree().NodeByPath(currentPath, s.shellCtx.Node)
		if err != nil {
			// Directory doesn't exist
			if !recursive {
				s.writeError(w, http.StatusNotFound, fmt.Errorf("parent directory doesn't exist: %s (use recursive=true to create)", currentPath))
				return
			}

			// Create the parent directory
			createParentId := parentId
			if currentNode.IsRoot() {
				createParentId = ""
			}
			parentDoc, err := s.ctx.CreateDir(createParentId, part, true)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create parent directory %s: %v", currentPath, err))
				return
			}
			s.ctx.Filetree().AddDocument(parentDoc)
			newNode := model.CreateNode(*parentDoc)
			checkNode = &newNode
		}

		if checkNode.IsFile() {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("path component is a file, not a directory: %s", currentPath))
			return
		}

		currentNode = checkNode
		parentId = currentNode.Id()
		if currentNode.IsRoot() {
			parentId = ""
		}
	}

	// Create the final directory
	newDir := parts[len(parts)-1]
	if newDir == "" || newDir == "." || newDir == ".." {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid directory name"))
		return
	}

	finalParentId := parentId
	if currentNode.IsRoot() {
		finalParentId = ""
	}

	document, err := s.ctx.CreateDir(finalParentId, newDir, true)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create directory: %v", err))
		return
	}

	s.ctx.Filetree().AddDocument(document)
	node := model.CreateNode(*document)
	s.writeSuccess(w, map[string]interface{}{
		"message": "Directory created",
		"node":    shell.NodeToJSON(&node),
	})
}

// DELETE /api/rm?path=<path>&recursive=<bool>
func (s *ApiServer) handleRm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	query := r.URL.Query()
	pathParam := query.Get("path")
	
	// If path is empty, try parsing raw query string (handles edge cases with encoding)
	if pathParam == "" {
		if rawQuery := r.URL.RawQuery; rawQuery != "" {
			if parsed, err := url.ParseQuery(rawQuery); err == nil {
				pathParam = parsed.Get("path")
			}
		}
	}
	
	if pathParam == "" {
		log.Trace.Printf("DELETE /api/rm: RawQuery=%s, Query()=%v", r.URL.RawQuery, r.URL.Query())
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path parameter is required"))
		return
	}
	
	// URL decode the path in case it's double-encoded or has special characters
	decodedPath, err := url.QueryUnescape(pathParam)
	if err != nil {
		// If decoding fails, use the original
		log.Trace.Printf("Failed to decode path '%s': %v, using original", pathParam, err)
		decodedPath = pathParam
	}
	path := decodedPath
	
	log.Trace.Printf("DELETE /api/rm: path='%s' (decoded from '%s')", path, pathParam)

	recursive := query.Get("recursive") == "true"

	nodes, err := s.ctx.Filetree().NodesByPath(path, s.shellCtx.Node, false)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	var deleted []string
	for _, node := range nodes {
		err = s.ctx.DeleteEntry(node, recursive, true)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to delete entry: %v", err))
			return
		}
		s.ctx.Filetree().DeleteNode(node)
		deleted = append(deleted, node.Name())
	}

	err = s.ctx.SyncComplete()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, map[string]interface{}{
		"message": "Entries deleted",
		"deleted": deleted,
	})
}

// POST /api/mv
func (s *ApiServer) handleMv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.Source == "" || req.Destination == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("source and destination are required"))
		return
	}

	srcNodes, err := s.ctx.Filetree().NodesByPath(req.Source, s.shellCtx.Node, false)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}
	if len(srcNodes) < 1 {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("no nodes found"))
		return
	}

	dstNode, _ := s.ctx.Filetree().NodeByPath(req.Destination, s.shellCtx.Node)
	if dstNode != nil && dstNode.IsFile() {
		s.writeError(w, http.StatusConflict, fmt.Errorf("destination entry already exists"))
		return
	}

	var moved []string
	if dstNode != nil && dstNode.IsDirectory() {
		for _, node := range srcNodes {
			if shell.IsSubdir(node, dstNode) {
				s.writeError(w, http.StatusBadRequest, fmt.Errorf("cannot move: %s in itself", node.Name()))
				return
			}

			n, err := s.ctx.MoveEntry(node, dstNode, node.Name())
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to move entry: %v", err))
				return
			}

			s.ctx.Filetree().MoveNode(node, n)
			moved = append(moved, node.Name())
		}
		err = s.ctx.SyncComplete()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("cannot notify: %v", err))
			return
		}
	} else {
		if len(srcNodes) > 1 {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("cannot rename multiple nodes"))
			return
		}

		srcNode := srcNodes[0]
		parentDir := path.Dir(req.Destination)
		newEntry := path.Base(req.Destination)

		parentNode, err := s.ctx.Filetree().NodeByPath(parentDir, s.shellCtx.Node)
		if err != nil || parentNode.IsFile() {
			s.writeError(w, http.StatusNotFound, fmt.Errorf("cannot move: %v", err))
			return
		}

		n, err := s.ctx.MoveEntry(srcNode, parentNode, newEntry)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to move entry: %v", err))
			return
		}

		err = s.ctx.SyncComplete()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("cannot notify: %v", err))
			return
		}

		s.ctx.Filetree().MoveNode(srcNode, n)
		moved = append(moved, srcNode.Name())
	}

	s.writeSuccess(w, map[string]interface{}{
		"message": "Entry moved",
		"moved":   moved,
	})
}

// POST /api/put
func (s *ApiServer) handlePut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var file io.ReadCloser
	var filename string
	var destDir, force, contentOnly, coverpageStr string

	// Check if it's multipart/form-data or raw binary
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle multipart/form-data upload
		err := r.ParseMultipartForm(32 << 20) // 32 MB max
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("failed to parse multipart form: %v", err))
			return
		}

		var header *multipart.FileHeader
		file, header, err = r.FormFile("file")
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("file is required: %v", err))
			return
		}
		filename = header.Filename

		destDir = r.FormValue("destination")
		force = r.FormValue("force")
		contentOnly = r.FormValue("contentOnly")
		coverpageStr = r.FormValue("coverpage")
	} else {
		// Handle raw binary upload
		// Get filename from X-Filename header or Content-Disposition header
		filename = r.Header.Get("X-Filename")
		if filename == "" {
			contentDisposition := r.Header.Get("Content-Disposition")
			if contentDisposition != "" {
				// Parse filename from Content-Disposition: attachment; filename="example.pdf"
				// Also handles: filename=example.pdf (without quotes)
				if idx := strings.Index(contentDisposition, "filename="); idx != -1 {
					filenamePart := contentDisposition[idx+9:]
					// Remove quotes if present
					filename = strings.Trim(filenamePart, `"`)
					// Handle cases like: filename="example.pdf"; or filename=example.pdf;
					if idx2 := strings.Index(filename, ";"); idx2 != -1 {
						filename = filename[:idx2]
					}
					filename = strings.TrimSpace(filename)
				}
			}
		}
		if filename == "" {
			// Default filename based on Content-Type
			if strings.Contains(contentType, "pdf") {
				filename = "document.pdf"
			} else {
				filename = "document"
			}
		}

		file = r.Body

		// Get parameters from query string or headers
		destDir = r.URL.Query().Get("destination")
		if destDir == "" {
			destDir = r.Header.Get("X-Destination")
		}
		force = r.URL.Query().Get("force")
		if force == "" {
			force = r.Header.Get("X-Force")
		}
		contentOnly = r.URL.Query().Get("contentOnly")
		if contentOnly == "" {
			contentOnly = r.Header.Get("X-Content-Only")
		}
		coverpageStr = r.URL.Query().Get("coverpage")
		if coverpageStr == "" {
			coverpageStr = r.Header.Get("X-Coverpage")
		}
	}
	defer file.Close()

	if destDir == "" {
		destDir = s.shellCtx.Path
	}

	forceBool := force == "true"
	contentOnlyBool := contentOnly == "true"

	if forceBool && contentOnlyBool {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("--force and --content-only cannot be used together"))
		return
	}

	var coverpageFlag *int
	if coverpageStr == "1" {
		val := 0
		coverpageFlag = &val
	}

	// Save uploaded file temporarily with the desired filename
	// First create a temp directory to avoid filename conflicts
	tmpDir, err := os.MkdirTemp("", "rmapi-upload-")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp directory: %v", err))
		return
	}
	defer os.RemoveAll(tmpDir)

	// Sanitize filename to prevent path traversal
	safeFilename := filepath.Base(filename) // Remove any directory components
	if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
		safeFilename = "document.pdf"
	}

	// Create temp file with the desired filename
	tmpFilePath := filepath.Join(tmpDir, safeFilename)
	tmpFile, err := os.Create(tmpFilePath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %v", err))
		return
	}

	_, err = io.Copy(tmpFile, file)
	tmpFile.Close()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to save uploaded file: %v", err))
		return
	}

	node, err := s.ctx.Filetree().NodeByPath(destDir, s.shellCtx.Node)
	if err != nil || node.IsFile() {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("directory doesn't exist"))
		return
	}

	docName, _ := util.DocPathToName(filename)

	if contentOnlyBool {
		_, ext := util.DocPathToName(filename)
		if ext != "pdf" {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("--content-only can only be used with PDF files"))
			return
		}

		existingNode, err := s.ctx.Filetree().NodeByPath(docName, node)
		if err != nil {
			// Document doesn't exist, create new one
			dstDir := node.Id()
			document, err := s.ctx.UploadDocument(dstDir, tmpFilePath, true, coverpageFlag)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to upload file: %v", err))
				return
			}
			s.ctx.Filetree().AddDocument(document)
			node := model.CreateNode(*document)
			s.writeSuccess(w, map[string]interface{}{
				"message": "File uploaded",
				"node":    shell.NodeToJSON(&node),
			})
			return
		}

		if existingNode.IsDirectory() {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("cannot replace directory with file"))
			return
		}

		if err := s.ctx.ReplaceDocumentFile(existingNode.Document.ID, tmpFilePath, true); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to replace content: %v", err))
			return
		}

		s.writeSuccess(w, map[string]interface{}{
			"message": "PDF content replaced",
		})
		return
	}

	// Handle regular upload or --force mode
	existingNode, err := s.ctx.Filetree().NodeByPath(docName, node)
	if err == nil {
		// File exists
		if !forceBool {
			s.writeError(w, http.StatusConflict, fmt.Errorf("entry already exists (use force=true to recreate, contentOnly=true to replace content)"))
			return
		}

		if existingNode.IsDirectory() {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("cannot overwrite directory with file"))
			return
		}

		// Delete existing document
		if err := s.ctx.DeleteEntry(existingNode, false, false); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to delete existing file: %v", err))
			return
		}
		s.ctx.Filetree().DeleteNode(existingNode)

		// Upload new document
		dstDir := node.Id()
		document, err := s.ctx.UploadDocument(dstDir, tmpFilePath, true, coverpageFlag)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to upload replacement file: %v", err))
			return
		}

		s.ctx.Filetree().AddDocument(document)
		node := model.CreateNode(*document)
		s.writeSuccess(w, map[string]interface{}{
			"message": "File replaced",
			"node":    shell.NodeToJSON(&node),
		})
		return
	}

	// File doesn't exist, upload new document
	dstDir := node.Id()
	document, err := s.ctx.UploadDocument(dstDir, tmpFilePath, true, coverpageFlag)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to upload file: %v", err))
		return
	}

	s.ctx.Filetree().AddDocument(document)
	newNode := model.CreateNode(*document)
	s.writeSuccess(w, map[string]interface{}{
		"message": "File uploaded",
		"node":    shell.NodeToJSON(&newNode),
	})
}

// GET /api/stat?path=<path>
func (s *ApiServer) handleStat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	query := r.URL.Query()
	srcName := query.Get("path")
	if srcName == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("path parameter is required"))
		return
	}

	node, err := s.ctx.Filetree().NodeByPath(srcName, s.shellCtx.Node)
	if err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("file doesn't exist"))
		return
	}

	s.writeSuccess(w, node.Document)
}

// GET /api/find?path=<path>&pattern=<regex>&compact=<bool>&starred=<bool>&tags=<comma-separated>
func (s *ApiServer) handleFind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	start := query.Get("path")
	if start == "" {
		start = s.shellCtx.Path
	}

	pattern := query.Get("pattern")
	compact := query.Get("compact") == "true"
	starredStr := query.Get("starred")
	starred := starredStr == "true"
	starredFilterEnabled := starredStr != ""
	tagsStr := query.Get("tags")
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}

	startNode, err := s.ctx.Filetree().NodeByPath(start, s.shellCtx.Node)
	if err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("start directory doesn't exist"))
		return
	}

	var matchRegexp *regexp.Regexp
	if pattern != "" {
		matchRegexp, err = regexp.Compile(pattern)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("failed to compile regexp: %v", err))
			return
		}
	}

	var matchedNodes []*model.Node
	var matchedPaths [][]string

	filetree.WalkTree(startNode, filetree.FileTreeVistor{
		Visit: func(node *model.Node, path []string) bool {
			// Filter by starred status if flag was set
			if starredFilterEnabled && node.Document != nil {
				if node.Document.Starred != starred {
					return false
				}
			}

			// Filter by tags if specified - using OR semantics
			if len(tags) > 0 && node.Document != nil {
				nodeTags := node.Document.Tags
				hasMatch := false
				for _, requiredTag := range tags {
					for _, nodeTag := range nodeTags {
						if nodeTag == requiredTag {
							hasMatch = true
							break
						}
					}
					if hasMatch {
						break
					}
				}
				if !hasMatch {
					return false
				}
			}

			entryName := shell.FormatEntry(compact, path, node)

			// Check regexp match if pattern is provided
			if matchRegexp != nil && !matchRegexp.Match([]byte(entryName)) {
				return false
			}

			matchedNodes = append(matchedNodes, node)
			matchedPaths = append(matchedPaths, path)

			return false
		},
	})

	jsonNodes := make([]shell.NodeJSON, len(matchedNodes))
	for i, node := range matchedNodes {
		jsonNodes[i] = shell.NodeToJSON(node)
	}

	s.writeSuccess(w, jsonNodes)
}

// GET /api/account
func (s *ApiServer) handleAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	s.mu.RLock()
	user := s.userInfo.User
	syncVersion := s.userInfo.SyncVersion
	s.mu.RUnlock()
	s.writeSuccess(w, map[string]interface{}{
		"user":        user,
		"syncVersion": syncVersion,
	})
}

// POST /api/refresh
func (s *ApiServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	has, gen, err := s.ctx.Refresh()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	n, err := s.ctx.Filetree().NodeByPath(s.shellCtx.Path, nil)
	if err != nil {
		s.shellCtx.Node = s.ctx.Filetree().Root()
		s.shellCtx.Path = s.shellCtx.Node.Name()
	} else {
		s.shellCtx.Node = n
	}

	s.writeSuccess(w, map[string]interface{}{
		"rootHash":    has,
		"generation":  gen,
		"currentPath": s.shellCtx.Path,
		"message":     "Tree refreshed",
	})
}

// POST /api/refresh-token
func (s *ApiServer) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	err := s.ctx.RefreshToken()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.mu.RLock()
	user := s.userInfo.User
	s.mu.RUnlock()

	s.writeSuccess(w, map[string]interface{}{
		"message": "Token refreshed successfully",
		"user":    user,
	})
}

// POST /api/refresh-tree
func (s *ApiServer) handleRefreshTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	has, gen, err := s.ctx.RefreshTree()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	n, err := s.ctx.Filetree().NodeByPath(s.shellCtx.Path, nil)
	if err != nil {
		s.shellCtx.Node = s.ctx.Filetree().Root()
		s.shellCtx.Path = s.shellCtx.Node.Name()
	} else {
		s.shellCtx.Node = n
	}

	s.writeSuccess(w, map[string]interface{}{
		"rootHash":    has,
		"generation":  gen,
		"currentPath": s.shellCtx.Path,
		"message":     "Tree refreshed",
	})
}

// GET /api/difference
func (s *ApiServer) handleDifference(w http.ResponseWriter, r *http.Request) {
	// Force log to stderr to ensure we see it
	fmt.Fprintf(os.Stderr, "handleDifference called\n")
	log.Info.Println("handleDifference called")
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.requireAuth(w, r) {
		fmt.Fprintf(os.Stderr, "handleDifference: auth failed\n")
		return
	}

	fmt.Fprintf(os.Stderr, "handleDifference: calling DiffTreeCache\n")
	diff, err := s.ctx.DiffTreeCache()
	if err != nil {
		fmt.Fprintf(os.Stderr, "handleDifference: DiffTreeCache error: %v\n", err)
		log.Error.Printf("DiffTreeCache error: %v", err)
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Return simplified JSON format by default
	simplified := sync15.FormatDiffJSON(diff)
	fmt.Fprintf(os.Stderr, "handleDifference: result - %d new, %d removed, %d moved, %d modified\n", 
		len(simplified.New), len(simplified.Removed), len(simplified.Moved), len(simplified.Modified))
	log.Info.Printf("Diff result: %d new, %d removed, %d moved, %d modified", 
		len(simplified.New), len(simplified.Removed), len(simplified.Moved), len(simplified.Modified))
	
	// Debug: log the actual simplified structure
	jsonBytes, _ := json.Marshal(simplified)
	fmt.Fprintf(os.Stderr, "handleDifference: Simplified JSON: %s\n", string(jsonBytes))
	log.Info.Printf("Simplified JSON: %s", string(jsonBytes))
	
	s.writeSuccess(w, simplified)
}

// GET /api/version
func (s *ApiServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.writeSuccess(w, map[string]string{"version": version.Version})
}

func runServerMode(port string) {
	// Run HTTP server mode
	server, err := NewApiServer()
	if err != nil {
		log.Error.Fatalf("Failed to initialize API server: %v", err)
	}

	mux := http.NewServeMux()

	// Authentication endpoints (no auth required)
	mux.HandleFunc("/api/auth", server.handleAuth)
	mux.HandleFunc("/api/auth/status", server.handleAuthStatus)

	// API endpoints (require authentication)
	mux.HandleFunc("/api/ls", server.handleLs)
	mux.HandleFunc("/api/pwd", server.handlePwd)
	mux.HandleFunc("/api/cd", server.handleCd)
	mux.HandleFunc("/api/get", server.handleGet)
	mux.HandleFunc("/api/convert", server.handleConvert)
	mux.HandleFunc("/api/hwr", server.handleHwr)
	mux.HandleFunc("/api/mkdir", server.handleMkdir)
	mux.HandleFunc("/api/rm", server.handleRm)
	mux.HandleFunc("/api/mv", server.handleMv)
	mux.HandleFunc("/api/put", server.handlePut)
	mux.HandleFunc("/api/stat", server.handleStat)
	mux.HandleFunc("/api/find", server.handleFind)
	mux.HandleFunc("/api/account", server.handleAccount)
	mux.HandleFunc("/api/refresh", server.handleRefresh)
	mux.HandleFunc("/api/refresh-token", server.handleRefreshToken)
	mux.HandleFunc("/api/refresh-tree", server.handleRefreshTree)
	mux.HandleFunc("/api/difference", server.handleDifference)
	mux.HandleFunc("/api/version", server.handleVersion)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Root endpoint with API documentation
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title>rmapi REST API</title>
</head>
<body>
	<h1>rmapi REST API</h1>
	<h2>Authentication:</h2>
	<ul>
		<li>GET /api/auth - Authenticate with one-time code via URL (e.g., <code>/api/auth?code=12345678</code>)</li>
		<li>POST /api/auth - Authenticate with one-time code via JSON body</li>
		<li>GET /api/auth/status - Check authentication status</li>
	</ul>
	<h2>Endpoints:</h2>
	<ul>
		<li>GET /api/ls - List directory</li>
		<li>GET /api/pwd - Get current directory</li>
		<li>POST /api/cd - Change directory</li>
		<li>GET /api/get - Download file (streams binary .rmdoc file)</li>
		<li>GET /api/convert - Convert file to PNG</li>
		<li>GET /api/hwr - Perform handwriting recognition (requires RMAPI_HWR_APPLICATIONKEY and RMAPI_HWR_HMAC env vars). Use inline=true to stream ZIP file with TXT files</li>
		<li>POST /api/mkdir - Create directory</li>
		<li>DELETE /api/rm - Delete entry</li>
		<li>POST /api/mv - Move/rename entry</li>
		<li>POST /api/put - Upload file</li>
		<li>GET /api/stat - Get file metadata</li>
		<li>GET /api/find - Find files</li>
		<li>GET /api/account - Get account info</li>
		<li>POST /api/refresh - Refresh file tree</li>
		<li>POST /api/refresh-token - Refresh authentication token only</li>
		<li>POST /api/refresh-tree - Refresh file tree</li>
		<li>GET /api/difference - Compare tree.cache with tree.cache.previous and show changes (JSON format)</li>
		<li>GET /api/version - Get version</li>
	</ul>
	<p><strong>Note:</strong> On first startup, authenticate using POST /api/auth with your one-time code from <a href="https://my.remarkable.com/device/browser/connect">https://my.remarkable.com/device/browser/connect</a></p>
</body>
</html>
		`)
	})

	log.Info.Printf("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Error.Fatalf("Server failed: %v", err)
	}
}


