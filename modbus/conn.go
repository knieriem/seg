package modbus

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/knieriem/modbus"
	"github.com/knieriem/modbus/rtu"
	"github.com/knieriem/seg"
)

type Conn struct {
	*seg.Seg
	buf *bytes.Buffer

	readMgr *rtu.ReadMgr
	ExitC   chan error
	dev     io.ReadWriter
}

func NewNetConn(conn io.ReadWriter, segSize int, name string) *Conn {
	m := new(Conn)
	m.Seg = seg.New(conn, segSize, name)
	m.dev = conn

	m.buf = new(bytes.Buffer)

	m.ExitC = make(chan error, 1)
	m.readMgr = rtu.NewReadMgr(m.ReadMsg, m.ExitC)

	return m
}

func (m *Conn) Name() string {
	return "seg"
}

func (m *Conn) Device() interface{} {
	return m.dev
}

func (m *Conn) MsgWriter() io.Writer {
	m.buf.Reset()
	return m.buf
}

func (m *Conn) Send() (buf []byte, err error) {
	err = m.readMgr.Start()
	if err != nil {
		return
	}
	buf = m.buf.Bytes()
	_, err = m.Write(buf)
	if err != nil {
		m.readMgr.Cancel()
	}
	return
}

func (m *Conn) Receive(ctx context.Context, tMax time.Duration, _ *modbus.ExpectedRespLenSpec) (modbus.ADU, error) {
	var adu modbus.ADU
	adu.PDUStart = 1
	adu.PDUEnd = 0

	b, err := m.readMgr.Read(ctx, tMax, 0)
	adu.Bytes = b
	if err != nil {
		if err == modbus.ErrTimeout && m.Seg.PrevWriteMultiple {
			m.Seg.WriteDelay += 5 * time.Millisecond
		}
		return adu, err
	}
	n := len(adu.Bytes)
	if n < 2 {
		err = modbus.NewInvalidLen(modbus.MsgContextADU, n, 2)
		return adu, err
	}
	return adu, nil
}
