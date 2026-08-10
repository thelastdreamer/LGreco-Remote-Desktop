package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type VideoFrameCallback func(data []byte, width, height int)
type InputEventHandler func(eventType string, data []byte)

type PeerConnection struct {
	serverURL    string
	sessionID    string
	wsConn       *websocket.Conn
	pc           *webrtc.PeerConnection
	videoTrack   *webrtc.TrackLocalStaticSample
	audioTrack   *webrtc.TrackLocalStaticSample
	dataChannel  *webrtc.DataChannel
	OnVideoFrame VideoFrameCallback
	OnInputEvent InputEventHandler
	mu           sync.Mutex
	connected    bool
	width        int
	height       int
}

func NewPeerConnection(serverURL, sessionID string) (*PeerConnection, error) {
	pc := &PeerConnection{
		serverURL: serverURL,
		sessionID: sessionID,
		width:     1280,
		height:    720,
	}
	return pc, nil
}

func (p *PeerConnection) Connect() error {
	url := fmt.Sprintf("%s?session=%s&role=viewer", p.serverURL, p.sessionID)
	log.Printf("connecting to signaling server: %s", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial error: %w", err)
	}
	p.wsConn = conn

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	p.pc = peerConnection

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "rd-video",
	)
	if err != nil {
		return fmt.Errorf("create video track: %w", err)
	}
	p.videoTrack = videoTrack

	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio", "rd-audio",
	)
	if err != nil {
		return fmt.Errorf("create audio track: %w", err)
	}
	p.audioTrack = audioTrack

	p.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("received track: %s, type: %s", track.ID(), track.Kind())
		go p.handleTrack(track)
	})

	p.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("data channel created: %s", dc.Label())
		p.dataChannel = dc
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if p.OnInputEvent != nil {
				p.OnInputEvent("input", msg.Data)
			}
		})
	})

	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("connection state: %s", state)
		if state == webrtc.PeerConnectionStateConnected {
			p.mu.Lock()
			p.connected = true
			p.mu.Unlock()
			log.Println("peer connection established")
		} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			p.mu.Lock()
			p.connected = false
			p.mu.Unlock()
		}
	})

	return nil
}

func (p *PeerConnection) Listen() {
	defer p.wsConn.Close()

	for {
		_, message, err := p.wsConn.ReadMessage()
		if err != nil {
			log.Printf("ws read error: %v", err)
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("failed to parse message: %v", err)
			continue
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "offer":
			p.handleOffer(msg)
		case "ice-candidate":
			p.handleICECandidate(msg)
		case "pong":
		default:
			log.Printf("unknown message type: %s", msgType)
		}
	}
}

func (p *PeerConnection) handleOffer(msg map[string]interface{}) {
	sdp, _ := msg["sdp"].(string)
	if sdp == "" {
		log.Println("offer has no SDP")
		return
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := p.pc.SetRemoteDescription(offer); err != nil {
		log.Printf("set remote description: %v", err)
		return
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("create answer: %v", err)
		return
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		log.Printf("set local description: %v", err)
		return
	}

	answerMsg, _ := json.Marshal(map[string]interface{}{
		"type": "answer",
		"sdp":  answer.SDP,
	})
	p.wsConn.WriteMessage(websocket.TextMessage, answerMsg)
	log.Println("sent answer to signaling server")
}

func (p *PeerConnection) handleICECandidate(msg map[string]interface{}) {
	cand, ok := msg["candidate"].(map[string]interface{})
	if !ok {
		return
	}
	candidateStr, _ := cand["candidate"].(string)
	if candidateStr == "" {
		return
	}

	sdpMLineIndexF, _ := cand["sdpMLineIndex"].(float64)
	sdpMid, _ := cand["sdpMid"].(string)

	candidate := webrtc.ICECandidateInit{
		Candidate:     candidateStr,
		SDPMLineIndex: (*uint16)(nil),
		SDPMid:        &sdpMid,
	}
	idx := uint16(sdpMLineIndexF)
	candidate.SDPMLineIndex = &idx

	if err := p.pc.AddICECandidate(candidate); err != nil {
		log.Printf("add ice candidate: %v", err)
	}
}

func (p *PeerConnection) handleTrack(track *webrtc.TrackRemote) {
	for {
		rtp, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("read rtp error: %v", err)
			return
		}

		if track.Kind() == webrtc.RTPCodecTypeVideo && p.OnVideoFrame != nil {
			p.OnVideoFrame(rtp.Payload, p.width, p.height)
		} else if track.Kind() == webrtc.RTPCodecTypeAudio {
			_ = rtp
		}
	}
}

func (p *PeerConnection) SendInput(eventType string, data []byte) error {
	if p.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}
	return p.dataChannel.Send(data)
}

func (p *PeerConnection) SendFileMetadata(name string, size int64) error {
	if p.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}
	meta, _ := json.Marshal(map[string]interface{}{
		"type":     "upload_start",
		"filename": name,
		"filesize": size,
	})
	return p.dataChannel.Send(meta)
}

func (p *PeerConnection) SendFileChunk(offset int64, chunk []byte) error {
	if p.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}
	msg, _ := json.Marshal(map[string]interface{}{
		"type":   "upload_chunk",
		"offset": offset,
		"chunk":  chunk,
	})
	return p.dataChannel.Send(msg)
}

func (p *PeerConnection) SendFileEnd(filename string) error {
	if p.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}
	msg, _ := json.Marshal(map[string]interface{}{
		"type":     "upload_end",
		"filename": filename,
	})
	return p.dataChannel.Send(msg)
}

func (p *PeerConnection) Close() {
	if p.dataChannel != nil {
		p.dataChannel.Close()
	}
	if p.pc != nil {
		p.pc.Close()
	}
	if p.wsConn != nil {
		p.wsConn.Close()
	}
}

var _ = media.Sample{}
