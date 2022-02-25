package util

import "bytes"

const (
	tcpStrDelim  = "\000"
	tcpByteDelim = 0
)

type Buffer struct {
	Data []byte
}

func (p *Buffer) AppendChunk(chunk []byte) []byte {
	p.Data = bytes.TrimLeft(append(p.Data, chunk...), tcpStrDelim)
	for {
		end := bytes.IndexByte(p.Data, tcpByteDelim)
		if end == -1 {
			return nil
		}
		data := p.Data[:end]
		p.Data = bytes.TrimLeft(p.Data[end+1:], tcpStrDelim)
		return data
	}
}
