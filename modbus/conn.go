package modbus

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/knieriem/modbus"
	"github.com/knieriem/modbus/rtu"
	"github.com/knieriem/seg"
	"github.com/knieriem/serframe"
)

type Conn struct {
	*seg.Seg
	rBuf []byte
	buf  *bytes.Buffer

	stream *serframe.Stream
	ExitC  chan error
	dev    io.ReadWriter
}

func NewNetConn(conn io.ReadWriter, segSize int, name string) *Conn {
	m := new(Conn)
	m.Seg = seg.New(conn, segSize, name)
	m.dev = conn

	m.rBuf = make([]byte, 254)
	m.buf = new(bytes.Buffer)

	m.stream = serframe.NewStream(nil,
		serframe.WithInternalReadBytesFunc(m.ReadMsg),
	)
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

func (m *Conn) Send() (adu modbus.ADU, err error) {
	buf := m.buf.Bytes()
	adu.PDUStart = 1
	adu.PDUEnd = 0
	adu.Bytes = buf

	err = m.stream.StartReception(m.rBuf)
	if err != nil {
		return adu, err
	}

	_, err = m.Write(buf)
	if err != nil {
		m.stream.CancelReception()
	}

	return adu, err
}

func (m *Conn) Receive(ctx context.Context, tMax time.Duration, _ *modbus.ExpectedRespLenSpec) (modbus.ADU, error) {
	var adu modbus.ADU
	adu.PDUStart = 1
	adu.PDUEnd = 0

	b, err := m.stream.ReadFrame(ctx, serframe.WithInitialTimeout(tMax))
	adu.Bytes = b
	if err != nil {
		err = rtu.ConvertSerframeError(err)
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
