package upnp

import (
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"

	"github.com/kenyonj/airbridge/internal/state"
)

// Player interface for audio playback backends.
type Player interface {
	Play(ctx context.Context, uri string, volume int) error
	Pause(ctx context.Context) error
	Stop(ctx context.Context) error
	SetVolume(ctx context.Context, volume int) error
}

// AVTransportHandler handles AVTransport SOAP requests.
func AVTransportHandler(st *state.PlayerState, player Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := st.Context()
		action := ParseSOAPAction(r.Header.Get("SOAPACTION"))
		body, _ := io.ReadAll(r.Body)
		controller := ControllerID(r)

		log.Printf("AVTransport action=%s from=%s", action, controller)

		switch action {
		case "SetAVTransportURI":
			if !st.AcquireOrCheckSession(controller, true) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			uri := XMLText(body, "CurrentURI")
			meta := XMLText(body, "CurrentURIMetaData")
			st.SetURI(uri, meta)
			log.Printf("SetAVTransportURI: %s", uri)
			WriteSOAPResponse(w, AVTransportType, "SetAVTransportURIResponse", "")

		case "Play":
			if !st.AcquireOrCheckSession(controller, true) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			uri, _ := st.GetURI()
			if uri == "" {
				WriteSOAPError(w, 714, "No content selected")
				return
			}

			// Start playback asynchronously
			go func() {
				st.SetTransportState(state.StateTransition)
				if err := player.Play(ctx, uri, st.GetVolume()); err != nil {
					log.Printf("Play error: %v", err)
					st.SetTransportState(state.StateStopped)
					return
				}
				st.SetTransportState(state.StatePlaying)
			}()
			WriteSOAPResponse(w, AVTransportType, "PlayResponse", "")

		case "Pause":
			if !st.HasSession(controller) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			go func() {
				if err := player.Pause(ctx); err != nil {
					log.Printf("Pause error: %v", err)
				}
				st.SetTransportState(state.StatePaused)
			}()
			WriteSOAPResponse(w, AVTransportType, "PauseResponse", "")

		case "Stop":
			if !st.HasSession(controller) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			go func() {
				if err := player.Stop(ctx); err != nil {
					log.Printf("Stop error: %v", err)
				}
				st.SetTransportState(state.StateStopped)
				st.ReleaseSession()
			}()
			WriteSOAPResponse(w, AVTransportType, "StopResponse", "")

		case "GetTransportInfo":
			ts := st.GetTransportState()
			resp := fmt.Sprintf(
				"<CurrentTransportState>%s</CurrentTransportState>"+
					"<CurrentTransportStatus>OK</CurrentTransportStatus>"+
					"<CurrentSpeed>1</CurrentSpeed>",
				ts)
			WriteSOAPResponse(w, AVTransportType, "GetTransportInfoResponse", resp)

		case "GetPositionInfo":
			uri, _ := st.GetURI()
			resp := fmt.Sprintf(
				"<Track>0</Track>"+
					"<TrackDuration>00:00:00</TrackDuration>"+
					"<TrackMetaData></TrackMetaData>"+
					"<TrackURI>%s</TrackURI>"+
					"<RelTime>00:00:00</RelTime>"+
					"<AbsTime>00:00:00</AbsTime>"+
					"<RelCount>0</RelCount>"+
					"<AbsCount>0</AbsCount>",
				html.EscapeString(uri))
			WriteSOAPResponse(w, AVTransportType, "GetPositionInfoResponse", resp)

		case "GetMediaInfo":
			uri, meta := st.GetURI()
			resp := fmt.Sprintf(
				"<NrTracks>1</NrTracks>"+
					"<MediaDuration>00:00:00</MediaDuration>"+
					"<CurrentURI>%s</CurrentURI>"+
					"<CurrentURIMetaData>%s</CurrentURIMetaData>"+
					"<NextURI></NextURI>"+
					"<NextURIMetaData></NextURIMetaData>"+
					"<PlayMedium>NETWORK</PlayMedium>"+
					"<RecordMedium>NOT_IMPLEMENTED</RecordMedium>"+
					"<WriteStatus>NOT_IMPLEMENTED</WriteStatus>",
				html.EscapeString(uri), html.EscapeString(meta))
			WriteSOAPResponse(w, AVTransportType, "GetMediaInfoResponse", resp)

		case "GetTransportSettings":
			WriteSOAPResponse(w, AVTransportType, "GetTransportSettingsResponse",
				"<PlayMode>NORMAL</PlayMode><RecQualityMode>NOT_IMPLEMENTED</RecQualityMode>")

		case "GetDeviceCapabilities":
			WriteSOAPResponse(w, AVTransportType, "GetDeviceCapabilitiesResponse",
				"<PlayMedia>NETWORK</PlayMedia><RecMedia>NOT_IMPLEMENTED</RecMedia><RecQualityModes>NOT_IMPLEMENTED</RecQualityModes>")

		default:
			WriteSOAPError(w, 401, "Invalid Action")
		}
	}
}

