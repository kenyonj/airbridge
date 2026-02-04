// Package cast provides Chromecast receiver functionality.
package cast

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// Namespace constants for CASTV2 protocol
const (
	NamespaceConnection = "urn:x-cast:com.google.cast.tp.connection"
	NamespaceHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	NamespaceReceiver   = "urn:x-cast:com.google.cast.receiver"
	NamespaceMedia      = "urn:x-cast:com.google.cast.media"
)

// Server is a CASTV2 protocol server.
type Server struct {
	port         int
	friendlyName string
	deviceID     string
	listener     net.Listener
	connections  map[net.Conn]*connection
	mu           sync.RWMutex
	running      bool

	// Callback when media URL is received
	OnMediaLoad func(url string, contentType string)
}

type connection struct {
	conn     net.Conn
	sourceID string
}

// NewServer creates a new CASTV2 server.
func NewServer(port int, friendlyName, deviceID string) *Server {
	return &Server{
		port:         port,
		friendlyName: friendlyName,
		deviceID:     deviceID,
		connections:  make(map[net.Conn]*connection),
	}
}

// Start starts the CASTV2 server.
func (s *Server) Start() error {
	cert, err := generateSelfSignedCert(s.friendlyName)
	if err != nil {
		return fmt.Errorf("generate cert: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", s.port), tlsConfig)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.listener = listener
	s.running = true

	go s.acceptLoop()

	log.Printf("CASTV2 server listening on port %d", s.port)
	return nil
}

// Stop stops the CASTV2 server.
func (s *Server) Stop() {
	s.running = false
	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	for conn := range s.connections {
		conn.Close()
	}
	s.connections = make(map[net.Conn]*connection)
	s.mu.Unlock()
}

func (s *Server) acceptLoop() {
	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				log.Printf("CASTV2 accept error: %v", err)
			}
			continue
		}

		log.Printf("CASTV2 connection from %s", conn.RemoteAddr())
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	s.mu.Lock()
	s.connections[conn] = &connection{conn: conn}
	s.mu.Unlock()

	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.connections, conn)
		s.mu.Unlock()
	}()

	for s.running {
		msg, err := readMessage(conn)
		if err != nil {
			if err != io.EOF && s.running {
				log.Printf("CASTV2 read error: %v", err)
			}
			return
		}

		s.handleMessage(conn, msg)
	}
}

func (s *Server) handleMessage(conn net.Conn, msg *CastMessage) {
	namespace := msg.GetNamespace()
	payload := msg.GetPayloadUtf8()

	log.Printf("CASTV2 [%s] %s -> %s: %s",
		namespace, msg.GetSourceId(), msg.GetDestinationId(), payload)

	switch namespace {
	case NamespaceConnection:
		s.handleConnection_ns(conn, msg)
	case NamespaceHeartbeat:
		s.handleHeartbeat(conn, msg)
	case NamespaceReceiver:
		s.handleReceiver(conn, msg)
	case NamespaceMedia:
		s.handleMedia(conn, msg)
	default:
		log.Printf("CASTV2 unknown namespace: %s", namespace)
	}
}

func (s *Server) handleConnection_ns(conn net.Conn, msg *CastMessage) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &parsed); err != nil {
		return
	}

	msgType, _ := parsed["type"].(string)
	if msgType == "CONNECT" {
		log.Printf("CASTV2 connection established from %s", msg.GetSourceId())
		// Store source ID
		s.mu.Lock()
		if c, ok := s.connections[conn]; ok {
			c.sourceID = msg.GetSourceId()
		}
		s.mu.Unlock()
	}
}

func (s *Server) handleHeartbeat(conn net.Conn, msg *CastMessage) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &parsed); err != nil {
		return
	}

	if parsed["type"] == "PING" {
		response := map[string]string{"type": "PONG"}
		s.sendJSON(conn, NamespaceHeartbeat, msg.GetDestinationId(), msg.GetSourceId(), response)
	}
}

