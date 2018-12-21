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

// func checkEOF(e error, msgs ...string) bool {
// 	if e == io.EOF {
// 		return true
// 	}
// 	return checkError(e, msgs...)
// }

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

func connectionGenerator(ctx context.Context, cc chan net.Conn, addr string) {
	log.Print("---> newConnection ")
	defer log.Print("newConnection --->")

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// run loop until new connection generated and sent to channel
	// loop is blocked if the generated connection is note consumed by channel cc
	// loop ends when ctx.Done() receives cancel signal
	for func() bool {
		conn, err := dialTarget(addr)
		if err == nil {
			cc <- conn
		}
		return err != nil
		// if conn, err := dialTarget(addr); err == nil {
		// 	cc <- conn
		// 	return false
		// }
		// return true
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

	go connectionGenerator(ctx, cc, addr)

	go func() {
		b := make([]byte, 32*1024)

		for {
			select {
			case <-ctx.Done():
				return
			case conn := <-cc:
				// When copyWithBuffer returns error, it most probably due to bad connection,
				// although it may also be corrupted buffer. In either case, retry the copy.
				// When pipe is closed without error, copyWithBuffer returns no error.
				// If copy returns no error, finish
				// If copy returns error, go to next loop. The case conn := <-cc, tries to
				// extract next available connection
				if err := copyWithBuffer(conn, pr); err != nil {
					conn.Close()
					go connectionGenerator(ctx, cc, addr)
				} else {
					conn.Close()
					return
				}
			default:
				// if no connection is ready, consumes pipe into garbag
				// if pipe is closed from writer side, quit
				_, err := pr.Read(b)
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

	s := bufio.NewScanner(conn)
	for s.Scan() {
		b := s.Bytes()

		// pipe is only closed from writer side, never from reader side
		// so no error will be returned here
		// no need to renew pipes here

		// pw1.Write(b)
		pw2.Write(b)
	}

	cancelPipes()

	if pw2 != nil {
		pw2.Close()
	}
}

func handlePipe1(addr string, pwc chan *io.PipeWriter) {
	log.Print("handlePipe1")
	buf := make([]byte, 32*1024)
	pr, pw := io.Pipe()
	pwc <- pw

	for {
		if n, err := pr.Read(buf); err == nil {
			log.Printf("hadlePipe1: read from %s [%2d] %s", addr, n, string(buf[:n]))
		} else {
			log.Print("hadlePipe1 done")
			pw.Close()
			return
		}
	}
}

func handlePipe2(addr string, pwc chan *io.PipeWriter) {
	log.Print("handlePipe2")

	for {
		conn, err := dialTarget(addr)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		pr, pw := io.Pipe()
		pwc <- pw

		if func() {
			err = copyWithBuffer(conn, pr)
			pw.Close()
			conn.Close()
		}(); err == nil {
			log.Print("handlePipe2 done")
			return
		}
	}
}

// Dial to target and return conn
// When error encountered, return nil conn
func dialTarget(addr string) (conn net.Conn, err error) {
	if conn, err = net.Dial("tcp", addr); err != nil {
		log.Print(err)
	} else {
		log.Printf("dial %s: ok", addr)
	}
	return
}

func copyWithBuffer(conn net.Conn, r io.Reader) (err error) {
	log := func(c interface{}) { log.Printf("copyWithBuffer: %s", c) }
	buf := newPipeBuffer()

	log("...")
	if conn == nil {
		log("no connection")
		return errors.New("no connection")
	}

	// inbound copy
	go func() {
		if _, err := io.Copy(buf, r); err == nil {
			log(err)
			buf.Stop()
		} else {
			log(err)
			buf.Close()
		}
	}()

	// outbound copy
	_, err = io.Copy(conn, buf)
	log(err)
	log("///")

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
