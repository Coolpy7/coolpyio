package main

import (
	"context"
	"coolpyio/mempool"
	"coolpyio/pollio"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var onlines *sync.Map

func init() {
	timeLocation, _ := time.LoadLocation("Asia/Shanghai") //使用时区码
	time.Local = timeLocation
}

func main() {
	onlines = new(sync.Map)
	setLimit()
	var (
		addr = flag.String("l", ":80", "绑定Host地址")
	)
	flag.Parse()
	poll := pollio.NewEngine(pollio.Config{
		Network:      "tcp", //"udp", "unix"
		Addrs:        []string{*addr},
		EPOLLONESHOT: unix.EPOLLONESHOT,
		EpollMod:     unix.EPOLLET,
	})
	poll.OnReadBufferAlloc(func(c *pollio.Conn) []byte {
		return mempool.Malloc(64)
	})
	poll.OnReadBufferFree(func(c *pollio.Conn, buf []byte) {
		mempool.Free(buf)
	})
	// handle new connection
	poll.OnOpen(OnConnect)
	// handle connection closed
	poll.OnClose(OnDisconnect)
	// handle data
	poll.OnData(OnMessage)

	go func() {
		if err := poll.Start(); err != nil {
			log.Println(err)
		}
	}()
	log.Println("Coolpy7 On Port", *addr)

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	cleanup := make(chan bool)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range signalChan {
			ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			go func() {
				_ = poll.Shutdown(ctx)
				cleanup <- true
			}()
			<-cleanup
			fmt.Println("safe exit")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

func OnConnect(conn *pollio.Conn) {
	onlines.Store(conn, time.Now())
	log.Println("connection opened", conn.RemoteAddr())
}

func OnDisconnect(conn *pollio.Conn, err error) {
	_, ok := onlines.LoadAndDelete(conn)
	if ok {
		log.Println("connection closed", err.Error())
	}
}

func OnMessage(conn *pollio.Conn, data []byte) {
	_, ok := onlines.Load(conn)
	if !ok {
		_ = conn.Close()
		return
	}
	fmt.Print(string(data))
	_, _ = conn.Write(data)
}

func setLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
}
