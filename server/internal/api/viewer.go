package api

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/lgreco/remote-desktop/server/internal/auth"
	"github.com/lgreco/remote-desktop/server/internal/db"
)

func handleGetViewer(w http.ResponseWriter, r *http.Request) {
	sessionID, containerName, err := getOwnedSessionTarget(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if containerName == "" {
		writeError(w, http.StatusConflict, "session container is not ready")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID,
		"viewer_url": fmt.Sprintf("/api/sessions/%s/novnc/vnc.html?autoconnect=true&resize=remote&path=/api/sessions/%s/novnc/websockify", sessionID, sessionID),
	})
}

func handleNoVNCProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, containerName, err := getOwnedSessionTarget(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if containerName == "" {
			writeError(w, http.StatusConflict, "session container is not ready")
			return
		}

		targetPath := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/api/sessions/%s/novnc", chi.URLParam(r, "sessionID")))
		if targetPath == "" || targetPath == "/" {
			targetPath = "/vnc.html"
		}

		if websocket.IsWebSocketUpgrade(r) {
			proxyWebSocket(w, r, containerName, targetPath)
			return
		}

		targetURL := &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:8081", containerName),
		}
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.URL.Path = targetPath
			req.Host = targetURL.Host
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del("X-Frame-Options")
			return nil
		}
		proxy.ServeHTTP(w, r)
	})
}

func proxyWebSocket(w http.ResponseWriter, r *http.Request, containerName, targetPath string) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	target := url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("%s:8081", containerName),
		Path:     targetPath,
		RawQuery: r.URL.RawQuery,
	}
	targetConn, _, err := websocket.DefaultDialer.Dial(target.String(), nil)
	if err != nil {
		_ = clientConn.WriteMessage(websocket.TextMessage, []byte("failed to connect to session viewer"))
		return
	}
	defer targetConn.Close()

	errCh := make(chan error, 2)
	go copyWebSocket(targetConn, clientConn, errCh)
	go copyWebSocket(clientConn, targetConn, errCh)
	<-errCh
}

func copyWebSocket(dst, src *websocket.Conn, errCh chan<- error) {
	for {
		msgType, payload, err := src.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}
		if err := dst.WriteMessage(msgType, payload); err != nil {
			errCh <- err
			return
		}
	}
}

func getOwnedSessionTarget(r *http.Request) (string, string, error) {
	userID, err := auth.UserIDFromContext(r)
	if err != nil {
		return "", "", err
	}

	sessionID := chi.URLParam(r, "sessionID")
	var containerName sql.NullString
	err = db.DB.QueryRow(
		`SELECT container_name FROM sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	).Scan(&containerName)
	if err != nil {
		return "", "", err
	}

	return sessionID, containerName.String, nil
}

var _ = io.Copy
