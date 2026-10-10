package ollama

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var ErrIdleTimeout = errors.New("Ollama stream idle timeout")
var ErrStreamLimit = errors.New("Ollama stream exceeds configured byte limit")

// StreamOptions bounds upstream NDJSON and transformed output independently.
// A caller should supply the gateway's configured limits and idle timeout.
type StreamOptions struct {
	MaxLineBytes int64
	MaxBytes     int64
	IdleTimeout  time.Duration
}

// NDJSONReader converts one bounded native line at a time. Close must unblock
// Read on body, as net/http response bodies do. No reader goroutine or content
// accumulator is used; context cancellation and inactivity close the body.
type NDJSONReader struct {
	body        io.ReadCloser
	reader      *bufio.Reader
	stream      *Stream
	options     StreamOptions
	ctx         context.Context
	stopCancel  func() bool
	mu          sync.Mutex
	timer       *time.Timer
	closed      bool
	idle        bool
	closeOnce   sync.Once
	closeErr    error
	pending     []byte
	err         error
	inputBytes  int64
	outputBytes int64
}

func NewNDJSONReader(ctx context.Context, body io.ReadCloser, model string, options StreamOptions) *NDJSONReader {
	if options.MaxLineBytes <= 0 {
		options.MaxLineBytes = 1 << 20
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = 64 << 20
	}
	if options.MaxLineBytes > options.MaxBytes {
		options.MaxLineBytes = options.MaxBytes
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = 60 * time.Second
	}
	r := &NDJSONReader{body: body, stream: NewStream(model), options: options, ctx: ctx}
	bufferSize := int64(4096)
	if options.MaxLineBytes < bufferSize {
		bufferSize = options.MaxLineBytes + 1
	}
	r.reader = bufio.NewReaderSize(&activityReader{parent: r}, int(bufferSize))
	r.timer = time.AfterFunc(options.IdleTimeout, func() {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return
		}
		r.idle = true
		r.mu.Unlock()
		r.closeUnderlying()
	})
	r.stopCancel = context.AfterFunc(ctx, func() { r.closeUnderlying() })
	return r
}

type activityReader struct{ parent *NDJSONReader }

func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.parent.body.Read(p)
	if n > 0 {
		r.parent.mu.Lock()
		if !r.parent.closed && !r.parent.idle {
			r.parent.timer.Reset(r.parent.options.IdleTimeout)
		}
		r.parent.mu.Unlock()
	}
	return n, err
}

func (r *NDJSONReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 && r.err == nil {
		if err := r.interrupted(); err != nil {
			r.err = err
			break
		}
		if r.stream.done {
			r.err = io.EOF
			break
		}
		line, err := r.readLine()
		if interrupted := r.interrupted(); interrupted != nil {
			r.err = interrupted
			break
		}
		if err != nil && err != io.EOF {
			r.err = err
			break
		}
		if len(line) > 0 {
			r.pending, r.err = r.stream.TransformEvent(line)
			if r.err != nil {
				break
			}
			r.outputBytes += int64(len(r.pending))
			if r.outputBytes > r.options.MaxBytes {
				r.pending = nil
				r.err = fmt.Errorf("%w: transformed stream", ErrStreamLimit)
				break
			}
		}
		if err == io.EOF {
			_, r.err = r.stream.Finish()
			if r.err == nil {
				r.err = io.EOF
			}
		}
		if r.stream.done {
			r.err = io.EOF
		}
	}
	if r.err != nil {
		_ = r.Close()
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *NDJSONReader) readLine() ([]byte, error) {
	var line []byte
	for {
		var piece []byte
		var err error
		// bufio enforces a minimum buffer of 16 bytes. Tiny configured limits
		// must still fail before waiting for that buffer or a newline to fill.
		if r.options.MaxLineBytes < 16 {
			var b byte
			b, err = r.reader.ReadByte()
			if err == nil {
				piece = []byte{b}
				if b != '\n' {
					err = bufio.ErrBufferFull
				}
			}
		} else {
			piece, err = r.reader.ReadSlice('\n')
		}
		r.inputBytes += int64(len(piece))
		if r.inputBytes > r.options.MaxBytes {
			return nil, fmt.Errorf("%w: native stream", ErrStreamLimit)
		}
		if int64(len(line))+int64(len(piece)) > r.options.MaxLineBytes {
			return nil, fmt.Errorf("%w: Ollama NDJSON line exceeds byte limit", ErrStreamLimit)
		}
		line = append(line, piece...)
		if err != bufio.ErrBufferFull {
			return line, err
		}
	}
}

func (r *NDJSONReader) interrupted() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idle {
		return ErrIdleTimeout
	}
	if r.closed {
		return io.ErrClosedPipe
	}
	return nil
}

func (r *NDJSONReader) closeUnderlying() {
	r.closeOnce.Do(func() { r.closeErr = r.body.Close() })
}

func (r *NDJSONReader) Close() error {
	r.mu.Lock()
	r.closed = true
	if r.timer != nil {
		r.timer.Stop()
	}
	r.mu.Unlock()
	if r.stopCancel != nil {
		r.stopCancel()
	}
	r.closeUnderlying()
	return r.closeErr
}
