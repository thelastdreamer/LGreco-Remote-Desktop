package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           int
	DBHost         string
	DBPort         int
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	JWTSecret      string
	RedisAddr      string
	TurnServer     string
	TurnUsername   string
	TurnPassword   string
	StunServer     string
	DockerNetwork  string
}

func Load() *Config {
	return &Config{
		Port:          getEnvInt("PORT", 8080),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnvInt("DB_PORT", 5432),
		DBUser:        getEnv("DB_USER", "rduser"),
		DBPassword:    getEnv("DB_PASSWORD", "rdpassword"),
		DBName:        getEnv("DB_NAME", "remote_desktop"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production-32-bytes!"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		TurnServer:    getEnv("TURN_SERVER", "turn:localhost:3478"),
		TurnUsername:  getEnv("TURN_USERNAME", "rduser"),
		TurnPassword:  getEnv("TURN_PASSWORD", "rdturnpass"),
		StunServer:    getEnv("STUN_SERVER", "stun:stun.l.google.com:19302"),
		DockerNetwork: getEnv("DOCKER_NETWORK", "rd-network"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
