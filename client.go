package main

import (
	"bytes"
	"log"
	"net"
)

type client struct {
	addr     string
	protocal string
	ch       chan []byte
}

// initialize new client and its channel
func newClient(addr string, protocal string) *client {
	c := client{addr: addr, protocal: protocal}
	c.ch = make(chan []byte)
	return &c
}

func (c *client) write(b *bytes.Buffer) {
	conn, err := net.Dial(c.protocal, c.addr)
	if err != nil {
		log.Print("Failed to open connection to %s: %s", c.addr, err)
		return
	}
	defer conn.Close()

	n, err := conn.Write([]byte("data"))
	// w := bufio.NewWriter(conn)
	// n, err := b.WriteTo(w)
	if err != nil {
		log.Print(err)
	} else {
		log.Print(n)
	}
}

func (c *client) run() {
	batchSize := 20
	buf := bytes.NewBuffer(nil)

	for {
		data := <-c.ch

		if buf.Len()+len(data) <= batchSize {
			// write data into buffer
			buf.Write(data)
			// log.Printf("Length of the buffer: %d", buf.Len())
		} else {
			// send data in buffer to outbound port
			log.Print(buf)
			c.write(buf)

			// clear buf
			buf.Reset()
		}
	}
}
