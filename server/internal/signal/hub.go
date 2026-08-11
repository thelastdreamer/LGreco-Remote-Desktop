package signal

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/lgreco/remote-desktop/server/internal/db"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

type Room struct {
	ID      string
	mu      sync.RWMutex
	clients map[*Client]bool
}

type Client struct {
	ID     string
	RoomID string
	Role   string // "viewer" or "host"
	Conn   *websocket.Conn
	Send   chan []byte
	hub    *Hub
	mu     sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Room),
	}
}

func (h *Hub) GetOrCreateRoom(id string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[id]; ok {
		return room
	}
	room := &Room{
		ID:      id,
		clients: make(map[*Client]bool),
	}
	h.rooms[id] = room
	return room
}

func (h *Hub) RemoveRoom(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, id)
}

func (r *Room) AddClient(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[c] = true
}

func (r *Room) RemoveClient(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[c]; ok {
		delete(r.clients, c)
		close(c.Send)
		c.Conn.Close()
	}
}

func (r *Room) Broadcast(sender *Client, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for c := range r.clients {
		if c != sender {
			select {
			case c.Send <- msg:
			default:
				go r.RemoveClient(c)
			}
		}
	}
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func validateSignalingKey(sessionID, key string) bool {
	if key == "" || sessionID == "" || db.DB == nil {
		return false
	}
	var stored string
	var status string
	err := db.DB.QueryRow(
		`SELECT signaling_key, status FROM sessions WHERE id = $1`,
		sessionID,
	).Scan(&stored, &status)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("signaling key lookup failed: %v", err)
		}
		return false
	}
	if status == "stopped" || status == "failed" {
		return false
	}
	return stored == key
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	role := r.URL.Query().Get("role")
	key := r.URL.Query().Get("key")
	if sessionID == "" {
		http.Error(w, "session required", http.StatusBadRequest)
		return
	}
	if role == "" {
		role = "viewer"
	}
	if !validateSignalingKey(sessionID, key) {
		http.Error(w, "invalid signaling key", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:     sessionID + "-" + role,
		RoomID: sessionID,
		Role:   role,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		hub:    h,
	}

	room := h.GetOrCreateRoom(sessionID)
	room.AddClient(client)

	go client.writePump()
	go client.readPump(room)

	log.Printf("client %s joined room %s as %s", client.ID, sessionID, role)
}

func (c *Client) readPump(room *Room) {
	defer func() {
		room.RemoveClient(c)
		if room.ClientCount() == 0 {
			c.hub.RemoveRoom(c.RoomID)
		}
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket read error: %v", err)
			}
			break
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "ping":
			ack, _ := json.Marshal(map[string]string{"type": "pong"})
			select {
			case c.Send <- ack:
			default:
			}
		default:
			room.Broadcast(c, message)
		}
	}
}

func (c *Client) writePump() {
	for msg := range c.Send {
		c.mu.Lock()
		err := c.Conn.WriteMessage(websocket.TextMessage, msg)
		c.mu.Unlock()
		if err != nil {
			return
		}
	}
}
