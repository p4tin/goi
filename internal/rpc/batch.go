package rpc

import (
	"sync"
	"time"
	"unicode/utf8"
)

// Output batching limits (R12.1): an event at most every flushInterval or
// once maxBatchBytes accumulate.
const (
	flushInterval = 30 * time.Millisecond
	maxBatchBytes = 64 << 10
)

// OutputChunk is a run of text from one stream.
type OutputChunk struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// outputBatcher coalesces stream output into ordered chunks and hands them
// to emit at most every flushInterval (or when maxBatchBytes is reached).
// Incomplete UTF-8 sequences at a chunk's end are carried to the next write
// of the same stream so multi-byte runes never split across events.
type outputBatcher struct {
	emit func([]OutputChunk)

	mu     sync.Mutex
	chunks []OutputChunk
	size   int
	carry  map[string][]byte
	timer  *time.Timer
	closed bool
}

func newOutputBatcher(emit func([]OutputChunk)) *outputBatcher {
	return &outputBatcher{emit: emit, carry: map[string][]byte{}}
}

// Write queues data from stream.
func (b *outputBatcher) Write(stream string, data []byte) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	buf := append(b.carry[stream], data...)
	cut := utf8Cut(buf)
	b.carry[stream] = append([]byte(nil), buf[cut:]...)
	text := string(buf[:cut])
	if text == "" {
		b.mu.Unlock()
		return
	}
	if n := len(b.chunks); n > 0 && b.chunks[n-1].Stream == stream {
		b.chunks[n-1].Text += text
	} else {
		b.chunks = append(b.chunks, OutputChunk{Stream: stream, Text: text})
	}
	b.size += len(text)
	if b.size >= maxBatchBytes {
		out := b.take()
		b.mu.Unlock()
		b.emit(out)
		return
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(flushInterval, b.Flush)
	}
	b.mu.Unlock()
}

// take returns and clears the pending chunks; b.mu must be held.
func (b *outputBatcher) take() []OutputChunk {
	out := b.chunks
	b.chunks, b.size = nil, 0
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	return out
}

// Flush emits pending chunks now, including carried partial runes.
func (b *outputBatcher) Flush() {
	b.mu.Lock()
	for stream, rest := range b.carry {
		if len(rest) > 0 {
			b.chunks = append(b.chunks, OutputChunk{Stream: stream, Text: string(rest)})
			b.carry[stream] = nil
		}
	}
	out := b.take()
	b.mu.Unlock()
	if len(out) > 0 {
		b.emit(out)
	}
}

// Close flushes and drops later writes.
func (b *outputBatcher) Close() {
	b.Flush()
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
}

// utf8Cut returns the length of the longest prefix of b that doesn't end in
// an incomplete UTF-8 sequence.
func utf8Cut(b []byte) int {
	// A rune is at most 4 bytes: look back up to 3 bytes for a start byte.
	for i := len(b) - 1; i >= 0 && i >= len(b)-3; i-- {
		c := b[i]
		if c < 0x80 {
			return len(b) // ASCII: complete
		}
		if utf8.RuneStart(c) {
			if utf8.FullRune(b[i:]) {
				return len(b)
			}
			return i
		}
	}
	return len(b)
}

// lineSplitter turns a byte stream into complete lines (without "\n").
type lineSplitter struct {
	buf []byte
}

// Write appends data and returns the complete lines it finished.
func (l *lineSplitter) Write(data []byte) []string {
	l.buf = append(l.buf, data...)
	var lines []string
	for {
		i := indexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		line := l.buf[:i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		lines = append(lines, string(line))
		l.buf = l.buf[i+1:]
	}
	return lines
}

// Rest returns and clears an unterminated last line.
func (l *lineSplitter) Rest() string {
	s := string(l.buf)
	l.buf = nil
	return s
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
