package util

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	tcpDialTimeout      = 1 * time.Second
	tcpWriteTimeout     = 1 * time.Second
	tcpReconnectTimeout = 5 * time.Second
	tcpReadBufSize      = 65536
	udpWriteTimeout     = 5 * time.Second
	udpReadBufSize      = 65536
)

func checkCtxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func sendConnToChan(ctx context.Context, connections chan net.Conn, conn net.Conn) bool {
	select {
	case <-ctx.Done():
		LogWarn("Failed sending conn (addr '%s') to output channel", conn.RemoteAddr().String())
		return false
	case connections <- conn:
		return true
	}
}

type NetMsg struct {
	Name string
	Addr net.Addr
	Data []byte
}

func sendMsgToChan(ctx context.Context, output chan NetMsg, msg NetMsg) bool {
	select {
	case <-ctx.Done():
		LogWarn("Failed sending msg (name '%s', addr '%s') to output channel", msg.Name, msg.Addr)
		return false
	case output <- msg:
		return true
	}
}

type NetErr struct {
	Name  string
	Addr  net.Addr
	Error error
}

func sendErrToChan(ctx context.Context, errs chan NetErr, err NetErr) bool {
	select {
	case <-ctx.Done():
		LogWarn("Failed sending err (name '%s', addr '%s') to output channel", err.Name, err.Addr)
		return false
	case errs <- err:
		return true
	}
}

// UDPListen opens a UDP conn bind to a local port,
// fails if any errors (TODO: post errors to a channel)
func UDPListen(connName string, listenPort int) *net.UDPConn {
	addr := &net.UDPAddr{Port: listenPort}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		LogFatal("%s UDP listening port %d failed: %s", connName, listenPort, err.Error())
	}
	LogInfo("%s UDP socket is bind to local port %d", connName, listenPort)
	return conn
}

// UDPDial opens a UDP conn bind to a remote address,
// fails if any errors (TODO: post errors to a channel)
func UDPDial(connName, addrStr string) *net.UDPConn {
	addr, err := net.ResolveUDPAddr("udp", addrStr)
	if err != nil {
		LogFatal("%s UDP resolving addr failed: %s", connName, err.Error())
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		LogFatal("%s UDP dialing addr '%s' failed: %s", connName, addrStr, err.Error())
	}
	LogInfo("%s UDP socket is bind to remote addr '%s'", connName, addrStr)
	return conn
}

// RunUDPReader starts a UDP reading loop,
// conn close or error
// NOTE: does NOT always stop on ctx expire - can hand on conn.ReadFrom
func RunUDPReader(ctx context.Context, connName string, conn *net.UDPConn, output chan NetMsg, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	LogInfo("%s UDP reader loop started", connName)
	defer LogInfo("%s UDP reader loop stopped", connName)
	var buf [udpReadBufSize]byte
	for {
		if checkCtxDone(ctx) {
			// aborted by user
			return
		}
		n, addr, err := conn.ReadFrom(buf[0:])
		if checkCtxDone(ctx) {
			// aborted by user
			return
		}
		if n == len(buf) {
			err = fmt.Errorf("%s UDP buffer size is too small", connName)
		}
		if err != nil {
			e, ok := err.(*net.OpError)
			if ok && (e.Op == "read" || e.Op == "write") {
				// read error means we closed the socket and want to quit
				// TODO: better solution is to stop this task, re-open the conn and re-run sender and receiver in the super-task (reconnect-like logic)
				return
			}
			LogError("%s UDP socket read error from addr '%s': %s", connName, addr.String(), err.Error())
			continue
		}
		if n == 0 {
			// skip empty lines
			LogWarn("%s UDP reader got empty string from addr '%s'", connName, addr.String())
			continue
		}
		// save message to channel (buffer is copied)
		tmp := append([]byte{}, buf[0:n]...)
		sendMsgToChan(ctx, output, NetMsg{
			Name: connName,
			Addr: addr,
			Data: tmp,
		})
	}
}

