package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lgreco/remote-desktop/server/internal/auth"
	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
	"github.com/lgreco/remote-desktop/server/internal/models"
	"github.com/lgreco/remote-desktop/server/internal/signal"
)

func handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var req models.CreateAgentRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Unnamed PC"
	}

	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	code, err := generateDeviceCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate device code")
		return
	}

	agent := models.Agent{}
	err = db.DB.QueryRow(
		`INSERT INTO agents (user_id, device_code, token_hash, name, status)
		 VALUES ($1, $2, $3, $4, 'offline')
		 RETURNING id, user_id, device_code, name, hostname, os, status, created_at`,
		userID, code, hashAgentToken(token), req.Name,
	).Scan(&agent.ID, &agent.UserID, &agent.DeviceCode, &agent.Name, &agent.Hostname, &agent.OS, &agent.Status, &agent.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create agent: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, models.CreateAgentResponse{
		Agent:      agent,
		AgentToken: token,
		InstallHint: fmt.Sprintf(
			`rd-agent.exe --server %s --token %s`,
			publicOrigin(r), token,
		),
	})
}

func handleListAgents(agentHub *signal.AgentHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		rows, err := db.DB.Query(
			`SELECT id, user_id, device_code, name, hostname, os, status, last_seen_at, created_at
			 FROM agents WHERE user_id = $1 ORDER BY created_at DESC`, userID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		agents := make([]models.Agent, 0)
		for rows.Next() {
			var a models.Agent
			var lastSeen sql.NullTime
			if err := rows.Scan(&a.ID, &a.UserID, &a.DeviceCode, &a.Name, &a.Hostname, &a.OS, &a.Status, &lastSeen, &a.CreatedAt); err != nil {
				continue
			}
			if lastSeen.Valid {
				t := lastSeen.Time
				a.LastSeenAt = &t
			}
			if agentHub.IsOnline(a.ID) {
				a.Status = "online"
			} else if a.Status == "online" {
				a.Status = "offline"
			}
			agents = append(agents, a)
		}
		writeJSON(w, http.StatusOK, agents)
	}
}

