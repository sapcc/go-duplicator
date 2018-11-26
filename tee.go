package main

import (
	"errors"
	"fmt"
	"log"
	"net"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listen = kingpin.Flag("listen", "Listen port.").Short('l').Required().String()
	tcp    = kingpin.Flag("tcp", "Output tcp port.").Short('t').String()
	udp    = kingpin.Flag("upd", "Output upd port.").Short('u').String()
)

func parseFlags() error {
	kingpin.Parse()

	// Must specify output port
	if *tcp == "" && *udp == "" {
		return errors.New("Output port ?")
	}

	if *tcp != "" {
		fmt.Println(fmt.Sprintf("Forwarding to tcp port %s", *tcp))
	}
	if *udp != "" {
		fmt.Println(fmt.Sprintf("Forwarding to udp port %s", *udp))
	}

	return nil
}

func main() {
	err := parseFlags()
	if err != nil {
		fmt.Println(err)
	}

	listener, err := net.Listen("tcp", "localhost:"+*listen)

	if err != nil {
		log.Fatalf("Failed to listen to %s: %s", *listen, err)
	} else {
		log.Printf("Listening on port %s", *listen)
		defer listener.Close()
	}

	var clients []*client

	clients = append(clients, newClient("localhost:4000", "tcp"))
	clients = append(clients, newClient("localhost:4001", "tcp"))

	for i := range clients {
		go clients[i].run() // reference client by array
	}

	for {
		// wait for connection
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("ERROR: failed to accept listener: %s", err)
		}

		buf := make([]byte, 1024)

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
		}(conn)
	}

}

// func handler(conn net.Conn, clients []client) {
// }

// func handler(conn net.Conn, clients []client) {
// 	defer conn.Close()

// 	buf := make([]byte, 1024)
// 	n, err := conn.Read(buf)

// 	if err != nil {
// 		log.Fatalf("Receive data failed: %s", err)
// 	} else {
// 		log.Printf("Recieve %d bytes data", n)
// 		clients[0].data <- buf[:n]
// 	}
// }
