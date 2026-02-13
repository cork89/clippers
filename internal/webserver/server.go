package webserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/database/sqlc"
	"github.com/cork89/clippers/internal/pipeline"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/views"
	"github.com/cork89/clippers/internal/workdir"
	"github.com/gorilla/websocket"
)

type Server struct {
	config              *config.Config
	workDir             *workdir.WorkDir
	db                  *database.DB
	mux                 *http.ServeMux
	server              *http.Server
	upgrader            *websocket.Upgrader
	projectsDir         string
	currentProject      string
	currentSegmentIndex int
	processingProgress  struct {
		currentStage int
		failedStage  int
		failedError  string
		isComplete   bool
		isFailed     bool
	}
	processingMu sync.Mutex
}

func NewServer(cfg *config.Config, workDir *workdir.WorkDir, db *database.DB, port int, projectsDir string) *Server {
	s := &Server{
		config:              cfg,
		workDir:             workDir,
		db:                  db,
		mux:                 http.NewServeMux(),
		projectsDir:         projectsDir,
		currentSegmentIndex: -1,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
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

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/static/", s.handleStatic)
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/projects/", s.handleProjectPage)
	s.mux.HandleFunc("/api/projects", s.handleProjectsList)
	s.mux.HandleFunc("/api/project", s.handleProject)
	s.mux.HandleFunc("/api/save", s.handleSave)
	s.mux.HandleFunc("/api/process", s.handleProcess)
	s.mux.HandleFunc("/api/process/status", s.handleProcessStatus)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/images", s.handleImagesAPI)
	s.mux.HandleFunc("/api/transcript", s.handleTranscript)
	s.mux.HandleFunc("/api/segment/", s.handleSegment)
	s.mux.HandleFunc("/api/image/", s.handleImage)
	s.mux.HandleFunc("/api/audio", s.handleAudio)
	s.mux.HandleFunc("/api/render", s.handleRender)
	s.mux.HandleFunc("/api/timeline/html", s.handleTimelineHTML)
	s.mux.HandleFunc("/api/images/html", s.handleImagesHTML)
	s.mux.HandleFunc("/api/ws/progress", s.handleWebSocket)
	s.mux.HandleFunc("/api/project/delete", s.handleDeleteProject)
	s.mux.HandleFunc("/api/shaders/", s.handleShaders)
	s.mux.HandleFunc("/api/shaders", s.handleShaders)
}

func (s *Server) Start() error {
	log.Printf("Starting server on http://localhost%s", s.server.Addr)
	log.Printf("Open http://localhost%s in your browser", s.server.Addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.showProjectSelector(w, r)
}

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

func (s *Server) handleProjectPage(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/projects/")
	if path == "" {
		http.Error(w, "Project name required", http.StatusBadRequest)
		return
	}

	projectName := filepath.Base(path)

	if s.currentProject != projectName || s.workDir == nil {
		projectPath := filepath.Join(s.projectsDir, projectName)

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

		s.config.AudioPath = filepath.Join(projectPath, foundProject.AudioFile)
		s.config.ImagesDir = foundProject.ImagesDir
		s.currentProject = projectName

		wd, err := workdir.New(r.Context(), s.config, s.db)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create work directory: %v", err), http.StatusInternalServerError)
			return
		}
		s.workDir = wd
	}

	exists, _ := s.db.Queries.TimelineExists(r.Context(), s.workDir.ProjectID())
	if exists != 1 {
		data := views.ProcessingData{
			ProjectName: projectName,
			AudioPath:   s.config.AudioPath,
			ImagesDir:   s.config.ImagesDir,
		}
		component := views.ProcessingPage(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering processing page: %v", err)
		}
		return
	}

	component := views.LayoutWithProject("Timeline Editor - "+projectName, projectName)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering timeline editor: %v", err)
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	staticPath := filepath.Join("internal", "views", "static", path)

	if strings.Contains(path, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, staticPath)
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	project, err := s.db.Queries.GetProject(r.Context(), s.workDir.ProjectID())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read project: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(project); err != nil {
		log.Printf("Error encoding project: %v", err)
	}
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	project, err := s.db.Queries.GetProject(r.Context(), s.workDir.ProjectID())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get project: %v", err), http.StatusInternalServerError)
		return
	}

	var updates types.Project
	if err := json.NewDecoder(r.Body).Decode(&updates); err == nil && updates.Settings != nil {
		settingsJSON, _ := json.Marshal(updates.Settings)
		project.Settings = sql.NullString{
			String: string(settingsJSON),
			Valid:  len(settingsJSON) > 0,
		}
		if updates.AudioPath != "" {
			project.AudioPath = updates.AudioPath
		}
		if updates.ImagesDir != "" {
			project.ImagesDir = updates.ImagesDir
		}
		if updates.OutputDir != "" {
			project.OutputDir = updates.OutputDir
		}
	}

	settingsJSON, _ := json.Marshal(project.Settings)
	err = s.db.Queries.UpdateProject(r.Context(), sqlc.UpdateProjectParams{
		AudioPath: project.AudioPath,
		ImagesDir: project.ImagesDir,
		OutputDir: project.OutputDir,
		Settings: sql.NullString{
			String: string(settingsJSON),
			Valid:  len(settingsJSON) > 0,
		},
		ID: s.workDir.ProjectID(),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save project: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "saved"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

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

func (s *Server) getTimeline(w http.ResponseWriter, r *http.Request) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	timeline, err := s.db.GetTimeline(r.Context(), s.workDir.ProjectID())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read timeline: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(timeline); err != nil {
		log.Printf("Error encoding timeline: %v", err)
	}
}

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

	if err := s.db.Queries.ClearTimeline(r.Context(), s.workDir.ProjectID()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to clear timeline: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.db.SaveTimeline(r.Context(), s.workDir.ProjectID(), &timeline); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "saved"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	transcript, err := s.db.GetFullTranscript(r.Context(), s.workDir.ProjectID())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read transcript: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(transcript); err != nil {
		log.Printf("Error encoding transcript: %v", err)
	}
}

func (s *Server) handleSegment(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/segment/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid segment ID", http.StatusBadRequest)
		return
	}

	segmentID := parts[0]

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

func (s *Server) getSegment(w http.ResponseWriter, r *http.Request, segmentID string) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	timeline, err := s.db.GetTimeline(r.Context(), s.workDir.ProjectID())
	if err != nil {
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

	s.currentSegmentIndex = index

	entry := timeline.Entries[index]

	transcriptText := ""
	transcript, err := s.db.GetFullTranscript(r.Context(), s.workDir.ProjectID())
	if err == nil {
		var texts []string
		for _, seg := range transcript.Segments {
			if seg.End > entry.Start && seg.Start < entry.End {
				texts = append(texts, seg.Text)
			}
		}
		transcriptText = strings.Join(texts, " ")
	}

	if r.Header.Get("HX-Request") == "true" {
		data := views.SegmentData{
			Index:       index,
			Entry:       entry,
			Transcript:  transcriptText,
			ProjectName: s.currentProject,
		}
		component := views.SegmentEditor(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering segment editor: %v", err)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entry); err != nil {
			log.Printf("Error encoding entry: %v", err)
		}
	}
}

func (s *Server) handleSegmentOperation(w http.ResponseWriter, r *http.Request, segmentID string, operation string) {
	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	timeline, err := s.db.GetTimeline(r.Context(), s.workDir.ProjectID())
	if err != nil {
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

		req.ImageID = r.URL.Query().Get("image_id")
		if req.ImageID == "" {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
				return
			}
		}

		timeline.Entries[index].ImageID = req.ImageID
		timeline.Entries[index].Image = filepath.Join(s.config.ImagesDir, req.ImageID)

		if err := s.db.Queries.ClearTimeline(r.Context(), s.workDir.ProjectID()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to clear timeline: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.db.SaveTimeline(r.Context(), s.workDir.ProjectID(), timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			transcriptText := ""
			transcript, err := s.db.GetFullTranscript(r.Context(), s.workDir.ProjectID())
			if err == nil {
				var texts []string
				for _, seg := range transcript.Segments {
					if seg.End > timeline.Entries[index].Start && seg.Start < timeline.Entries[index].End {
						texts = append(texts, seg.Text)
					}
				}
				transcriptText = strings.Join(texts, " ")
			}

			data := views.SegmentData{
				Index:       index,
				Entry:       timeline.Entries[index],
				Transcript:  transcriptText,
				ProjectName: s.currentProject,
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

	case "shader":
		shader := r.URL.Query().Get("shader")
		if shader == "" {
			var req struct {
				Shader string `json:"shader"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
				return
			}
			shader = req.Shader
		}

		if shader != "" && !config.IsValidShader(shader) {
			http.Error(w, fmt.Sprintf("Invalid shader: %s", shader), http.StatusBadRequest)
			return
		}

		timeline.Entries[index].Shader = shader

		if err := s.db.Queries.ClearTimeline(r.Context(), s.workDir.ProjectID()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to clear timeline: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.db.SaveTimeline(r.Context(), s.workDir.ProjectID(), timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			transcriptText := ""
			transcript, err := s.db.GetFullTranscript(r.Context(), s.workDir.ProjectID())
			if err == nil {
				var texts []string
				for _, seg := range transcript.Segments {
					if seg.End > timeline.Entries[index].Start && seg.Start < timeline.Entries[index].End {
						texts = append(texts, seg.Text)
					}
				}
				transcriptText = strings.Join(texts, " ")
			}

			data := views.SegmentData{
				Index:       index,
				Entry:       timeline.Entries[index],
				Transcript:  transcriptText,
				ProjectName: s.currentProject,
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

		newEntry := entry
		newEntry.Start = midpoint
		entry.End = midpoint

		timeline.Entries = append(timeline.Entries[:index+1], append([]types.TimelineEntry{newEntry}, timeline.Entries[index+1:]...)...)
		timeline.Entries[index] = entry

		if err := s.db.Queries.ClearTimeline(r.Context(), s.workDir.ProjectID()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to clear timeline: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.db.SaveTimeline(r.Context(), s.workDir.ProjectID(), timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		defaultName := ""
		for _, e := range timeline.Entries {
			if e.ImageID == "default.png" || e.ImageID == "default.jpg" {
				defaultName = e.ImageID
				break
			}
		}

		data := views.TimelineData{
			Timeline:    *timeline,
			DefaultName: defaultName,
			ProjectName: s.currentProject,
		}
		component := views.TimelineGrid(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering timeline grid: %v", err)
		}

		if err := s.db.Queries.ClearTimeline(r.Context(), s.workDir.ProjectID()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to clear timeline: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.db.SaveTimeline(r.Context(), s.workDir.ProjectID(), timeline); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save timeline: %v", err), http.StatusInternalServerError)
			return
		}

		defaultName = ""
		for _, e := range timeline.Entries {
			if e.ImageID == "default.png" || e.ImageID == "default.jpg" {
				defaultName = e.ImageID
				break
			}
		}

		data = views.TimelineData{
			Timeline:    *timeline,
			DefaultName: defaultName,
			ProjectName: s.currentProject,
		}
		component = views.TimelineGrid(data)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering timeline grid: %v", err)
		}

	default:
		http.Error(w, "Unknown operation", http.StatusBadRequest)
	}
}

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

	if !strings.HasPrefix(imagePath, s.config.ImagesDir) {
		http.Error(w, "Invalid image path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, imagePath)
}

func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectName := r.URL.Query().Get("project")

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	if projectName != "" {
		w.Header().Set("Cache-Control", fmt.Sprintf("no-cache, no-store, must-revalidate, private; query=%s", projectName))
	}

	if s.config.AudioPath == "" {
		log.Printf("Audio endpoint called but no AudioPath configured (project: %s)", projectName)
		http.Error(w, "No audio file", http.StatusNotFound)
		return
	}

	audioPath := s.config.AudioPath
	if !strings.HasPrefix(audioPath, s.projectsDir) && !strings.HasPrefix(audioPath, ".") && !filepath.IsAbs(audioPath) {
		absAudioPath, err := filepath.Abs(audioPath)
		if err != nil {
			log.Printf("Failed to get absolute path for audio: %v", err)
			http.Error(w, "Invalid audio path", http.StatusBadRequest)
			return
		}
		audioPath = absAudioPath
	}

	log.Printf("Serving audio from: %s (project: %s)", audioPath, projectName)

	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, audioPath)
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "started"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

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

func (s *Server) handleImagesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workDir == nil {
		http.Error(w, "No project loaded", http.StatusBadRequest)
		return
	}

	catalog, err := s.db.GetImageCatalog(r.Context(), s.workDir.ProjectID())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read images: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(catalog); err != nil {
		log.Printf("Error encoding catalog: %v", err)
	}
}

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

	timeline, err := s.db.GetTimeline(r.Context(), s.workDir.ProjectID())
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No timeline found. Run the pipeline first.</p></div>`)); err != nil {
			log.Printf("Error writing no timeline message: %v", err)
		}
		return
	}

	defaultName := ""
	for _, entry := range timeline.Entries {
		if entry.ImageID != "" && (entry.ImageID == "default.png" || entry.ImageID == "default.jpg") {
			defaultName = entry.ImageID
			break
		}
	}

	data := views.TimelineData{
		Timeline:    *timeline,
		DefaultName: defaultName,
		ProjectName: s.currentProject,
	}

	component := views.TimelineGrid(data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering timeline grid: %v", err)
	}
}

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

	catalog, err := s.db.GetImageCatalog(r.Context(), s.workDir.ProjectID())
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<div class="loading"><p>No images found.</p></div>`)); err != nil {
			log.Printf("Error writing no images message: %v", err)
		}
		return
	}

	data := views.ImagesData{
		Images:      catalog.Images,
		ProjectName: s.currentProject,
	}

	component := views.ImageCatalog(data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering image catalog: %v", err)
	}
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectName := r.Header.Get("project")
	if projectName == "" {
		projectName = s.currentProject
	}

	s.processingMu.Lock()
	s.processingProgress.currentStage = -1
	s.processingProgress.failedStage = -1
	s.processingProgress.failedError = ""
	s.processingProgress.isComplete = false
	s.processingProgress.isFailed = false
	s.processingMu.Unlock()

	if s.workDir == nil || s.currentProject != projectName {
		projectPath := filepath.Join(s.projectsDir, projectName)

		projects, err := DiscoverProjects(s.projectsDir)
		if err != nil {
			component := views.ProcessingErrorUI(fmt.Sprintf("Failed to list projects: %v", err), projectName)
			if err := component.Render(r.Context(), w); err != nil {
				log.Printf("Error rendering error: %v", err)
			}
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
			component := views.ProcessingErrorUI("Project not found", projectName)
			if err := component.Render(r.Context(), w); err != nil {
				log.Printf("Error rendering error: %v", err)
			}
			return
		}

		s.config.AudioPath = filepath.Join(projectPath, foundProject.AudioFile)
		s.config.ImagesDir = foundProject.ImagesDir
		s.currentProject = projectName

		wd, err := workdir.New(r.Context(), s.config, s.db)
		if err != nil {
			component := views.ProcessingErrorUI(fmt.Sprintf("Failed to create work directory: %v", err), projectName)
			if err := component.Render(r.Context(), w); err != nil {
				log.Printf("Error rendering error: %v", err)
			}
			return
		}
		s.workDir = wd
	}

	w.Header().Set("Content-Type", "text/html")

	stages := []struct {
		name   string
		action func() error
	}{
		{"Preflight", func() error { return pipeline.Preflight(s.config, s.workDir) }},
		{"Normalizing audio", func() error {
			_, err := pipeline.NormalizeAudio(s.workDir, s.config.AudioPath, s.config.Force)
			return err
		}},
		{"Transcribing audio", func() error {
			_, err := pipeline.Transcribe(r.Context(), s.workDir, s.config, s.db, s.config.Force)
			return err
		}},
		{"Captioning images", func() error {
			_, err := pipeline.CaptionImages(r.Context(), s.workDir, s.config, s.db, s.config.Force)
			return err
		}},
		{"Building text windows", func() error {
			transcript, err := s.db.GetFullTranscript(r.Context(), s.workDir.ProjectID())
			if err != nil {
				return err
			}
			_, err = pipeline.BuildTextWindows(r.Context(), s.workDir, s.config, s.db, transcript, s.config.Force)
			return err
		}},
		{"Planning timeline", func() error {
			windows, err := s.db.GetTextWindows(r.Context(), s.workDir.ProjectID())
			if err != nil {
				return err
			}
			catalog, err := s.db.GetImageCatalog(r.Context(), s.workDir.ProjectID())
			if err != nil {
				return err
			}
			_, err = pipeline.PlanTimeline(r.Context(), s.workDir, s.config, s.db, windows, catalog, s.config.Force)
			return err
		}},
	}

	for i := range stages {
		s.processingMu.Lock()
		s.processingProgress.currentStage = i
		s.processingMu.Unlock()

		if err := stages[i].action(); err != nil {
			s.processingMu.Lock()
			s.processingProgress.failedStage = i
			s.processingProgress.failedError = err.Error()
			s.processingProgress.isFailed = true
			s.processingMu.Unlock()

			component := views.ProcessingErrorUI(fmt.Sprintf("%s failed: %v", stages[i].name, err), projectName)
			if err := component.Render(r.Context(), w); err != nil {
				log.Printf("Error rendering error: %v", err)
			}
			return
		}
	}

	s.processingMu.Lock()
	s.processingProgress.currentStage = len(stages)
	s.processingProgress.isComplete = true
	s.processingMu.Unlock()

	component := views.ProcessingCompleteUI(projectName)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering complete: %v", err)
	}
}

func (s *Server) handleProcessStatus(w http.ResponseWriter, r *http.Request) {
	s.processingMu.Lock()
	defer s.processingMu.Unlock()

	w.Header().Set("Content-Type", "text/html")

	if s.processingProgress.isComplete {
		component := views.ProcessingCompleteUI(s.currentProject)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering complete: %v", err)
		}
		return
	}

	if s.processingProgress.isFailed {
		component := views.ProcessingErrorUI(s.processingProgress.failedError, s.currentProject)
		if err := component.Render(r.Context(), w); err != nil {
			log.Printf("Error rendering error: %v", err)
		}
		return
	}

	currentStage := s.processingProgress.currentStage
	if currentStage < 0 {
		currentStage = 0
	}

	component := views.ProcessingStages(currentStage, s.processingProgress.failedStage, s.processingProgress.failedError)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering stages: %v", err)
	}
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	log.Printf("Delete request received for project: %s", r.FormValue("project"))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectName := r.FormValue("project")
	if projectName == "" {
		log.Printf("Delete failed: project name is empty")
		http.Error(w, "Project name required", http.StatusBadRequest)
		return
	}

	projectPath := filepath.Join(s.projectsDir, projectName)
	log.Printf("Delete: project path = %s", projectPath)

	projects, err := DiscoverProjects(s.projectsDir)
	if err != nil {
		log.Printf("Delete failed: DiscoverProjects error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to find project: %v", err), http.StatusInternalServerError)
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
		log.Printf("Delete failed: project not found in DiscoverProjects")
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	audioPath := filepath.Join(projectPath, foundProject.AudioFile)
	imagesDir := foundProject.ImagesDir
	log.Printf("Delete: audioPath = %s, imagesDir = %s", audioPath, imagesDir)

	audioHash, err := hashFileForDelete(audioPath)
	if err != nil {
		log.Printf("Delete failed: hashFileForDelete error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to hash audio: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("Delete: audioHash = %s", audioHash[:16])

	imagesHash, err := hashDirectoryForDelete(imagesDir)
	if err != nil {
		log.Printf("Delete failed: hashDirectoryForDelete error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to hash images: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("Delete: imagesHash = %s", imagesHash[:16])

	log.Printf("Delete: MinShotSec = %.1f, BlurStrength = %d", s.config.MinShotSec, s.config.BlurStrength)

	combined := fmt.Sprintf("%s:%s:%.1f:%d", audioHash[:16], imagesHash[:16], s.config.MinShotSec, s.config.BlurStrength)
	log.Printf("Delete: combined string = %s", combined)
	h := sha256.Sum256([]byte(combined))
	projectID := hex.EncodeToString(h[:])[:16]
	log.Printf("Delete: calculated projectID = %s", projectID)

	if err := s.db.Queries.DeleteProject(r.Context(), projectID); err != nil {
		log.Printf("Delete failed: DeleteProject error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to delete project: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Delete: successfully deleted project %s", projectID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func hashFileForDelete(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDirectoryForDelete(dir string) (string, error) {
	if dir == "" {
		return "empty", nil
	}

	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isImageFileForDelete(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		fileHash, err := hashFileForDelete(f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f + ":" + fileHash + "\n"))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func isImageFileForDelete(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func (s *Server) handleShaders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/shaders")

	if path == "" || path == "/" {
		shaders := types.ListShaders()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(shaders); err != nil {
			log.Printf("Error encoding shaders: %v", err)
		}
		return
	}

	shaderName := strings.TrimPrefix(path, "/")

	if shaderName == "vertex" {
		content, err := types.GetVertexShader()
		if err != nil {
			http.Error(w, "Shader not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(content))
		return
	}

	content, err := types.GetShader(types.ShaderType(shaderName))
	if err != nil {
		http.Error(w, "Shader not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}
