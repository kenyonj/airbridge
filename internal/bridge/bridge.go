// Package bridge provides a unified DLNA bridge with embedded renderer devices.
package bridge

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/internal/player"
	"github.com/kenyonj/airbridge/internal/ssdp"
	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/internal/upnp"
)

const (
	rootUUID = "airbridge-root-device"
)

// RendererInstance represents a single embedded renderer.
type RendererInstance struct {
	ID           string
	Name         string
	AirPlayName  string
	Device       *discovery.AirPlayDevice
	State        *state.PlayerState
	Player       *player.RAOPPlayer
	EventManager *upnp.EventManager
	cancel       context.CancelFunc
}

// Bridge is a unified DLNA bridge with multiple embedded renderers.
type Bridge struct {
	db        *database.DB
	disco     *discovery.Service
	localIP   string
	port      int
	startTime time.Time
	server    *http.Server

	renderers map[string]*RendererInstance // keyed by renderer ID (UUID)
	mu        sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewBridge creates a new unified bridge.
func NewBridge(db *database.DB, disco *discovery.Service, port int) *Bridge {
	return &Bridge{
		db:        db,
		disco:     disco,
		port:      port,
		renderers: make(map[string]*RendererInstance),
	}
}

// Start initializes and starts the bridge.
func (b *Bridge) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.startTime = time.Now()

	// Get local IP
	b.localIP = getLocalIP()
	if b.localIP == "" {
		return fmt.Errorf("could not determine local IP address")
	}

	// Load enabled renderers from database
	if err := b.loadRenderers(); err != nil {
		return fmt.Errorf("load renderers: %w", err)
	}

	// Start HTTP server
	if err := b.startHTTPServer(); err != nil {
		return fmt.Errorf("start HTTP server: %w", err)
	}

	// Start SSDP announcements
	b.startSSDP()

	return nil
}

// Stop shuts down the bridge.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Stop all renderers
	for _, r := range b.renderers {
		b.stopRenderer(r)
	}

	// Shutdown HTTP server
	if b.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b.server.Shutdown(shutdownCtx)
	}

	if b.cancel != nil {
		b.cancel()
	}
}

// loadRenderers loads enabled renderers from the database.
func (b *Bridge) loadRenderers() error {
	renderers, err := b.db.ListRenderers()
	if err != nil {
		return err
	}

	for _, r := range renderers {
		if r.Enabled {
			if err := b.addRenderer(&r); err != nil {
				log.Printf("Failed to load renderer %s: %v", r.Name, err)
			}
		}
	}

	return nil
}

// addRenderer adds a renderer instance.
func (b *Bridge) addRenderer(r *database.Renderer) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already exists
	if _, exists := b.renderers[r.ID]; exists {
		return nil
	}

	// Find the AirPlay device
	device := b.disco.GetDevice(r.AirPlayDeviceID)
	if device == nil {
		return fmt.Errorf("AirPlay device not found: %s", r.AirPlayDeviceID)
	}

	// Create renderer context
	ctx, cancel := context.WithCancel(b.ctx)

	inst := &RendererInstance{
		ID:           r.ID,
		Name:         r.Name,
		AirPlayName:  r.AirPlayName,
		Device:       device,
		State:        state.New(ctx),
		Player:       player.NewRAOPPlayer(device),
		EventManager: upnp.NewEventManager(),
		cancel:       cancel,
	}

	b.renderers[r.ID] = inst
	log.Printf("Added renderer: %s -> %s:%d", r.Name, device.Host, device.Port)

	return nil
}

// RemoveRenderer removes a renderer instance.
func (b *Bridge) RemoveRenderer(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if r, exists := b.renderers[id]; exists {
		b.stopRenderer(r)
		delete(b.renderers, id)
		log.Printf("Removed renderer: %s", r.Name)
	}
}

// stopRenderer stops a single renderer.
func (b *Bridge) stopRenderer(r *RendererInstance) {
	if r.cancel != nil {
		r.cancel()
	}
	if r.State != nil {
		r.State.Stop()
	}
}

// GetRenderer returns a renderer by ID.
func (b *Bridge) GetRenderer(id string) *RendererInstance {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.renderers[id]
}

// GetRenderers returns all active renderers.
func (b *Bridge) GetRenderers() []*RendererInstance {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*RendererInstance, 0, len(b.renderers))
	for _, r := range b.renderers {
		result = append(result, r)
	}
	return result
}

// IsRunning checks if a renderer is active.
func (b *Bridge) IsRunning(id string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.renderers[id]
	return exists
}

// RunningCount returns the number of active renderers.
func (b *Bridge) RunningCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.renderers)
}

// Uptime returns how long the bridge has been running.
func (b *Bridge) Uptime() time.Duration {
	return time.Since(b.startTime)
}

