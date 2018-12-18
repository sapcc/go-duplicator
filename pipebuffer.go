package main

import (
	"bytes"
	"errors"
	"fmt"
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
	log := func(c interface{}) { log.Printf("pipeBuffer write: %s", c) }
	log("start")
	log(string(p))

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
	log := func(c interface{}) { log.Printf("pipeBuffer read: %s", c) }
	log("start")

	// Read buffer is empty
	if m.buf[m.rd].Len() == 0 {
		log("m.rd is empty")
		select {
		case <-m.done:
			return 0, ErrBufferClosed
		default:
		}
		select {
		case <-m.stop:
			log("buffer stopped")
			if m.buf[m.wr].Len() == 0 {
				return 0, io.EOF
			}
		case <-m.swap:
			log("swap read/write")
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
		s, _ := fmt.Printf("%d bytes", n)
		log(s)
		return n, nil
	}
}

// // Unblock buffer
// func (m *pipeBuffer) UnBlock() error {
// 	select {
// 	case <-m.done:
// 		return ErrBufferClosed
// 	default:
// 		m.blocked = false
// 		return nil
// 	}
// }

// // Block buffer
// func (m *pipeBuffer) Block() (err error) {
// 	select {
// 	case <-m.done:
// 		return ErrBufferClosed
// 	default:
// 		m.blocked = true
// 		return nil
// 	}
// }

// WriteTo
// Write buffer A to Writer w, if buffer A is not emptpy
// Otherwise wait either buffer B is not empyt or kill signal
// func (m *pipeBuffer) WriteTo(w io.Writer) (int64, error) {
// 	log.Print("writeto")
// 	if m.buf[m.rd].Len() == 0 {
// 		select {
// 		case <-m.done:
// 			return 0, ErrBufferClosed
// 		case <-m.swap:
// 			m.mtx.Lock()
// 			m.rd, m.wr = m.wr, m.rd
// 			m.mtx.Unlock()
// 		}
// 	}
// 	return m.buf[m.rd].WriteTo(w)
// }

// func (m *pipeBuffer) readFromAll(p []byte) (n int) {
// 	m.mtx.Lock()
// 	defer m.mtx.Unlock()
// 	var lp = p[:]

// 	for i := 0; i < 2; i++ {
// 		written, _ := m.buf[m.rd].Read(lp)
// 		n += written
// 		lp = p[n:]
// 		if m.buf[m.rd].Len() != 0 {
// 			return
// 		}
// 		m.rd, m.wr = m.wr, m.rd
// 	}
// 	return
// }

// func (m *pipeBuffer) Flush(w io.Writer) (int64, error) {
// 	m.mtx.Lock()
// 	defer m.mtx.Unlock()
// 	var n1, n2 int64
// 	if n1, err := m.buf[m.wr].WriteTo(w); err != nil {
// 		return n1, err
// 	}
// 	if n2, err := m.buf[m.rd].WriteTo(w); err != nil {
// 		return n1 + n2, err
// 	}
// 	return n1 + n2, nil
// }

// ErrBufferEmpty means buffer A and B are both empty
// var ErrBufferEmpty = errors.New("Buffer Empty")
