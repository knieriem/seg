package segcan

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
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

	fdMode bool
	txID   uint32
	rxID   uint32
	txExt  bool
	rxExt  bool
	segMax int
}

func openCAN(cf *netconn.Conf) (*canRW, error) {
	devSpec := cf.Device

	var c canRW
	err := decodeOptions(&c, cf)
	if err != nil {
		return nil, err
	}

	if len(cf.Options) != 0 {
		devSpec += "," + strings.Join(cf.Options, ",")
	}
	if c.rxExt {
		devSpec += "," + fmt.Sprintf("f%08x", c.rxID)
	} else {
		devSpec += "," + fmt.Sprintf("f%03x", c.rxID)
	}

	dev, err := can.Open(devSpec)
	if err != nil {
		return nil, err
	}

	c.Conf = cf
	c.dev = dev
	c.buf = make([]can.Msg, 64)
	return &c, nil
}

func decodeOptions(c *canRW, cf *netconn.Conf) error {
	canOpts := cf.Options[:0]

	c.segMax = 8

	// Use defaults from deprecated, explicit fields
	c.txID = cf.Txid.ID
	c.txExt = cf.Txid.Extframe
	c.rxID = cf.Rxid.ID
	c.rxExt = cf.Rxid.Extframe

	for i, o := range cf.Options {
		stem, ok := strings.CutPrefix(o, "seg.")
		if !ok {
			if len(canOpts) == i {
				canOpts = canOpts[:i+1]
				continue
			}
			canOpts = append(canOpts, o)
			continue
		}
		if len(stem) < 2 {
			return errors.New("seg: syntax error")
		}
		i := strings.IndexByte(stem, ':')
		if i == -1 {
			if stem == "fd" {
				c.fdMode = true
				continue
			}
			return errors.New("seg: missing colon")
		}

		key, val := stem[:i], stem[i+1:]
		switch key {
		case "max":
			i, err := strconv.Atoi(val)
			if err != nil {
				return err
			}
			if !slices.Contains(can.ValidFDSizes, i) {
				return fmt.Errorf("seg.max: invalid value: %q", val)
			}
			c.segMax = i
			c.fdMode = true

		case "tx":
			id, ext, err := parseID(val)
			if err != nil {
				return err
			}
			c.txID = id
			c.txExt = ext

		case "rx":
			id, ext, err := parseID(val)
			if err != nil {
				return err
			}
			c.rxID = id
			c.rxExt = ext

		default:
			return fmt.Errorf("seg: invalid key: %q", key)
		}
	}

	if c.fdMode && c.segMax == 8 {
		// Ensure default if seg.max has not been set
		c.segMax = 64
	}
	if c.txID == 0 {
		return errors.New("seg: missing tx id")
	}
	if c.rxID == 0 {
		return errors.New("seg: missing rx id")
	}
	cf.Options = canOpts
	return nil
}

func parseID(v string) (id uint32, extFrame bool, err error) {
	n := len(v) - strings.Count(v, "_")
	if n > 3 {
		extFrame = true
	}
	u, err := strconv.ParseUint("0x"+v, 0, 32)
	if err != nil {
		return 0, false, err
	}
	return uint32(u), extFrame, nil
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
		if c.rxExt != msg.ExtFrame() {
			continue
		}
		if msg.Id == c.rxID {
			data := msg.Data()
			copy(buf, data)
			n = len(data)
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
	var data can.PlainData

	if c.txExt {
		m.Flags |= can.ExtFrame
	}
	m.Id = c.txID
	data = buf
	m.Attach(&data)
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
				fmt.Fprintf(errOut, "-> CAN %0*X\t% x\n", idDigits(m), m.Id, m.Data())
			} else {
				fmt.Fprintf(errOut, "-> CAN %s\n", flags(m))
			}
		}
	}
	return
}

func (t *CANTracer) WriteMsg(m *can.Msg) error {
	if t.enabled && !m.IsStatus() {
		fmt.Fprintf(errOut, "<- CAN %0*X\t% x\n", idDigits(m), m.Id, m.Data())
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
