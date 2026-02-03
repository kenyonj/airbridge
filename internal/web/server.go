// Package web provides the admin web UI for Airbridge.
package web

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server provides the web admin interface.
type Server struct {
	db        *database.DB
	disco     *discovery.Service
	renderers RendererController
	templates *template.Template
	basePort  int
}

// DeviceInfo represents an AirPlay device for the UI.
type DeviceInfo struct {
	DeviceID   string
	Name       string
	Host       string
	Port       int
	Model      string
	Configured bool
}

// RendererView represents a renderer with runtime status for the UI.
type RendererView struct {
	database.Renderer
	Running bool
	DLNAURL string
}

// RendererController is the interface for managing renderer lifecycle.
type RendererController interface {
	IsRunning(id string) bool
	RunningCount() int
	Uptime() time.Duration
	LocalIP() string
	StartRenderer(id string) error
	StopRenderer(id string)
	RestartAll()
}

// NewServer creates a new web server.
func NewServer(db *database.DB, disco *discovery.Service, renderers RendererController, basePort int) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Server{
		db:        db,
		disco:     disco,
		renderers: renderers,
		templates: tmpl,
		basePort:  basePort,
	}, nil
}

// RegisterRoutes registers the admin routes on the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin", s.handleDashboard)
	mux.HandleFunc("/admin/", s.handleDashboard)
	mux.HandleFunc("/admin/devices", s.handleDevices)
	mux.HandleFunc("/admin/renderers", s.handleRenderers)
	mux.HandleFunc("/admin/renderers/", s.handleRendererAction)
	mux.HandleFunc("/admin/server/status", s.handleServerStatus)
	mux.HandleFunc("/admin/server/restart", s.handleServerRestart)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	renderers, err := s.db.ListRenderers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build renderer views with running status and DLNA URLs
	localIP := s.renderers.LocalIP()
	var views []RendererView
	for _, r := range renderers {
		views = append(views, RendererView{
			Renderer: r,
			Running:  s.renderers.IsRunning(r.ID),
			DLNAURL:  fmt.Sprintf("http://%s:%d/renderer/%s/device.xml", localIP, s.basePort, r.ID),
		})
	}

	// Format uptime
	uptime := s.renderers.Uptime()
	uptimeStr := formatDuration(uptime)

	data := map[string]interface{}{
		"Title":        "Dashboard",
		"Active":       "dashboard",
		"Renderers":    views,
		"RunningCount": s.renderers.RunningCount(),
		"TotalCount":   len(renderers),
		"Uptime":       uptimeStr,
		"LocalIP":      localIP,
		"DeviceCount":  len(s.disco.GetDevices()),
	}

	s.render(w, "layout.html", data)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices := s.disco.GetDevices()
	renderers, _ := s.db.ListRenderers()

	// Get filter from query params
	filterModel := r.URL.Query().Get("model")

	// Build a set of configured device IDs
	configuredIDs := make(map[string]bool)
	for _, r := range renderers {
		configuredIDs[r.AirPlayDeviceID] = true
	}

	// Collect unique models for filter dropdown
	modelSet := make(map[string]bool)

	// Convert to DeviceInfo
	var deviceInfos []DeviceInfo
	for _, d := range devices {
		modelSet[d.Model] = true

		// Apply filter
		if filterModel != "" && d.Model != filterModel {
			continue
		}

		deviceInfos = append(deviceInfos, DeviceInfo{
			DeviceID:   d.DeviceID,
			Name:       d.Name,
			Host:       d.Host,
			Port:       d.Port,
			Model:      d.Model,
			Configured: configuredIDs[d.DeviceID],
		})
	}

	// Sort by name
	sort.Slice(deviceInfos, func(i, j int) bool {
		return strings.ToLower(deviceInfos[i].Name) < strings.ToLower(deviceInfos[j].Name)
	})

	// Build sorted model list for filter
	var models []string
	for m := range modelSet {
		models = append(models, m)
	}
	sort.Strings(models)

	data := map[string]interface{}{
		"Devices":     deviceInfos,
		"Models":      models,
		"FilterModel": filterModel,
	}

	s.renderPartial(w, "devices.html", data)
}

func (s *Server) handleRenderers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		s.createRenderer(w, r)
	default:
		s.listRenderers(w, r)
	}
}

