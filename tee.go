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
		go writeFromPipeToRemote9(r1, *out1)
		go writeFromPipeToRemote9(r2, *out2)
	}
}

func handleConnection(conn net.Conn, w1, w2 *io.PipeWriter) {
	log.Print("Handle connection")
	defer conn.Close()
	defer w1.CloseWithError(errors.New("done"))
	defer w2.Close()
	// gracefully send EOF to the pipes

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

func customWrite3(c net.Conn, p []byte) (int, error) {
	if len(p) > 0 {
		return c.Write(p)
	}
	return 0, nil
}

// func customWrite3(c net.Conn, p []byte) (int, error) {
// 	if len(p) > 0 {
// 		time.Sleep(1500 * time.Millisecond)
// 		n, err := c.Write(p)
// 		if err != nil {
// 			log.Print(err)
// 			return 0, err
// 		}
// 		log.Printf("read from %s [%2d] %s", "c2", len(p), string(p))
// 		return n, err
// 	}
// 	return 0, nil
// }

func dialTarget(addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Print(err)
		return nil
	}
	log.Printf("dial %s: connected", addr)
	return conn
}

func writeFromPipeToRemote9(r *io.PipeReader, addr string) {
	buf := newMyBuffer()
	closeChan := make(chan struct{}, 1)

	conn := dialTarget(addr)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	ticker := time.NewTicker(10 * time.Second).C

	// read from pipe and save data to buffer
	// in case of error, inclduing EOF error, stop reading
	// send close signal to connection manager
	go func() {
		rb := bufio.NewReader(r)
		for {
			b, err := rb.ReadBytes('\n')
			if err != nil {
				buf.Close()
				break
			}
			if conn != nil {
				buf.Write(b)
			}
		}
		closeChan <- struct{}{}
	}()

	// Read from buffer and write to target
	// Read() is blocked until buffer is not empty or buffer Close() is called
	// reset conn to nil when write fails
	go func() {
		for {
			if b, err := buf.Read(); err != nil {
				break
			} else {
				n, err := customWrite3(conn, b)
				if err != nil {
					log.Print(err)
					conn = nil
				} else {
					log.Printf("%d bytes written to %s", n, addr)
				}
			}
		}
		log.Print("exiting from write")
	}()

L:
	for {
		select {
		case <-ticker:
			if conn == nil {
				conn = dialTarget(addr)
			}
		case <-closeChan:
			break L
		}
	}

	// flush data
	log.Print("bye")
}

func writeFromPipeToRemote7(r *io.PipeReader, addr string) {
	buf := newMyBuffer()
	closeChan := make(chan struct{}, 1)

	conn := dialTarget(addr)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	ticker := time.NewTicker(10 * time.Second).C

	// read from pipe and save data to buffer
	// in case of error, stop reading
	go func() {
		rb := bufio.NewReader(r)
		for {
			b, err := rb.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					if conn != nil {
						buf.Write(b)
					}
					buf.Close()
				}
				closeChan <- struct{}{}
				return
			}
			if conn != nil {
				buf.Write(b)
			}
		}
	}()

	// write to target
	go func() {
		for {
			// block until buffer is not empty
			b, err := buf.Read()
			if err != nil {
				if err == io.EOF {
					if len(b) > 0 {
						_, err = customWrite3(conn, b)
						if err != nil {
							conn = nil
						}
					}
				}
				log.Print("exiting from write")
				return
			}
			_, err = customWrite3(conn, b)
			if err != nil {
				conn = nil
			}
		}
	}()

	for {
		select {
		case <-ticker:
			if conn == nil {
				conn = dialTarget(addr)
			}
		case <-closeChan:
			log.Print("bye")
			return
		}
	}

	// flush data
}

/*
func writeFromPipeToRemote8(r *io.PipeReader, addr string) {
	rb := bufio.NewReader(r)
	buf := newMyBuffer()

	conn := dialTarget(addr)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	// read from pipe and save data to buffer
	// in case of error, stop reading
	// go func() {
	// 	rb := bufio.NewReader(r)
	// 	for {
	// 		b, err := rb.ReadBytes('\n')
	// 		if err != nil {
	// 			if err == io.EOF {
	// 				if conn != nil {
	// 					buf.Write(b)
	// 				}
	// 				buf.Close()
	// 			}
	// 			closeChan <- struct{}{}
	// 			return
	// 		}
	// 		if conn != nil {
	// 			buf.Write(b)
	// 		}
	// 	}
	// }()

	// write to target
	go func() {
		for {
			// block until buffer is not empty
			b, err := buf.Read()
			if err != nil {
				break
			}
			_, err = customWrite3(conn, b)
			if err != nil {
				conn = nil
			}
		}
	}()

	ticker := time.NewTicker(10 * time.Second).C
	go func() {
		select {
		case <-ticker:
			if conn == nil {
				conn = dialTarget(addr)
			}
		}
	}()

	for {
		if b, err := rb.ReadBytes('\n'); err != nil {
			buf.Close()
			break
		} else {
			if conn != nil {
				buf.Write(b)
			}
		}
	}

	log.Print("bye")
	return
	// flush data
}
*/

// func customWrite2(p []byte, conn *net.Conn, addr string) (int, error) {
// 	log.Print("start")
// 	var c *net.Conn
// 	var err error
// 	if *conn == nil {
// 		*c, err = net.Dial("tcp", addr)
// 		if err != nil {
// 			return 0, err
// 		}
// 		conn = c
// 	}
// 	n, err := (*c).Write(p)
// 	if err != nil {
// 		return 0, err
// 	}
// 	return n, nil
// }

