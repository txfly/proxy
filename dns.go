package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"golang.org/x/net/dns/dnsmessage"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type QueryQueue struct {
	queries map[string][]net.UDPAddr
	server  []net.UDPAddr
	mutex   sync.Mutex
}

func (q *QueryQueue) Init() {
	q.queries = make(map[string][]net.UDPAddr)
	q.server = append(q.server, net.UDPAddr{IP: net.IP{223, 5, 5, 5}, Port: 53})
	q.server = append(q.server, net.UDPAddr{IP: net.IP{114, 114, 114, 114}, Port: 53})
	q.server = append(q.server, net.UDPAddr{IP: net.IP{8, 8, 8, 8}, Port: 53})
	q.server = append(q.server, net.UDPAddr{IP: net.IP{119, 29, 29, 29}, Port: 53})
}

func (q *QueryQueue) Push(key string, addr net.UDPAddr) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	_, ok := q.queries[key]
	if ok {
		q.queries[key] = append(q.queries[key], addr)
	} else {
		q.queries[key] = []net.UDPAddr{addr}
	}
}

func (q *QueryQueue) Get(key string) ([]net.UDPAddr, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if item, ok := q.queries[key]; ok {
		return append([]net.UDPAddr{}, item...), true
	}
	return []net.UDPAddr{}, false
}
func (q *QueryQueue) Clear(key string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	_, ok := q.queries[key]
	if ok {
		delete(q.queries, key)
	}
}

type Answer struct {
	Resource []dnsmessage.Resource
	Time     time.Time
}

type Cache struct {
	answer   map[string]*Answer
	initSize int
	mutex    sync.Mutex
}

func (c *Cache) Init() {
	c.answer = make(map[string]*Answer)
}

func (c *Cache) Push(key string, r []dnsmessage.Resource) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	result := fmt.Sprintf("♦ %s\t%d\n", key, len(r))
	for i := 0; i < len(r); i++ {
		switch resource := r[i].Body.(type) {
		case *dnsmessage.AResource:
			result += fmt.Sprintf("  ♢ %s\t%ds\t%s\n", r[i].Header.Name.String(), r[i].Header.TTL, net.IP(resource.A[:]).String())
		case *dnsmessage.AAAAResource:
			result += fmt.Sprintf("  ♢ %s\t%ds\t%s\n", r[i].Header.Name.String(), r[i].Header.TTL, net.IP(resource.AAAA[:]).String())
		case *dnsmessage.CNAMEResource:
			result += fmt.Sprintf("  ♢ %s\t%ds\t%s\n", r[i].Header.Name.String(), r[i].Header.TTL, resource.CNAME.String())
		default:
			result += fmt.Sprintf("  ♢ %s\t%ds\t%s\n", r[i].Header.Name.String(), r[i].Header.TTL, r[i].Body.GoString())
		}
	}
	if _, ok := c.answer[key]; !ok {
		c.answer[key] = &Answer{
			Resource: r,
			Time:     time.Now(),
		}
	} else {
		c.answer[key].Resource = append(c.answer[key].Resource, r...)
	}
	result += fmt.Sprintf("cached: %d\n", len(c.answer))
	fmt.Print(result)
}
func (c *Cache) Get(key string) (Answer, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if item, ok := c.answer[key]; ok {
		return *item, true
	}
	return Answer{}, false
}
func (c *Cache) Contains(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	var ok bool
	_, ok = c.answer[key]
	return ok
}
func (c *Cache) Clear(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.answer, key)
}
func (c *Cache) Load() {
	reader, err := os.Open("dns.cache")
	if err != nil {
		fmt.Println(err)
	}
	enc := gob.NewDecoder(reader)
	err = enc.Decode(&c.answer)
	if err != nil {
		fmt.Println(err)
		return
	}
	c.initSize = len(c.answer)
	fmt.Println("load cache:", c.initSize)

}
func (c *Cache) Save() {
	if c.initSize > len(c.answer) {
		return
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(c.answer)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile("dns.cache", buf.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("saved, cache=%d, +%d\n", len(c.answer), len(c.answer)-c.initSize)
}

var query QueryQueue
var cache Cache

func init() {
	gob.Register(&dnsmessage.AResource{})
	gob.Register(&dnsmessage.NSResource{})
	gob.Register(&dnsmessage.CNAMEResource{})
	gob.Register(&dnsmessage.SOAResource{})
	gob.Register(&dnsmessage.PTRResource{})
	gob.Register(&dnsmessage.MXResource{})
	gob.Register(&dnsmessage.AAAAResource{})
	gob.Register(&dnsmessage.SRVResource{})
	gob.Register(&dnsmessage.TXTResource{})
	gob.Register(&dnsmessage.PTRResource{})
}

func main() {
	fmt.Println("按CTRL + C 退出")
	query.Init()
	cache.Init()
	cache.Load()

	exitChan := make(chan os.Signal, syscall.SIGTERM)
	defer close(exitChan)
	signal.Notify(exitChan, syscall.SIGINT, syscall.SIGTERM)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 53})
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	go func() {
		<-exitChan
		conn.Close()
		fmt.Println("CTRL + C")
	}()

	buf := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("closed.", err)
				break
			} else if err, ok := err.(*net.OpError); ok {
				log.Println(n, addr, err)
			} else {
				log.Println(err)
			}
			continue
		}
		var m dnsmessage.Message
		err = m.Unpack(buf[:n])
		if err != nil {
			log.Println(err)
			continue
		}
		if len(m.Questions) == 0 {
			continue
		}
		go ServeDNS(conn, addr, m)
	}
	cache.Save()
	fmt.Println("exit")
}

