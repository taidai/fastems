// Package framelog keeps a bounded in-memory log of raw Modbus frames.
package framelog

import (
	"encoding/hex"
	"sync"
	"time"
)

// Frame is one raw request or response.
type Frame struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	Peer      string    `json:"peer,omitempty"`
	Direction string    `json:"direction"`
	Hex       string    `json:"hex"`
}

// Log is a bounded ring buffer shared by all channels.
type Log struct {
	mu     sync.Mutex
	frames []Frame
	max    int
	next   int64
}

// Default is the process-wide frame log used by the runtime transports.
var Default = New(500)

// New returns a bounded log.
func New(max int) *Log {
	if max <= 0 {
		max = 500
	}
	return &Log{max: max, frames: make([]Frame, 0, max)}
}

// Record appends one frame, dropping the oldest when full.
func (l *Log) Record(peer, direction string, data []byte) {
	if l == nil || len(data) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	frame := Frame{ID: l.next, Time: time.Now(), Peer: peer, Direction: direction, Hex: hex.EncodeToString(data)}
	if len(l.frames) < l.max {
		l.frames = append(l.frames, frame)
		return
	}
	copy(l.frames, l.frames[1:])
	l.frames[len(l.frames)-1] = frame
}

// List returns newest first.
func (l *Log) List(limit int) []Frame {
	if l == nil {
		return []Frame{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 || limit > len(l.frames) {
		limit = len(l.frames)
	}
	out := make([]Frame, 0, limit)
	for i := len(l.frames) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, l.frames[i])
	}
	return out
}

// Clear drops every frame.
func (l *Log) Clear() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.frames = l.frames[:0]
	l.mu.Unlock()
}
