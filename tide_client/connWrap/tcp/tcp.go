package tcp

import (
	"errors"
	"log"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"tide/tide_client/connWrap"
	"time"
	"unsafe"
)

type Tcp struct {
	addr        string
	conn        *net.TCPConn
	readTimeout time.Duration
	readBuf     []byte
	inReconnect atomic.Bool
}

func NewTcp(addr string, readTimeout uint32) (connWrap.ConnCommon, error) {
	c := &Tcp{
		addr:        addr,
		conn:        new(net.TCPConn),
		readTimeout: time.Duration(readTimeout) * time.Millisecond,
		readBuf:     make([]byte, 1024),
	}
	go c.reopenUntilSuccess()
	return c, nil
}

func (c *Tcp) reopenUntilSuccess() {
	if !c.inReconnect.CompareAndSwap(false, true) {
		return
	}
	defer c.inReconnect.Store(false)
	if c.conn != nil {
		_ = c.conn.Close()
	}
	var err error
	for {
		if err = c.open(); err != nil {
			log.Printf("connect to %s failed: %s\n", c.addr, err)
		} else {
			log.Printf("connected to: %s\n", c.addr)
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *Tcp) open() error {
	newConn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	c.conn = newConn.(*net.TCPConn)
	err = c.conn.SetKeepAlive(true)
	if err != nil {
		return err
	}
	err = c.conn.SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		return err
	}
	rawConn, err := c.conn.SyscallConn()
	if err != nil {
		return err
	}
	var sysErr error
	err = rawConn.Control(func(fd uintptr) {
		sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 1)
	})
	if err != nil {
		return err
	}
	if sysErr != nil {
		return sysErr
	}
	return err
}

func (c *Tcp) Read(b []byte) (n int, err error) {
	defer func() { c.handleErr(err) }()
	_ = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	n, err = c.conn.Read(b)
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Tcp) Write(b []byte) (n int, err error) {
	defer func() { c.handleErr(err) }()
	n, err = c.conn.Write(b)
	return
}

func (c *Tcp) ReadyToRead() (n uint32, err error) {
	defer func() { c.handleErr(err) }()
	rawConn, err := c.conn.SyscallConn()
	if err != nil {
		return 0, err
	}

	err = rawConn.Control(func(fd uintptr) {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCINQ, uintptr(unsafe.Pointer(&n)))
	})
	return
}

func (c *Tcp) ResetInputBuffer() (err error) {
	defer func() { c.handleErr(err) }()
	rawConn, err := c.conn.SyscallConn()
	if err != nil {
		return err
	}
	err = c.conn.SetReadDeadline(time.Time{})
	if err != nil {
		return
	}
	err = rawConn.Read(func(fd uintptr) bool {
		_, _ = syscall.Read(int(fd), c.readBuf)
		return true
	})
	return
}

func (c *Tcp) handleErr(err error) {
	if err == nil || errors.Is(err, connWrap.ErrTimeout) {
		return
	}
	go c.reopenUntilSuccess()
}
