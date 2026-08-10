package audio

import "log"

type Output struct {
	enabled    bool
	bufferSize int
	buffer     []int16
	pos        int
}

func NewOutput() *Output {
	return &Output{
		enabled:    true,
		bufferSize: 48000,
		buffer:     make([]int16, 48000),
	}
}

func (o *Output) PushSamples(samples []int16) {
	if !o.enabled {
		return
	}
	if len(samples) > 0 {
		o.pos += len(samples)
		if o.pos >= o.bufferSize {
			o.Flush()
		}
		copy(o.buffer[o.pos-len(samples):o.pos], samples)
	}
}

func (o *Output) Flush() {
	if o.pos == 0 {
		return
	}
	log.Printf("audio: flushed %d samples", o.pos)
	o.pos = 0
}

func (o *Output) SetEnabled(enabled bool) {
	o.enabled = enabled
}

func (o *Output) Close() {
	o.Flush()
}
