package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
)

type ctxKey string

var (
	listen     = kingpin.Flag("listen", "Listen port.").Short('l').Default("2003").String()
	out1       = kingpin.Flag("outport1", "Output port 1.").Short('1').Default("4001").String()
	out2       = kingpin.Flag("outport2", "Output port 2.").Short('2').Default("4002").String()
	debug      = kingpin.Flag("debug", "Debug.").Short('d').Default("false").Bool()
	ctxKeyAddr = ctxKey("address")
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
func dialTarget(ctx context.Context, cc chan net.Conn) {
	log.Print(">>> dialTarget ")
	defer log.Print("dialTarget --->")

	addr, _ := ctx.Value(ctxKeyAddr).(string)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Infinite loop until new connection is generated and sent
	// via channel. The loop can be cancelled by ctx.Done()
	for func() bool {
		var err error
		var conn net.Conn
		if conn, err = net.Dial("tcp", addr); err == nil {
			// send connection and stop loop
			log.Printf("dial %s: ok", addr)
			cc <- conn
			return false
		}
		// continue to next iteration
		// log.Print(err)
		return true
	}() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// copy from pipe to remote. Pipereader reads data from pipe. Remote
// connection is retrieved from channel cc. When no connection is available,
// data read from pipe is ignored. Copy finishes when pipe reader returns
// error, including EOF error (pipe writer is closed without error).
func copyToRemote(pr *io.PipeReader, cc <-chan net.Conn) {
	log.Print(">>> copyToRemote")
	defer log.Print("copyToRemote --->")
	var conn net.Conn
	b := make([]byte, 32*1024)
	pb := newPipeBuffer()
	for {
		if n, err := pr.Read(b); err != nil {
			break
		} else {
			select {
			case conn = <-cc:
				go pb.FlushTo(conn) // copy data to remote
				defer conn.Close()
			default:
			}
			if conn != nil {
				if _, err := pb.Write(b[:n]); err != nil {
					log.Print(err)
					pr.CloseWithError(err)
					pb.CloseWrite() // close buffer from write side; data flushed until empty
					return
				}
			}
		}
	}
}

func handleConnection(conn net.Conn, addr1, addr2 string) {
	log.Print(">>> handleConnection")
	defer log.Print("handleConnection --->")

	var pr1, pr2 *io.PipeReader
	var pw1, pw2 *io.PipeWriter
	cc1 := make(chan net.Conn)
	cc2 := make(chan net.Conn)

	//context for dialTarget
	ctx1 := context.WithValue(context.Background(), ctxKeyAddr, addr1)
	ctxDial1, cancelDial1 := context.WithCancel(ctx1) // cancle dialing
	ctx2 := context.WithValue(context.Background(), ctxKeyAddr, addr2)
	ctxDial2, cancelDial2 := context.WithCancel(ctx2) // cancle dialing

	defer cancelDial1()
	defer cancelDial2()
	defer conn.Close()

	pr1, pw1 = io.Pipe()
	go dialTarget(ctxDial1, cc1)
	go copyToRemote(pr1, cc1)
	pr2, pw2 = io.Pipe()
	go dialTarget(ctxDial2, cc2)
	go copyToRemote(pr2, cc2)

	s := bufio.NewScanner(conn)
	for s.Scan() {
		b := append(s.Bytes(), '\n')

		// therefor no return error from writing to pipe.
		// Pipe is closed only when inbound connection is closed
		// when pipe is closed from reader side, create a new pipe
		if _, err := pw1.Write(b); err != nil {
			pr1, pw1 = io.Pipe()
			go dialTarget(ctxDial1, cc1)
			go copyToRemote(pr1, cc1)
			pw1.Write(b)
		}

		if _, err := pw2.Write(b); err != nil {
			pr2, pw2 = io.Pipe()
			go dialTarget(ctxDial2, cc2)
			go copyToRemote(pr2, cc2)
			pw2.Write(b)
		}
	}

	// close pipe; EOF
	pw1.Close()
	pw2.Close()
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
