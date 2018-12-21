package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"sync"
	"time"
)

type pipeBuffer struct {
	blocked bool
	rd, wr  int8
	buf     []bytes.Buffer
	swap    chan struct{}
	done    chan struct{}
	stop    chan struct{}
	mtx     *sync.Mutex
}

func newPipeBuffer() *pipeBuffer {
	return &pipeBuffer{
		rd:   0,
		wr:   1,
		buf:  make([]bytes.Buffer, 2),
		swap: make(chan struct{}),
		done: make(chan struct{}),
		stop: make(chan struct{}),
		mtx:  &sync.Mutex{},
	}
}

// ErrBufferStopped means write to buffer is not allowed
var ErrBufferStopped = errors.New("buffer is stopped")

// ErrBufferClosed means neither write nor read is allowed
var ErrBufferClosed = errors.New("buffer is closed")

// close buffer
func (m *pipeBuffer) Close() {
	select {
	case <-m.done:
		return
	default:
	}
	m.buf[0].Reset()
	m.buf[1].Reset()
	close(m.done)
}

// Stop buffer
// After buffer is stopped,  Write() is not allowed, while Read() is ok
func (m *pipeBuffer) Stop() {
	select {
	case <-m.stop:
		return
	default:
	}
	close(m.stop)
}

// Write []byte to buffer. The return error is ErrBufferClosed when the buffer
// is closed. The return error is ErrBufferStopped when the buffer is
// blocked.
func (m *pipeBuffer) Write(p []byte) (int, error) {
	select {
	case <-m.done:
		return 0, ErrBufferClosed
	default:
	}

	select {
	case <-m.stop:
		return 0, ErrBufferStopped
	default:
		m.mtx.Lock()
		defer m.mtx.Unlock()
	}

	if m.buf[m.wr].Len() == 0 {
		m.swap <- struct{}{} // prevent swap empty buffers
	}

	return m.buf[m.wr].Write(p)
}

func (m *pipeBuffer) empty() bool {
	return m.buf[m.rd].Len() == 0 && m.buf[m.wr].Len() == 0
}

func (m *pipeBuffer) Read(p []byte) (int, error) {
	// Read buffer is empty
	if m.buf[m.rd].Len() == 0 {
		select {
		case <-m.done:
			return 0, ErrBufferClosed
		case <-m.stop:
			// write is blocked, drain the buffer
			m.mtx.Lock()
			defer m.mtx.Unlock()
			if m.buf[m.wr].Len() == 0 {
				return 0, io.EOF
			}
			m.rd, m.wr = m.wr, m.rd
			return 0, nil
		case <-m.swap:
			// block read until write buffer is not empty
			m.mtx.Lock()
			m.rd, m.wr = m.wr, m.rd
			m.mtx.Unlock()
			return 0, nil
		}
	}

	// Read buffer is not empty
	select {
	case <-m.done:
		return 0, ErrBufferClosed
	default:
		n, _ := m.buf[m.rd].Read(p)
		time.Sleep(500 * time.Millisecond)
		log.Printf("%d bytes read", n)
		return n, nil
	}
}
