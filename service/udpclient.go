package service

import (
	"net"
	"util"
)

type UDPClient struct {
	Addr net.Addr
}

// current UDP active clients
var UDPClients = map[string]*UDPClient{}

func GetOrAddUDPClient(addr net.Addr) *UDPClient {
	s := addr.String()
	cl, ok := UDPClients[s]
	if !ok {
		util.LogInfo("Adding a new UDP client (%s)", s)
		cl = &UDPClient{Addr: addr}
	}
	UDPClients[s] = cl
	return cl
}
