package main

import (
	"io"
	"log"
	"net"
)

func send(conn net.Conn, clt client) {
	client, err := net.Dial(clt.protocal, clt.addr)
	if err != nil {
		log.Fatalf("Failed to dial client %s", err)
	}
	go func() {
		defer client.Close()
		defer conn.Close()
		io.Copy(client, conn)
	}()
	go func() {
		defer client.Close()
		defer conn.Close()
		io.Copy(conn, client)
	}()
}
