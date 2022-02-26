#!/usr/bin/env python3

""" Simple test scenarios for testing UDP client-server interactions
"""

from netconn import UDPConn

upstream_udp_local_port = 2222
service_udp_port4upstream = 3333
service_udp_port4clients  = 5555

# open the port on the upstream side
upstream_conn = UDPConn(local_port=upstream_udp_local_port)
err = upstream_conn.start()
if err:
    exit(f"Failed opening port on upstream side: {err}")

print(f"Upstream opened")

# open the port on the client side
client_conn = UDPConn(remote_addr=service_udp_port4clients)
err = client_conn.start()
if err:
    exit(f"Failed opening port on clients side: {err}")

print(f"Client opened")

# send message from client
data = "TEST1"
err = client_conn.send(data)
if err:
    exit(f"Failed sending data from client side: {err}")

print(f"Data sent from client: {data}")

# expect same message on upstream side
reply, addr, err = upstream_conn.recv()
if not reply:
    exit(f"Failed recv data on upstream side: {err}")

print(f"Data received from {addr} on upstream: {reply}")

reply = reply.decode()
if reply != data:
    exit(f"TEST FAILED: '{reply}' != '{data}'")

# send message from upstream
data = "TEST2"
err = upstream_conn.send(data, addr)
if err:
    exit(f"Failed sending data from upstream side: {err}")

print(f"Data sent from upstream: {data}")

# expect same message on clients side
reply, addr, err = client_conn.recv()
if not reply:
    exit(f"Failed recv data on client side: {err}")

print(f"Data received from {addr} on client: {reply}")

reply = reply.decode()
if reply != data:
    exit(f"TEST FAILED: '{reply}' != '{data}'")

print("TEST PASSED")