func handleDeleteAgent(agentHub *signal.AgentHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		agentID := chi.URLParam(r, "agentID")
		res, err := db.DB.Exec(`DELETE FROM agents WHERE id = $1 AND user_id = $2`, agentID, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func handleConnectAgent(cfg *config.Config, agentHub *signal.AgentHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		agentIDStr := chi.URLParam(r, "agentID")
		var agentID int64
		var status string
		err = db.DB.QueryRow(
			`SELECT id, status FROM agents WHERE id = $1 AND user_id = $2`,
			agentIDStr, userID,
		).Scan(&agentID, &status)
		if err != nil {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		if !agentHub.IsOnline(agentID) {
			writeError(w, http.StatusConflict, "agent is offline — start rd-agent on that PC")
			return
		}

		signalingKey, err := auth.GenerateSignalingKey()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate signaling key")
			return
		}

		session := models.Session{}
		err = db.DB.QueryRow(
			`INSERT INTO sessions (user_id, type, status, signaling_key, resolution, audio_enabled, clipboard_sync, expires_at, agent_id)
			 VALUES ($1, 'agent', 'running', $2, 'native', true, true, $3, $4)
			 RETURNING id, user_id, type, status, signaling_key, resolution, audio_enabled, clipboard_sync, created_at, expires_at, COALESCE(agent_id, 0)`,
			userID, signalingKey, time.Now().Add(24*time.Hour), agentID,
		).Scan(
			&session.ID, &session.UserID, &session.Type, &session.Status, &session.SignalingKey,
			&session.Resolution, &session.AudioEnabled, &session.ClipboardSync, &session.CreatedAt, &session.ExpiresAt, &session.AgentID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create agent session: "+err.Error())
			return
		}

		ice := []models.ICEServer{
			{URLs: cfg.StunServer},
			{URLs: cfg.TurnServer, Username: cfg.TurnUsername, Credential: cfg.TurnPassword},
		}
		signalPath := fmt.Sprintf("/ws/signal?session=%d&key=%s", session.ID, signalingKey)
		notified := agentHub.NotifyConnect(agentID, map[string]interface{}{
			"type":          "connect",
			"session_id":    session.ID,
			"signaling_key": signalingKey,
			"signal_url":    signalPath,
			"role":          "host",
			"ice_servers":   ice,
		})
		if !notified {
			_, _ = db.DB.Exec(`UPDATE sessions SET status = 'failed' WHERE id = $1`, session.ID)
			writeError(w, http.StatusConflict, "failed to reach agent")
			return
		}

		_, _ = db.DB.Exec(`UPDATE agents SET status = 'busy' WHERE id = $1`, agentID)

		writeJSON(w, http.StatusCreated, models.AgentConnectResponse{
			Session:    session,
			ICEServers: ice,
			SignalURL:  signalPath + "&role=viewer",
			ViewerURL:  fmt.Sprintf("/viewer.html?session=%d&key=%s", session.ID, signalingKey),
		})
	}
}

func handleAgentPresence(agentHub *signal.AgentHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerOrQueryToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "agent token required")
			return
		}
		agent, err := lookupAgentByToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}

		var body struct {
			Hostname string `json:"hostname"`
			OS       string `json:"os"`
			Name     string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Hostname != "" || body.OS != "" || body.Name != "" {
			_, _ = db.DB.Exec(
				`UPDATE agents SET hostname = COALESCE(NULLIF($2,''), hostname),
				 os = COALESCE(NULLIF($3,''), os),
				 name = COALESCE(NULLIF($4,''), name),
				 status = 'online',
				 last_seen_at = NOW()
				 WHERE id = $1`,
				agent.ID, body.Hostname, body.OS, body.Name,
			)
		} else {
			_, _ = db.DB.Exec(`UPDATE agents SET status = 'online', last_seen_at = NOW() WHERE id = $1`, agent.ID)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"agent_id":    agent.ID,
			"device_code": agent.DeviceCode,
			"status":      "online",
		})
	}
}

func AgentSocketHandler(agentHub *signal.AgentHub) http.HandlerFunc {
	return handleAgentSocket(agentHub)
}

func handleAgentSocket(agentHub *signal.AgentHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerOrQueryToken(r)
		if token == "" {
			http.Error(w, "agent token required", http.StatusUnauthorized)
			return
		}
		agent, err := lookupAgentByToken(token)
		if err != nil {
			http.Error(w, "invalid agent token", http.StatusUnauthorized)
			return
		}
		_, _ = db.DB.Exec(`UPDATE agents SET status = 'online', last_seen_at = NOW() WHERE id = $1`, agent.ID)
		agentHub.HandleAgentSocket(w, r, agent.ID)
		_, _ = db.DB.Exec(
			`UPDATE agents SET status = 'offline', last_seen_at = NOW() WHERE id = $1`,
			agent.ID,
		)
	}
}

func lookupAgentByToken(token string) (*models.Agent, error) {
	a := &models.Agent{}
	err := db.DB.QueryRow(
		`SELECT id, user_id, device_code, name, hostname, os, status, created_at
		 FROM agents WHERE token_hash = $1`,
		hashAgentToken(token),
	).Scan(&a.ID, &a.UserID, &a.DeviceCode, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func bearerOrQueryToken(r *http.Request) string {
	if t := auth.ParseAuthHeader(r); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

func generateDeviceCode() (string, error) {
	const digits = "0123456789"
	var b strings.Builder
	for i := 0; i < 9; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		b.WriteByte(digits[n.Int64()])
	}
	raw := b.String()
	return raw[0:3] + " " + raw[3:6] + " " + raw[6:9], nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func publicOrigin(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}
