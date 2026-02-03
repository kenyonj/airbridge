// Package upnp provides UPnP eventing support.
package upnp

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Subscription represents a UPnP event subscriber.
type Subscription struct {
	SID        string
	Callback   string
	Timeout    time.Duration
	ExpiresAt  time.Time
	ServiceID  string
	SeqNumber  uint32
}

// EventManager manages UPnP event subscriptions and notifications.
type EventManager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription // keyed by SID
}

// NewEventManager creates a new event manager.
func NewEventManager() *EventManager {
	return &EventManager{
		subscriptions: make(map[string]*Subscription),
	}
}

// generateSID creates a globally unique subscription ID.
func generateSID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("uuid:%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Subscribe handles a SUBSCRIBE request and returns the SID.
func (em *EventManager) Subscribe(r *http.Request, serviceID string) (sid string, timeout int, err error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Parse callback URL from CALLBACK header: <http://host:port/path>
	callbackHeader := r.Header.Get("CALLBACK")
	callback := parseCallback(callbackHeader)
	if callback == "" {
		return "", 0, fmt.Errorf("no callback URL")
	}

	// Parse timeout from TIMEOUT header: Second-1800
	timeoutSec := 1800
	if to := r.Header.Get("TIMEOUT"); to != "" {
		_, _ = fmt.Sscanf(to, "Second-%d", &timeoutSec)
	}

	// Generate globally unique SID
	sid = generateSID()

	sub := &Subscription{
		SID:       sid,
		Callback:  callback,
		Timeout:   time.Duration(timeoutSec) * time.Second,
		ExpiresAt: time.Now().Add(time.Duration(timeoutSec) * time.Second),
		ServiceID: serviceID,
		SeqNumber: 0,
	}
	em.subscriptions[sid] = sub

	log.Printf("Event subscription: SID=%s callback=%s service=%s", sid, callback, serviceID)
	return sid, timeoutSec, nil
}

// Unsubscribe removes a subscription.
func (em *EventManager) Unsubscribe(sid string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	delete(em.subscriptions, sid)
	log.Printf("Event unsubscribe: SID=%s", sid)
}

// NotifyTransportState sends a NOTIFY for transport state change.
func (em *EventManager) NotifyTransportState(state string) {
	log.Printf("NotifyTransportState called with state: %s", state)
	em.mu.RLock()
	subs := make([]*Subscription, 0)
	for _, sub := range em.subscriptions {
		log.Printf("Checking subscription: SID=%s service=%s", sub.SID, sub.ServiceID)
		if strings.Contains(sub.ServiceID, "avtransport") {
			subs = append(subs, sub)
		}
	}
	em.mu.RUnlock()

	log.Printf("Found %d avtransport subscriptions to notify", len(subs))
	for _, sub := range subs {
		em.sendNotify(sub, buildTransportStateEvent(state))
	}
}

// NotifyVolume sends a NOTIFY for volume change.
func (em *EventManager) NotifyVolume(volume int, mute bool) {
	em.mu.RLock()
	subs := make([]*Subscription, 0)
	for _, sub := range em.subscriptions {
		if strings.Contains(sub.ServiceID, "renderingcontrol") {
			subs = append(subs, sub)
		}
	}
	em.mu.RUnlock()

	for _, sub := range subs {
		em.sendNotify(sub, buildVolumeEvent(volume, mute))
	}
}

func (em *EventManager) sendNotify(sub *Subscription, body string) {
	em.mu.Lock()
	sub.SeqNumber++
	seq := sub.SeqNumber
	em.mu.Unlock()

	req, err := http.NewRequest("NOTIFY", sub.Callback, bytes.NewReader([]byte(body)))
	if err != nil {
		log.Printf("Notify error creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("NTS", "upnp:propchange")
	req.Header.Set("SID", sub.SID)
	req.Header.Set("SEQ", fmt.Sprintf("%d", seq))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Notify error sending to %s: %v", sub.Callback, err)
		return
	}
	resp.Body.Close()
	log.Printf("Notify sent to %s (SEQ=%d, status=%d)", sub.Callback, seq, resp.StatusCode)
}

func buildTransportStateEvent(state string) string {
	lastChange := fmt.Sprintf(
		`<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/">`+
			`<InstanceID val="0">`+
			`<TransportState val="%s"/>`+
			`<TransportStatus val="OK"/>`+
			`<CurrentTransportActions val="Play,Stop,Pause"/>`+
			`</InstanceID></Event>`,
		html.EscapeString(state))

	return fmt.Sprintf(
		`<?xml version="1.0" encoding="utf-8"?>`+
			`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">`+
			`<e:property><LastChange>%s</LastChange></e:property>`+
			`</e:propertyset>`,
		html.EscapeString(lastChange))
}

func buildVolumeEvent(volume int, mute bool) string {
	muteVal := "0"
	if mute {
		muteVal = "1"
	}
	lastChange := fmt.Sprintf(
		`<Event xmlns="urn:schemas-upnp-org:metadata-1-0/RCS/">`+
			`<InstanceID val="0">`+
			`<Volume channel="Master" val="%d"/>`+
			`<Mute channel="Master" val="%s"/>`+
			`</InstanceID></Event>`,
		volume, muteVal)

	return fmt.Sprintf(
		`<?xml version="1.0" encoding="utf-8"?>`+
			`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">`+
			`<e:property><LastChange>%s</LastChange></e:property>`+
			`</e:propertyset>`,
		html.EscapeString(lastChange))
}

var callbackRe = regexp.MustCompile(`<([^>]+)>`)

func parseCallback(header string) string {
	matches := callbackRe.FindStringSubmatch(header)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
