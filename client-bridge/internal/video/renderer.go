package video

import (
	"sync"
)

type Frame struct {
	Data   []byte
	Width  int
	Height int
	Stride int
}

type Renderer struct {
	currentFrame Frame
	mu           sync.RWMutex
	callbacks    []func(frame Frame)
}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) PushFrame(data []byte, width, height int) {
	r.mu.Lock()
	r.currentFrame = Frame{
		Data:   data,
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	callbacks := make([]func(frame Frame), len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.Unlock()

	for _, cb := range callbacks {
		cb(r.currentFrame)
	}
}

func (r *Renderer) GetCurrentFrame() Frame {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentFrame
}

func (r *Renderer) OnFrame(callback func(frame Frame)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

func (r *Renderer) GetResolution() (width, height int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentFrame.Width, r.currentFrame.Height
}
