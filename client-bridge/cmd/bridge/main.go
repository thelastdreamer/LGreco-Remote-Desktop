package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lgreco/remote-desktop/client-bridge/internal/pipe"
	"github.com/lgreco/remote-desktop/client-bridge/internal/webrtc"
)

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws/signal", "Signaling server WebSocket URL")
	sessionID := flag.String("session", "", "Session ID to join")
	pipeName := flag.String("pipe", `\\.\pipe\rd-bridge`, "Named pipe for WPF communication")
	flag.Parse()

	if *sessionID == "" {
		log.Fatal("--session is required")
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("rd-bridge starting...")

	pipeServer := pipe.NewServer(*pipeName)
	if err := pipeServer.Start(); err != nil {
		log.Fatalf("failed to start named pipe server: %v", err)
	}
	defer pipeServer.Stop()

	pc, err := webrtc.NewPeerConnection(*serverURL, *sessionID)
	if err != nil {
		log.Fatalf("failed to create peer connection: %v", err)
	}

	pc.OnVideoFrame = func(data []byte, width, height int) {
		pipeServer.WriteVideoFrame(data, width, height)
	}

	pc.OnInputEvent = pipeServer.GetInputHandler()

	if err := pc.Connect(); err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer pc.Close()

	go pipeServer.Serve()
	go pc.Listen()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("rd-bridge shutting down...")
}