func ServeDNS(conn *net.UDPConn, addr *net.UDPAddr, m dnsmessage.Message) {
	key := fmt.Sprintf("%s%03d", m.Questions[0].Name.String(), m.Questions[0].Type)
	if m.Header.Response {
		// 发送给对应请求
		if item, ok := query.Get(key); ok {
			fmt.Println("➜", key, addr.String(), len(m.Answers)) // ❗
			for _, v := range item {
				fmt.Println("↪", m.Questions[0].Name.String(), "=>", v.String())
				if err := send(conn, &v, m); err != nil {
					log.Println(err)
				}
			}
			query.Clear(key)
			if !cache.Contains(key) {
				cache.Push(key, m.Answers)
			}
		}
		return
	}

	// 从缓存读取
	if answer, ok := cache.Get(key); ok {
		// 过期
		if time.Now().Sub(answer.Time).Seconds() > 172800 { // 2 * 24 * 3600
			fmt.Println("✘ expire 48H, remove", key)
			cache.Clear(key)
		} else {
			m.Response = true
			m.Answers = append(m.Answers, answer.Resource...)
			fmt.Println("✔", m.Questions[0].Name.String(), "=>", addr.String())
			if err := send(conn, addr, m); err != nil {
				log.Println(err)
			}
			return
		}
	}
	// 从服务器查询
	fmt.Println("❓", key, addr.String())
	query.Push(key, *addr)
	resolver := &net.UDPAddr{IP: net.IP{8, 8, 4, 4}, Port: 53}
	if err := send(conn, resolver, m); err != nil {
		log.Println(err)
	}
	//for _, resolver := range query.server {
	//	//resolver := &net.UDPAddr{IP: net.IP{223, 5, 5, 5}, Port: 53}
	//	go func(r net.UDPAddr) {
	//		if err := send(conn, &r, m); err != nil {
	//			log.Println(err)
	//		}
	//	}(resolver)
	//}
}

func send(conn *net.UDPConn, addr *net.UDPAddr, m dnsmessage.Message) error {
	packed, err := m.Pack()
	if err != nil {
		return err
	}
	if _, err = conn.WriteToUDP(packed, addr); err != nil {
		return err
	}
	return nil
}
