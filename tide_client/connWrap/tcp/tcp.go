//go:build linux

package tcp

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"tide/tide_client/connWrap"
	"time"
	"unsafe"
)

type Tcp struct {
	addr        string
	connMu      sync.RWMutex
	conn        *net.TCPConn
	readTimeout time.Duration
	readBuf     []byte
	inReconnect atomic.Bool
}

func StartTcp(addr string, readTimeout uint32) (connWrap.ConnCommon, error) {
	c := &Tcp{
		addr:        addr,
		conn:        nil,
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

	if conn := c.loadConn(); conn != nil {
		_ = conn.Close()
	}
	for {
		if err := c.open(); err != nil {
			slog.Error("Failed to connect to TCP endpoint", "addr", c.addr, "error", err)
			time.Sleep(10 * time.Second)
			continue
		}

		slog.Info("Connected to TCP endpoint", "addr", c.addr)
		return
	}
}

func (c *Tcp) open() (err error) {
	newConn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	conn := newConn.(*net.TCPConn)
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	err = conn.SetKeepAlive(true)
	if err != nil {
		return err
	}
	err = conn.SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		return err
	}
	rawConn, err := conn.SyscallConn()
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
	c.storeConn(conn)
	return nil
}

func (c *Tcp) Read(b []byte) (n int, err error) {
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	_ = conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	n, err = conn.Read(b)
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Tcp) Write(b []byte) (n int, err error) {
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	n, err = conn.Write(b)
	return
}

func (c *Tcp) ReadyToRead() (n uint32, err error) {
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}

	err = rawConn.Control(func(fd uintptr) {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCINQ, uintptr(unsafe.Pointer(&n)))
	})
	return
}

func (c *Tcp) ResetInputBuffer() (err error) {
	conn := c.loadConn()
	if conn == nil {
		return os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	err = conn.SetReadDeadline(time.Time{})
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

func (c *Tcp) loadConn() *net.TCPConn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Tcp) storeConn(conn *net.TCPConn) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.conn = conn
}
