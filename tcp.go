package main

import (
	"io"
	"log"
	"net"
)

var localServerHost = "localhost:8880"
var remoteServerHost = "localhost:8881"

func main() {

	ln, err := net.Listen("tcp", localServerHost)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Port forwarding server up and listening on ", localServerHost)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go handleConnection(conn)
	}
}

func forward(src, dest net.Conn) {
	defer src.Close()
	defer dest.Close()
	io.Copy(src, dest)
}

func handleConnection(c net.Conn) {

	log.Println("Connection from : ", c.RemoteAddr())

	remote, err := net.Dial("tcp", remoteServerHost)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to ", remoteServerHost)

	// go routines to initiate bi-directional communication for local server with a
	// remote server
	go forward(c, remote)
	go forward(remote, c)
}
