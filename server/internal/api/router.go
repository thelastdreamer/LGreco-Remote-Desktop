package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/lgreco/remote-desktop/server/internal/auth"
	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
	"github.com/lgreco/remote-desktop/server/internal/models"
	"github.com/lgreco/remote-desktop/server/internal/orchestration"
)

func NewRouter(cfg *config.Config) chi.Router {
	r := chi.NewRouter()
	orch := orchestration.New(cfg)

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Post("/register", handleRegister)
		r.Post("/login", handleLogin)

		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware())
			r.Use(auth.Authenticator())

			r.Get("/me", handleMe)
			r.Post("/change-password", handleChangePassword)
			r.Get("/bootstrap", handleBootstrap)

			r.Group(func(r chi.Router) {
				r.Use(requirePasswordRotation)
				r.Post("/sessions", handleCreateSession(cfg, orch))
				r.Get("/sessions", handleListSessions)
				r.Get("/sessions/{sessionID}", handleGetSession)
				r.Get("/sessions/{sessionID}/viewer", handleGetViewer)
				r.Handle("/sessions/{sessionID}/novnc/*", handleNoVNCProxy())
				r.Delete("/sessions/{sessionID}", handleDeleteSession(orch))
				r.Get("/sessions/{sessionID}/ice-servers", handleICEServers(cfg))
			})
		})
	})

	return r
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username, email, and password required")
		return
	}
	user, err := auth.RegisterUser(req.Username, req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	token, err := auth.GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	writeJSON(w, http.StatusCreated, models.LoginResponse{Token: token, User: *user})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := auth.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	token, err := auth.GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	writeJSON(w, http.StatusOK, models.LoginResponse{Token: token, User: *user})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	user, err := loadUserByID(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func handleBootstrap(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := loadUserByID(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	sessions, err := loadSessionsForUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":     user,
		"sessions": sessions,
	})
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password required")
		return
	}
	if err := auth.ChangePassword(userID, req.CurrentPassword, req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := loadUserByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func handleCreateSession(cfg *config.Config, orch *orchestration.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		var req models.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req = models.CreateSessionRequest{
				Type:       "desktop",
				Resolution: "1280x720",
			}
		}
		if req.Type == "" {
			req.Type = "desktop"
		}
		if req.Resolution == "" {
			req.Resolution = "1280x720"
		}
		if req.Type == "relay" && req.TargetHost == "" {
			writeError(w, http.StatusBadRequest, "target_host required for relay sessions")
			return
		}
		if req.TargetPort == 0 {
			req.TargetPort = 3389
		}

		signalingKey, err := auth.GenerateSignalingKey()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate signaling key")
			return
		}

		session := &models.Session{}
		err = db.DB.QueryRow(
			`INSERT INTO sessions (user_id, type, status, signaling_key, resolution, audio_enabled, clipboard_sync, expires_at)
			 VALUES ($1, $2, 'pending', $3, $4, $5, $6, $7)
			 RETURNING id, user_id, type, status, signaling_key, resolution, audio_enabled, clipboard_sync, created_at, expires_at`,
			userID, req.Type, signalingKey, req.Resolution, req.AudioEnabled, req.ClipboardSync,
			time.Now().Add(24*time.Hour),
		).Scan(
			&session.ID, &session.UserID, &session.Type, &session.Status,
			&session.SignalingKey, &session.Resolution, &session.AudioEnabled,
			&session.ClipboardSync, &session.CreatedAt, &session.ExpiresAt,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create session")
			return
		}

		if req.Type == "relay" {
			_, _, err = orch.CreateRelayContainer(session.ID, signalingKey, req.TargetHost, req.TargetPort, req.Resolution)
		} else {
			_, _, err = orch.CreateDesktopContainer(session.ID, signalingKey, req.Resolution)
		}
		if err != nil {
			_, _ = db.DB.Exec(`DELETE FROM sessions WHERE id = $1`, session.ID)
			writeError(w, http.StatusInternalServerError, "failed to start session container: "+err.Error())
			return
		}

		session.Status = "running"

		writeJSON(w, http.StatusCreated, models.SessionResponse{
			Session: *session,
			ICEServers: []models.ICEServer{
				{URLs: cfg.StunServer},
				{URLs: cfg.TurnServer, Username: cfg.TurnUsername, Credential: cfg.TurnPassword},
			},
			SignalURL: "/ws/signal",
			ViewerURL: fmt.Sprintf("/api/sessions/%d/novnc/vnc.html?autoconnect=true&resize=remote&path=/api/sessions/%d/novnc/websockify", session.ID, session.ID),
		})
	}
}

func handleListSessions(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	sessions, err := loadSessionsForUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var s models.Session
	var cid, cname sql.NullString
	err = db.DB.QueryRow(
		`SELECT id, user_id, type, status, container_id, container_name, signaling_key, resolution, audio_enabled, clipboard_sync, created_at, expires_at
		 FROM sessions WHERE id = $1 AND user_id = $2`, sessionID, userID,
	).Scan(&s.ID, &s.UserID, &s.Type, &s.Status, &cid, &cname,
		&s.SignalingKey, &s.Resolution, &s.AudioEnabled, &s.ClipboardSync, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	s.ContainerID = cid.String
	s.ContainerName = cname.String
	writeJSON(w, http.StatusOK, s)
}

func handleDeleteSession(orch *orchestration.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionID")
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		var owned bool
		err = db.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1 AND user_id = $2)`, sessionID, userID).Scan(&owned)
		if err != nil || !owned {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}

		var sid int64
		if _, err := fmt.Sscan(sessionID, &sid); err != nil {
			writeError(w, http.StatusBadRequest, "invalid session id")
			return
		}
		if err := orch.StopContainer(sid); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	}
}

func handleICEServers(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []models.ICEServer{
			{URLs: cfg.StunServer},
			{URLs: cfg.TurnServer, Username: cfg.TurnUsername, Credential: cfg.TurnPassword},
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, models.ErrorResponse{Error: msg, Code: status})
}

func requirePasswordRotation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.UserIDFromContext(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		var passwordChangeRequired bool
		err = db.DB.QueryRow(`SELECT password_change_required FROM users WHERE id = $1`, userID).Scan(&passwordChangeRequired)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if passwordChangeRequired {
			writeError(w, http.StatusForbidden, "password change required before accessing sessions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loadUserByID(userID int64) (*models.User, error) {
	user := &models.User{}
	err := db.DB.QueryRow(
		`SELECT id, username, email, password_change_required, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordChangeRequired, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func loadSessionsForUser(userID int64) ([]models.Session, error) {
	rows, err := db.DB.Query(
		`SELECT id, user_id, type, status, container_id, container_name, resolution, audio_enabled, clipboard_sync, created_at, expires_at
		 FROM sessions WHERE user_id = $1 AND status != 'stopped' ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		var cid, cname sql.NullString
		if err := rows.Scan(&s.ID, &s.UserID, &s.Type, &s.Status, &cid, &cname,
			&s.Resolution, &s.AudioEnabled, &s.ClipboardSync, &s.CreatedAt, &s.ExpiresAt); err != nil {
			continue
		}
		s.ContainerID = cid.String
		s.ContainerName = cname.String
		sessions = append(sessions, s)
	}
	return sessions, nil
}
