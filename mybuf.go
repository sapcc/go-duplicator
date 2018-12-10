package main

import (
	"bytes"
	"io"
	"sync"
)

type myBuf struct {
	rd, wr   int8
	ctx      []bytes.Buffer
	notEmpty chan struct{}
	mtx      *sync.Mutex
}

func newMyBuffer() *myBuf {
	b := &myBuf{
		rd:       0,
		wr:       1,
		ctx:      make([]bytes.Buffer, 2),
		notEmpty: make(chan struct{}, 1),
		mtx:      &sync.Mutex{},
	}
	return b
}

// read
// return nil error if EOF
func (m *myBuf) Read() ([]byte, error) {
	var buf []byte

	// switching context when reading buffer is empty
	// switching is blocked until the other buffer is written
	// switching context and writing buffer can block each other
	if m.ctx[m.rd].Len() == 0 {
		<-m.notEmpty
		m.mtx.Lock()
		m.rd, m.wr = m.wr, m.rd
		m.mtx.Unlock()
	}

	// read all from current reading context
	// logic that limits the reading range can be implemented here
	for {
		b, err := m.ctx[m.rd].ReadBytes('\n')
		buf = append(buf, b...)

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	return buf, nil
}

func (m *myBuf) Write(p []byte) (int, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.ctx[m.wr].Len() == 0 {
		m.notEmpty <- struct{}{}
	}
	n, err := m.ctx[m.wr].Write(p)
	return n, err
}

// flush buffer
// return nil error if EOF
func (m *myBuf) Flush() ([]byte, error) {
	b1, err := m.Read()
	if err != nil {
		return b1, err
	}
	b2, err := m.Read()
	return append(b1, b2...), nil
}
