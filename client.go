package main

import (
	"bufio"
	"log"
	"net"
	"time"
)

type client struct {
	addr     string
	protocal string
	data     chan []byte
	done     chan bool
	conn     net.Conn
	bw       *bufio.Writer
}

// initialize new client and its channel
func newClient(addr string, protocal string) *client {
	c := client{addr: addr, protocal: protocal}
	c.data = make(chan []byte)
	c.done = make(chan bool)
	return &c
}

func (c *client) newConnection() net.Conn {
	conn, err := net.Dial(c.protocal, c.addr)
	if err != nil {
		log.Print(err)
		return nil
	}
	if c.bw == nil {
		c.bw = bufio.NewWriterSize(conn, 4*1024)
	} else {
		c.bw.Reset(conn)
	}
	return conn
}

func (c *client) run(debug bool) {
	var conn net.Conn

	defer func() {
		log.Println("Close connection")
		if conn != nil {
			c.bw.Flush()
			conn.Close()
		}
	}()

	// Try to re-open connection every 5 minutes
	tick := time.NewTicker(time.Minute).C

	conn = c.newConnection()
	for {
		select {
		case data := <-c.data:
			if debug {
				log.Printf("[Debug][%s] Received %d bytes data", c.addr, len(data))
			}

			// Send data to target client, ignore them if the connection is not ready
			if conn != nil && c.bw != nil {
				_, err := c.bw.Write(append(data, '\n'))
				if err != nil {
					log.Printf("[Warning][%s] %s", c.addr, err)
				}
				if debug {
					log.Printf("[Debug][%s] Buffered: %d bytes", c.addr, c.bw.Buffered())
				}
			}
		case <-tick:
			// Reopen connection if it is closed
			if conn == nil {
				log.Printf("[%s] Try to reopen", c.addr)
				conn = c.newConnection()
			}
		}
	}
}