// RunUDPWriter starts a UDP writer loop,
// stops on ctx expire or input channel closed,
// if the UDPConn is bind to a remote addr - the Addr field in Messages should be nil
func RunUDPWriter(ctx context.Context, connName string, conn *net.UDPConn, input chan NetMsg, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	LogInfo("%s UDP writer loop started", connName)
	defer LogInfo("%s UDP writer loop stopped", connName)
	for {
		select {
		case <-ctx.Done():
			// aborted by user
			return
		case msg, ok := <-input:
			if !ok {
				// input channel closed => quit
				return
			}
			conn.SetWriteDeadline(time.Now().Add(udpWriteTimeout))
			var err error
			if conn.RemoteAddr() == nil {
				// send to explicit addr
				if msg.Addr != nil {
					_, err = conn.WriteTo(msg.Data, msg.Addr)
				} else {
					LogError("Got nil destination addr when sending UDP message")
				}
			} else {
				// send to bind remote addr
				if msg.Addr == nil || msg.Addr.String() == conn.RemoteAddr().String() {
					// explicit addr is not given or is same with remote addr
					_, err = conn.Write(msg.Data)
				} else {
					LogError("Got explicit destination addr %s != bind remote UDP addr %s", msg.Addr.String(), conn.RemoteAddr().String())
				}
			}
			if checkCtxDone(ctx) {
				// aborted by user
				return
			}
			if err != nil {
				// skipping all errors
				// TODO: post errors to a channel so that the outer code could react on them
				LogError("%s UDP socket write failed for addr '%s': %s", connName, msg.Addr.String(), err.Error())
				continue
			}
		}
	}
}

// TCPConnect connects with a context,
// returns either conn or nil (if and only if ctx expired)
func TCPConnect(ctx context.Context, connString string) net.Conn {
	dialer := net.Dialer{}
	LogInfo("Connecting TCP to '%s' ...", connString)
	for {
		if conn, err := dialer.DialContext(ctx, "tcp", connString); err == nil {
			// success
			return conn
		}
		if checkCtxDone(ctx) {
			// aborted by user
			return nil
		}
		// wait and retry
		time.Sleep(tcpDialTimeout)
	}
}

// TCPListen starts listening TCP at given port,
// returns a valid listener or fails
func TCPListen(connName string, port int) *net.TCPListener {
	ourAddr := net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	}
	listener, err := net.ListenTCP("tcp", &ourAddr)
	if err != nil {
		LogFatal("%s TCP listening at port %d failed: %s", connName, port, err.Error())
	}
	LogInfo("%s TCP listener opened at port %d", connName, port)
	return listener
}

// RunTCPAcceptor starts an accept loop and posts connections to the channel,
// stops on listen error or ctx expire
func RunTCPAcceptor(ctx context.Context, connName string, listener *net.TCPListener, connections chan net.Conn, errors chan error, wg *sync.WaitGroup) error {
	LogInfo("%s TCP acceptor loop started at addr '%s'", connName, listener.Addr().String())
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	defer LogInfo("%s TCP acceptor loop stopped", connName)
	for {
		if checkCtxDone(ctx) {
			// aborted by user
			return nil
		}
		// NOTE: sometimes listener may accept even being closed
		// https://github.com/golang/go/issues/10527
		// but in our case this will not be critical as we'll just
		// pass the conn to the channel and the outer code will ignore it
		// as it knows that the listener is closed
		conn, err := listener.AcceptTCP()
		if err != nil {
			errors <- err
			return err
		}
		//conn.SetNoDelay(true)
		if !sendConnToChan(ctx, connections, conn) {
			// aborted by user
			conn.Close()
			return nil
		}
	}
}

