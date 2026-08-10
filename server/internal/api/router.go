package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/lgreco/remote-desktop/server/internal/auth"
	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
	"github.com/lgreco/remote-desktop/server/internal/models"
)

func NewRouter(cfg *config.Config) chi.Router {
	r := chi.NewRouter()

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
			r.Post("/sessions", handleCreateSession(cfg))
			r.Get("/sessions", handleListSessions)
			r.Get("/sessions/{sessionID}", handleGetSession)
			r.Delete("/sessions/{sessionID}", handleDeleteSession)
			r.Get("/sessions/{sessionID}/ice-servers", handleICEServers(cfg))
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
	user := &models.User{}
	err = db.DB.QueryRow(
		`SELECT id, username, email, created_at, updated_at FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func handleCreateSession(cfg *config.Config) http.HandlerFunc {
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

		writeJSON(w, http.StatusCreated, models.SessionResponse{
			Session: *session,
			ICEServers: []models.ICEServer{
				{URLs: cfg.StunServer},
				{URLs: cfg.TurnServer, Username: cfg.TurnUsername, Credential: cfg.TurnPassword},
			},
			SignalURL: "/ws/signal",
		})
	}
}

func handleListSessions(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	rows, err := db.DB.Query(
		`SELECT id, user_id, type, status, container_id, container_name, resolution, audio_enabled, clipboard_sync, created_at, expires_at
		 FROM sessions WHERE user_id = $1 AND status != 'stopped' ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	writeJSON(w, http.StatusOK, sessions)
}

func handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	var s models.Session
	var cid, cname sql.NullString
	err := db.DB.QueryRow(
		`SELECT id, user_id, type, status, container_id, container_name, signaling_key, resolution, audio_enabled, clipboard_sync, created_at, expires_at
		 FROM sessions WHERE id = $1`, sessionID,
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

func handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	result, err := db.DB.Exec(
		`UPDATE sessions SET status = 'stopped' WHERE id = $1 AND user_id = $2`, sessionID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
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
