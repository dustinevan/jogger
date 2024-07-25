package job

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrOutputStreamerClosed = errors.New("output streamer is closed")

type OutputStreamerOption func(*OutputStreamer)

func WithStreamMessageSize(size int) OutputStreamerOption {
	return func(o *OutputStreamer) {
		if size < 1 {
			panic("stream message size must be greater than 0")
		}
		o.streamMessageSize = size
	}
}

// A OutputStreamer is an io.Writer that collects data written to it and fans it out
// to clients who want to read that data as a stream. Callers of NewStream() are provided
// a channel that will receive all data written since the streamer was created.
//
// When the context passed to NewStream() is canceled, the channel will be
// closed immediately without writing any further data.
//
// OutputStreamer also implements the io.Closer interface. Closing an OutputStreamer
// means that we don't expect any more data to be written to it. After an OutputStreamer
// instance is closed, any calls to Write() will return an error. And channels returned
// from NewStream() will be closed after all data has been written to them.
type OutputStreamer struct {
	output            []byte
	mu                sync.RWMutex
	writerClosed      atomic.Bool
	streamMessageSize int

	length atomic.Int64
}

func NewOutputStreamer(options ...OutputStreamerOption) *OutputStreamer {
	o := &OutputStreamer{
		streamMessageSize: 1024,
		output:            make([]byte, 0),
	}

	for _, opt := range options {
		opt(o)
	}

	return o
}

// Write appends data to the internal buffer. This implements the io.Writer interface,
// making an instance of OutputStreamer usable as the STDOUT and STDERR fields in an exec.Cmd.
func (o *OutputStreamer) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writerClosed.Load() {
		return 0, ErrOutputStreamerClosed
	}
	o.output = append(o.output, b...)
	o.length.Store(int64(len(o.output)))
	return len(b), nil
}

func (o *OutputStreamer) CloseWriter() {
	o.writerClosed.Store(true)
}

// Next returns the next chunk of data to be read from the OutputStreamer.
// Note: no copies of the data are made, so the caller should not modify the returned slice.
// This design enables large output buffers to be read by many clients without incurring the cost of
// copying the data.
func (o *OutputStreamer) Next(index int) []byte {
	if int64(index) >= o.length.Load() {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if index+o.streamMessageSize > len(o.output) {
		return o.output[index:]
	}
	return o.output[index : index+o.streamMessageSize]
}

// NewStream returns a channel that will receive all data written to the OutputStreamer.
// When a job is running and writing data to the OutputStreamer, the channel will
// receive data in chunks of, at most, streamMessageSize bytes.
//
// The reader is configured to check for new data at least once per second. When there
// is new data, it catches up to the end of stream without waiting.
//
// When the job exits, the OutputStreamer is closed to writes, but the data remains
// available to NewStream() callers until the server is shutdown.
func (o *OutputStreamer) NewStream(ctx context.Context) <-chan []byte {
	stream := make(chan []byte, 2)

	go func() {
		// Note: internally the ticker channel has a buffer of 1, so we won't
		// build up a backlog of ticks if there is a lot of initial data to
		// send, or some other delay.
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		index := 0
		for {
			// send more data if there is any
			if int64(index) < o.length.Load() {
				msg := o.Next(index)
				index += len(msg)
				stream <- msg
				// this loops so that we don't wait on the ticker to check for more data
				continue
			}
			if int64(index) == o.length.Load() {
				// only close the channel if the OutputStreamer is no longer being written to
				// this happens when the job has exited
				if o.writerClosed.Load() {
					close(stream)
					return
				}
			}
			// wait for the next tick or the context to be canceled
			select {
			case <-ctx.Done():
				close(stream)
				return
			case <-ticker.C:
				// check for more data by looping again
			}
		}
	}()

	return stream
}
