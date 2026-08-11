package signal

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// AgentHub pushes connect requests to online agents over a persistent WS.
type AgentHub struct {
	mu      sync.RWMutex
	agents  map[int64]*AgentConn
}

type AgentConn struct {
	AgentID int64
	Send    chan []byte
	Conn    interface{ Close() error }
}

func NewAgentHub() *AgentHub {
	return &AgentHub{agents: make(map[int64]*AgentConn)}
}

func (h *AgentHub) Register(agentID int64, send chan []byte, closer interface{ Close() error }) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.agents[agentID]; ok {
		close(prev.Send)
		_ = prev.Conn.Close()
	}
	h.agents[agentID] = &AgentConn{AgentID: agentID, Send: send, Conn: closer}
}

func (h *AgentHub) Unregister(agentID int64, send chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.agents[agentID]; ok && cur.Send == send {
		delete(h.agents, agentID)
	}
}

func (h *AgentHub) IsOnline(agentID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.agents[agentID]
	return ok
}

func (h *AgentHub) NotifyConnect(agentID int64, payload interface{}) bool {
	h.mu.RLock()
	conn, ok := h.agents[agentID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	select {
	case conn.Send <- body:
		return true
	case <-time.After(2 * time.Second):
		log.Printf("agent %d connect notify timed out", agentID)
		return false
	}
}

// HandleAgentSocket is the control-plane websocket for installed agents.
// Auth is done by the caller before upgrade; agentID is trusted.
func (h *AgentHub) HandleAgentSocket(w http.ResponseWriter, r *http.Request, agentID int64) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("agent websocket upgrade error: %v", err)
		return
	}

	send := make(chan []byte, 32)
	h.Register(agentID, send, conn)
	defer func() {
		h.Unregister(agentID, send)
		_ = conn.Close()
	}()

	hello, _ := json.Marshal(map[string]string{"type": "welcome"})
	_ = conn.WriteMessage(websocket.TextMessage, hello)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var envelope map[string]interface{}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				continue
			}
			if t, _ := envelope["type"].(string); t == "ping" {
				pong, _ := json.Marshal(map[string]string{"type": "pong"})
				select {
				case send <- pong:
				default:
				}
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case msg, ok := <-send:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}