// RunTCPReader starts the reader loop for the connection,
// stops on connection error (eg. closed)
func RunTCPReader(ctx context.Context, connName string, conn net.Conn, output chan NetMsg, errors chan NetErr, wg *sync.WaitGroup) {
	LogInfo("%s TCP reader loop started", connName)
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	defer LogInfo("%s TCP reader loop stopped", connName)
	buffer := make([]byte, tcpReadBufSize)
	for {
		if checkCtxDone(ctx) {
			// aborted by user
			return
		}
		n, err := conn.Read(buffer)
		if checkCtxDone(ctx) {
			// aborted by user
			return
		}
		if err != nil {
			sendErrToChan(ctx, errors, NetErr{
				Name:  connName,
				Addr:  conn.RemoteAddr(),
				Error: fmt.Errorf("%s TCP reader error: %s", connName, err.Error()),
			})
			return
		}
		if n > 0 {
			// save message to channel
			sendMsgToChan(ctx, output, NetMsg{
				Name: connName,
				Addr: conn.RemoteAddr(),
				Data: append([]byte{}, buffer[:n]...), // buffer is copied
			})
		}
	}
}

// RunTCPWriter starts writing loop for the connection,
// stops on: connection error, input channel closed or ctx expired.
// NOTE: stop this thread BEFORE closing the connection
// so that prevent from writing to a closed connection
func RunTCPWriter(ctx context.Context, connName string, conn net.Conn, input chan []byte, errors chan NetErr, wg *sync.WaitGroup) {
	LogInfo("%s TCP writer loop started", connName)
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	defer LogInfo("%s TCP writer loop stopped", connName)
	for {
		select {
		case <-ctx.Done():
			// aborted by user
			return
		case data, ok := <-input:
			if !ok {
				LogError("%s TCP writer: input channel closed (better use ctx to stop the thread)", connName)
			}
			// send data to conn (with timeout)
			conn.SetWriteDeadline(time.Now().Add(tcpWriteTimeout))
			_, err := conn.Write(data)
			if checkCtxDone(ctx) {
				// aborted by user
				return
			}
			if err != nil {
				// report error
				sendErrToChan(ctx, errors, NetErr{
					Name:  connName,
					Addr:  conn.RemoteAddr(),
					Error: fmt.Errorf("%s TCP writer error: %s", connName, err.Error()),
				})
				return
			}
		}
	}
}

// RunTCPReconnect starts reconnection loop,
// stops on ctx expired.
// NOTE: never close the channels while this thread is running -
// this may lead to infinite reconnections (the writer will always fail
// to get data from the input channel)
func RunTCPReconnect(ctx context.Context, connName string, connString string, incoming chan NetMsg, outgoing chan []byte, errors chan NetErr, wg *sync.WaitGroup) {
	LogInfo("%s TCP reconnect loop started, remote addr: '%s'", connName, connString)
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}
	defer LogInfo("%s TCP reconnect loop stopped", connName)
	for {
		// establish connection
		conn := TCPConnect(ctx, connString)
		if conn == nil {
			// aborted by user
			return
		}
		// notify connection ready
		if !sendErrToChan(ctx, errors, NetErr{
			Name:  connName,
			Addr:  conn.RemoteAddr(),
			Error: nil, // nil => connection OK
		}) {
			// aborted by user
			conn.Close()
			return
		}
		LogInfo("%s connected to '%s'", connName, connString)
		// start reader and writer
		c, cancel := context.WithCancel(ctx)
		w := sync.WaitGroup{}
		go func() {
			defer cancel()
			RunTCPReader(c, connName, conn, incoming, errors, &w)
		}()
		go func() {
			defer cancel()
			RunTCPWriter(c, connName, conn, outgoing, errors, &w)
		}()
		// wait reader/writer stopped or user cancel
		<-c.Done()
		// close connection and wait all
		conn.Close()
		w.Wait()
		if checkCtxDone(ctx) {
			// aborted by user => stop the loop
			return
		}
		LogInfo("Reconnecting %s TCP in a while ...", connName)
		time.Sleep(tcpReconnectTimeout)
	}
}
