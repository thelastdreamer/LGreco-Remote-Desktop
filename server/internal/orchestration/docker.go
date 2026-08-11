package orchestration

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
)

type Orchestrator struct {
	cfg       *config.Config
	buildMu   sync.Mutex
	builtOnce sync.Map
}

type RuntimeStatus struct {
	DesktopImage          string `json:"desktop_image"`
	RelayImage            string `json:"relay_image"`
	DesktopReady          bool   `json:"desktop_ready"`
	RelayReady            bool   `json:"relay_ready"`
	DesktopContext        string `json:"desktop_context"`
	RelayContext          string `json:"relay_context"`
	DesktopContextExists  bool   `json:"desktop_context_exists"`
	RelayContextExists    bool   `json:"relay_context_exists"`
	DockerNetwork         string `json:"docker_network"`
}

func New(cfg *config.Config) *Orchestrator {
	return &Orchestrator{cfg: cfg}
}

func (o *Orchestrator) RuntimeStatus() RuntimeStatus {
	return RuntimeStatus{
		DesktopImage:         o.cfg.DesktopImage,
		RelayImage:           o.cfg.RelayImage,
		DesktopReady:         imageExists(o.cfg.DesktopImage),
		RelayReady:           imageExists(o.cfg.RelayImage),
		DesktopContext:       o.cfg.DesktopBuildContext,
		RelayContext:         o.cfg.RelayBuildContext,
		DesktopContextExists: pathExists(o.cfg.DesktopBuildContext),
		RelayContextExists:   pathExists(o.cfg.RelayBuildContext),
		DockerNetwork:        o.cfg.DockerNetwork,
	}
}

func (o *Orchestrator) EnsureRuntimeImages() error {
	if err := o.ensureImage(o.cfg.DesktopImage, o.cfg.DesktopBuildContext); err != nil {
		return fmt.Errorf("desktop image: %w", err)
	}
	if err := o.ensureImage(o.cfg.RelayImage, o.cfg.RelayBuildContext); err != nil {
		return fmt.Errorf("relay image: %w", err)
	}
	return nil
}

func (o *Orchestrator) CreateDesktopContainer(sessionID int64, signalingKey, resolution string) (containerID, containerName string, err error) {
	if err := o.ensureImage(o.cfg.DesktopImage, o.cfg.DesktopBuildContext); err != nil {
		return "", "", fmt.Errorf("desktop image unavailable: %w", err)
	}

	name := fmt.Sprintf("rd-session-%d", sessionID)
	width, height := splitResolution(resolution)

	args := []string{
		"run", "-d",
		"--pull", "never",
		"--name", name,
		"--network", o.cfg.DockerNetwork,
		"-e", fmt.Sprintf("RESOLUTION=%sx%s", width, height),
		"-e", fmt.Sprintf("SESSION_ID=%d", sessionID),
		"-e", fmt.Sprintf("SIGNAL_URL=ws://api:8080/ws/signal?session=%d&role=host&key=%s", sessionID, signalingKey),
		"-e", fmt.Sprintf("STUN_SERVER=%s", o.cfg.StunServer),
		"-e", fmt.Sprintf("TURN_SERVER=%s", o.cfg.TurnServer),
		"-e", fmt.Sprintf("TURN_USERNAME=%s", o.cfg.TurnUsername),
		"-e", fmt.Sprintf("TURN_PASSWORD=%s", o.cfg.TurnPassword),
		"--shm-size=1g",
		o.cfg.DesktopImage,
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

	log.Printf("created container %s for session %d", shortID(containerID), sessionID)
	return containerID, name, nil
}

func (o *Orchestrator) CreateRelayContainer(sessionID int64, signalingKey, targetHost string, targetPort int, resolution string) (containerID, containerName string, err error) {
	if err := o.ensureImage(o.cfg.RelayImage, o.cfg.RelayBuildContext); err != nil {
		return "", "", fmt.Errorf("relay image unavailable: %w", err)
	}

	name := fmt.Sprintf("rd-relay-%d", sessionID)
	width, height := splitResolution(resolution)

	args := []string{
		"run", "-d",
		"--pull", "never",
		"--name", name,
		"--network", o.cfg.DockerNetwork,
		"-e", fmt.Sprintf("TARGET_HOST=%s", targetHost),
		"-e", fmt.Sprintf("TARGET_PORT=%d", targetPort),
		"-e", fmt.Sprintf("RESOLUTION=%sx%s", width, height),
		"-e", fmt.Sprintf("SESSION_ID=%d", sessionID),
		"-e", fmt.Sprintf("SIGNAL_URL=ws://api:8080/ws/signal?session=%d&role=host&key=%s", sessionID, signalingKey),
		"-e", fmt.Sprintf("STUN_SERVER=%s", o.cfg.StunServer),
		"-e", fmt.Sprintf("TURN_SERVER=%s", o.cfg.TurnServer),
		"-e", fmt.Sprintf("TURN_USERNAME=%s", o.cfg.TurnUsername),
		"-e", fmt.Sprintf("TURN_PASSWORD=%s", o.cfg.TurnPassword),
		"--shm-size=512m",
		o.cfg.RelayImage,
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

	log.Printf("created relay container %s for session %d", shortID(containerID), sessionID)
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
	_ = rmCmd.Run()

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

func (o *Orchestrator) ensureImage(imageName, buildContext string) error {
	if imageName == "" {
		return fmt.Errorf("image name is empty")
	}
	if imageExists(imageName) {
		return nil
	}

	o.buildMu.Lock()
	defer o.buildMu.Unlock()

	if imageExists(imageName) {
		return nil
	}

	if buildContext == "" {
		return fmt.Errorf("image %s is missing and no build context is configured", imageName)
	}
	if !pathExists(buildContext) {
		return fmt.Errorf("image %s is missing and build context %s is unavailable", imageName, buildContext)
	}

	log.Printf("building missing runtime image %s from %s (this can take several minutes)", imageName, buildContext)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageName, buildContext)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build %s failed: %w, output: %s", imageName, err, truncate(string(out), 4000))
	}

	if !imageExists(imageName) {
		return fmt.Errorf("docker build completed but image %s is still missing", imageName)
	}

	o.builtOnce.Store(imageName, true)
	log.Printf("runtime image ready: %s", imageName)
	return nil
}

func imageExists(imageName string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	return cmd.Run() == nil
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func splitResolution(resolution string) (string, string) {
	parts := strings.Split(resolution, "x")
	width, height := parts[0], "720"
	if len(parts) >= 2 {
		height = parts[1]
	}
	return width, height
}

func shortID(id string) string {
	if len(id) >= 12 {
		return id[:12]
	}
	return id
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
