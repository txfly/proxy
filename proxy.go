package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
)

var cliPort = flag.Int("port", 8080, "listen port")

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	server, err := net.Listen("tcp", fmt.Sprintf(":%d", *cliPort))
	if err != nil {
		fmt.Printf("Listen failed: %v\n", err)
		return
	}
	defer server.Close()
	fmt.Println("Listen on:", server.Addr().String())
	for {
		conn, err := server.Accept()
		if err != nil {
			fmt.Printf("Accept failed: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	var buf [4096]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		conn.Close()
		return
	}
	if buf[0] == 0x05 {
		handleSocks5(conn)
	} else {
		handleHttps(conn, buf[:n])
	}

}

func handleHttps(conn net.Conn, buf []byte) {
	defer func() {
		conn.Close()
		log.Println("[HTTPS]", conn.RemoteAddr().String(), "closed")
	}()
	var method, host, address string
	_, err := fmt.Sscanf(string(buf[:bytes.IndexByte(buf, '\n')]), "%s%s", &method, &host)
	if err != nil {
		log.Println(err)
		return
	}
	hostPortURL, err := url.Parse(host)
	if err != nil {
		log.Println(err)
		return
	}
	//log.Printf("------------- header -------------\n%s\n", strings.Trim(string(buf), "\r\n"))
	log.Println(method, hostPortURL.String())
	if hostPortURL.Opaque == "443" { //https访问
		address = hostPortURL.Scheme + ":443"
	} else { //http访问
		if strings.Index(hostPortURL.Host, ":") == -1 { //host不带端口， 默认80
			address = hostPortURL.Host + ":80"
		} else {
			address = hostPortURL.Host
		}
	}
	//获得了请求的host和port，就开始拨号吧
	server, err := net.Dial("tcp", address)
	if err != nil {
		log.Println(err)
		return
	}
	defer server.Close()
	if method == "CONNECT" {
		fmt.Fprint(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
	} else {
		_, err = server.Write(buf)
		if err != nil {
			log.Println(err)
			return
		}
	}
	go func() {
		written, err := io.Copy(conn, server)
		if err == nil && written > 0 {
			fmt.Printf("[HTTPS] %s => %.4fK\n", address, float32(written)/1024.0)
		}
	}()
	written, err := io.Copy(server, conn)
	if err == nil && written > 0 {
		fmt.Printf("[HTTPS] %s => %.4fK\n", conn.RemoteAddr().String(), float32(written)/1024.0)
	}
}

// handleSocks5 socks5转发， conn 远程客户端
func handleSocks5(conn net.Conn) {
	defer func() {
		conn.Close()
		log.Println("[SOCKS5]", conn.RemoteAddr().String(), "closed")
	}()
	conn.Write([]byte{0x05, 0x00})
	var buf [4096]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		log.Println(err)
		return
	}
	var host []byte
	var port int
	switch buf[1] {
	case 0x01:
		//cmd = "tcp"
	case 0x03:
		//cmd = "udp"
	default:
		fmt.Println("connect cmd", buf[1])
	}
	switch buf[3] {
	case 0x01: //IP V4
		host = buf[4 : 4+net.IPv4len]
	case 0x03: //域名
		host = buf[5 : n-2] //buf[4]表示域名的长度
		ipAddr, err := net.ResolveIPAddr("ip", string(host))
		if err != nil {
			return
		}
		host = ipAddr.IP
	case 0x04: //IP V6
		host = buf[4 : 4+net.IPv6len]
	}
	port = int(buf[n-2])<<8 | int(buf[n-1])
	addr := net.TCPAddr{IP: host, Port: port}
	//fmt.Printf("%s %s://%s\n", cmd, addr.Network(), addr.String())
	server, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		conn.Write([]byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // 失败
		log.Println(err)
		return
	}
	defer server.Close()
	log.Printf("%s <=> %s\n", conn.RemoteAddr().String(), server.RemoteAddr().String())
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //响应客户端连接成功
	//进行转发
	go func() {
		written, err := io.Copy(conn, server)
		if err == nil && written > 0 {
			fmt.Printf("[SOCKS5] %s => %.4fK\n", server.RemoteAddr().String(), float32(written)/1024.0)
		}
	}()
	written, err := io.Copy(server, conn)
	if err == nil && written > 0 {
		fmt.Printf("[SOCKS5] %s => %.4fK\n", conn.RemoteAddr().String(), float32(written)/1024.0)
	}
}
