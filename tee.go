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
		go writeFromPipeToRemote(r2, *out2)
	}
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

func dialTarget(addr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// log.Printf("dial %s: connected", addr)
func writeFromPipeToRemote(src io.Reader, addr string) {
	buf := newQueueBuffer()

	// Copy from buffer to dst
	// Wait for buffer's close signal to quit
	// io.Copy calls buf.Read() in for loop, until buf.Read() returns io.EOF error
	// rBuffer returns io.EOF only when the buffer is closed, and no data to be copied
	// rBuffer.Read() will read the remaining data, even if it is closed. But rBuffer.Write()
	// will return ErrBufferBlocked error
	go func() {
		var conn net.Conn
		defer func() {
			if conn != nil {
				conn.Close()
			}
		}()

		for {
			var err error
			if conn, err = dialTarget(addr); err != nil {
				log.Print(err)
				time.Sleep(3 * time.Second)
				continue
			} else {
				log.Printf("%s connected", addr)
				buf.UnBlock()
			}

			// if err is nil, copy is done, quit
			// if err is not nil, something wrong with conn, block buffer, retry connection
			if _, err = io.Copy(conn, buf); err != nil {
				log.Print("conn<-buffer: ", err)
				buf.Block()
				continue
			} else {
				break
			}
		}

		log.Print("Write done")
	}()

	// Copy from src to buffer
	for {
		_, err := io.Copy(buf, src)
		if err == nil {
			break
		} else if err == ErrBufferBlocked {
			continue
		} else {
			log.Fatal(err)
		}
	}
	buf.Close()
	log.Print("bye")
}
