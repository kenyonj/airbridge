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

// Version is the current Airbridge version, displayed in the web UI.
const Version = "0.0.4"

//go:embed templates/*.html
var templateFS embed.FS

// Server provides the web admin interface.
type Server struct {
	db        DBInterface
	disco     DiscoveryInterface
	renderers RendererController
	templates *template.Template
	basePort  int
	hub       *Hub
}

// DBInterface defines the database operations needed by the web server.
type DBInterface interface {
	ListRenderers() ([]database.Renderer, error)
	GetRenderer(id string) (*database.Renderer, error)
	CreateRenderer(r *database.Renderer) error
	UpdateRenderer(r *database.Renderer) error
	DeleteRenderer(id string) error
	GetNextPort(basePort int) (int, error)
	ToggleRenderer(id string) error
	ToggleCastReceiver(id string) error
	RenameRenderer(id, name string) error
}

// DiscoveryInterface defines the discovery operations needed by the web server.
type DiscoveryInterface interface {
	GetDevices() []*discovery.Device
	GetDevice(deviceID string) *discovery.Device
	GetAllDevices() []*discovery.Device
	GetDeviceUnified(deviceID string) *discovery.Device
}

// DeviceInfo represents a device (AirPlay or Chromecast) for the UI.
type DeviceInfo struct {
	DeviceID   string
	DeviceType string // "airplay" or "chromecast"
	Name       string
	Host       string
	Port       int
	Model      string
	Configured bool
}

// RendererView represents a renderer with runtime status for the UI.
type RendererView struct {
	database.Renderer
	Running        bool
	DLNAURL        string
	TransportState string // "PLAYING", "STOPPED", "PAUSED_PLAYBACK", etc.
}

// RendererController is the interface for managing renderer lifecycle.
type RendererController interface {
	IsRunning(id string) bool
	RunningCount() int
	Uptime() time.Duration
	LocalIP() string
	StartRenderer(id string) error
	StopRenderer(id string)
	StopAll()
	RestartAll()
	GetTransportState(id string) string
	EnableCastReceiver(id string, port int) error
	DisableCastReceiver(id string)
	IsCastReceiverEnabled(id string) bool
}

// NewServer creates a new web server.
func NewServer(db DBInterface, disco DiscoveryInterface, renderers RendererController, basePort int) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	hub := NewHub()
	go hub.Run()

	return &Server{
		db:        db,
		disco:     disco,
		renderers: renderers,
		templates: tmpl,
		basePort:  basePort,
		hub:       hub,
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
	mux.HandleFunc("/admin/settings", s.handleSettings)
	mux.HandleFunc("/admin/settings/reset", s.handleSettingsReset)
	mux.HandleFunc("/admin/ws", s.hub.HandleWebSocket)
}

// BroadcastStateUpdate sends a state update to all WebSocket clients.
func (s *Server) BroadcastStateUpdate(rendererID, transportState string, running bool) {
	s.hub.Broadcast(StateUpdate{
		Type:           "state_update",
		RendererID:     rendererID,
		TransportState: transportState,
		Running:        running,
	})
}

