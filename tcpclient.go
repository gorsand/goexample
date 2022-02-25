package main

import (
	"context"
	"fmt"
	"net"
	"sync"
)

type TCPClient struct {
	conn   net.Conn
	buf    Buffer
	sink   chan []byte
	wg     *sync.WaitGroup
	cancel context.CancelFunc
}

func (cl TCPClient) addrStr() string {
	return cl.conn.RemoteAddr().String()
}

func (cl *TCPClient) Start(outChan chan NetMsg, errChan chan NetErr) {
	cl.sink = make(chan []byte, 1024) // TODO: try to use 0- or 1-sized channels
	cl.buf = Buffer{buf: []byte{}}
	ctx, cancel := context.WithCancel(context.Background())
	cl.cancel = cancel
	cl.wg = &sync.WaitGroup{}
	name := fmt.Sprintf("Client[%s]", cl.conn.RemoteAddr().String())
	go RunTCPReader(ctx, name, cl.conn, outChan, errChan, cl.wg)
	go RunTCPWriter(ctx, name, cl.conn, cl.sink, errChan, cl.wg)
}

func (cl *TCPClient) SendMsg(data []byte) {
	// HERE can be some message encoding logic
	cl.sink <- data
}

func (cl *TCPClient) Stop() {
	LogInfo("Stopping client [%s]", cl.addrStr())
	cl.cancel()
	cl.conn.Close()
}

func (cl *TCPClient) Wait() {
	cl.wg.Wait()
	LogInfo("Client [%s] stopped", cl.addrStr())
}

// current TCP cleints
var tcpClients = map[string]*TCPClient{}

func AddTCPClient(conn net.Conn) *TCPClient {
	s := conn.RemoteAddr().String()
	cl, ok := tcpClients[s]
	if !ok {
		LogInfo("Adding a new TCP client with addr '%s'", s)
		cl = &TCPClient{
			conn: conn,
			sink: make(chan []byte, 1024),
		}
	}
	tcpClients[s] = cl
	return cl
}

func DelTCPClient(cl *TCPClient) {
	delete(tcpClients, cl.addrStr())
}
