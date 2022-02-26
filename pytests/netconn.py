
__all__ = [
    "UDPConn",
    "TCPConn",
    "tcp_listen",
]

import socket as _socket
import time as _time

# NOTE: usually socket is automatically shut down when all references to the system socket
# are closed. So calling close() is enough. Whereas shutdown() must be called
# only if one has same socket opened multiple times (eg. forked the process)
# and now wants to definetely send the FIN command. However, shutdown() doesn't release the memory,
# so close() is anyway required after shutdown().

def _addr_as_tuple(addr):
    # use this method in init to construct the addr tuple to use it in connection
    if addr is None:
        return None
    if isinstance(addr, tuple):
        # addr is already a tuple
        if not isinstance(addr[0], str) or not isinstance(addr[1], int):
            raise Exception(f"Addr tuple must be (str, int), got '{addr}'")
        return addr
    # integer-convertable value means we got port only
    try:
        return ("localhost", int(addr))
    except:
        pass
    # string value means we got a [host]:port pair
    if isinstance(addr, str):
        if ':' not in addr:
            raise Exception(f"Expected semicolon in addr, got '{addr}'")
        host, port = addr.split(":")
        if not host:
            host = "localhost"
        return (host, int(port))
    raise Exception(f"Usupported type for addr: {type(addr)} (need int or string)")

#
# UDP connection
#

class UDPConn(object):
    """ Stores local port, default remote addr and separator (to be stripped from data) """

    def __init__(self, **kw):
        self._sock = None
        self._local_port = kw.pop('local_port', None)
        self._remote_addr = _addr_as_tuple(kw.pop('remote_addr', None))
        self._separator = kw.pop('separator', None)
        if kw:
            raise Exception(f"Unexpected kw args: {kw}")

    def __bool__(self):
        return self._sock is not None

    def start(self):
        """ Creates UDP socket, binds to local addr (if any), returns error """
        try:
            self._sock = _socket.socket(_socket.AF_INET, _socket.SOCK_DGRAM)
            if self._local_port:
                self._sock.bind(("localhost", self._local_port))
            return None
        except Exception as e:
            return e

    def send(self, data, remote_addr=None, **kw):
        """ sends UDP data, returns error """
        if self._sock is None:
            return "UDP socket is None"
        if remote_addr:
            try:
                remote_addr = _addr_as_tuple(remote_addr)
            except Exception as e:
                return e
        else:
            remote_addr = self._remote_addr
        if not remote_addr:
            return "Remote addr not provided"
        if isinstance(data, str):
            data = data.encode()
        try:
            self._sock.sendto(data, remote_addr)
            return None
        except Exception as e:
            return e

    def recv(self, tmout=None):
        """ Receives UDP data, returns (data, remote_addr, error) OR Nones if timed out """
        try:
            self._sock.settimeout(tmout)
            data, addr = self._sock.recvfrom(65536)
            # remove separator (if needed)
            if self._separator:
                data = data.lstrip(self._separator).rstrip(self._separator)
            return data, addr, None
        except _socket.timeout:
            return None, None, None
        except Exception as e:
            return None, None, e

    def close(self):
        """ Closes socket """
        if self._sock:
            self._sock.close()
            self._sock = None

#
# TCP connection
# 

def tcp_listen(acceptor_port):
    """ Creates TCP listen socket, returns (socket, error) """
    try:
        sock = _socket.socket(_socket.AF_INET, _socket.SOCK_STREAM)
        # enable reuse addr to allow connect/accept at same machine
        sock.setsockopt(_socket.SOL_SOCKET, _socket.SO_REUSEADDR, 1)
        sock.bind(("localhost", acceptor_port))
        sock.listen()
        return sock, None
    except Exception as e:
        return None, e

# !
# TODO: use reader callback to parse messages from the buffer
# (currently only separator and end scanner can be provided for custom parsing)
# !

class TCPConn(object):
    """ Stores local port, default remote addr, msg separator and end scanner, manages read buffer """

    def __init__(self, **kw):
        self._separator = kw.pop('separator', '\0')
        self._end_scanner = kw.pop('end_scanner', None)
        self._remote_addr = _addr_as_tuple(kw.pop('remote_addr', None))
        self._local_port = kw.pop('local_port', None)
        self._dflt_tmout = _socket.getdefaulttimeout() or kw.pop('default_tmout', 1)
        if kw:
            raise Exception(f"Unexpected kw args: {kw}")
        self._sock = None
        # buffer for TCP part-by-part reading
        self._buffer = bytes()
        # the separator is automatically stripped on recv and appended on send
        if self._separator:
            self._separator = self._separator.encode()
        # the default end scanner is find next separator
        if not self._end_scanner:
            if not self._separator:
                raise Exception("Either separator or end scanner must be provided")
            self._end_scanner = lambda data: data.find(self._separator)

    def __bool__(self):
        return self._sock is not None

    def connect(self, tmout=None):
        """ Creates TCP socket, tries to connect, returns error """
        if not self._remote_addr:
            return "Remote addr not provided"
        try:
            local_addr = ("localhost", self._local_port) if self._local_port else None
            self._sock = _socket.create_connection(self._remote_addr, tmout, local_addr)
            return None
        except _socket.timeout:
            return "Timeout"
        except Exception as e:
            return e

    def accept(self, listen_sock, tmout=1):
        """ Accepts TCP connection for given listen socket, saves the connected socket, returns (remote_addr, error) """
        try:
            listen_sock.settimeout(tmout)
            self._sock, self._remote_addr = listen_sock.accept()
            return self._remote_addr, None
        except _socket.timeout:
            return None, "Timeout"
        except Exception as e:
            return None, e

    def send(self, data, tmout=None):
        """ Sends TCP data, returns error """
        if self._sock is None:
            return "Attempt to send while TCP socket is None"
        if self._remote_addr is None:
            return "Remote addr not provided"
        if isinstance(data, str):
            data = data.encode()
        try:
            if self._separator and not data.endswith(self._separator):
                data += self._separator
            self._sock.settimeout(tmout)
            self._sock.sendall(data)
            return None
        except _socket.timeout:
            return f"Timeout"
        except Exception as e:
            return e

    def recv(self, tmout=None):
        """ Receives TCP data, returns (data, error) OR Nones if timed out """
        if self._sock is None:
            return None, "Attempt to receive while TCP socket is None"
        tm = _time.time()
        tmout = tmout or self._dflt_tmout
        end_tm = tm + tmout
        try:
            while tm < end_tm:
                # find new message end
                i = self._end_scanner(self._buffer)
                if i != -1:
                    # got new message
                    data = self._buffer[:i]
                    self._buffer = self._buffer[i:]
                    # strip leading separators (if needed)
                    if self._separator:
                        self._buffer = self._buffer.lstrip(self._separator)
                    return data, None
                # read new piece of data
                self._sock.settimeout(end_tm - tm)
                self._buffer += self._sock.recv(65536)
                tm = _time.time()
            return None, None # end tm reached
        except _socket.timeout:
            return None, None # end tm reached
        except Exception as e:
            return None, e

    def close(self):
        """ Closes socket """
        if self._sock is not None:
            self._sock.close()
            self._sock = None
