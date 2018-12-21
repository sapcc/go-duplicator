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
		// pwc1 := make(chan *io.PipeWriter)
		// pwc2 := make(chan *io.PipeWriter)
		// go handlePipe1(*out1, pwc1)
		// go handlePipe2(*out2, pwc2)
	}
}

func handlePipe(conn net.Conn) *io.PipeWriter {
	pr, pw := io.Pipe()

	go func() {
		var err error
		if func() {
			err = copyWithBuffer(conn, pr)
			pr.Close()
			conn.Close()
		}(); err == nil {
			log.Print("handlePipe done")
			return
		}
	}()

	return pw
}

func handleConnection(conn net.Conn, addr1, addr2 string) {
	var pw1, pw2 *io.PipeWriter
	cancelall := make(chan struct{})

	defer conn.Close()

	go func() {
		var c net.Conn
		var err error
		if c, err = dialTarget(addr2); err != nil {
		L:
			for {
				ticker := time.NewTicker(5 * time.Second)
				select {
				case <-cancelall:
					log.Print("ok1")
					return
				case <-ticker.C:
					log.Print("ok2")
					if c, err = dialTarget(addr2); err == nil {
						log.Print("ok3")
						break L
					}
				}
			}
		}

		select {
		case <-cancelall:
			log.Print("ok4")
		default:
			log.Print("ok5")
			pw2 = handlePipe(c)
		}
	}()

	s := bufio.NewScanner(conn)
	for s.Scan() {
		b := s.Bytes()

		if pw1 != nil {
			if _, err := pw1.Write(b); err != nil {
				pw1 = nil
			}
		}

		if pw2 != nil {
			if _, err := pw2.Write(b); err != nil {
				pw2 = nil
			}
		}
	}

	close(cancelall)

	if pw1 != nil {
		pw1.Close()
	}

	if pw2 != nil {
		pw2.Close()
	}

	// r := bufio.NewReader(conn)
	// for {
	// 	// read from inbound connection.
	// 	// break loop, when read returns error (including EOF).
	// 	b, err := r.ReadBytes('\n')
	// 	if err != nil {
	// 		break
	// 	}

	// 	select {
	// 	case pw1 = <-pwc1:
	// 	default:
	// 	}

	// 	if pw1 != nil {
	// 		if _, err := pw1.Write(b); err != nil {
	// 			pw1 = nil
	// 		}
	// 	}

	// 	select {
	// 	case pw2 = <-pwc2:
	// 	default:
	// 	}

	// 	if pw2 != nil {
	// 		if _, err := pw2.Write(b); err != nil {
	// 			pw2 = nil
	// 		}
	// 	}
	// }
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
