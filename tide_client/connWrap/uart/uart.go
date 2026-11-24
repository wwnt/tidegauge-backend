package uart

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"tide/tide_client/connWrap"
	"time"

	"go.bug.st/serial"
)

func getParity(parity string) serial.Parity {
	parity = strings.ToLower(parity)
	if p, ok := parityMap[parity]; ok {
		return p
	} else {
		return serial.NoParity
	}
}

var parityMap = map[string]serial.Parity{
	"none":  serial.NoParity,
	"odd":   serial.OddParity,
	"even":  serial.EvenParity,
	"mark":  serial.MarkParity,
	"space": serial.SpaceParity,
}

type Mode struct {
	BaudRate int    `json:"baud_rate"` // The serial port bitrate (aka Baud rate)
	DataBits int    `json:"data_bits"` // Size of the character (must be 5, 6, 7 or 8)
	Parity   string `json:"parity"`    // Parity: None, Odd, Even, Mark, Space
}

type Uart struct {
	mode     serial.Mode
	portName string
	connMu   sync.RWMutex
	conn     serial.Port
	// connClosed 为 true 表示 conn 已被重连流程关闭、新端口尚未就绪。由 connMu 保护。
	connClosed  bool
	readTimeout time.Duration
	inReconnect atomic.Bool
	// reopenPause 是关闭旧端口到打开新端口之间的间隔，见 connWrap.ReopenPause。
	reopenPause time.Duration
	// openPort 打开串口，nil 表示用 serial.Open。测试用它换掉真实串口。
	openPort func(name string, mode *serial.Mode) (serial.Port, error)
}

var _ connWrap.Reopener = (*Uart)(nil)

// errPortClosed 表示端口已关闭、正在后台重连；调用方按一次普通的传输失败处理。
var errPortClosed = fmt.Errorf("uart port is reconnecting: %w", os.ErrClosed)

func StartUart(name string, readTimeout uint32, mode Mode) (connWrap.ConnCommon, error) {
	c := &Uart{
		portName:    name,
		conn:        nil,
		readTimeout: time.Duration(readTimeout) * time.Millisecond,
		reopenPause: connWrap.ReopenPause,
		mode: serial.Mode{
			BaudRate: mode.BaudRate,
			DataBits: mode.DataBits,
			Parity:   getParity(mode.Parity),
			StopBits: serial.OneStopBit,
		},
	}
	go c.reopenUntilSuccess()
	return c, nil
}

func (c *Uart) reopenUntilSuccess() {
	if !c.inReconnect.CompareAndSwap(false, true) {
		return
	}
	defer c.inReconnect.Store(false)

	if c.closeCurrent() {
		time.Sleep(c.reopenPause)
	}
	for {
		if err := c.open(); err != nil {
			slog.Error("Failed to connect to UART port", "port", c.portName, "error", err)
			time.Sleep(10 * time.Second)
			continue
		}

		slog.Info("Successfully connected to UART port", "port", c.portName)
		return
	}
}

// Reopen 在后台关闭并重新打开串口，立即返回。实现 connWrap.Reopener。
//
// 读写出错时本类型会自己重连；这个入口留给上层处理"什么错都不报、但对端就是不回话"
// 的情况：串口转接芯片的接收卡住，或者它后面的单片机死机，在这一侧看到的都只是超时。
// 重开端口会重新配置转接芯片，并在 DTR 上产生一次脉冲——带自动复位电路的板子
// （Arduino）会因此复位，效果和重启本进程相同。
func (c *Uart) Reopen(reason string) {
	slog.Warn("Reopening UART port", "port", c.portName, "reason", reason)
	go c.reopenUntilSuccess()
}

// closeCurrent 关闭当前端口并把它标记为已关闭，返回是否真的关闭了一个端口。
//
// 已关闭的端口对象仍然保持发布，直到新端口打开：并发的 Read 拿到的是它，会得到
// 端口自己的"已关闭"错误，而不是空指针。但 Write/ResetInputBuffer 不能再碰它——
// go.bug.st/serial 的这两个方法不检查端口是否已关闭，直接对保存的 fd 号做系统调用；
// 而 fd 号在关闭后会被下一次 open() 复用（多总线站点上就是另一条总线的串口），
// 结果是清空别的总线的输入缓冲、把本总线的请求字节写到别的总线上。
// 持写锁完成关闭，保证没有正在进行中的 Write/ResetInputBuffer 跨过关闭点。
func (c *Uart) closeCurrent() bool {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == nil || c.connClosed {
		return false
	}
	c.connClosed = true
	_ = c.conn.Close()
	return true
}

func (c *Uart) open() error {
	openPort := c.openPort
	if openPort == nil {
		openPort = serial.Open
	}
	port, err := openPort(c.portName, &c.mode)
	if err != nil {
		return err
	}
	if err = port.SetReadTimeout(c.readTimeout); err != nil {
		_ = port.Close()
		return err
	}
	c.storeConn(port)
	return nil
}

func (c *Uart) SerialBaudRate() int {
	return c.mode.BaudRate
}

func (c *Uart) loadConn() serial.Port {
	// Readers operate on a stable snapshot so reconnect can swap the port
	// reference without exposing a transient nil to concurrent callers.
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Uart) storeConn(conn serial.Port) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.conn = conn
	c.connClosed = false
}

func (c *Uart) Read(b []byte) (n int, err error) {
	// Read 不需要 connClosed 检查：库的 Read 自己会判断端口是否已关闭。
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	n, err = conn.Read(b)
	if n == 0 && err == nil {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Uart) Write(b []byte) (n int, err error) {
	defer func() { c.handleErr(err) }()
	// 整个写操作持读锁，与 closeCurrent 的写锁互斥，见 closeCurrent 的说明。
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	if c.conn == nil {
		return 0, os.ErrInvalid
	}
	if c.connClosed {
		return 0, errPortClosed
	}
	return c.conn.Write(b)
}

func (c *Uart) ResetInputBuffer() (err error) {
	defer func() { c.handleErr(err) }()
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	if c.conn == nil {
		return os.ErrInvalid
	}
	if c.connClosed {
		return errPortClosed
	}
	return c.conn.ResetInputBuffer()
}

func (c *Uart) handleErr(err error) {
	// os.ErrInvalid / errPortClosed 都表示"端口还没准备好"，重连已经在进行中，无需再触发。
	if err == nil || errors.Is(err, connWrap.ErrTimeout) || errors.Is(err, os.ErrInvalid) || errors.Is(err, errPortClosed) {
		return
	}
	// Any non-timeout I/O error means the port is no longer trustworthy.
	// Reopen in the background; the current caller will observe the original error.
	go c.reopenUntilSuccess()
}
