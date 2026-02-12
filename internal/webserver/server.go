// ./internal/webserver/server.go
package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/pipeline"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/views"
	"github.com/cork89/clippers/internal/workdir"
	"github.com/gorilla/websocket"
)

// Server holds the HTTP server and its dependencies
type Server struct {
	config              *config.Config
	workDir             *workdir.WorkDir
	mux                 *http.ServeMux
	server              *http.Server
	upgrader            *websocket.Upgrader
	projectsDir         string
	currentProject      string
	currentSegmentIndex int
}

// NewServer creates a new web server instance
func NewServer(cfg *config.Config, workDir *workdir.WorkDir, port int, projectsDir string) *Server {
	s := &Server{
		config:              cfg,
		workDir:             workDir,
		mux:                 http.NewServeMux(),
		projectsDir:         projectsDir,
		currentSegmentIndex: -1,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Local development only
			},
		},
	}

	s.setupRoutes()

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: s.mux,
	}

	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Static files
	s.mux.HandleFunc("/static/", s.handleStatic)

	// Main page - shows project selector if no project loaded, otherwise timeline
	s.mux.HandleFunc("/", s.handleIndex)

	// Project selection and loading
	s.mux.HandleFunc("/projects/", s.handleProjectPage)
	s.mux.HandleFunc("/api/projects", s.handleProjectsList)

	// API routes - JSON
	s.mux.HandleFunc("/api/project", s.handleProject)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/images", s.handleImagesAPI)
	s.mux.HandleFunc("/api/transcript", s.handleTranscript)
	s.mux.HandleFunc("/api/segment/", s.handleSegment)
	s.mux.HandleFunc("/api/image/", s.handleImage)
	s.mux.HandleFunc("/api/render", s.handleRender)

	// API routes - HTML (for HTMX)
	s.mux.HandleFunc("/api/timeline/html", s.handleTimelineHTML)
	s.mux.HandleFunc("/api/images/html", s.handleImagesHTML)

	// WebSocket for progress
	s.mux.HandleFunc("/api/ws/progress", s.handleWebSocket)
}

// Start begins the server and handles graceful shutdown
func (s *Server) Start() error {
	log.Printf("Starting server on http://localhost%s", s.server.Addr)
	log.Printf("Open http://localhost%s in your browser", s.server.Addr)

	// Start server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// handleIndex serves the project selector page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Always show project selector at root
	s.showProjectSelector(w, r)
}

// showProjectSelector displays the project selection page
func (s *Server) showProjectSelector(w http.ResponseWriter, r *http.Request) {
	projects, err := DiscoverProjects(s.projectsDir)
	if err != nil {
		log.Printf("Error discovering projects: %v", err)
	}

	viewProjects := make([]views.ProjectInfo, len(projects))
	for i, p := range projects {
		viewProjects[i] = views.ProjectInfo{
			Name:       p.Name,
			AudioFile:  p.AudioFile,
			HasImages:  p.HasImages,
			ImageCount: p.ImageCount,
			ModifiedAt: p.ModifiedAt.Format("Jan 2, 2006 3:04 PM"),
		}
	}

	data := views.ProjectsData{
		Projects: viewProjects,
		BasePath: "",
	}

	component := views.ProjectSelector(data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering project selector: %v", err)
	}
}

// handleProjectsList returns the list of projects as JSON
func (s *Server) handleProjectsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projects, err := DiscoverProjects(s.projectsDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list projects: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(projects); err != nil {
		log.Printf("Error encoding projects: %v", err)
	}
}

