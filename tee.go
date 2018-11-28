package main

import (
	"log"
	"net"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listen = kingpin.Flag("listen", "Listen port.").Short('l').Required().String()
	out1   = kingpin.Flag("outport1", "Output port a.").Short('1').Required().String()
	out2   = kingpin.Flag("outport2", "Output port b.").Short('2').Required().String()
	debug  = kingpin.Flag("debug", "Debug.").Default("false").Bool()
)

func main() {
	kingpin.Parse()

	listener, err := net.Listen("tcp", "localhost:"+*listen)

	if err != nil {
		log.Fatalf("Failed to listen to %s: %s", *listen, err)
	} else {
		log.Printf("Listening on port %s, forwarding to port %s and %s", *listen, *out1, *out2)
		defer listener.Close()
	}

	var clients []*client

	clients = append(clients, newClient("localhost:"+*out1, "tcp"))
	clients = append(clients, newClient("localhost:"+*out2, "tcp"))

	for i := range clients {
		go clients[i].run() // reference client by array
	}

	for {
		// wait for connection
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("ERROR: failed to accept listener: %s", err)
		}

		buf := make([]byte, 2048)

		// send data from inbound connection to outbound clients
		go func(conn net.Conn) {
			defer conn.Close()
			// buf = buf[:0]

			n, err := conn.Read(buf)

			if err != nil {
				log.Fatalf("Receive data failed: %s", err)
			} else {
				log.Printf("Recieve %d bytes data", n)
				for _, c := range clients {
					c.ch <- buf[:n]
				}
			}

			if *debug {
				log.Print(buf[:n])
			}
		}(conn)
	}
}