// LocalIP returns the local IP address.
func (b *Bridge) LocalIP() string {
	return b.localIP
}

// StartRenderer starts a renderer by ID.
func (b *Bridge) StartRenderer(id string) error {
	renderer, err := b.db.GetRenderer(id)
	if err != nil {
		return err
	}
	if renderer == nil {
		return fmt.Errorf("renderer not found: %s", id)
	}
	return b.addRenderer(renderer)
}

// StopRenderer stops a renderer by ID.
func (b *Bridge) StopRenderer(id string) {
	b.RemoveRenderer(id)
}

// RestartAll stops and restarts all enabled renderers.
func (b *Bridge) RestartAll() {
	log.Println("Restarting all renderers...")

	// Stop all
	b.mu.Lock()
	for _, r := range b.renderers {
		b.stopRenderer(r)
	}
	b.renderers = make(map[string]*RendererInstance)
	b.mu.Unlock()

	// Reload from database
	if err := b.loadRenderers(); err != nil {
		log.Printf("Failed to reload renderers: %v", err)
	}

	log.Println("All renderers restarted")
}

// startHTTPServer starts the unified HTTP server.
func (b *Bridge) startHTTPServer() error {
	mux := http.NewServeMux()

	// Root device description (shows all embedded devices)
	mux.HandleFunc("/device.xml", b.handleDeviceDescription)

	// Per-renderer device descriptions (for SSDP discovery)
	mux.HandleFunc("/renderer/", b.handleRendererRequest)

	// Service descriptions (shared)
	mux.HandleFunc("/upnp/service/avtransport.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDAVTransportXML()))
	})
	mux.HandleFunc("/upnp/service/renderingcontrol.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDRenderingControlXML()))
	})
	mux.HandleFunc("/upnp/service/connectionmanager.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDConnectionManagerXML()))
	})

	b.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", b.port),
		Handler: logMiddleware(mux),
	}

	go func() {
		log.Printf("Bridge HTTP server listening on port %d", b.port)
		if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// handleDeviceDescription returns the root device XML with embedded renderers.
func (b *Bridge) handleDeviceDescription(w http.ResponseWriter, r *http.Request) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	baseURL := fmt.Sprintf("http://%s:%d", b.localIP, b.port)
	xml := b.generateDeviceXML(baseURL)

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write([]byte(xml))
}