// handleProjectPage loads a project and displays the timeline editor
func (s *Server) handleProjectPage(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/projects/")
	if path == "" {
		http.Error(w, "Project name required", http.StatusBadRequest)
		return
	}

	// Sanitize project name to prevent directory traversal
	projectName := filepath.Base(path)

	// Check if we need to load this project
	if s.currentProject != projectName || s.workDir == nil {
		projectPath := filepath.Join(s.projectsDir, projectName)

		// Verify the project exists and is valid
		projects, err := DiscoverProjects(s.projectsDir)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list projects: %v", err), http.StatusInternalServerError)
			return
		}

		var foundProject *ProjectInfo
		for i := range projects {
			if projects[i].Name == projectName {
				foundProject = &projects[i]
				break
			}
		}

		if foundProject == nil {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}

		// Update config with project paths
		s.config.AudioPath = filepath.Join(projectPath, foundProject.AudioFile)
		s.config.ImagesDir = foundProject.ImagesDir
		s.currentProject = projectName

		// Create workdir for this project
		wd, err := workdir.New(s.config)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create work directory: %v", err), http.StatusInternalServerError)
			return
		}
		s.workDir = wd
	}

	// Check if we need to run the pipeline
	if !s.workDir.Exists("timeline.json") {
		// Show processing page
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>Processing Project - Clippers</title>
    <link rel="stylesheet" href="/static/style.css"/>
    <meta http-equiv="refresh" content="5;url=/projects/` + projectName + `">
</head>
<body class="projects-page">
    <header class="app-header">
        <h1>🎬 Clippers</h1>
    </header>
    <main class="projects-main">
        <div class="no-projects">
            <h2>Project Needs Processing</h2>
            <p>This project hasn't been processed yet. Please run the CLI command:</p>
            <pre style="background: #f5f5f5; padding: 1rem; border-radius: 4px; overflow-x: auto;">
clippers server -a "` + s.config.AudioPath + `" -i "` + s.config.ImagesDir + `"</pre>
            <p>Or use the <code>clippers run</code> command first to create the timeline.</p>
            <p><a href="/" class="btn btn-primary">Back to Projects</a></p>
        </div>
    </main>
</body>
</html>`)); err != nil {
			log.Printf("Error writing processing page: %v", err)
		}
		return
	}

	// Render the timeline editor with project name in the breadcrumb
	component := views.LayoutWithProject("Timeline Editor - "+projectName, projectName)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering timeline editor: %v", err)
	}
}

// handleStatic serves static files (CSS, JS)
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	staticPath := filepath.Join("internal", "views", "static", path)

	// Security: prevent directory traversal
	if strings.Contains(path, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, staticPath)
}

// handleProject returns project metadata
func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var project types.Project
	if err := s.workDir.ReadJSON("project.json", &project); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read project: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(project); err != nil {
		log.Printf("Error encoding project: %v", err)
	}
}

// handleTimeline returns or updates the current timeline
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getTimeline(w, r)
	case http.MethodPut:
		s.updateTimeline(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getTimeline handles GET /api/timeline
func (s *Server) getTimeline(w http.ResponseWriter, r *http.Request) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var timeline types.Timeline
	if err := s.workDir.ReadJSON("timeline.json", &timeline); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read timeline: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(timeline); err != nil {
		log.Printf("Error encoding timeline: %v", err)
	}
}

// updateTimeline handles PUT /api/timeline
func (s *Server) updateTimeline(w http.ResponseWriter, r *http.Request) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var timeline types.Timeline
	if err := json.NewDecoder(r.Body).Decode(&timeline); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.workDir.WriteJSON("timeline.json", &timeline); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "saved"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// handleTranscript returns the transcript
func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var transcript types.Transcript
	if err := s.workDir.ReadJSON("transcript.json", &transcript); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read transcript: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(transcript); err != nil {
		log.Printf("Error encoding transcript: %v", err)
	}
}

// handleSegment handles individual segment operations
func (s *Server) handleSegment(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/segment/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid segment ID", http.StatusBadRequest)
		return
	}

	segmentID := parts[0]

	// Handle "current" as a special case referring to the selected segment
	if segmentID == "current" {
		if s.currentSegmentIndex < 0 {
			http.Error(w, "No segment selected", http.StatusBadRequest)
			return
		}
		segmentID = fmt.Sprintf("%d", s.currentSegmentIndex)
	}

	switch r.Method {
	case http.MethodGet:
		s.getSegment(w, r, segmentID)
	case http.MethodPost:
		if len(parts) < 2 {
			http.Error(w, "Invalid operation", http.StatusBadRequest)
			return
		}
		operation := parts[1]
		s.handleSegmentOperation(w, r, segmentID, operation)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getSegment handles GET /api/segment/{id}
func (s *Server) getSegment(w http.ResponseWriter, r *http.Request, segmentID string) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var timeline types.Timeline
	if err := s.workDir.ReadJSON("timeline.json", &timeline); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read timeline: %v", err), http.StatusInternalServerError)
		return
	}

	// Find segment by index
	var index int
	if _, err := fmt.Sscanf(segmentID, "%d", &index); err != nil {
		http.Error(w, "Invalid segment ID", http.StatusBadRequest)
		return
	}

	if index < 0 || index >= len(timeline.Entries) {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return
	}

	// Track this as the current segment
	s.currentSegmentIndex = index

	entry := timeline.Entries[index]

	// Get transcript text for this segment
	transcriptText := ""
	var transcript types.Transcript
	if err := s.workDir.ReadJSON("transcript.json", &transcript); err == nil {
		// Find segments that overlap with this time range
		var texts []string
		for _, seg := range transcript.Segments {
			if seg.End > entry.Start && seg.Start < entry.End {
				texts = append(texts, seg.Text)
			}
		}
		transcriptText = strings.Join(texts, " ")
	}

	// Check if this is an HTMX request
	if r.Header.Get("HX-Request") == "true" {
		// Return HTML component
		data := views.SegmentData{
			Index:      index,
			Entry:      entry,
			Transcript: transcriptText,
		}
		component := views.SegmentEditor(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering segment editor: %v", err)
		}
	} else {
		// Return JSON
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entry); err != nil {
			log.Printf("Error encoding entry: %v", err)
		}
	}
}

// handleSegmentOperation handles POST operations on segments
func (s *Server) handleSegmentOperation(w http.ResponseWriter, r *http.Request, segmentID string, operation string) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var timeline types.Timeline
	if err := s.workDir.ReadJSON("timeline.json", &timeline); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read timeline: %v", err), http.StatusInternalServerError)
		return
	}

	var index int
	if _, err := fmt.Sscanf(segmentID, "%d", &index); err != nil {
		http.Error(w, "Invalid segment ID", http.StatusBadRequest)
		return
	}

	if index < 0 || index >= len(timeline.Entries) {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return
	}

	switch operation {
	case "image":
		var req struct {
			ImageID string `json:"image_id"`
		}

		// Try to get from query parameter first (for HTMX), then body
		req.ImageID = r.URL.Query().Get("image_id")
		if req.ImageID == "" {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Update the segment's image
		timeline.Entries[index].ImageID = req.ImageID
		timeline.Entries[index].Image = filepath.Join(s.config.ImagesDir, req.ImageID)

		if err := s.workDir.WriteJSON("timeline.json", &timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		// Return HTML for HTMX, JSON otherwise
		if r.Header.Get("HX-Request") == "true" {
			// Get transcript text
			transcriptText := ""
			var transcript types.Transcript
			if err := s.workDir.ReadJSON("transcript.json", &transcript); err == nil {
				var texts []string
				for _, seg := range transcript.Segments {
					if seg.End > timeline.Entries[index].Start && seg.Start < timeline.Entries[index].End {
						texts = append(texts, seg.Text)
					}
				}
				transcriptText = strings.Join(texts, " ")
			}

			data := views.SegmentData{
				Index:      index,
				Entry:      timeline.Entries[index],
				Transcript: transcriptText,
			}
			component := views.SegmentEditor(data)
			if err := component.Render(r.Context(), w); err != nil {
				log.Printf("Error rendering segment editor: %v", err)
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(timeline.Entries[index]); err != nil {
				log.Printf("Error encoding entry: %v", err)
			}
		}

	case "split":
		entry := timeline.Entries[index]
		midpoint := (entry.Start + entry.End) / 2

		// Create two new entries
		newEntry := entry
		newEntry.Start = midpoint
		entry.End = midpoint

		// Insert new entry after current
		timeline.Entries = append(timeline.Entries[:index+1], append([]types.TimelineEntry{newEntry}, timeline.Entries[index+1:]...)...)
		timeline.Entries[index] = entry

		if err := s.workDir.WriteJSON("timeline.json", &timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		// Return updated timeline HTML
		defaultName := ""
		for _, e := range timeline.Entries {
			if e.ImageID == "default.png" || e.ImageID == "default.jpg" {
				defaultName = e.ImageID
				break
			}
		}

		data := views.TimelineData{
			Timeline:    timeline,
			DefaultName: defaultName,
		}
		component := views.TimelineGrid(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering timeline grid: %v", err)
		}

	case "merge":
		if index >= len(timeline.Entries)-1 {
			http.Error(w, "Cannot merge last segment", http.StatusBadRequest)
			return
		}

		// Merge with next entry
		timeline.Entries[index].End = timeline.Entries[index+1].End
		// Remove next entry
		timeline.Entries = append(timeline.Entries[:index+1], timeline.Entries[index+2:]...)

		if err := s.workDir.WriteJSON("timeline.json", &timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		// Return updated timeline HTML
		defaultName := ""
		for _, e := range timeline.Entries {
			if e.ImageID == "default.png" || e.ImageID == "default.jpg" {
				defaultName = e.ImageID
				break
			}
		}

		data := views.TimelineData{
			Timeline:    timeline,
			DefaultName: defaultName,
		}
		component := views.TimelineGrid(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering timeline grid: %v", err)
		}

	default:
		http.Error(w, "Unknown operation", http.StatusBadRequest)
	}
}

// handleImage serves image files
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/image/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid image ID", http.StatusBadRequest)
		return
	}

	imageID := parts[0]
	imagePath := filepath.Join(s.config.ImagesDir, imageID)
	fmt.Println("imagesdir: " + s.config.ImagesDir)

	// Security check
	if !strings.HasPrefix(imagePath, s.config.ImagesDir) {
		http.Error(w, "Invalid image path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, imagePath)
}

// handleRender starts video rendering
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Trigger render via WebSocket progress
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "started"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// handleWebSocket upgrades to WebSocket for real-time progress
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// TODO: Implement progress broadcasting
	// For now, just echo back
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(messageType, p); err != nil {
			return
		}
	}
}

// handleImagesAPI returns the image catalog as JSON
func (s *Server) handleImagesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	var catalog pipeline.ImageCatalog
	if err := s.workDir.ReadJSON("images/captions.json", &catalog); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read images: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(catalog); err != nil {
		log.Printf("Error encoding catalog: %v", err)
	}
}

// handleTimelineHTML returns the timeline as HTML for HTMX
func (s *Server) handleTimelineHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No project loaded. <a href="/">Select a project</a></p></div>`)); err != nil {
			log.Printf("Error writing no project message: %v", err)
		}
		return
	}

	var timeline types.Timeline
	if err := s.workDir.ReadJSON("timeline.json", &timeline); err != nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No timeline found. Run the pipeline first.</p></div>`)); err != nil {
			log.Printf("Error writing no timeline message: %v", err)
		}
		return
	}

	// Detect default image
	defaultName := ""
	for _, entry := range timeline.Entries {
		if entry.ImageID != "" && (entry.ImageID == "default.png" || entry.ImageID == "default.jpg") {
			defaultName = entry.ImageID
			break
		}
	}

	data := views.TimelineData{
		Timeline:    timeline,
		DefaultName: defaultName,
	}

	component := views.TimelineGrid(data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering timeline grid: %v", err)
	}
}

// handleImagesHTML returns the images as HTML for HTMX
func (s *Server) handleImagesHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No project loaded.</p></div>`)); err != nil {
			log.Printf("Error writing no project message: %v", err)
		}
		return
	}

	var catalog pipeline.ImageCatalog
	if err := s.workDir.ReadJSON("images/captions.json", &catalog); err != nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No images found.</p></div>`)); err != nil {
			log.Printf("Error writing no images message: %v", err)
		}
		return
	}

	data := views.ImagesData{
		Images: catalog.Images,
	}

	component := views.ImageCatalog(data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering image catalog: %v", err)
	}
}
