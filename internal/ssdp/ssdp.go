// Package ssdp implements SSDP device discovery for UPnP.
package ssdp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	ssdpAddr     = "239.255.255.250:1900"
	cacheMaxAge  = 1800 // seconds
	announceFreq = 30 * time.Second
)

// UPnP service types
const (
	DeviceType     = "urn:schemas-upnp-org:device:MediaRenderer:1"
	AVTransportType = "urn:schemas-upnp-org:service:AVTransport:1"
	RenderingType  = "urn:schemas-upnp-org:service:RenderingControl:1"
)

// Announce sends SSDP NOTIFY messages to advertise the device.
// It runs in a loop until the context is canceled.
func Announce(ctx context.Context, baseURL, deviceUUID, serverName string) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	targets := []struct{ st, usn string }{
		{DeviceType, deviceUUID + "::" + DeviceType},
		{AVTransportType, deviceUUID + "::" + AVTransportType},
		{RenderingType, deviceUUID + "::" + RenderingType},
		{"upnp:rootdevice", deviceUUID + "::upnp:rootdevice"},
		{deviceUUID, deviceUUID},
	}

	ticker := time.NewTicker(announceFreq)
	defer ticker.Stop()

	sendAlive := func() {
		for _, t := range targets {
			msg := fmt.Sprintf(
				"NOTIFY * HTTP/1.1\r\n"+
					"HOST: %s\r\n"+
					"CACHE-CONTROL: max-age=%d\r\n"+
					"LOCATION: %s/device.xml\r\n"+
					"NT: %s\r\n"+
					"NTS: ssdp:alive\r\n"+
					"SERVER: %s\r\n"+
					"USN: %s\r\n"+
					"BOOTID.UPNP.ORG: 1\r\n"+
					"CONFIGID.UPNP.ORG: 1\r\n\r\n",
				ssdpAddr, cacheMaxAge, baseURL, t.st, serverName, t.usn)
			conn.Write([]byte(msg))
		}
	}

	sendByeBye := func() {
		for _, t := range targets {
			msg := fmt.Sprintf(
				"NOTIFY * HTTP/1.1\r\n"+
					"HOST: %s\r\n"+
					"NT: %s\r\n"+
					"NTS: ssdp:byebye\r\n"+
					"USN: %s\r\n\r\n",
				ssdpAddr, t.st, t.usn)
			conn.Write([]byte(msg))
		}
	}

	// Initial announcement
	sendAlive()

	for {
		select {
		case <-ctx.Done():
			sendByeBye()
			return
		case <-ticker.C:
			sendAlive()
		}
	}
}

// SearchResponder listens for M-SEARCH requests and responds.
func SearchResponder(ctx context.Context, baseURL, deviceUUID, serverName string) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := conn.SetReadBuffer(65536); err != nil {
		return
	}
	buf := make([]byte, 8192)

	validTargets := map[string]bool{
		"ssdp:all":          true,
		"upnp:rootdevice":   true,
		DeviceType:          true,
		AVTransportType:     true,
		RenderingType:       true,
		deviceUUID:          true,
	}

	for {
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			continue
		}

		text := string(buf[:n])
		if !strings.HasPrefix(text, "M-SEARCH * HTTP/1.1") {
			continue
		}
		if !strings.Contains(strings.ToUpper(text), "MAN: \"SSDP:DISCOVER\"") {
			continue
		}

		st := headerValue(text, "ST")
		if st == "" || !validTargets[st] {
			continue
		}

		usn := deviceUUID
		if st != deviceUUID && st != "ssdp:all" {
			usn = deviceUUID + "::" + st
		}

		resp := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\n"+
				"CACHE-CONTROL: max-age=%d\r\n"+
				"DATE: %s\r\n"+
				"EXT:\r\n"+
				"LOCATION: %s/device.xml\r\n"+
				"SERVER: %s\r\n"+
				"ST: %s\r\n"+
				"USN: %s\r\n"+
				"BOOTID.UPNP.ORG: 1\r\n"+
				"CONFIGID.UPNP.ORG: 1\r\n\r\n",
			cacheMaxAge, time.Now().UTC().Format(time.RFC1123), baseURL, serverName, st, usn)
		conn.WriteToUDP([]byte(resp), src)
	}
}

func headerValue(raw, key string) string {
	lines := strings.Split(raw, "\r\n")
	key = strings.ToUpper(key)
	for _, ln := range lines {
		if i := strings.IndexByte(ln, ':'); i > 0 {
			k := strings.ToUpper(strings.TrimSpace(ln[:i]))
			if k == key {
				return strings.TrimSpace(ln[i+1:])
			}
		}
	}
	return ""
}