// BroadcastRendererChange notifies clients that a renderer was created/deleted/updated.
func (s *Server) BroadcastRendererChange(rendererID, action string) {
	s.hub.Broadcast(WebSocketMessage{
		Type:       "renderer_changed",
		RendererID: rendererID,
		Action:     action,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Only handle exact /admin or /admin/ paths
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}

	renderers, err := s.db.ListRenderers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build renderer views with running status and DLNA URLs
	localIP := s.renderers.LocalIP()
	var views []RendererView
	playingCount := 0
	for _, r := range renderers {
		transportState := s.renderers.GetTransportState(r.ID)
		if transportState == "PLAYING" {
			playingCount++
		}
		views = append(views, RendererView{
			Renderer:       r,
			Running:        s.renderers.IsRunning(r.ID),
			DLNAURL:        fmt.Sprintf("http://%s:%d/renderer/%s/device.xml", localIP, s.basePort, r.ID),
			TransportState: transportState,
		})
	}

	data := map[string]interface{}{
		"Title":        "Dashboard",
		"Active":       "dashboard",
		"Version":      Version,
		"Renderers":    views,
		"RunningCount": s.renderers.RunningCount(),
		"TotalCount":   len(renderers),
		"PlayingCount": playingCount,
		"DeviceCount":  len(s.disco.GetAllDevices()),
	}

	s.render(w, "layout.html", data)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	allDevices := s.disco.GetAllDevices()
	renderers, _ := s.db.ListRenderers()

	// Get filter from query params
	filterModel := r.URL.Query().Get("model")
	filterType := r.URL.Query().Get("type") // "airplay", "chromecast", or ""

	// Build a set of configured device IDs (for both types)
	configuredIDs := make(map[string]bool)
	for _, r := range renderers {
		configuredIDs[r.AirPlayDeviceID] = true
		if r.DeviceID != "" {
			configuredIDs[r.DeviceID] = true
		}
	}

	// Collect unique models for filter dropdown
	modelSet := make(map[string]bool)

	// Convert devices to DeviceInfo
	var deviceInfos []DeviceInfo
	for _, d := range allDevices {
		modelSet[d.Model] = true

		// Apply filters
		if filterModel != "" && d.Model != filterModel {
			continue
		}
		if filterType != "" && string(d.DeviceType) != filterType {
			continue
		}

		deviceInfos = append(deviceInfos, DeviceInfo{
			DeviceID:   d.DeviceID,
			DeviceType: string(d.DeviceType),
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

	// Handle /admin/renderers/new specially
	if id == "new" && r.Method == "GET" {
		s.newRendererForm(w, r)
		return
	}

	switch {
	case action == "toggle" && r.Method == "POST":
		s.toggleRenderer(w, r, id)
	case action == "toggle-cast" && r.Method == "POST":
		s.toggleCastReceiver(w, r, id)
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

func (s *Server) newRendererForm(w http.ResponseWriter, r *http.Request) {
	// Get all available devices for the dropdown
	allDevices := s.disco.GetDevices()
	renderers, _ := s.db.ListRenderers()

	// Build a set of configured device IDs
	configuredIDs := make(map[string]bool)
	for _, r := range renderers {
		configuredIDs[r.AirPlayDeviceID] = true
		if r.DeviceID != "" {
			configuredIDs[r.DeviceID] = true
		}
	}

	// Convert to DeviceInfo, excluding already configured devices
	var deviceInfos []DeviceInfo
	for _, d := range allDevices {
		if configuredIDs[d.DeviceID] {
			continue
		}
		deviceInfos = append(deviceInfos, DeviceInfo{
			DeviceID:   d.DeviceID,
			DeviceType: string(d.DeviceType),
			Name:       d.Name,
			Host:       d.Host,
			Port:       d.Port,
			Model:      d.Model,
		})
	}

	// Sort by name
	sort.Slice(deviceInfos, func(i, j int) bool {
		return strings.ToLower(deviceInfos[i].Name) < strings.ToLower(deviceInfos[j].Name)
	})

	data := map[string]interface{}{
		"Devices": deviceInfos,
	}

	s.renderPartial(w, "renderer_create.html", data)
}

func (s *Server) createRenderer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	deviceID := r.FormValue("device_id")
	deviceType := r.FormValue("device_type")
	name := r.FormValue("name")
	targetName := r.FormValue("target_name") // From new form

	if deviceID == "" || name == "" {
		http.Error(w, "Missing device_id or name", http.StatusBadRequest)
		return
	}

	// Default to airplay if not specified
	if deviceType == "" {
		deviceType = "airplay"
	}

	// Get next available port
	port, err := s.db.GetNextPort(s.basePort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate deterministic ID
	id := generateID(deviceID)

	// Find the device to get its name based on type
	var deviceName string
	if targetName != "" {
		// New form provides target_name
		deviceName = targetName
	} else {
		// Look up device name from unified discovery
		device := s.disco.GetDevice(deviceID)
		if device != nil {
			deviceName = device.Name
		} else {
			deviceName = name
		}
	}

	renderer := &database.Renderer{
		ID:              id,
		Name:            name, // Use the user-provided name directly
		DeviceType:      deviceType,
		DeviceID:        deviceID,
		DeviceName:      deviceName,
		AirPlayDeviceID: deviceID, // For backward compatibility
		AirPlayName:     deviceName,
		Port:            port,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}

	if err := s.db.CreateRenderer(renderer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Created %s renderer: %s -> %s on port %d", deviceType, renderer.Name, deviceName, renderer.Port)

	// Notify WebSocket clients of the change
	s.BroadcastRendererChange(renderer.ID, "created")

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
	localIP := s.renderers.LocalIP()
	var views []RendererView
	for _, r := range renderers {
		views = append(views, RendererView{
			Renderer:       r,
			Running:        s.renderers.IsRunning(r.ID),
			DLNAURL:        fmt.Sprintf("http://%s:%d/renderer/%s/device.xml", localIP, s.basePort, r.ID),
			TransportState: s.renderers.GetTransportState(r.ID),
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

func (s *Server) toggleCastReceiver(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.db.ToggleCastReceiver(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get updated state and enable/disable cast advertisement
	renderer, err := s.db.GetRenderer(id)
	if err == nil && renderer != nil {
		if renderer.CastEnabled {
			if err := s.renderers.EnableCastReceiver(id, renderer.CastPort); err != nil {
				log.Printf("Warning: failed to enable Cast receiver: %v", err)
			}
		} else {
			s.renderers.DisableCastReceiver(id)
		}
	}

	log.Printf("Toggled Cast receiver for: %s", id)
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

	// Notify WebSocket clients of the change
	s.BroadcastRendererChange(id, "deleted")

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
	_ = r.ParseForm()
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

	// Count playing renderers
	playingCount := 0
	for _, rend := range renderers {
		if s.renderers.GetTransportState(rend.ID) == "PLAYING" {
			playingCount++
		}
	}

	data := map[string]interface{}{
		"RunningCount": s.renderers.RunningCount(),
		"TotalCount":   len(renderers),
		"PlayingCount": playingCount,
		"DeviceCount":  len(s.disco.GetAllDevices()),
	}

	s.renderPartial(w, "server_status.html", data)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	uptime := s.renderers.Uptime()

	data := map[string]interface{}{
		"Title":   "Settings",
		"Active":  "settings",
		"Version": Version,
		"Uptime":  formatDuration(uptime),
		"LocalIP": s.renderers.LocalIP(),
	}

	s.render(w, "layout.html", data)
}

func (s *Server) handleSettingsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stop all running renderers
	s.renderers.StopAll()

	// Delete all renderers from database
	renderers, err := s.db.ListRenderers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, rend := range renderers {
		if err := s.db.DeleteRenderer(rend.ID); err != nil {
			log.Printf("Error deleting renderer %s: %v", rend.ID, err)
		}
	}

	log.Printf("Deleted all %d renderers from database", len(renderers))

	// Broadcast change to all clients
	s.BroadcastRendererChange("", "reset")

	// Redirect to dashboard
	w.Header().Set("HX-Redirect", "/admin")
	w.WriteHeader(http.StatusOK)
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
