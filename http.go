package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type handle struct {
}

func handleTunneling(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Host)
	dest_conn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	go transfer(dest_conn, client_conn)
	go transfer(client_conn, dest_conn)
}
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	io.Copy(destination, source)
}

func (h *handle) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "CONNECT" {
		handleTunneling(writer, request)
		return
	}
	log.Println(request.URL)
	u, _ := url.Parse(request.URL.String())
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ServeHTTP(writer, request)

}
func main() {
	r := handle{}
	err := http.ListenAndServe(":80", &r)
	if err != nil {
		fmt.Println("HTTP server failed,err:", err)
		return
	}
}
