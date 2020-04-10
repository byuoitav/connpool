package connpool

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// Conn .
type Conn interface {
	net.Conn

	ReadWriter() *bufio.ReadWriter
	EmptyReadBuffer(timeout time.Duration) ([]byte, error)
	ReadUntil(delim byte, timeout time.Duration) ([]byte, error)
}

type conn struct {
	rw *bufio.ReadWriter
	net.Conn
}

// Wrap .
func Wrap(c net.Conn) Conn {
	return &conn{
		rw:   bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c)),
		Conn: c,
	}
}

func (c *conn) ReadWriter() *bufio.ReadWriter {
	return c.rw
}

func (c *conn) Write(p []byte) (int, error) {
	n, err := c.rw.Write(p)
	if err != nil {
		return n, err
	}

	return n, c.rw.Flush()
}

func (c *conn) Read(p []byte) (int, error) {
	return c.rw.Read(p)
}

func (c *conn) EmptyReadBuffer(timeout time.Duration) ([]byte, error) {
	c.SetReadDeadline(time.Now().Add(timeout))

	total := c.rw.Reader.Buffered()
	bytes := make([]byte, 0, total)

	for len(bytes) < total {
		buf := make([]byte, total-len(bytes))
		_, err := c.rw.Read(buf)
		if err != nil {
			return bytes, fmt.Errorf("unable to empty read buffer: %s", err)
		}

		bytes = append(bytes, buf...)
	}

	return bytes, nil
}

func (c *conn) ReadUntil(delim byte, timeout time.Duration) ([]byte, error) {
	c.SetReadDeadline(time.Now().Add(timeout))
	return c.rw.ReadBytes(delim)
}
