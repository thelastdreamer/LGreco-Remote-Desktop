package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lgreco/remote-desktop/agent/internal/capture"
	"github.com/lgreco/remote-desktop/agent/internal/input"
	"github.com/pion/webrtc/v4"
)

type ICEServer struct {
	URLs       string
	Username   string
	Credential string
}

// RunSession hosts a real-PC remote session: JPEG frames + input over datachannels.
func RunSession(signalURL string, ice []ICEServer) error {
	ws, _, err := websocket.DefaultDialer.Dial(signalURL, nil)
	if err != nil {
		return fmt.Errorf("signal dial: %w", err)
	}
	defer ws.Close()

	iceServers := make([]webrtc.ICEServer, 0, len(ice))
	for _, s := range ice {
		if s.URLs == "" {
			continue
		}
		entry := webrtc.ICEServer{URLs: []string{s.URLs}}
		if s.Username != "" {
			entry.Username = s.Username
			entry.Credential = s.Credential
		}
		iceServers = append(iceServers, entry)
	}
	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
	if err != nil {
		return err
	}
	defer pc.Close()

	screenDC, err := pc.CreateDataChannel("screen", nil)
	if err != nil {
		return err
	}
	controlDC, err := pc.CreateDataChannel("control", nil)
	if err != nil {
		return err
	}

	controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		var ev input.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return
		}
		_ = input.Apply(ev)
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cand := c.ToJSON()
		payload, _ := json.Marshal(map[string]interface{}{
			"type": "ice-candidate",
			"candidate": map[string]interface{}{
				"candidate":     cand.Candidate,
				"sdpMid":        cand.SDPMid,
				"sdpMLineIndex": cand.SDPMLineIndex,
			},
		})
		_ = ws.WriteMessage(websocket.TextMessage, payload)
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return err
	}
	offerMsg, _ := json.Marshal(map[string]string{"type": "offer", "sdp": offer.SDP})
	if err := ws.WriteMessage(websocket.TextMessage, offerMsg); err != nil {
		return err
	}

	stopCapture := make(chan struct{})
	defer close(stopCapture)
	go streamScreen(screenDC, stopCapture)

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "answer":
			sdp, _ := msg["sdp"].(string)
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}); err != nil {
				log.Printf("set remote answer: %v", err)
			}
		case "ice-candidate":
			candMap, _ := msg["candidate"].(map[string]interface{})
			if candMap == nil {
				continue
			}
			candidate, _ := candMap["candidate"].(string)
			var mid *string
			if v, ok := candMap["sdpMid"].(string); ok {
				mid = &v
			}
			var index *uint16
			if v, ok := candMap["sdpMLineIndex"].(float64); ok {
				i := uint16(v)
				index = &i
			}
			_ = pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     candidate,
				SDPMid:        mid,
				SDPMLineIndex: index,
			})
		case "pong":
		}
		if pc.ConnectionState() == webrtc.PeerConnectionStateFailed ||
			pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return fmt.Errorf("peer closed")
		}
	}
}

func streamScreen(dc *webrtc.DataChannel, stop <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond) // ~10 FPS JPEG MVP
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if dc.ReadyState() != webrtc.DataChannelStateOpen {
				continue
			}
			img, err := capture.PrimaryScreen()
			if err != nil {
				continue
			}
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 55}); err != nil {
				continue
			}
			if err := dc.Send(buf.Bytes()); err != nil {
				return
			}
		}
	}
}
