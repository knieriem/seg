package segcan

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/knieriem/can"
	"github.com/knieriem/modbus/netconn"
)

var (
	errOut = os.Stderr
)

type canRW struct {
	*netconn.Conf
	dev       can.Device
	buf, rBuf []can.Msg
}

func openCAN(cf *netconn.Conf) (c *canRW, err error) {
	devSpec := cf.Device
	if len(cf.Options) != 0 {
		devSpec += "," + strings.Join(cf.Options, ",")
	}

	dev, err := can.Open(devSpec)
	if err != nil {
		return
	}
	c = &canRW{
		Conf: cf,
		dev:  dev,
		buf:  make([]can.Msg, 64),
	}
	return
}

func (c *canRW) Read(buf []byte) (n int, err error) {
	for {
		if len(c.rBuf) == 0 {
			err = c.fillBuf()
			if err != nil {
				return
			}
		}
		msg := &c.rBuf[0]
		c.rBuf = c.rBuf[1:]
		if msg.IsStatus() {
			continue
		}
		if c.Rxid.Extframe && !msg.ExtFrame() {
			continue
		}
		if !c.Rxid.Extframe && msg.ExtFrame() {
			continue
		}
		if msg.Id == c.Rxid.ID {
			copy(buf, msg.Data[:])
			n = msg.Len
			break
		}
	}
	return
}

func (c *canRW) fillBuf() (err error) {
	n, err := c.dev.Read(c.buf)
	if err != nil {
		return
	}
	if n == 0 {
		err = errors.New("zero messages in CAN buffer")
	}
	c.rBuf = c.buf[:n]
	return
}

func (c *canRW) Write(buf []byte) (n int, err error) {
	var m can.Msg

	if c.Txid.Extframe {
		m.Flags |= can.ExtFrame
	}
	m.Id = c.Txid.ID
	copy(m.Data[:], buf)
	n = len(buf)
	m.Len = n
	err = c.dev.WriteMsg(&m)
	return
}

func (c *canRW) Close() error {
	return c.dev.Close()
}

type WrapFunc func(dev can.Device, devID string) can.Device

type deviceWrapper struct {
	mu sync.Mutex
	f  WrapFunc
}

func (dw *deviceWrapper) wrap(dev can.Device, id string) can.Device {
	dw.mu.Lock()
	if dw.f != nil {
		dev = dw.f(dev, id)
	}
	dw.mu.Unlock()
	return dev
}

func SetDeviceWrapper(f WrapFunc) {
	dw := &devWrapper
	dw.mu.Lock()
	dw.f = f
	dw.mu.Unlock()
}

var devWrapper deviceWrapper

// CANTracer wraps a simple message tracer around a can.Device,
// implementing a can.Device itself.
type CANTracer struct {
	can.Device
	w       io.Writer
	enabled bool
	mu      sync.Mutex
}

func NewCANTracer(w io.Writer, dev can.Device) *CANTracer {
	return &CANTracer{w: w, Device: dev}
}

func (t *CANTracer) SetEnabled(e bool) {
	t.mu.Lock()
	t.enabled = e
	t.mu.Unlock()
}

func (t *CANTracer) Read(buf []can.Msg) (n int, err error) {
	n, err = t.Device.Read(buf)
	if t.enabled && err == nil {
		for i := range buf[:n] {
			m := &buf[i]
			if !m.IsStatus() {
				fmt.Fprintf(errOut, "-> CAN %0*X\t% x\n", idDigits(m), m.Id, m.Data[:m.Len])
			} else {
				fmt.Fprintf(errOut, "-> CAN %s\n", flags(m))
			}
		}
	}
	return
}

func (t *CANTracer) WriteMsg(m *can.Msg) error {
	if t.enabled && !m.IsStatus() {
		fmt.Fprintf(errOut, "<- CAN %0*X\t% x\n", idDigits(m), m.Id, m.Data[:m.Len])
	}
	return t.Device.WriteMsg(m)
}

func idDigits(m *can.Msg) int {
	if m.ExtFrame() {
		return 8
	}
	return 3
}

func flags(m *can.Msg) (s string) {
	if m.Test(can.ErrorActive) {
		s += "ERROR ACTIVE"
	}
	if m.Test(can.ErrorPassive) {
		s += "ERROR PASSIVE"
	}
	if m.Test(can.BusOff) {
		s += "BUSOFF"
	}
	return
}

var canAdapters = netconn.InterfaceGroup{
	Name:       "CAN adapters",
	Interfaces: canInterfaces,
	Type:       "can",
}

func canInterfaces() (list []netconn.Interface) {
	for _, name := range can.Scan() {
		list = append(list, netconn.Interface{
			Name: name.String(),
			Desc: name.Format("<OMIT ID>", ", ", ""),
			Elem: name,
		})
	}
	return
}
