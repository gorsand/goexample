package main

import "bytes"

const (
	tcpStrDelim  = "\000"
	tcpByteDelim = 0
)

type Buffer struct {
	buf []byte
}

func (p *Buffer) AppendChunk(chunk []byte) []byte {
	p.buf = bytes.TrimLeft(append(p.buf, chunk...), tcpStrDelim)
	for {
		end := bytes.IndexByte(p.buf, tcpByteDelim)
		if end == -1 {
			return nil
		}
		data := p.buf[:end]
		p.buf = bytes.TrimLeft(p.buf[end+1:], tcpStrDelim)
		return data
	}
}