func (s *Server) handleRendererAction(w http.ResponseWriter, r *http.Request) {
	// Parse /admin/renderers/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/admin/renderers/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "toggle" && r.Method == "POST":
		s.toggleRenderer(w, r, id)
	case action == "start" && r.Method == "POST":
		s.startRenderer(w, r, id)
	case action == "stop" && r.Method == "POST":
		s.stopRenderer(w, r, id)
	case action == "restart" && r.Method == "POST":
		s.restartRenderer(w, r, id)
	case action == "edit" && r.Method == "GET":
		s.editRendererForm(w, r, id)
	case action == "rename" && r.Method == "POST":
		s.renameRenderer(w, r, id)
	case action == "" && r.Method == "DELETE":
		s.deleteRenderer(w, r, id)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (s *Server) createRenderer(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	deviceID := r.FormValue("device_id")
	name := r.FormValue("name")

	if deviceID == "" || name == "" {
		http.Error(w, "Missing device_id or name", http.StatusBadRequest)
		return
	}

	// Get next available port
	port, err := s.db.GetNextPort(s.basePort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate deterministic ID
	id := generateID(deviceID)

	// Find the device to get its name
	device := s.disco.GetDevice(deviceID)
	airplayName := name
	if device != nil {
		airplayName = device.Name
	}

	renderer := &database.Renderer{
		ID:              id,
		Name:            fmt.Sprintf("Airbridge (%s)", name),
		AirPlayDeviceID: deviceID,
		AirPlayName:     airplayName,
		Port:            port,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}

	if err := s.db.CreateRenderer(renderer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Created renderer: %s -> %s on port %d", renderer.Name, renderer.AirPlayName, renderer.Port)

	// Start the renderer
	if err := s.renderers.StartRenderer(renderer.ID); err != nil {
		log.Printf("Warning: failed to start renderer: %v", err)
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) listRenderers(w http.ResponseWriter, r *http.Request) {
	renderers, err := s.db.ListRenderers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build renderer views with running status
	var views []RendererView
	for _, r := range renderers {
		views = append(views, RendererView{
			Renderer: r,
			Running:  s.renderers.IsRunning(r.ID),
		})
	}

	data := map[string]interface{}{
		"Renderers": views,
	}

	s.renderPartial(w, "renderers_list.html", data)
}

func (s *Server) toggleRenderer(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.db.ToggleRenderer(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get updated state and start/stop accordingly
	renderer, err := s.db.GetRenderer(id)
	if err == nil && renderer != nil {
		if renderer.Enabled {
			if err := s.renderers.StartRenderer(id); err != nil {
				log.Printf("Warning: failed to start renderer: %v", err)
			}
		} else {
			s.renderers.StopRenderer(id)
		}
	}

	log.Printf("Toggled renderer: %s", id)
	s.listRenderers(w, r)
}

func (s *Server) deleteRenderer(w http.ResponseWriter, r *http.Request, id string) {
	// Stop the renderer first
	s.renderers.StopRenderer(id)

	if err := s.db.DeleteRenderer(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Deleted renderer: %s", id)
	s.listRenderers(w, r)
}

func (s *Server) startRenderer(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.renderers.StartRenderer(id); err != nil {
		log.Printf("Failed to start renderer %s: %v", id, err)
	} else {
		log.Printf("Started renderer: %s", id)
	}
	s.listRenderers(w, r)
}

func (s *Server) stopRenderer(w http.ResponseWriter, r *http.Request, id string) {
	s.renderers.StopRenderer(id)
	log.Printf("Stopped renderer: %s", id)
	s.listRenderers(w, r)
}

func (s *Server) restartRenderer(w http.ResponseWriter, r *http.Request, id string) {
	s.renderers.StopRenderer(id)
	if err := s.renderers.StartRenderer(id); err != nil {
		log.Printf("Failed to restart renderer %s: %v", id, err)
	} else {
		log.Printf("Restarted renderer: %s", id)
	}
	s.listRenderers(w, r)
}

func (s *Server) editRendererForm(w http.ResponseWriter, r *http.Request, id string) {
	renderer, err := s.db.GetRenderer(id)
	if err != nil || renderer == nil {
		http.Error(w, "Renderer not found", http.StatusNotFound)
		return
	}

	data := map[string]interface{}{
		"Renderer": RendererView{
			Renderer: *renderer,
			Running:  s.renderers.IsRunning(id),
		},
	}

	s.renderPartial(w, "renderer_edit.html", data)
}

func (s *Server) renameRenderer(w http.ResponseWriter, r *http.Request, id string) {
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))

	if name == "" {
		http.Error(w, "Name cannot be empty", http.StatusBadRequest)
		return
	}

	if err := s.db.RenameRenderer(id, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Renamed renderer %s to: %s", id, name)
	s.listRenderers(w, r)
}

func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	renderers, _ := s.db.ListRenderers()
	uptime := s.renderers.Uptime()

	data := map[string]interface{}{
		"RunningCount": s.renderers.RunningCount(),
		"TotalCount":   len(renderers),
		"Uptime":       formatDuration(uptime),
		"LocalIP":      s.renderers.LocalIP(),
		"DeviceCount":  len(s.disco.GetDevices()),
	}

	s.renderPartial(w, "server_status.html", data)
}

func (s *Server) handleServerRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.renderers.RestartAll()
	log.Println("Server restart triggered from web UI")

	// Return updated status
	s.handleServerStatus(w, r)
}

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func generateID(deviceID string) string {
	hash := sha256.Sum256([]byte("airbridge:" + deviceID))
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
