package main

import (
	"bufio"
	"bytes"
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

func checkEof(e error, msgs ...string) bool {
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
		go writeFromPipeToRemote6(r2, *out2)
	}
}

func customWrite(w *bufio.Writer, p []byte) {
	// n, err := w.Write(p)
	// checkError(err)
	if w != nil {
		time.Sleep(1500 * time.Millisecond)
		log.Printf("read from %s [%2d] %s", "c2", len(p), string(p))
	}
}

func writeFromPipeToRemote6(r *io.PipeReader, c string) {
	rb := bufio.NewReader(r)
	buf := newMyBuffer()
	close := make(chan bool)

	var conn *net.Conn
	var wr *bufio.Writer

	// open connection
	go func() {
		for {
			log.Print(wr == nil, conn == nil)
			if wr == nil {
				log.Print("Open connection")
				conn, err := net.Dial("tcp", c)
				if err != nil {
					log.Print(err)
				}
				wr = bufio.NewWriter(conn)
			}
			time.Sleep(10 * time.Second)
		}
	}()

	go func() {

		for {
			var b []byte
			var err error
			select {
			case <-close:
				// flush buffer
				b, err = buf.Flush()
				checkError(err)
				break
			default:
				b, err = buf.Read()
				checkError(err)
			}

			customWrite(wr, b)
		}
	}()

	for {
		b, err := rb.ReadBytes('\n')
		if checkEof(err, "Read from pipe") {
			close <- true
			break
		}
		if conn != nil && wr != nil {
			_, err = buf.Write(b)
			checkError(err, "Write to myBuffer")
		}
	}
}

func writeFromPipeToRemote5(r *io.PipeReader, c string) {
	rb := bufio.NewReader(r)
	wr := testWriter{c}
	buf := newMyBuffer()
	close := make(chan bool)

	go func() {
		for {
			var b []byte
			var err error
			select {
			case <-close:
				// flush buffer
				b, err = buf.Flush()
				checkError(err)
				break
			default:
				b, err = buf.Read()
				checkError(err)
			}
			wr.Write(b)
		}
	}()

	for {
		b, err := rb.ReadBytes('\n')
		if checkEof(err, "Read from pipe") {
			close <- true
			break
		}
		_, err = buf.Write(b)
		checkError(err, "Write to myBuffer")
	}
}

// read from pipereader, and save in ring buffer
func writeFromPipeToRemote4(r *io.PipeReader, c string) {
	rb := bufio.NewReader(r)
	ringBuffer := make(chan []byte, 20)
	wr := testWriter{c}

	go func() {
		for b := range ringBuffer {
			wr.Write(b)
		}
	}()

	for {
		b, err := rb.ReadBytes('\n')
		if err != nil {
			close(ringBuffer)
			break
		}
		ringBuffer <- b
	}
}

func writeFromPipeToRemote3(r *io.PipeReader, c string) {
	wr := testWriter{c}
	wrDone := make(chan bool)
	buf := new(bytes.Buffer)
	ch := make(chan int, 400)

	go func() {
		b := make([]byte, 256)
		for {
			n := <-ch
			if n == -1 {
				break
			}
			// b, err := buf.ReadBytes('\n')
			n, err := buf.Read(b)
			// _, err := buf.WriteTo(&wr)
			if err != nil && err != io.EOF {
				break
			}
			log.Print("write ", n, err)
			wr.Write(b[:n])
		}
		wrDone <- true
	}()

	// read from pipe
	b := make([]byte, 256)
	for {
		n, err := r.Read(b)
		if err != nil {
			log.Print("read done")
			ch <- -1
			break
		}
		buf.Write(b[:n])
		ch <- n
		log.Print(buf.Len())
	}

	<-wrDone
}

func writeFromPipeToRemote2(r *io.PipeReader, c string) {
	// b := make([]byte, 256)
	// r.Read(b)
	rb := bufio.NewReader(r)
	w := testWriter{"c2"}
	n, err := rb.WriteTo(&w)
	log.Print(n)
	if err != nil {
		log.Print("buffer", err)
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
	time.Sleep(1000 * time.Millisecond)
	log.Printf("read from %s [%2d] %s", w.name, len(p), string(p))
	return len(p), nil
}

func writeFromPipeToRemote(r *io.PipeReader, c string) {
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

func handleConnection(conn net.Conn, w1, w2 *io.PipeWriter) {
	log.Print("Handle connection")
	defer conn.Close()
	defer w1.CloseWithError(errors.New("done"))
	// defer w2.CloseWithError(errors.New("done"))
	defer w2.Close()

	u := io.MultiWriter(w1, w2)

	r := bufio.NewReader(conn)
	for {
		b, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		u.Write(b)
	}

	// scan input stream and write data to pipe
	// l := bufio.NewScanner(conn)
	// for ok := l.Scan(); ok; ok = l.Scan() {
	// 	buf := l.Bytes()
	// 	// u.Write(append(buf, '\n'))
	// 	u.Write(buf)
	// }

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