// func customWrite(w *bufio.Writer, p []byte) {
// 	// n, err := w.Write(p)
// 	// checkError(err)
// 	log.Print("customWrite", w)
// 	if w != nil {
// 		time.Sleep(1500 * time.Millisecond)
// 		log.Printf("read from %s [%2d] %s", "c2", len(p), string(p))
// 	}
// }

// func writeFromPipeToRemote6(r *io.PipeReader, target string) {
// 	rb := bufio.NewReader(r)
// 	buf := newMyBuffer()
// 	op := make(chan bool)
// 	cl := make(chan bool)
// 	done := make(chan bool)

// 	// ticker := time.NewTicker(time.Minute).C
// 	ticker := time.NewTicker(10 * time.Second).C

// 	var conn net.Conn
// 	var wr *bufio.Writer

// 	dialTarget := func(t string) (*bufio.Writer, net.Conn) {
// 		c, err := net.Dial("tcp", t)
// 		if err != nil {
// 			log.Print(err)
// 			return nil, nil
// 		}
// 		w := bufio.NewWriter(c)
// 		log.Printf("Connection to %s established", t)
// 		return w, c
// 	}

// 	// open connection
// 	go func() {
// 		for {
// 			select {
// 			case <-ticker:
// 				if wr == nil {
// 					wr, conn = dialTarget(target)
// 					// log.Print("ticker", wr)
// 				}
// 			case <-op:
// 				if wr == nil {
// 					wr, conn = dialTarget(target)
// 					// log.Print("open", wr)
// 				}
// 			case <-cl:
// 				if conn != nil {
// 					conn.Close()
// 				}
// 				done <- true
// 			}
// 		}
// 	}()

// 	// write to target
// 	go func() {
// 		for {
// 			// block until buffer is not empty
// 			b, err := buf.Read()
// 			if checkEOF(err) {
// 				break
// 			}
// 			customWrite(wr, b)
// 			// customWrite(b, tgtConn, target)
// 		}
// 	}()

// 	op <- true

// 	for {
// 		b, err := rb.ReadBytes('\n')
// 		if checkEOF(err, "Read from pipe") {
// 			b2, _ := buf.Flush()
// 			if len(b2)+len(b) > 0 {
// 				customWrite(wr, append(b2, b...))
// 			}
// 			break
// 		}
// 		if conn != nil && wr != nil {
// 			_, err = buf.Write(b)
// 			checkError(err, "Write to myBuffer")
// 		}
// 	}

// 	cl <- true
// 	<-done

// 	log.Print("bye")
// }

// func writeFromPipeToRemote5(r *io.PipeReader, c string) {
// 	rb := bufio.NewReader(r)
// 	wr := testWriter{c}
// 	buf := newMyBuffer()
// 	close := make(chan bool)

// 	go func() {
// 		for {
// 			var b []byte
// 			var err error
// 			select {
// 			case <-close:
// 				// flush buffer
// 				b, err = buf.Flush()
// 				checkError(err)
// 				break
// 			default:
// 				b, err = buf.Read()
// 				checkError(err)
// 			}
// 			wr.Write(b)
// 		}
// 	}()

// 	for {
// 		b, err := rb.ReadBytes('\n')
// 		if checkEOF(err, "Read from pipe") {
// 			close <- true
// 			break
// 		}
// 		_, err = buf.Write(b)
// 		checkError(err, "Write to myBuffer")
// 	}
// }

// // read from pipereader, and save in ring buffer
// func writeFromPipeToRemote4(r *io.PipeReader, c string) {
// 	rb := bufio.NewReader(r)
// 	ringBuffer := make(chan []byte, 20)
// 	wr := testWriter{c}

// 	go func() {
// 		for b := range ringBuffer {
// 			wr.Write(b)
// 		}
// 	}()

// 	for {
// 		b, err := rb.ReadBytes('\n')
// 		if err != nil {
// 			close(ringBuffer)
// 			break
// 		}
// 		ringBuffer <- b
// 	}
// }

// func writeFromPipeToRemote3(r *io.PipeReader, c string) {
// 	wr := testWriter{c}
// 	wrDone := make(chan bool)
// 	buf := new(bytes.Buffer)
// 	ch := make(chan int, 400)

// 	go func() {
// 		b := make([]byte, 256)
// 		for {
// 			n := <-ch
// 			if n == -1 {
// 				break
// 			}
// 			// b, err := buf.ReadBytes('\n')
// 			n, err := buf.Read(b)
// 			// _, err := buf.WriteTo(&wr)
// 			if err != nil && err != io.EOF {
// 				break
// 			}
// 			log.Print("write ", n, err)
// 			wr.Write(b[:n])
// 		}
// 		wrDone <- true
// 	}()

// 	// read from pipe
// 	b := make([]byte, 256)
// 	for {
// 		n, err := r.Read(b)
// 		if err != nil {
// 			log.Print("read done")
// 			ch <- -1
// 			break
// 		}
// 		buf.Write(b[:n])
// 		ch <- n
// 		log.Print(buf.Len())
// 	}

// 	<-wrDone
// }

// func writeFromPipeToRemote2(r *io.PipeReader, c string) {
// 	// b := make([]byte, 256)
// 	// r.Read(b)
// 	rb := bufio.NewReader(r)
// 	w := testWriter{"c2"}
// 	n, err := rb.WriteTo(&w)
// 	log.Print(n)
// 	if err != nil {
// 		log.Print("buffer", err)
// 		return
// 	}
// }

// type testWriter struct {
// 	name string
// }

// func (w *testWriter) Write(p []byte) (int, error) {
// 	if len(p) == 0 {
// 		return 0, nil
// 	}
// 	time.Sleep(1000 * time.Millisecond)
// 	log.Printf("read from %s [%2d] %s", w.name, len(p), string(p))
// 	return len(p), nil
// }
