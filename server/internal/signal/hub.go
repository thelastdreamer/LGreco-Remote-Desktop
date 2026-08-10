package signal

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
}

type Room struct {
	ID       string
	mu       sync.RWMutex
	clients  map[*Client]bool
}

type Client struct {
	ID        string
	RoomID    string
	Role      string // "viewer" or "host"
	Conn      *websocket.Conn
	Send      chan []byte
	hub       *Hub
	mu        sync.Mutex
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

func (r *Room) GetOtherClients(sender *Client) []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var others []*Client
	for c := range r.clients {
		if c != sender {
			others = append(others, c)
		}
	}
	return others
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	sessionID := r.URL.Query().Get("session")
	role := r.URL.Query().Get("role")
	if sessionID == "" {
		sessionID = "default"
	}
	if role == "" {
		role = "viewer"
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
		if len(room.GetOtherClients(nil)) == 0 {
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
			c.mu.Lock()
			c.Send <- ack
			c.mu.Unlock()
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
