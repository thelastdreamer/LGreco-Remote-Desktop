package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lgreco/remote-desktop/agent/internal/host"
)

func main() {
	server := flag.String("server", "", "Control panel base URL, e.g. http://host:8080")
	token := flag.String("token", "", "Agent token from the control panel")
	name := flag.String("name", "", "Optional display name for this PC")
	flag.Parse()

	if *server == "" || *token == "" {
		fmt.Println("Usage: rd-agent --server http://your-panel --token <agent-token>")
		os.Exit(2)
	}

	base := strings.TrimRight(*server, "/")
	hostname, _ := os.Hostname()
	if err := announce(base, *token, hostname, *name); err != nil {
		log.Fatalf("presence failed: %v", err)
	}
	log.Printf("registered with %s as %s (%s/%s)", base, hostname, runtime.GOOS, runtime.GOARCH)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	for {
		err := runAgentLoop(base, *token, hostname, *name, interrupt)
		if err == errInterrupted {
			log.Println("agent stopped")
			return
		}
		log.Printf("agent socket disconnected: %v — reconnecting in 3s", err)
		select {
		case <-interrupt:
			return
		case <-time.After(3 * time.Second):
		}
		_ = announce(base, *token, hostname, *name)
	}
}

var errInterrupted = fmt.Errorf("interrupted")

func announce(base, token, hostname, name string) error {
	body, _ := json.Marshal(map[string]string{
		"hostname": hostname,
		"os":       runtime.GOOS + "/" + runtime.GOARCH,
		"name":     name,
	})
	req, err := http.NewRequest(http.MethodPost, base+"/api/agents/presence", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("status %d: %s", res.StatusCode, string(raw))
	}
	return nil
}

func runAgentLoop(base, token, hostname, name string, interrupt <-chan os.Signal) error {
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/ws/agent"
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Println("control channel connected — waiting for remote sessions")

	type connectMsg struct {
		Type         string `json:"type"`
		SessionID    int64  `json:"session_id"`
		SignalingKey string `json:"signaling_key"`
		SignalURL    string `json:"signal_url"`
		ICEServers   []struct {
			URLs       string `json:"urls"`
			Username   string `json:"username"`
			Credential string `json:"credential"`
		} `json:"ice_servers"`
	}

	done := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				done <- err
				return
			}
			var msg connectMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "welcome", "pong":
				continue
			case "connect":
				log.Printf("incoming remote session #%d", msg.SessionID)
				go func(m connectMsg) {
					ice := make([]host.ICEServer, 0, len(m.ICEServers))
					for _, s := range m.ICEServers {
						ice = append(ice, host.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential})
					}
					signalURL := absoluteWS(base, fmt.Sprintf("/ws/signal?session=%d&role=host&key=%s", m.SessionID, m.SignalingKey))
					if err := host.RunSession(signalURL, ice); err != nil {
						log.Printf("session #%d ended: %v", m.SessionID, err)
					}
				}(msg)
			}
		}
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-interrupt:
			return errInterrupted
		case err := <-done:
			return err
		case <-ticker.C:
			_ = announce(base, token, hostname, name)
			_ = conn.WriteJSON(map[string]string{"type": "ping"})
		}
	}
}

func absoluteWS(base, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		return path
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/") + path
}
