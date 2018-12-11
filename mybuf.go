package main

import (
	"bytes"
	"io"
	"log"
	"sync"
)

type myBuf struct {
	rd, wr      int8
	ctx         []bytes.Buffer
	notEmpty    chan struct{}
	closeBuffer chan struct{}
	mtx         *sync.Mutex
}

func newMyBuffer() *myBuf {
	b := &myBuf{
		rd:          0,
		wr:          1,
		ctx:         make([]bytes.Buffer, 2),
		notEmpty:    make(chan struct{}, 1),
		closeBuffer: make(chan struct{}, 1),
		mtx:         &sync.Mutex{},
	}
	return b
}

// read
// return nil error if EOF
func (m *myBuf) Read() ([]byte, error) {

	if m.ctx[m.rd].Len() == 0 {
		select {
		case <-m.closeBuffer:
			log.Print("closeBuffer")
			return nil, io.EOF

		// switching context when reading buffer is empty
		// switching is blocked until the other buffer is written
		// switching context and writing buffer can block each other
		case <-m.notEmpty:
			m.mtx.Lock()
			log.Print("notEmpty", m.ctx[m.wr].Len())
			m.rd, m.wr = m.wr, m.rd
			m.mtx.Unlock()
		}
	}

	// read all from current reading context
	// logic that limits the reading range can be implemented here
	return m.readContex(m.rd)
	// for {
	// 	b, err := m.ctx[m.rd].ReadBytes('\n')
	// 	buf = append(buf, b...)

	// 	if err != nil {
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		return nil, err
	// 	}
	// }
	// return buf, nil
}

func (m *myBuf) Write(p []byte) (int, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if m.ctx[m.wr].Len() == 0 {
		m.notEmpty <- struct{}{}
	}
	n, err := m.ctx[m.wr].Write(p)
	return n, err
}

func (m *myBuf) readContex(id int8) ([]byte, error) {
	var buf []byte
	for {
		b, err := m.ctx[id].ReadBytes('\n')
		buf = append(buf, b...)
		if err != nil {
			if err == io.EOF {
				return buf, nil
			}
			return nil, err
		}
	}
}

// flush buffer
// return nil error if EOF
func (m *myBuf) Flush() ([]byte, error) {
	// no write anymore
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var buf []byte

	for _, i := range []int8{m.rd, m.wr} {
		if m.ctx[i].Len() != 0 {
			b, err := m.readContex(i)
			buf = append(buf, b...)
			if err != nil {
				return buf, err
			}
		}
	}

	return buf, nil
}

func (m *myBuf) Close() {
	m.closeBuffer <- struct{}{}
}
