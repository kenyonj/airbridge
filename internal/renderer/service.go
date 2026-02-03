// Package renderer manages DLNA renderer instances backed by database config.
package renderer

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/kenyonj/airbridge/internal/database"
	"github.com/kenyonj/airbridge/internal/discovery"
	"github.com/kenyonj/airbridge/internal/httpserver"
	"github.com/kenyonj/airbridge/internal/player"
	"github.com/kenyonj/airbridge/internal/ssdp"
	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/internal/upnp"
)

// Service manages DLNA renderer instances based on database configuration.
type Service struct {
	db        *database.DB
	disco     *discovery.Service
	localIP   string
	startTime time.Time
	instances map[string]*Instance // keyed by renderer ID
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewService creates a new renderer service.
func NewService(db *database.DB, disco *discovery.Service) *Service {
	return &Service{
		db:        db,
		disco:     disco,
		instances: make(map[string]*Instance),
	}
}

// Start initializes the service and starts all enabled renderers.
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.startTime = time.Now()

	// Get local IP
	s.localIP = getLocalIP()
	if s.localIP == "" {
		return fmt.Errorf("could not determine local IP address")
	}

	// Start all enabled renderers
	renderers, err := s.db.ListRenderers()
	if err != nil {
		return fmt.Errorf("list renderers: %w", err)
	}

	for _, r := range renderers {
		if r.Enabled {
			rCopy := r // take address of copy
			if err := s.startRenderer(&rCopy); err != nil {
				log.Printf("Failed to start renderer %s: %v", r.Name, err)
			}
		}
	}

	return nil
}

// Stop shuts down all renderer instances.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instances {
		s.stopInstance(inst)
	}

	if s.cancel != nil {
		s.cancel()
	}
}

// StartRenderer starts a renderer by ID.
func (s *Service) StartRenderer(id string) error {
	renderer, err := s.db.GetRenderer(id)
	if err != nil {
		return err
	}
	if renderer == nil {
		return fmt.Errorf("renderer not found: %s", id)
	}
	return s.startRenderer(renderer)
}

// StopRenderer stops a renderer by ID.
func (s *Service) StopRenderer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if inst, exists := s.instances[id]; exists {
		s.stopInstance(inst)
		delete(s.instances, id)
		log.Printf("Stopped renderer: %s", inst.FriendlyName)
	}
}

// IsRunning checks if a renderer is currently running.
func (s *Service) IsRunning(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.instances[id]
	return exists
}

// GetRunningRenderers returns IDs of all running renderers.
func (s *Service) GetRunningRenderers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.instances))
	for id := range s.instances {
		ids = append(ids, id)
	}
	return ids
}

// RunningCount returns the number of running renderer instances.
func (s *Service) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.instances)
}

// Uptime returns how long the service has been running.
func (s *Service) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// LocalIP returns the local IP address being used.
func (s *Service) LocalIP() string {
	return s.localIP
}

// RestartAll stops and restarts all enabled renderers.
func (s *Service) RestartAll() {
	log.Println("Restarting all renderers...")

	// Stop all running instances
	s.mu.Lock()
	for _, inst := range s.instances {
		s.stopInstance(inst)
	}
	s.instances = make(map[string]*Instance)
	s.mu.Unlock()

	// Start all enabled renderers
	renderers, err := s.db.ListRenderers()
	if err != nil {
		log.Printf("Failed to list renderers: %v", err)
		return
	}

	for _, r := range renderers {
		if r.Enabled {
			rCopy := r
			if err := s.startRenderer(&rCopy); err != nil {
				log.Printf("Failed to start renderer %s: %v", r.Name, err)
			}
		}
	}

	log.Println("All renderers restarted")
}

// startRenderer starts a single renderer instance.
func (s *Service) startRenderer(r *database.Renderer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already running
	if _, exists := s.instances[r.ID]; exists {
		return nil
	}

	// Find the AirPlay device
	device := s.disco.GetDevice(r.AirPlayDeviceID)
	if device == nil {
		return fmt.Errorf("AirPlay device not found: %s", r.AirPlayDeviceID)
	}

	// Create instance
	ctx, cancel := context.WithCancel(s.ctx)
	inst := &Instance{
		Device:       device,
		UUID:         r.ID, // Use the renderer ID as UUID (already deterministic)
		FriendlyName: r.Name,
		Port:         r.Port,
		cancel:       cancel,
	}

	// Create state
	inst.State = state.New(ctx)

	// Create player
	inst.Player = player.NewRAOPPlayer(device)

	// Create event manager
	inst.EventManager = upnp.NewEventManager()

	// Setup HTTP server
	baseURL := fmt.Sprintf("http://%s:%d", s.localIP, r.Port)
	mux := http.NewServeMux()
	httpserver.RegisterHTTP(mux, baseURL, inst.UUID, r.Name, "Airbridge", inst.State, inst.Player, inst.EventManager)

	inst.Server = &http.Server{
		Addr:    fmt.Sprintf(":%d", r.Port),
		Handler: httpserver.LogMiddleware(mux),
	}

	// Start SSDP announcements
	serverName := "Airbridge/1.0"
	go ssdp.Announce(ctx, baseURL, "uuid:"+inst.UUID, serverName)
	go ssdp.SearchResponder(ctx, baseURL, "uuid:"+inst.UUID, serverName)

	// Start HTTP server
	go func() {
		log.Printf("Starting renderer: %s on port %d -> %s:%d",
			r.Name, r.Port, device.Host, device.Port)
		if err := inst.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error for %s: %v", r.Name, err)
		}
	}()

	s.instances[r.ID] = inst
	return nil
}

// stopInstance shuts down a single instance.
func (s *Service) stopInstance(inst *Instance) {
	if inst.cancel != nil {
		inst.cancel()
	}
	if inst.Server != nil {
		inst.Server.Shutdown(context.Background())
	}
	if inst.State != nil {
		inst.State.Stop()
	}
}
