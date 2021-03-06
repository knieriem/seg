package modbus

import (
	"bytes"
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

func (m *Conn) SetIntrC(c <-chan error) {
	m.readMgr.IntrC = c
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

func (m *Conn) Receive(tMax time.Duration, _ *modbus.ExpectedRespLenSpec) (buf, msg []byte, err error) {
	buf, err = m.readMgr.Read(tMax, 0)
	if err != nil {
		if err == modbus.ErrTimeout && m.Seg.PrevWriteMultiple {
			m.Seg.WriteDelay += 5 * time.Millisecond
		}
		return
	}
	if len(buf) < 2 {
		err = modbus.NewInvalidMsgLen(len(buf), 2)
		return
	}
	msg = buf
	return
}
