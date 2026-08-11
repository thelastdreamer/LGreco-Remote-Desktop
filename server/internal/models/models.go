package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Type          string    `json:"type"` // "desktop" or "relay"
	Status        string    `json:"status"` // "pending", "running", "stopped"
	ContainerID   string    `json:"container_id,omitempty"`
	ContainerName string    `json:"container_name,omitempty"`
	SignalingKey  string    `json:"-"`
	Resolution    string    `json:"resolution"`
	AudioEnabled  bool      `json:"audio_enabled"`
	ClipboardSync bool      `json:"clipboard_sync"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CreateSessionRequest struct {
	Type          string `json:"type"`
	Resolution    string `json:"resolution"`
	AudioEnabled  bool   `json:"audio_enabled"`
	ClipboardSync bool   `json:"clipboard_sync"`
	TargetHost    string `json:"target_host,omitempty"`
	TargetPort    int    `json:"target_port,omitempty"`
}

type SessionResponse struct {
	Session     Session       `json:"session"`
	ICEServers  []ICEServer   `json:"ice_servers"`
	SignalURL   string        `json:"signal_url"`
	ViewerURL   string        `json:"viewer_url,omitempty"`
}

type ICEServer struct {
	URLs       string `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
}

type WebRTCMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
	SDP       string      `json:"sdp,omitempty"`
	Candidate struct {
		Candidate     string `json:"candidate"`
		SDPMid       *string `json:"sdpMid"`
		SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
	} `json:"candidate,omitempty"`
}

type InputEvent struct {
	Type    string  `json:"type"` // "keydown", "keyup", "mousemove", "mousedown", "mouseup", "wheel"
	KeyCode int     `json:"keycode,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Button  int     `json:"button,omitempty"`
	Delta   int     `json:"delta,omitempty"`
}

type FileTransferMessage struct {
	Type     string `json:"type"` // "upload_start", "upload_chunk", "upload_end", "download_request"
	FileName string `json:"filename,omitempty"`
	FileSize int64  `json:"filesize,omitempty"`
	Chunk    []byte `json:"chunk,omitempty"`
	Offset   int64  `json:"offset,omitempty"`
}
