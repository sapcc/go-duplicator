package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listen = kingpin.Flag("listen", "Listen port.").Short('l').Default("2003").String()
	out1   = kingpin.Flag("outport1", "Output port 1.").Short('1').Default("4001").String()
	out2   = kingpin.Flag("outport2", "Output port 2.").Short('2').Default("4002").String()
	debug  = kingpin.Flag("debug", "Debug.").Short('d').Default("false").Bool()
)

func main() {
	kingpin.Parse()

	if *debug {
		log.Print("Debug mode enabled")
	}

	// Initialize clients to output port
	if idx := strings.Index(*out1, ":"); idx <= 0 {
		*out1 = "localhost:" + (*out1)[idx+1:]
	}
	if idx := strings.Index(*out2, ":"); idx <= 0 {
		*out2 = "localhost:" + (*out2)[idx+1:]
	}

	// Start server listener
	listener, err := net.Listen("tcp", "localhost:"+*listen)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Listening on port %s, forwarding to %s and %s", *listen, *out1, *out2)
		defer listener.Close()
	}

	for {
		conn, err := listener.Accept()
		checkError(err, "Listener accept")

		// handleConnection() reads data from incoming connection
		// writeFromPipeToRemote() wirte data stream out to remote address
		// both asynchronous io read/write operation, therefore are wrapped in goroutine
		// they are interconnected with io.Pipe
		go handleConnection(conn, *out1, *out2)
	}
}

// Dial to addr. It keeps trying by a fix interval unitl
// a connection is eastablished. It can be cancel by the ctx.Done().
// To get a new connection, the function needs to be invoked again.
func dialTarget(ctx context.Context, cc chan net.Conn, addr string) {
	log.Print("---> newConnection ")
	defer log.Print("newConnection --->")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// run loop until new connection generated and sent to channel
	// loop is blocked if the generated connection is note consumed by channel cc
	// loop ends when ctx.Done() receives cancel signal
	for func() bool {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log.Print(err)
		} else {
			log.Printf("dial %s: ok", addr)
			cc <- conn
		}
		return err != nil
	}() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func newOutboundPipe(ctx context.Context, addr string) (out *io.PipeWriter) {
	log.Print("--> newOutboundPipe")
	defer log.Print("newOutboundPipe -->")
	cc := make(chan net.Conn)
	pr, pw := io.Pipe()
	out = pw

	go dialTarget(ctx, cc, addr)

	go func() {
		b := make([]byte, 32*1024)

		for {
			select {
			case <-ctx.Done():
				return
			case conn := <-cc: // extract next available connection
				log.Print("ok1")
				// flush data in temporary buffer
				if _, err := conn.Write(b); err != nil {
					conn.Close()
					go dialTarget(ctx, cc, addr)
				}
				// When inbound connection is closed, pipe writer is closed without
				// error in handleConnection(). Subsequent read from Pipe will get
				// EOF error, and copyWithBuffer() returns without error.
				// CopyWithBuffer() may return error, because of bad outbound connection.
				// It may also due to corrupted buffer. In both cases, retry the copy.
				if err := copyWithBuffer(conn, pr); err != nil {
					conn.Close()
					go dialTarget(ctx, cc, addr)
				} else {
					conn.Close()
					return
				}
			default:
				// if no connection is ready, consumes pipe into garbag
				// if pipe is closed from writer side, quit
				log.Print("ok2")
				_, err := pr.Read(b)
				log.Print(len(b), string(b))
				if err != nil {
					return
				}
			}
		}
	}()

	return
}

func handleConnection(conn net.Conn, addr1, addr2 string) {
	log.Print("--> handleConnection")
	defer log.Print("handleConnection -->")
	defer conn.Close()

	ctx, cancelPipes := context.WithCancel(context.Background())
	pw2 := newOutboundPipe(ctx, *out2)
	pw1 := newOutboundPipe(ctx, *out1)

	s := bufio.NewScanner(conn)
	for s.Scan() {
		b := s.Bytes()

		// pipe is never closed from reader side
		// therefor no return error from writing to pipe.
		// Pipe is closed only when inbound connection is closed
		pw1.Write(append(b, '\n'))
		pw2.Write(append(b, '\n'))
	}

	cancelPipes()

	if pw2 != nil {
		pw2.Close()
	}
}

// func handlePipe1(addr string, pwc chan *io.PipeWriter) {
func newOutboundPipe1(ctx context.Context, addr string) (out *io.PipeWriter) {
	log.Print("--> newOutboundPipe1")
	defer log.Print("newOutboundPipe1 -->")
	pr, pw := io.Pipe()
	out = pw

	go func() {
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if n, err := pr.Read(buf); err == nil {
				log.Printf("Pipe1: read from %s [%2d] %s", addr, n, string(buf[:n]))
			} else {
				log.Print("Pipe1 done")
				pw.Close()
				return
			}
		}
	}()
	return pw
}

func copyWithBuffer(conn net.Conn, r io.Reader) (err error) {
	log.Print("--> copyWithBuffer")
	defer log.Print("copyWithBuffer -->")
	buf := newPipeBuffer()

	if conn == nil {
		return errors.New("nil connection")
	}

	// inbound copy
	go func() {
		log.Print("copyWithBuffer > inbound copy")
		defer log.Print("copyWithBuffer > inbound copy -->")
		if _, err := io.Copy(buf, r); err == nil {
			log.Print("copyWithBuffer > inbound copy > buf.Stop")
			buf.Stop()
		} else {
			log.Print("copyWithBuffer > inbound copy > buf.Close")
			buf.Close()
		}
	}()

	// outbound copy
	_, err = io.Copy(conn, buf)

	return
}

func checkError(e error, msgs ...string) {
	if e != nil {
		if len(msgs) > 0 {
			m := strings.Join(msgs, ", ")
			log.Fatalf(m+": %v", e)
		} else {
			log.Fatal(e)
		}
	}
}
