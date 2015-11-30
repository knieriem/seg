package seg

import (
	"io"
)

type Seg struct {
	conn io.ReadWriter
	name string

	rMsg []byte
	rBuf []byte
	nErr int
	wBuf []byte

	Tracef func(format string, a ...interface{})
}

func New(conn io.ReadWriter, size int, name string) *Seg {
	s := new(Seg)
	s.conn = conn
	s.name = name
	s.rBuf = make([]byte, size)
	s.wBuf = make([]byte, size)
	return s
}

const (
	startBit byte = 1 << 7
)
const (
	expectStartOrSingle = iota
	expectContinuation
)

func (s *Seg) ReadMsg() ([]byte, error) {
	var iCont, nCont byte

	s.rMsg = s.rMsg[:0]
	state := expectStartOrSingle
	b := s.rBuf
	for {
		n, err := s.conn.Read(b)
		if err != nil {
			return nil, err
		}
		if n < 1 {
			s.trace("->", "??", []byte{})
			state = expectStartOrSingle
			s.nErr++
			continue
		}
		c := b[0]
		frame := b[:n]
		switch state {
		case expectStartOrSingle:
			if c & ^startBit == 0 {
				// single message
				s.trace("->", "single", frame)
				return b[1:n], nil
			}
			if (c & startBit) == 0 {
				// no start frame, skip
				s.nErr++
				s.trace("->", "??", frame)
				continue
			}
			state = expectContinuation
			iCont = 0
			nCont = c ^ startBit
			s.trace("->", "start", frame)

		case expectContinuation:
			if (c&startBit) != 0 || c != iCont {
				state = expectStartOrSingle
				s.nErr++
				s.trace("->", "??", frame)
				continue
			}
			s.trace("->", "cont", frame)
		}
		s.rMsg = append(s.rMsg, b[1:n]...)
		if iCont == nCont {
			break
		}
		iCont++
	}
	return s.rMsg, nil
}

func (s *Seg) Write(msg []byte) (nMsg int, err error) {
	var event string
	var iCont byte
	b := s.wBuf
	nb := len(b) - 1
	for {
		n := len(msg)
		if n <= nb {
			// its the first (single) or last message
			if iCont == 0 {
				b[0] = 0
				event = "single"
			} else {
				b[0] = iCont
				event = "cont"
			}
			copy(b[1:], msg)
			frame := b[:n+1]
			_, err = s.conn.Write(frame)
			s.trace("<-", event, frame)
			nMsg += n
			return
		}
		if iCont == 0 {
			b[0] = byte((n-1)/nb) | startBit
			event = "start"
		} else {
			b[0] = iCont
			event = "cont"
		}
		copy(b[1:], msg)
		msg = msg[nb:]
		_, err = s.conn.Write(b)
		s.trace("<-", event, b)
		if err != nil {
			return
		}
		nMsg += nb
		iCont++
	}
}

func (s *Seg) trace(dir, event string, frame []byte) {
	if s.Tracef == nil {
		return
	}
	s.Tracef("%s seg/%s %s % x\n", dir, s.name, event, frame)
}
