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

func checkError(e error, msgs ...string) {
	var m string
	for _, s := range msgs {
		m += ", " + s
	}
	if e != nil {
		if m != "" {
			log.Fatalf(m+": %v", e)
		} else {
			log.Fatal(e)
		}
	}
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
	checkError(err, "Listen")
	defer listener.Close()
	log.Printf("Listening on port %s, forwarding to %s and %s", *listen, *out1, *out2)

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
		go writeFromPipeToRemote(r1, "c1")
		go writeFromPipeToRemote2(r2, "c2")
	}
}

func writeFromPipeToRemote2(r *io.PipeReader, c string) {
	// b := make([]byte, 256)
	// r.Read(b)
	rb := bufio.NewReader(r)
	w := testWriter{"c2"}
	n, err := rb.WriteTo(&w)
	log.Print(n)
	if err != nil {
		log.Print(err)
		return
	}
}

type testWriter struct {
	name string
}

func (w *testWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	log.Printf("read from %s [%2d] %s", w.name, len(p), string(p))
	return len(p), nil
}

func writeFromPipeToRemote(r *io.PipeReader, c string) {
	buf := make([]byte, 256)
	for {
		n, err := r.Read(buf)
		if err != nil {
			break
		}
		if c == "c1" {
			time.Sleep(3 * time.Second)
		}
		log.Printf("read from %s [%2d] %s", c, n, string(buf[:n]))
	}
}

func handleConnection(conn net.Conn, w1, w2 *io.PipeWriter) {
	log.Print("Handle connection")
	defer conn.Close()
	defer w1.CloseWithError(errors.New("done"))
	defer w2.CloseWithError(errors.New("done"))

	u := io.MultiWriter(w1, w2)

	// scan input stream and write data to pipe
	l := bufio.NewScanner(conn)
	for ok := l.Scan(); ok; ok = l.Scan() {
		buf := l.Bytes()
		u.Write(buf)
	}

	// alternative style
	// lineScanner := bufio.NewScanner(conn)
	// for {
	// 	if ok := lineScanner.Scan(); !ok {
	// 		break
	// 	}
	// 	buf := lineScanner.Bytes()
	// 	u.Write(buf)
	// }
}

func sendData(c net.Conn, r io.Reader, done chan bool) (int, error) {
	var buf []byte
	n, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	log.Print(buf)
	done <- true
	return n, nil
}
