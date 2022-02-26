#!/usr/bin/env python3

""" Simple test scenarios for testing TCP client-server interactions
"""

from netconn import TCPConn, tcp_listen

upstream_tcp_accept_port = 1111
service_tcp_port4clients  = 4444

# open listener on the upstream side
upstream_listener, err = tcp_listen(upstream_tcp_accept_port)
if err:
    exit(f"Failed listening on upstream side: {err}")

# accept the upstream connection from the service
upstream_conn = TCPConn()
addr, err = upstream_conn.accept(upstream_listener)
if err:
    exit(f"Failed accept on upstream side: {err}")

print(f"Accepted upstream connection from {addr}")

# initiate connection from the client side
client_conn = TCPConn(remote_addr=service_tcp_port4clients)
err = client_conn.connect()
if err:
    exit(f"Failed connecting from clients side: {err}")

print(f"Client connected")

# send message from client
data = "TEST1"
err = client_conn.send(data)
if err:
    exit(f"Failed sending data from client side: {err}")

print(f"Data sent from client: {data}")

# expect same message on upstream side
reply, err = upstream_conn.recv()
if not reply:
    exit(f"Failed recv data on upstream side: {err}")

print(f"Data received on upstream: {reply}")

reply = reply.decode()
if reply != data:
    exit(f"TEST FAILED: '{reply}' != '{data}'")

# send message from upstream
data = "TEST2"
err = upstream_conn.send(data)
if err:
    exit(f"Failed sending data from upstream side: {err}")

print(f"Data sent from upstream: {data}")

# expect same message on clients side
reply, err = client_conn.recv()
if not reply:
    exit(f"Failed recv data on client side: {err}")

print(f"Data received on client: {reply}")

reply = reply.decode()
if reply != data:
    exit(f"TEST FAILED: '{reply}' != '{data}'")

print("TEST PASSED")