// RenderingControlHandler handles RenderingControl SOAP requests.
func RenderingControlHandler(st *state.PlayerState, player Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := st.Context()
		action := ParseSOAPAction(r.Header.Get("SOAPACTION"))
		body, _ := io.ReadAll(r.Body)
		controller := ControllerID(r)

		log.Printf("RenderingControl action=%s from=%s", action, controller)

		switch action {
		case "SetVolume":
			if !st.HasSession(controller) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			vStr := XMLText(body, "DesiredVolume")
			v := 80
			fmt.Sscanf(vStr, "%d", &v)
			if v < 0 {
				v = 0
			}
			if v > 100 {
				v = 100
			}
			st.SetVolume(v)
			if err := player.SetVolume(ctx, v); err != nil {
				log.Printf("SetVolume error: %v", err)
			}
			WriteSOAPResponse(w, RenderingType, "SetVolumeResponse", "")

		case "GetVolume":
			v := st.GetVolume()
			WriteSOAPResponse(w, RenderingType, "GetVolumeResponse",
				fmt.Sprintf("<CurrentVolume>%d</CurrentVolume>", v))

		case "SetMute":
			if !st.HasSession(controller) {
				WriteSOAPError(w, 712, "Session in use")
				return
			}
			mStr := XMLText(body, "DesiredMute")
			m := mStr == "1" || mStr == "true"
			st.SetMute(m)
			WriteSOAPResponse(w, RenderingType, "SetMuteResponse", "")

		case "GetMute":
			m := st.GetMute()
			val := "0"
			if m {
				val = "1"
			}
			WriteSOAPResponse(w, RenderingType, "GetMuteResponse",
				fmt.Sprintf("<CurrentMute>%s</CurrentMute>", val))

		default:
			WriteSOAPError(w, 401, "Invalid Action")
		}
	}
}

// ConnectionManagerHandler handles ConnectionManager SOAP requests.
func ConnectionManagerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := ParseSOAPAction(r.Header.Get("SOAPACTION"))

		log.Printf("ConnectionManager action=%s", action)

		switch action {
		case "GetProtocolInfo":
			// Audio formats we can accept
			sink := "http-get:*:audio/mpeg:*," +
				"http-get:*:audio/mp3:*," +
				"http-get:*:audio/wav:*," +
				"http-get:*:audio/x-wav:*," +
				"http-get:*:audio/flac:*," +
				"http-get:*:audio/x-flac:*," +
				"http-get:*:audio/ogg:*," +
				"http-get:*:audio/aac:*," +
				"http-get:*:audio/mp4:*," +
				"http-get:*:audio/L16:*," +
				"http-get:*:audio/*:*"
			WriteSOAPResponse(w, ConnectionType, "GetProtocolInfoResponse",
				fmt.Sprintf("<Source></Source><Sink>%s</Sink>", sink))

		case "GetCurrentConnectionIDs":
			WriteSOAPResponse(w, ConnectionType, "GetCurrentConnectionIDsResponse",
				"<ConnectionIDs>0</ConnectionIDs>")

		case "GetCurrentConnectionInfo":
			resp := "<RcsID>0</RcsID>" +
				"<AVTransportID>0</AVTransportID>" +
				"<ProtocolInfo></ProtocolInfo>" +
				"<PeerConnectionManager></PeerConnectionManager>" +
				"<PeerConnectionID>-1</PeerConnectionID>" +
				"<Direction>Input</Direction>" +
				"<Status>OK</Status>"
			WriteSOAPResponse(w, ConnectionType, "GetCurrentConnectionInfoResponse", resp)

		default:
			WriteSOAPError(w, 401, "Invalid Action")
		}
	}
}

// EventHandler handles UPnP event subscriptions (stub).
func EventHandler(w http.ResponseWriter, r *http.Request) {
	// UPnP eventing is complex; for now just accept subscriptions
	if r.Method == "SUBSCRIBE" {
		w.Header().Set("SID", "uuid:event-subscription-1")
		w.Header().Set("TIMEOUT", "Second-1800")
		w.WriteHeader(200)
		return
	}
	w.WriteHeader(200)
}
