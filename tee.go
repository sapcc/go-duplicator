package main

import (
	"bufio"
	"log"
	"net"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listen = kingpin.Flag("listen", "Listen port.").Short('l').Required().String()
	out1   = kingpin.Flag("outport1", "Output port 1.").Short('1').Required().String()
	out2   = kingpin.Flag("outport2", "Output port 2.").Short('2').Required().String()
	debug  = kingpin.Flag("debug", "Debug.").Short('d').Default("false").Bool()
)

func main() {
	kingpin.Parse()

	if *debug {
		log.Print("Debug mode enabled")
	}

	// Listen on incoming port
	listener, err := net.Listen("tcp", "localhost:"+*listen)
	if err != nil {
		log.Fatalf("Failed to listen to %s: %s", *listen, err)
	} else {
		log.Printf("Listening on port %s, forwarding to port %s and %s", *listen, *out1, *out2)
	}
	defer listener.Close()

	// Initialize clients to output port
	if !(strings.Contains(*out1, ":")) {
		log.Print(*out1)
		*out1 = "localhost:" + *out1
	}
	if !(strings.Contains(*out2, ":")) {
		log.Print(*out2)
		*out2 = "localhost:" + *out2
	}
	clients := [2]*client{
		newClient(*out1, "tcp"),
		newClient(*out2, "tcp"),
	}
	for i := range clients {
		go clients[i].run(*debug) // reference client by array index
	}

	for {
		// wait for connection
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("ERROR: failed to accept listener: %s", err)
		}

		// send data from inbound connection to outbound clients
		go func() {
			defer conn.Close()
			lineScanner := bufio.NewScanner(conn)
			for {
				if ok := lineScanner.Scan(); !ok {
					break
				}
				for _, c := range clients {
					c.data <- lineScanner.Bytes()
				}
			}
		}()
	}
}
