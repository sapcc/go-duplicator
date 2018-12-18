package main

import (
	"bufio"
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

func checkError(e error, msgs ...string) bool {
	m := strings.Join(msgs, ", ")
	if e != nil {
		if m != "" {
			log.Fatalf(m+": %v", e)
		} else {
			log.Fatal(e)
		}
	}
	return false
}

func checkEOF(e error, msgs ...string) bool {
	if e == io.EOF {
		return true
	}
	return checkError(e, msgs...)
}

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
		r1, w1 := io.Pipe()
		r2, w2 := io.Pipe()
		go handleConnection(conn, w1, w2)
		go writeFromPipeToRemote1(r1, *out1)
		go handlePipe(r2, *out2)
	}
}

func dumpReader(r io.Reader, stop <-chan struct{}) error {
	log := func(c interface{}) { log.Printf("dumpReader: %s", c) }

	b := make([]byte, 4*1024)
	for {
		select {
		case <-stop:
			log("stop")
			return errors.New("dump forcefully stopped")
		default:
			if _, err := r.Read(b); err != nil {
				log(err)
				return nil
			}
		}
	}
}

func dialTarget2(addr string, stopme <-chan struct{}) net.Conn {
	for {
		conn, err := dialTarget(addr)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
			// stopdump <- struct{}{}
		}
		return conn
	}
}
func handlePipe(r io.Reader, addr string) {
	var conn net.Conn
	done := make(chan bool)
	stopdump := make(chan struct{})
	ticker := time.NewTicker(5 * time.Second)

	defer func() {
		close(stopdump)
		ticker.Stop()
	}()

	go func() {
		for {
			if err := dumpReader(r, stopdump); err == nil {
				break
			}
			if err := copyWithBuffer(conn, r); err != nil {
				log.Print("handlePipe ", err)
				conn.Close()
				conn = nil
			} else {
				log.Print("handlePipe ")
				conn.Close()
				break
			}
		}
		done <- true
	}()

L:
	for {
		select {
		case <-done:
			break L
		default:
			if conn == nil {
				c, err := dialTarget(addr)
				if err == nil {
					conn = c
					stopdump <- struct{}{}
				}
			}
			time.Sleep(5 * time.Second)
		}
	}

	log.Print("handlePipe done")
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

func handleConnection(conn net.Conn, w1, w2 *io.PipeWriter) {
	log.Print("Handle connection")
	defer conn.Close()
	defer w1.CloseWithError(errors.New("done"))
	defer w2.Close() // gracefully send EOF to the pipes

	u := io.MultiWriter(w1, w2)

	r := bufio.NewReader(conn)
	for {
		b, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		u.Write(b)
	}
}

func writeFromPipeToRemote1(r io.Reader, c string) {
	buf := make([]byte, 256)
	for {
		n, err := r.Read(buf)
		if err != nil {
			log.Print("byte ", err)
			break
		}
		log.Printf("read from %s [%2d] %s", c, n, string(buf[:n]))
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

// log.Printf("dial %s: connected", addr)
// func writeFromPipeToRemote(src io.Reader, addr string) {
// 	buf := newQueueBuffer()

// 	// Copy from buffer to dst
// 	// Wait for buffer's close signal to quit
// 	// io.Copy calls buf.Read() in for loop, until buf.Read() returns io.EOF error
// 	// rBuffer returns io.EOF only when the buffer is closed, and no data to be copied
// 	// rBuffer.Read() will read the remaining data, even if it is closed. But rBuffer.Write()
// 	// will return ErrBufferBlocked error
// 	go func() {
// 		var conn net.Conn
// 		defer func() {
// 			if conn != nil {
// 				conn.Close()
// 			}
// 		}()

// 		for {
// 			var err error
// 			if conn, err = dialTarget(addr); err != nil {
// 				log.Print(err)
// 				time.Sleep(3 * time.Second)
// 				continue
// 			} else {
// 				log.Printf("%s connected", addr)
// 				buf.UnBlock()
// 			}

// 			// if err is nil, copy is done, quit
// 			// if err is not nil, something wrong with conn, block buffer, retry connection
// 			if _, err = io.Copy(conn, buf); err != nil {
// 				if s, ok := err.(*net.OpError); ok {
// 					log.Print("conn<-buffer: ", s)
// 					break
// 				} else {
// 				}
// 				buf.Block()
// 				continue
// 			} else {
// 				break
// 			}
// 		}

// 		log.Print("Write done")
// 	}()

// 	// inbound copy
// 	for {
// 		_, err := io.Copy(buf, src)
// 		log.Print("inbound copy error: ", err)

// 		if err == nil {
// 			break
// 		} else if err == ErrBufferBlocked {
// 			continue
// 		} else {
// 			log.Fatal(err)
// 		}
// 	}
// 	buf.Close()
// 	log.Print("bye")
// }
