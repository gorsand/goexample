package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"util"
)

type TCPClient struct {
	conn   net.Conn
	buf    util.Buffer
	sink   chan []byte
	wg     *sync.WaitGroup
	cancel context.CancelFunc
}

func (cl TCPClient) AddrStr() string {
	return cl.conn.RemoteAddr().String()
}

func (cl *TCPClient) Start(outChan chan util.NetMsg, errChan chan util.NetErr) {
	cl.sink = make(chan []byte, 1024) // TODO: try to use 0- or 1-sized channels
	cl.buf = util.Buffer{Data: []byte{}}
	ctx, cancel := context.WithCancel(context.Background())
	cl.cancel = cancel
	cl.wg = &sync.WaitGroup{}
	name := fmt.Sprintf("Client[%s]", cl.conn.RemoteAddr().String())
	go util.RunTCPReader(ctx, name, cl.conn, outChan, errChan, cl.wg)
	go util.RunTCPWriter(ctx, name, cl.conn, cl.sink, errChan, cl.wg)
}

func (cl *TCPClient) SendMsg(data []byte) {
	// HERE can be some message encoding logic
	cl.sink <- data
}

func (cl *TCPClient) AppendChunk(data []byte) []byte {
	return cl.buf.AppendChunk(data)
}

func (cl *TCPClient) Stop() {
	util.LogInfo("Stopping client [%s]", cl.AddrStr())
	cl.cancel()
	cl.conn.Close()
}

func (cl *TCPClient) Wait() {
	cl.wg.Wait()
	util.LogInfo("Client [%s] stopped", cl.AddrStr())
}

// current TCP cleints
var TCPClients = map[string]*TCPClient{}

func AddTCPClient(conn net.Conn) *TCPClient {
	s := conn.RemoteAddr().String()
	cl, ok := TCPClients[s]
	if !ok {
		util.LogInfo("Adding a new TCP client with addr '%s'", s)
		cl = &TCPClient{
			conn: conn,
			sink: make(chan []byte, 1024),
		}
	}
	TCPClients[s] = cl
	return cl
}

func DelTCPClient(cl *TCPClient) {
	delete(TCPClients, cl.AddrStr())
}
