package orchestration

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
)

type Orchestrator struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Orchestrator {
	return &Orchestrator{cfg: cfg}
}

func (o *Orchestrator) CreateDesktopContainer(sessionID int64, resolution string) (containerID, containerName string, err error) {
	name := fmt.Sprintf("rd-session-%d", sessionID)
	parts := strings.Split(resolution, "x")
	width, height := parts[0], "720"
	if len(parts) >= 2 {
		height = parts[1]
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"--network", o.cfg.DockerNetwork,
		"-e", fmt.Sprintf("RESOLUTION=%sx%s", width, height),
		"-e", fmt.Sprintf("SESSION_ID=%d", sessionID),
		"-e", fmt.Sprintf("SIGNAL_URL=/ws/signal?session=%d&role=host", sessionID),
		"-e", fmt.Sprintf("STUN_SERVER=%s", o.cfg.StunServer),
		"-e", fmt.Sprintf("TURN_SERVER=%s", o.cfg.TurnServer),
		"-e", fmt.Sprintf("TURN_USERNAME=%s", o.cfg.TurnUsername),
		"-e", fmt.Sprintf("TURN_PASSWORD=%s", o.cfg.TurnPassword),
		"--shm-size=1g",
		"rd-desktop:latest",
	}

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("docker run failed: %w, output: %s", err, string(out))
	}
	containerID = strings.TrimSpace(string(out))

	_, err = db.DB.Exec(
		`UPDATE sessions SET container_id = $1, container_name = $2, status = 'running' WHERE id = $3`,
		containerID, name, sessionID,
	)
	if err != nil {
		log.Printf("failed to update session %d: %v", sessionID, err)
	}

	log.Printf("created container %s for session %d", containerID[:12], sessionID)
	return containerID, name, nil
}

func (o *Orchestrator) CreateRelayContainer(sessionID int64, targetHost string, targetPort int, resolution string) (containerID, containerName string, err error) {
	name := fmt.Sprintf("rd-relay-%d", sessionID)
	parts := strings.Split(resolution, "x")
	width, height := parts[0], "720"
	if len(parts) >= 2 {
		height = parts[1]
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"--network", o.cfg.DockerNetwork,
		"-e", fmt.Sprintf("TARGET_HOST=%s", targetHost),
		"-e", fmt.Sprintf("TARGET_PORT=%d", targetPort),
		"-e", fmt.Sprintf("RESOLUTION=%sx%s", width, height),
		"-e", fmt.Sprintf("SESSION_ID=%d", sessionID),
		"-e", fmt.Sprintf("SIGNAL_URL=/ws/signal?session=%d&role=host", sessionID),
		"-e", fmt.Sprintf("STUN_SERVER=%s", o.cfg.StunServer),
		"-e", fmt.Sprintf("TURN_SERVER=%s", o.cfg.TurnServer),
		"-e", fmt.Sprintf("TURN_USERNAME=%s", o.cfg.TurnUsername),
		"-e", fmt.Sprintf("TURN_PASSWORD=%s", o.cfg.TurnPassword),
		"--shm-size=512m",
		"rd-relay:latest",
	}

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("docker run failed: %w, output: %s", err, string(out))
	}
	containerID = strings.TrimSpace(string(out))

	_, err = db.DB.Exec(
		`UPDATE sessions SET container_id = $1, container_name = $2, status = 'running' WHERE id = $3`,
		containerID, name, sessionID,
	)
	if err != nil {
		log.Printf("failed to update session %d: %v", sessionID, err)
	}

	log.Printf("created relay container %s for session %d", containerID[:12], sessionID)
	return containerID, name, nil
}

func (o *Orchestrator) StopContainer(sessionID int64) error {
	var containerName string
	err := db.DB.QueryRow(
		`SELECT COALESCE(container_name, 'rd-session-' || $1::text) FROM sessions WHERE id = $1`,
		sessionID,
	).Scan(&containerName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "stop", "-t", "5", containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("docker stop warning: %v, output: %s", err, string(out))
	}

	rmCmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
	rmCmd.Run()

	_, err = db.DB.Exec(
		`UPDATE sessions SET status = 'stopped', container_id = NULL, container_name = NULL WHERE id = $1`,
		sessionID,
	)
	if err != nil {
		return err
	}
	log.Printf("stopped container for session %d", sessionID)
	return nil
}
