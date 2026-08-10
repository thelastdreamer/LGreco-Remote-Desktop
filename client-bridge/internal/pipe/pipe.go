package pipe

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"syscall"
	"unsafe"
)

const (
	pipeBufferSize      = 65536
	sharedMemoryName    = "rd-video-frame"
	sharedMemorySize    = 1920 * 1080 * 4
	cmdConnect          = 1
	cmdDisconnect       = 2
	cmdSendInput        = 3
	cmdSetResolution    = 4
	cmdGetStatus        = 5
	cmdFileUploadStart  = 6
	cmdFileUploadChunk  = 7
	cmdFileUploadEnd    = 8
)

type Command struct {
	Cmd      int             `json:"cmd"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type ConnectRequest struct {
	ServerURL  string `json:"server_url"`
	SessionID  string `json:"session_id"`
	Token      string `json:"token"`
}

type InputEvent struct {
	Type    string  `json:"type"`
	KeyCode int     `json:"keycode,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Button  int     `json:"button,omitempty"`
	Delta   int     `json:"delta,omitempty"`
}

type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type StatusResponse struct {
	Connected bool `json:"connected"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
}

type Server struct {
	pipeName       string
	shmHandle      syscall.Handle
	shmAddr        uintptr
	shmSize        int
	videoWidth     int
	videoHeight    int
	inputCallback  func(eventType string, data []byte)
	mu             sync.RWMutex
	done           chan struct{}
	connected      bool
}

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procCreateFile    = kernel32.NewProc("CreateFileW")
	procCreateFileMap = kernel32.NewProc("CreateFileMappingW")
	procMapViewOfFile = kernel32.NewProc("MapViewOfFile")
	procUnmapViewOfFile = kernel32.NewProc("UnmapViewOfFile")
	procCloseHandle   = kernel32.NewProc("CloseHandle")
)

func NewServer(pipeName string) *Server {
	return &Server{
		pipeName: pipeName,
		done:     make(chan struct{}),
	}
}

func (s *Server) Start() error {
	s.videoWidth = 1280
	s.videoHeight = 720
	s.shmSize = s.videoWidth * s.videoHeight * 4
	return nil
}

func (s *Server) Stop() {
	close(s.done)
}

func (s *Server) Serve() {
	log.Printf("named pipe server listening on %s", s.pipeName)

	for {
		select {
		case <-s.done:
			return
		default:
		}

		pipePath, _ := syscall.UTF16PtrFromString(s.pipeName)
		hPipe, _, err := procCreateFile.Call(
			uintptr(unsafe.Pointer(pipePath)),
			syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			0, 0,
			syscall.OPEN_EXISTING,
			syscall.FILE_FLAG_OVERLAPPED,
			0,
		)
		if hPipe == uintptr(syscall.InvalidHandle) {
			continue
		}
		defer procCloseHandle.Call(hPipe)

		buf := make([]byte, pipeBufferSize)
		var bytesRead uint32
		err = syscall.ReadFile(syscall.Handle(hPipe), buf, &bytesRead, nil)
		if err != nil {
			continue
		}

		if bytesRead > 0 {
			s.handleCommand(syscall.Handle(hPipe), buf[:bytesRead])
		}
	}
}

func (s *Server) handleCommand(pipe syscall.Handle, data []byte) {
	var cmd Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		s.sendResponse(pipe, map[string]string{"error": "invalid command"})
		return
	}

	switch cmd.Cmd {
	case cmdConnect:
		var req ConnectRequest
		json.Unmarshal(cmd.Data, &req)
		s.mu.Lock()
		s.connected = true
		s.mu.Unlock()
		s.sendResponse(pipe, map[string]string{"status": "connected"})

	case cmdDisconnect:
		s.mu.Lock()
		s.connected = false
		s.mu.Unlock()
		s.sendResponse(pipe, map[string]string{"status": "disconnected"})

	case cmdSendInput:
		if s.inputCallback != nil {
			s.inputCallback("input", cmd.Data)
		}

	case cmdSetResolution:
		var res Resolution
		json.Unmarshal(cmd.Data, &res)
		s.mu.Lock()
		s.videoWidth = res.Width
		s.videoHeight = res.Height
		s.shmSize = res.Width * res.Height * 4
		s.mu.Unlock()
		s.sendResponse(pipe, map[string]string{"status": "ok"})

	case cmdGetStatus:
		s.mu.RLock()
		resp := StatusResponse{
			Connected: s.connected,
			Width:     s.videoWidth,
			Height:    s.videoHeight,
		}
		s.mu.RUnlock()
		respBytes, _ := json.Marshal(resp)
		s.sendResponse(pipe, respBytes)

	case cmdFileUploadStart, cmdFileUploadChunk, cmdFileUploadEnd:
		s.sendResponse(pipe, map[string]string{"status": "queued"})
	}
}

func (s *Server) sendResponse(pipe syscall.Handle, data interface{}) {
	var respBytes []byte
	switch v := data.(type) {
	case map[string]string:
		respBytes, _ = json.Marshal(v)
	case []byte:
		respBytes = v
	default:
		respBytes, _ = json.Marshal(data)
	}

	var written uint32
	syscall.WriteFile(pipe, respBytes, &written, nil)
}

func (s *Server) WriteVideoFrame(data []byte, width, height int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.shmAddr == 0 {
		return
	}

	stride := width * 4
	copySize := stride * height
	if copySize > s.shmSize {
		copySize = s.shmSize
	}

	var dst []byte
	sh := (*sliceHeader)(unsafe.Pointer(&dst))
	sh.Data = s.shmAddr
	sh.Len = copySize
	sh.Cap = copySize

	copy(dst, data[:copySize])
}

func (s *Server) GetInputHandler() func(eventType string, data []byte) {
	return func(eventType string, data []byte) {
		s.inputCallback = nil
	}
}

func (s *Server) SetInputHandler(handler func(eventType string, data []byte)) {
	s.inputCallback = handler
}

type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

var _ = binary.LittleEndian
var _ = fmt.Sprint
