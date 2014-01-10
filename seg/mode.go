package seg

import (
	"bytes"
	"io"
	"time"

	"modbus"
	"modbus/rtu"
)

type Mode struct {
	*Seg
	buf *bytes.Buffer

	readMgr *rtu.ReadMgr
	ExitC   chan int
}

func NewTransmissionMode(conn io.ReadWriter, segSize int, name string) *Mode {
	m := new(Mode)
	m.Seg = New(conn, segSize, name)

	m.buf = new(bytes.Buffer)

	m.ExitC = make(chan int, 1)
	m.readMgr = rtu.NewReadMgr(m.ReadMsg, m.ExitC)

	return m
}

func (m *Mode) Name() string {
	return "seg"
}

func (m *Mode) MsgWriter() io.Writer {
	m.buf.Reset()
	return m.buf
}

func (m *Mode) Send() (buf []byte, err error) {
	m.readMgr.Start()
	buf = m.buf.Bytes()
	_, err = m.Write(buf)
	return
}

func (m *Mode) Receive(tMax time.Duration) (buf, msg []byte, err error) {
	buf, err = m.readMgr.Read(tMax, 0)
	if err != nil {
		return
	}
	if len(buf) < 2 {
		err = modbus.ErrMsgTooShort
		return
	}
	msg = buf
	return
}