func (s *Server) handleReceiver(conn net.Conn, msg *CastMessage) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &parsed); err != nil {
		return
	}

	msgType, _ := parsed["type"].(string)
	requestID, _ := parsed["requestId"].(float64)

	switch msgType {
	case "GET_STATUS":
		s.sendReceiverStatus(conn, msg, int(requestID))
	case "GET_APP_AVAILABILITY":
		s.sendAppAvailability(conn, msg, parsed)
	case "LAUNCH":
		// For now, just respond with status
		s.sendReceiverStatus(conn, msg, int(requestID))
	}
}

func (s *Server) handleMedia(conn net.Conn, msg *CastMessage) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &parsed); err != nil {
		return
	}

	msgType, _ := parsed["type"].(string)

	if msgType == "LOAD" {
		if media, ok := parsed["media"].(map[string]interface{}); ok {
			contentID, _ := media["contentId"].(string)
			contentType, _ := media["contentType"].(string)
			log.Printf("CASTV2 LOAD request: %s (%s)", contentID, contentType)

			if s.OnMediaLoad != nil {
				s.OnMediaLoad(contentID, contentType)
			}
		}
	}
}

func (s *Server) sendReceiverStatus(conn net.Conn, msg *CastMessage, requestID int) {
	status := map[string]interface{}{
		"requestId": requestID,
		"type":      "RECEIVER_STATUS",
		"status": map[string]interface{}{
			"applications":  []interface{}{},
			"isActiveInput": true,
			"volume": map[string]interface{}{
				"level": 1.0,
				"muted": false,
			},
		},
	}
	s.sendJSON(conn, NamespaceReceiver, msg.GetDestinationId(), msg.GetSourceId(), status)
}

func (s *Server) sendAppAvailability(conn net.Conn, msg *CastMessage, parsed map[string]interface{}) {
	requestID, _ := parsed["requestId"].(float64)
	appIDs, _ := parsed["appId"].([]interface{})

	availability := make(map[string]string)
	for _, appID := range appIDs {
		if id, ok := appID.(string); ok {
			// Report Default Media Receiver as available
			if id == "CC1AD845" {
				availability[id] = "APP_AVAILABLE"
			} else {
				availability[id] = "APP_UNAVAILABLE"
			}
		}
	}

	response := map[string]interface{}{
		"requestId":    int(requestID),
		"type":         "GET_APP_AVAILABILITY",
		"availability": availability,
	}
	s.sendJSON(conn, NamespaceReceiver, msg.GetDestinationId(), msg.GetSourceId(), response)
}

func (s *Server) sendJSON(conn net.Conn, namespace, sourceID, destID string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("CASTV2 marshal error: %v", err)
		return
	}

	payloadStr := string(payload)
	msg := &CastMessage{
		ProtocolVersion: CastMessage_CASTV2_1_0.Enum(),
		SourceId:        &sourceID,
		DestinationId:   &destID,
		Namespace:       &namespace,
		PayloadType:     CastMessage_STRING.Enum(),
		PayloadUtf8:     &payloadStr,
	}

	if err := writeMessage(conn, msg); err != nil {
		log.Printf("CASTV2 write error: %v", err)
	}
}

func readMessage(conn net.Conn) (*CastMessage, error) {
	// Read 4-byte length prefix (big-endian)
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > 1<<20 { // 1MB max
		return nil, fmt.Errorf("message too large: %d", length)
	}

	// Read message body
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}

	msg := &CastMessage{}
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return msg, nil
}

func writeMessage(conn net.Conn, msg *CastMessage) error {
	body, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Write 4-byte length prefix (big-endian)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(body)))

	if _, err := conn.Write(lenBuf); err != nil {
		return err
	}
	if _, err := conn.Write(body); err != nil {
		return err
	}

	return nil
}

func generateSelfSignedCert(commonName string) (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, nil
}
