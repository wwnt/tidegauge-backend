package tcp

import (
	"errors"
	"log"
	"net"
	"os"
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
}

func NewTcp(addr string, readTimeout uint32) (connWrap.ConnCommon, error) {
	p := &Tcp{addr: addr, readTimeout: time.Duration(readTimeout) * time.Millisecond, readBuf: make([]byte, 1024)}
	err := p.open()
	return p, err
}

func (c *Tcp) open() error {
	if c.conn == nil {
		c.conn = new(net.TCPConn)
	} else {
		_ = c.conn.Close()
	}
	newConn, err := net.Dial("tcp", c.addr)
	if err == nil {
		log.Println("connected to", newConn.RemoteAddr())
		c.conn = newConn.(*net.TCPConn)
		rawConn, err := c.conn.SyscallConn()
		if err != nil {
			return err
		}
		var sysErr error
		err = rawConn.Control(func(fd uintptr) {
			sysErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 1)
		})
		if sysErr != nil {
			return sysErr
		}
	}
	return err
}

func (c *Tcp) Read(b []byte) (n int, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	n, err = c.conn.Read(b)
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		return 0, connWrap.ErrTimeout
	}
	if err != nil {
		if err2 := c.open(); err2 != nil {
			err = errors.New(err.Error() + ". Reopen: " + err2.Error())
		}
	}
	return n, err
}

func (c *Tcp) Write(b []byte) (n int, err error) {
	if err = c.ResetInputBuffer(); err != nil {
		return 0, err
	}
	n, err = c.conn.Write(b)
	if err != nil {
		if err2 := c.open(); err2 != nil {
			err = errors.New(err.Error() + ". Reopen: " + err2.Error())
		}
	}
	return n, err
}

func (c *Tcp) ReadyToRead() (n uint32, err error) {
	rawConn, err := c.conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var errno syscall.Errno
	err = rawConn.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCINQ, uintptr(unsafe.Pointer(&n)))
	})
	if errno != 0 {
		err = errno
	}
	return
}

func (c *Tcp) ResetInputBuffer() (err error) {
	rawConn, err := c.conn.SyscallConn()
	if err != nil {
		return err
	}
	err = rawConn.Control(func(fd uintptr) {
		_, _ = syscall.Read(int(fd), c.readBuf)
	})
	return
}
