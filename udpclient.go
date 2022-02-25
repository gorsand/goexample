package main

import "net"

type UDPClient struct {
	Addr net.Addr
}

// current UDP active clients
var udpClients = map[string]*UDPClient{}

func GetOrAddUDPClient(addr net.Addr) *UDPClient {
	s := addr.String()
	cl, ok := udpClients[s]
	if !ok {
		LogInfo("Adding a new UDP client (%s)", s)
		cl = &UDPClient{Addr: addr}
	}
	udpClients[s] = cl
	return cl
}