// generateDeviceXML creates the root device description with embedded devices.
func (b *Bridge) generateDeviceXML(baseURL string) string {
	var embeddedDevices strings.Builder

	// Sort renderers by name for consistent ordering
	renderers := make([]*RendererInstance, 0, len(b.renderers))
	for _, r := range b.renderers {
		renderers = append(renderers, r)
	}
	sort.Slice(renderers, func(i, j int) bool {
		return renderers[i].Name < renderers[j].Name
	})

	for _, r := range renderers {
		embeddedDevices.WriteString(fmt.Sprintf(`
    <device>
      <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
      <friendlyName>%s</friendlyName>
      <manufacturer>Airbridge</manufacturer>
      <modelName>Airbridge Renderer</modelName>
      <modelDescription>DLNA to AirPlay Bridge - %s</modelDescription>
      <modelNumber>1.0</modelNumber>
      <UDN>uuid:%s</UDN>
      <serviceList>
        <service>
          <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
          <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
          <SCPDURL>/upnp/service/avtransport.xml</SCPDURL>
          <controlURL>/renderer/%s/control/avtransport</controlURL>
          <eventSubURL>/renderer/%s/event/avtransport</eventSubURL>
        </service>
        <service>
          <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
          <serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
          <SCPDURL>/upnp/service/renderingcontrol.xml</SCPDURL>
          <controlURL>/renderer/%s/control/renderingcontrol</controlURL>
          <eventSubURL>/renderer/%s/event/renderingcontrol</eventSubURL>
        </service>
        <service>
          <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
          <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
          <SCPDURL>/upnp/service/connectionmanager.xml</SCPDURL>
          <controlURL>/renderer/%s/control/connectionmanager</controlURL>
          <eventSubURL>/renderer/%s/event/connectionmanager</eventSubURL>
        </service>
      </serviceList>
    </device>`, r.Name, r.AirPlayName, r.ID, r.ID, r.ID, r.ID, r.ID, r.ID, r.ID))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion>
    <major>1</major>
    <minor>0</minor>
  </specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:Basic:1</deviceType>
    <friendlyName>Airbridge</friendlyName>
    <manufacturer>Airbridge</manufacturer>
    <modelName>Airbridge Hub</modelName>
    <modelDescription>DLNA to AirPlay Bridge</modelDescription>
    <modelNumber>1.0</modelNumber>
    <UDN>uuid:%s</UDN>
    <deviceList>%s
    </deviceList>
  </device>
</root>`, rootUUID, embeddedDevices.String())
}

// handleRendererRequest routes requests to the appropriate renderer.
func (b *Bridge) handleRendererRequest(w http.ResponseWriter, r *http.Request) {
	// Parse /renderer/{uuid}/...
	path := strings.TrimPrefix(r.URL.Path, "/renderer/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	uuid := parts[0]
	reqType := parts[1] // "device.xml", "control", or "event"

	renderer := b.GetRenderer(uuid)
	if renderer == nil {
		http.Error(w, "Renderer not found", http.StatusNotFound)
		return
	}

	switch reqType {
	case "device.xml":
		// Per-renderer device description (standalone, not embedded)
		b.handleRendererDeviceXML(w, r, renderer)
	case "control":
		if len(parts) < 3 {
			http.Error(w, "Missing service", http.StatusBadRequest)
			return
		}
		b.handleControl(w, r, renderer, parts[2])
	case "event":
		if len(parts) < 3 {
			http.Error(w, "Missing service", http.StatusBadRequest)
			return
		}
		b.handleEvent(w, r, renderer, parts[2])
	default:
		http.Error(w, "Invalid request type", http.StatusBadRequest)
	}
}

// handleRendererDeviceXML returns a standalone device description for a single renderer.
func (b *Bridge) handleRendererDeviceXML(w http.ResponseWriter, r *http.Request, renderer *RendererInstance) {
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion>
    <major>1</major>
    <minor>0</minor>
  </specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>Airbridge</manufacturer>
    <modelName>Airbridge Renderer</modelName>
    <modelDescription>DLNA to AirPlay Bridge - %s</modelDescription>
    <modelNumber>1.0</modelNumber>
    <UDN>uuid:%s</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
        <SCPDURL>/upnp/service/avtransport.xml</SCPDURL>
        <controlURL>/renderer/%s/control/avtransport</controlURL>
        <eventSubURL>/renderer/%s/event/avtransport</eventSubURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
        <SCPDURL>/upnp/service/renderingcontrol.xml</SCPDURL>
        <controlURL>/renderer/%s/control/renderingcontrol</controlURL>
        <eventSubURL>/renderer/%s/event/renderingcontrol</eventSubURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
        <SCPDURL>/upnp/service/connectionmanager.xml</SCPDURL>
        <controlURL>/renderer/%s/control/connectionmanager</controlURL>
        <eventSubURL>/renderer/%s/event/connectionmanager</eventSubURL>
      </service>
    </serviceList>
  </device>
</root>`, renderer.Name, renderer.AirPlayName, renderer.ID,
		renderer.ID, renderer.ID, renderer.ID, renderer.ID, renderer.ID, renderer.ID)

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write([]byte(xml))
}

// handleControl handles SOAP control requests.
func (b *Bridge) handleControl(w http.ResponseWriter, r *http.Request, renderer *RendererInstance, service string) {
	action := upnp.ParseSOAPAction(r.Header.Get("SOAPACTION"))
	log.Printf("%s action=%s from=%s renderer=%s", service, action, r.RemoteAddr, renderer.Name)

	switch service {
	case "avtransport":
		upnp.AVTransportHandler(renderer.State, renderer.Player, renderer.EventManager)(w, r)
	case "renderingcontrol":
		upnp.RenderingControlHandler(renderer.State, renderer.Player)(w, r)
	case "connectionmanager":
		upnp.ConnectionManagerHandler()(w, r)
	default:
		http.Error(w, "Unknown service", http.StatusBadRequest)
	}
}

// handleEvent handles UPnP event subscriptions.
func (b *Bridge) handleEvent(w http.ResponseWriter, r *http.Request, renderer *RendererInstance, service string) {
	upnp.EventHandlerWithState(renderer.EventManager, service, renderer.State)(w, r)
}

// startSSDP starts SSDP announcements for each renderer as standalone devices.
func (b *Bridge) startSSDP() {
	baseURL := fmt.Sprintf("http://%s:%d", b.localIP, b.port)
	serverName := "Airbridge/1.0"

	// Announce each renderer as a standalone device with its own device.xml
	b.mu.RLock()
	for _, r := range b.renderers {
		uuid := "uuid:" + r.ID
		locationPath := fmt.Sprintf("/renderer/%s/device.xml", r.ID)
		go ssdp.AnnounceWithLocation(b.ctx, baseURL, locationPath, uuid, serverName)
		go ssdp.SearchResponderWithLocation(b.ctx, baseURL, locationPath, uuid, serverName)
	}
	b.mu.RUnlock()
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("HTTP %s %s from=%s duration=%v", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
