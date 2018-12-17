package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"sync"
	"time"
)

type queueBuffer struct {
	blocked            bool
	closed             bool
	rd, wr             int8
	ctx                []bytes.Buffer
	notEmptyCh         chan struct{}
	closeCh            chan struct{}
	mtx                *sync.Mutex
	blockedBcChRunning bool
}

func newQueueBuffer() *queueBuffer {
	return &queueBuffer{
		rd:         0,
		wr:         1,
		blocked:    true,
		closed:     false,
		ctx:        make([]bytes.Buffer, 2),
		notEmptyCh: make(chan struct{}),
		closeCh:    make(chan struct{}, 1),
		mtx:        &sync.Mutex{},
	}
}

// ErrBufferBlocked (Write to blocked buffer is illegal; while read is still possible)
var ErrBufferBlocked = errors.New("Blocked Buffer")

// ErrBufferClosed (after close signal received)
var ErrBufferClosed = errors.New("Buffer Closed")

// ErrBufferEmpty means buffer A and B are both empty
var ErrBufferEmpty = errors.New("Buffer Empty")

// Unblock buffer to receive data
// what happens unblocking a closed buffer
func (m *queueBuffer) UnBlock() error {
	if m.closed {
		return ErrBufferClosed
	}
	m.blocked = false
	return nil
}

// Block buffer and refuse data
// what happens blocking a closed buffer
func (m *queueBuffer) Block() error {
	if m.closed {
		return ErrBufferClosed
	}
	m.blocked = true
	return nil
}

// Boradcast kill signal
func (m *queueBuffer) Close() {
	m.closed = true
	m.blocked = false
	m.closeCh <- struct{}{}
}

// // IsBlocked
// // Write to blocked buffer will get ErrBufferBlocked error
// func (m *queueBuffer) IsBlocked() bool {
// 	return m.blocked
// }

// Write to buffer. The return error is ErrBufferClosed when the buffer
// is closed. The return error is ErrBufferBlocked when the buffer is
// blocked.
func (m *queueBuffer) Write(p []byte) (int, error) {
	if m.blocked {
		return 0, ErrBufferBlocked
	}
	if m.closed {
		return 0, ErrBufferClosed
	}
	if len(p) == 0 {
		return 0, nil
	}

	m.mtx.Lock()
	defer m.mtx.Unlock()

	// read buffer will wait until something has been written
	if m.ctx[m.wr].Len() == 0 {
		m.notEmptyCh <- struct{}{}
	}
	return m.ctx[m.wr].Write(p)
}

func (m *queueBuffer) readFromAll(p []byte) (n int) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var lp = p[:]

	for i := 0; i < 2; i++ {
		written, _ := m.ctx[m.rd].Read(lp)
		n += written
		lp = p[n:]
		if m.ctx[m.rd].Len() != 0 {
			return
		}
		m.rd, m.wr = m.wr, m.rd
	}
	return
}

func (m *queueBuffer) empty() bool {
	return m.ctx[m.rd].Len() == 0 && m.ctx[m.wr].Len() == 0
}

func (m *queueBuffer) Read(p []byte) (n int, err error) {
	log.Print("rBuffer Read")
	// When all data in buffer is read, and buffer is in state "closed"
	// error io.EOF is returned
	if m.closed {
		log.Print("rBuffer Read: m.closed")
		if m.empty() {
			if len(p) == 0 {
				return 0, nil
			}
			log.Print("rBuffer Read: 0, io.EOF")
			return 0, io.EOF
		}
		n = m.readFromAll(p)
		return
	}

	if m.ctx[m.rd].Len() == 0 {
		select {
		case <-m.closeCh:
			log.Println("rBuffer Read: <-m.closeCh")
			m.mtx.Lock()
			m.closed = true
			m.mtx.Unlock()
			return
		case <-m.notEmptyCh:
			log.Println("rBuffer Read: <-m.notEmptyCh")
			m.mtx.Lock()
			m.rd, m.wr = m.wr, m.rd
			m.mtx.Unlock()
		}
	}

	// Never return io.EOF error unless buffer is closed and empty
	n, _ = m.ctx[m.rd].Read(p)
	time.Sleep(1000 * time.Millisecond)

	log.Print("rBuffer Read: ", n, " bytes")
	return
}

// WriteTo
// Write buffer A to Writer w, if buffer A is not emptpy
// Otherwise wait either buffer B is not empyt or kill signal
// func (m *queueBuffer) WriteTo(w io.Writer) (int64, error) {
// 	log.Print("writeto")
// 	if m.ctx[m.rd].Len() == 0 {
// 		select {
// 		case <-m.closeCh:
// 			return 0, ErrBufferClosed
// 		case <-m.notEmptyCh:
// 			m.mtx.Lock()
// 			m.rd, m.wr = m.wr, m.rd
// 			m.mtx.Unlock()
// 		}
// 	}
// 	return m.ctx[m.rd].WriteTo(w)
// }

func (m *queueBuffer) Flush(w io.Writer) (int64, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var n1, n2 int64
	if n1, err := m.ctx[m.wr].WriteTo(w); err != nil {
		return n1, err
	}
	if n2, err := m.ctx[m.rd].WriteTo(w); err != nil {
		return n1 + n2, err
	}
	return n1 + n2, nil
}
