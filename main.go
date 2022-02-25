package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	tcpAddrStrForUpstream    = "localhost:1111"
	udpAddrStrForUpstream    = "localhost:2222"
	udpListenPortForUpstream = 3333
	tcpListenPortForClients  = 4444
	udpListenPortForClients  = 5555
	wakeUpTmoutMsec          = 100
	periodicUpdateMsec       = 5000
)

// upsteam current connection status
var isTCPUpstreamConnected = false

func main() {

	// create connections and listeners
	// NOTE: may Fatal, so do it before any goroutuine started
	udpConnForUpstream := UDPDial("UDPUpstream", udpAddrStrForUpstream)
	udpConnForClients := UDPListen("UDPClients", udpListenPortForClients)
	tcpListenerForClients := TCPListen("TCPClients", tcpListenPortForClients)

	// start upstream UDP connection
	udpDataFromUpstream := make(chan NetMsg, 1024)
	udpDataToUpstream := make(chan NetMsg, 1024)
	ctx, udpUpCancel := context.WithCancel(context.Background())
	udpUpWg := sync.WaitGroup{}
	go RunUDPReader(ctx, "UDPUpstream", udpConnForUpstream, udpDataFromUpstream, &udpUpWg)
	go RunUDPWriter(ctx, "UDPUpstream", udpConnForUpstream, udpDataToUpstream, &udpUpWg)

	// start upstream TCP recconect loop
	tcpUpstreamBuffer := Buffer{buf: []byte{}}
	tcpDataFromUpstream := make(chan NetMsg, 1024)
	tcpDataToUpstream := make(chan []byte, 1024)
	tcpErrFromUpstream := make(chan NetErr, 2)
	ctx, tcpUpCancel := context.WithCancel(context.Background())
	tcpUpWg := sync.WaitGroup{}
	go RunTCPReconnect(ctx, "TCPUpstream", tcpAddrStrForUpstream, tcpDataFromUpstream, tcpDataToUpstream, tcpErrFromUpstream, &tcpUpWg)

	// start clients UDP connection
	udpDataFromClients := make(chan NetMsg, 1024)
	udpDataToClients := make(chan NetMsg, 1024)
	ctx, udpClCancel := context.WithCancel(context.Background())
	udpClWg := sync.WaitGroup{}
	go RunUDPReader(ctx, "UDPClients", udpConnForClients, udpDataFromClients, &udpClWg)
	go RunUDPWriter(ctx, "UDPClients", udpConnForClients, udpDataToClients, &udpClWg)

	// start client TCP acceptor
	tcpConnectionsFromClients := make(chan net.Conn, 1024)
	tcpAcceptorErrors := make(chan error, 2)
	ctx, clAcceptorCancel := context.WithCancel(context.Background())
	clAcceptorWg := sync.WaitGroup{}
	go RunTCPAcceptor(ctx, "ClientsAcceptor", tcpListenerForClients, tcpConnectionsFromClients, tcpAcceptorErrors, &clAcceptorWg)

	// create TCP clients channels
	tcpDataFromClients := make(chan NetMsg, 1024)
	tcpErrorsFromClients := make(chan NetErr, 1024)

	// redirect OS signals to a channel
	signalsFromOS := make(chan os.Signal, 1)
	signal.Notify(signalsFromOS, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)

	// create wake-up timer
	wakeUpTimer := time.NewTicker(time.Millisecond * wakeUpTmoutMsec)

	// timer for periodic update
	lastUpdateMsec := int64(0)

	//
	// EVENT LOOP
	//

	for {

		nowMsec := time.Now().UnixNano() / 1000 / 1000

		select {

		//
		// handle TCP clients
		//

		case err := <-tcpAcceptorErrors:

			LogInfo("Client TCP acceptor error: %s", err.Error())

			clAcceptorCancel()
			clAcceptorWg.Wait() // NOTE: bad idea! we freeze event loop here

			// just run the acceptor again
			// TODO: reopen the listener as well
			// TODO: shold we clear the channels? better - have a channel of connections
			ctx, cancel := context.WithCancel(context.Background())
			clAcceptorCancel = cancel
			go RunTCPAcceptor(ctx, "ClientsAcceptor", tcpListenerForClients, tcpConnectionsFromClients, tcpAcceptorErrors, &clAcceptorWg)

		case conn := <-tcpConnectionsFromClients:

			LogInfo("RECV a connection from a new client with addr '%s'", conn.RemoteAddr().String())

			cl := AddTCPClient(conn)
			cl.Start(tcpDataFromClients, tcpErrorsFromClients)

		case msg := <-tcpDataFromClients:

			cl, ok := tcpClients[msg.Addr.String()]
			if !ok {
				LogError("SKIP TCP message from unknown addr '%s'", msg.Addr.String())
				break
			}

			// append piece of message and check have complete data
			if data := cl.buf.AppendChunk(msg.Data); data != nil {
				LogInfo("RECV TCP message from client (%s): %s", msg.Addr.String(), string(data))

				//
				// HERE we should build the request to the upstream
				// now - simply bypass the data to the upstream
				//

				LogInfo("SEND TCP message to client (%s): %s", cl.addrStr(), string(data))
				tcpDataToUpstream <- msg.Data
			}

		case err := <-tcpErrorsFromClients:

			addrStr := err.Addr.String()
			cl, ok := tcpClients[addrStr]

			if !ok {
				if err.Error == nil {
					LogError("SKIP TCP nil error from unknown addr '%s'", addrStr)
				} else {
					LogError("SKIP TCP errorfrom unknwon addr '%s': %s", addrStr, err.Error.Error())
				}
				break
			}
			LogInfo("RECV TCP error from client (%s): %s", addrStr, err.Error.Error())
			//  just stop and remove the client
			cl.Stop()
			DelTCPClient(cl)

		//
		// handle UDP clients
		//

		case msg := <-udpDataFromClients:

			cl := GetOrAddUDPClient(msg.Addr)
			LogInfo("RECV UDP message from client (%s): %s", msg.Addr.String(), string(msg.Data))

			//
			// HERE we should build the request to the upstream
			// now - simply bypass the data to the upstream via UDP
			//

			LogInfo("SEND UDP message to client (%s): %s", cl.Addr.String(), string(msg.Data))
			udpDataToUpstream <- NetMsg{
				Addr: udpConnForUpstream.RemoteAddr(),
				Data: msg.Data,
			}

		//
		// handle TCP from upstream
		//

		case err := <-tcpErrFromUpstream:

			if err.Error == nil && !isTCPUpstreamConnected {
				isTCPUpstreamConnected = true
				LogInfo("Upstream TCP connection is UP")
				//
				// HERE we should do something when connection is UP
				// Eg. notify all clients
				//
			}

			if err.Error != nil && isTCPUpstreamConnected {
				isTCPUpstreamConnected = false
				LogInfo("Upstream TCP conenction is DOWN, reason: %s", err.Error.Error())
				//
				// HERE we should do something when connection is DOWN
				// Eg. notify all clients
				//
			}

		case msg := <-tcpDataFromUpstream:

			if data := tcpUpstreamBuffer.AppendChunk(msg.Data); data != nil {
				LogInfo("RECV TCP message from upstream: %s", string(data))

				//
				// HERE we should build and send the replies to some of clients
				// now - simply bypass the data to all the TCP clients
				//

				LogInfo("SEND message to all TCP clients: %s", string(data))
				for _, cl := range tcpClients {
					cl.SendMsg(msg.Data)
				}
			}

		//
		// handle UDP from upstream
		//

		case msg := <-udpDataFromUpstream:

			LogInfo("RECV UDP message from upstream: %s", string(msg.Data))

			//
			// HERE we should build and send the replies to some of clients
			// now - simply bypass the data to all the UDP clients
			//

			LogInfo("SEND message to all UDP clients: %s", string(msg.Data))
			for _, cl := range udpClients {
				udpDataToClients <- NetMsg{Addr: cl.Addr, Data: msg.Data}
			}

		//
		// handle OS signals
		//

		case sig := <-signalsFromOS:

			LogInfo("Received OS signal: %s, shutting down ...", sig.String())

			// stop clients listener (so as not to get new connections)
			tcpListenerForClients.Close()

			// stop TCP clients readers and writers
			for _, cl := range tcpClients {
				cl.Stop()
				cl.Wait()
			}

			// stop UDP clients listener
			udpClCancel()
			udpClWg.Wait()
			udpConnForClients.Close()

			// stop upstream UDP conn
			udpUpCancel()
			udpUpWg.Wait()
			udpConnForUpstream.Close()

			// stop upstream TCP conn
			tcpUpCancel()
			tcpUpWg.Wait()
			return

		//
		// handle timer
		//

		case <-wakeUpTimer.C:
			// exit select and proceed the event loop

		} // select end

		if nowMsec >= lastUpdateMsec+periodicUpdateMsec {
			if lastUpdateMsec != 0 {
				//
				// HERE we should do any periodic stuff
				//
				LogInfo("Waiting events...")
			}
			lastUpdateMsec = nowMsec
		}
	}
}
