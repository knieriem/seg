package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/knieriem/can"
	_ "github.com/knieriem/can/drv/can4linux"
	_ "github.com/knieriem/can/drv/canrpc"
	_ "github.com/knieriem/can/drv/pcan"
)

var (
	traceCAN = flag.Bool("C", false, "trace CAN msgs")

	canTxID = flag.Uint("can-txid", 0x12345678, "seg/can tx identifier")
	canRxID = flag.Uint("can-rxid", 0x18FA1900, "seg/can rx identifier")
	canExt  = flag.Bool("can-ext", true, "use extended identifiers")

	errOut = os.Stderr
)

type canRW struct {
	sendID    uint32
	receiveID uint32
	dev       can.Device
	buf, rBuf []can.Msg
}

func openCAN(devSpec string) (c io.ReadWriteCloser, err error) {
	dev, err := can.Open(devSpec)
	if err != nil {
		return
	}
	dev = &tracer{dev}
	c = &canRW{
		sendID:    uint32(*canTxID),
		receiveID: uint32(*canRxID),
		dev:       dev,
		buf:       make([]can.Msg, 64),
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
		if msg.Id == c.receiveID {
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

	if *canExt {
		m.Flags |= can.ExtFrame
	}
	m.Id = c.sendID
	m.SetData(buf)
	err = c.dev.WriteMsg(&m)
	return
}

func (c canRW) Close() error {
	return c.dev.Close()
}

type tracer struct {
	can.Device
}

func (t *tracer) Read(buf []can.Msg) (n int, err error) {
	n, err = t.Device.Read(buf)
	if *traceCAN && err == nil {
		for i := range buf[:n] {
			m := &buf[i]
			if !m.IsStatus() {
				fmt.Fprintf(errOut, "-> CAN %08X\t%s\t% x\n", m.Id, flags(m), m.Data())
			}
		}
	}
	return
}

func (t *tracer) WriteMsg(m *can.Msg) error {
	if *traceCAN && !m.IsStatus() {
		fmt.Fprintf(errOut, "<- CAN %08X\t%s\t% x\n", m.Id, flags(m), m.Data())
	}
	return t.Device.WriteMsg(m)
}

func flags(m *can.Msg) (s string) {
	if m.ExtFrame() {
		s += "X"
	}
	return
}
