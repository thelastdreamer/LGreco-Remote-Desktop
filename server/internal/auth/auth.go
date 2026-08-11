package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
	"github.com/lgreco/remote-desktop/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

var TokenAuth *jwtauth.JWTAuth

func Init(cfg *config.Config) {
	TokenAuth = jwtauth.New("HS256", []byte(cfg.JWTSecret), nil)
}

func Middleware() func(http.Handler) http.Handler {
	return jwtauth.Verifier(TokenAuth)
}

func Authenticator() func(http.Handler) http.Handler {
	return jwtauth.Authenticator(TokenAuth)
}

func RegisterUser(username, email, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &models.User{}
	err = db.DB.QueryRow(
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, username, email, password_change_required, created_at, updated_at`,
		username, email, string(hash),
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordChangeRequired, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func AuthenticateUser(username, password string) (*models.User, error) {
	user := &models.User{}
	err := db.DB.QueryRow(
		`SELECT id, username, email, password_hash, password_change_required, created_at, updated_at
		 FROM users WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.PasswordChangeRequired, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("invalid credentials")
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

func EnsureDefaultAdmin(cfg *config.Config) error {
	if cfg.DefaultAdminUsername == "" || cfg.DefaultAdminPassword == "" {
		return nil
	}

	var existingID int64
	err := db.DB.QueryRow(`SELECT id FROM users WHERE username = $1`, cfg.DefaultAdminUsername).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check default admin: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.DefaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}

	_, err = db.DB.Exec(
		`INSERT INTO users (username, email, password_hash, password_change_required)
		 VALUES ($1, $2, $3, true)`,
		cfg.DefaultAdminUsername, cfg.DefaultAdminEmail, string(hash),
	)
	if err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	return nil
}

func ChangePassword(userID int64, currentPassword, newPassword string) error {
	if len(strings.TrimSpace(newPassword)) < 10 {
		return errors.New("new password must be at least 10 characters")
	}

	var passwordHash string
	err := db.DB.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = db.DB.Exec(
		`UPDATE users
		 SET password_hash = $2, password_change_required = false, updated_at = NOW()
		 WHERE id = $1`,
		userID, string(hash),
	)
	return err
}

func GenerateToken(user *models.User) (string, error) {
	_, claims, _ := jwtauth.FromContext(context.Background())
	_ = claims
	_, tokenString, err := TokenAuth.Encode(map[string]interface{}{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})
	return tokenString, err
}

func GenerateSignalingKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func UserIDFromContext(r *http.Request) (int64, error) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		return 0, err
	}
	uid, ok := claims["user_id"]
	if !ok {
		return 0, errors.New("user_id not in token")
	}
	switch v := uid.(type) {
	case float64:
		return int64(v), nil
	default:
		return 0, errors.New("invalid user_id type")
	}
}

func ParseAuthHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// SetJWTCookie writes the session JWT so same-origin iframe/WebSocket
// requests (noVNC) can authenticate without an Authorization header.
func SetJWTCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    token,
		Path:     "/",
		MaxAge:   int((72 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearJWTCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
