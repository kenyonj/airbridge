// Package httpserver provides the HTTP server for UPnP services.
package httpserver

import (
	"log"
	"net/http"
	"time"

	"github.com/kenyonj/airbridge/internal/state"
	"github.com/kenyonj/airbridge/internal/upnp"
)

// RegisterHTTP sets up the HTTP routes for UPnP services.
func RegisterHTTP(mux *http.ServeMux, baseURL, deviceUUID, friendlyName, manufacturer string, st *state.PlayerState, player upnp.Player) {
	// Device description
	mux.HandleFunc("/device.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(upnp.DeviceDescriptionXML(baseURL, deviceUUID, friendlyName, manufacturer)))
	})

	// Service descriptions
	mux.HandleFunc("/upnp/service/avtransport.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDAVTransportXML()))
	})
	mux.HandleFunc("/upnp/service/renderingcontrol.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDRenderingControlXML()))
	})
	mux.HandleFunc("/upnp/service/connectionmanager.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(upnp.SCPDConnectionManagerXML()))
	})

	// Service control endpoints
	mux.HandleFunc("/upnp/control/avtransport", upnp.AVTransportHandler(st, player))
	mux.HandleFunc("/upnp/control/renderingcontrol", upnp.RenderingControlHandler(st, player))
	mux.HandleFunc("/upnp/control/connectionmanager", upnp.ConnectionManagerHandler())

	// Event endpoints
	mux.HandleFunc("/upnp/event/avtransport", upnp.EventHandler)
	mux.HandleFunc("/upnp/event/renderingcontrol", upnp.EventHandler)
	mux.HandleFunc("/upnp/event/connectionmanager", upnp.EventHandler)

	// Root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Airbridge DLNA Renderer running\n"))
	})
}

// LogMiddleware adds request logging.
func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("HTTP %s %s from=%s duration=%s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}
