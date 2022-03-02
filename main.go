package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
	"syscall"
	"unsafe"
)

// go build -ldflags "-s -w -H=windowsgui"

func IntPtr(n int) uintptr {
	return uintptr(n)
}

func StrPtr(s string) uintptr {
	return uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(s)))
}

func ShowMessage(title, text string) {
	user32dll, _ := syscall.LoadLibrary("user32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	MessageBoxW := user32.NewProc("MessageBoxW")
	MessageBoxW.Call(IntPtr(0), StrPtr(text), StrPtr(title), IntPtr(0))
	defer syscall.FreeLibrary(user32dll)
}

func main() {
	ShowMessage("报告管理系统", "报告管理系统正在运行，按确定继续。")
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	l, err := net.Listen("tcp", ":80")
	if err != nil {
		log.Panic(err)
	}
	log.Println("listening on :80")
	for {
		client, err := l.Accept()
		if err != nil {
			log.Panic(err)
		}
		log.Println("accept", client.RemoteAddr().String())
		go handleClientRequest(client)
	}
}
func handleClientRequest(conn net.Conn) {
	if conn == nil {
		return
	}
	var b [4096]byte
	n, err := conn.Read(b[:])
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}
	if n == 3 {

		fmt.Println("---------", n)
	}
	var method, host, address string
	fmt.Sscanf(string(b[:bytes.IndexByte(b[:], '\n')]), "%s%s", &method, &host)
	hostPortURL, err := url.Parse(host)
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}
	log.Printf("header: %d\n%s\n", n, strings.Trim(string(b[:n]), "\n"))

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
		conn.Close()
		return
	}
	//defer server.Close()
	if method == "CONNECT" {
		fmt.Fprint(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
	} else {
		_, err = server.Write(b[:n])
		if err != nil {
			log.Println(err)
			conn.Close()
			return
		}
	}
	go func() {
		defer server.Close()
		io.Copy(server, conn)
	}()
	defer func() {
		defer conn.Close()
		io.Copy(conn, server)
	}()
}
