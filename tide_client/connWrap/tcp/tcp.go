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

var _ connWrap.Reopener = (*Tcp)(nil)

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
		// 对端的串口转发程序在连接断开时关闭串口、新连接进来时再打开；
		// 留出间隔的原因见 connWrap.ReopenPause。
		time.Sleep(connWrap.ReopenPause)
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

// Reopen 在后台断开并重新建立连接，立即返回。实现 connWrap.Reopener。
//
// 用于"读写都不报错、但对端一直不回话"的情况，见 connWrap.Bus。对端是串口转发程序时，
// 重新连接会让它重新打开串口，效果与直连串口时重开端口相同。
func (c *Tcp) Reopen(reason string) {
	slog.Warn("Reconnecting TCP endpoint", "addr", c.addr, "reason", reason)
	go c.reopenUntilSuccess()
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
