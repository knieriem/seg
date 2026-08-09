package seg

import (
	"io"
	"iter"
	"time"
)

var DefaultWriteDelay time.Duration = 0

// Strategy returns the total frame count n and an iterator yielding the data capacity for each frame.
type Strategy func(msgLen int) (nFrames int, seq iter.Seq[int])

type Option func(*Seg)

// WithStrategy sets a custom framing strategy.
func WithStrategy(st Strategy) Option {
	return func(s *Seg) {
		s.strategy = st
	}
}

// defaultStrategy creates a uniform greedy strategy based on max frame capacity (segSize - 1 control byte).
func defaultStrategy(segSize int) Strategy {
	maxCap := segSize - 1
	if maxCap < 1 {
		maxCap = 1
	}

	return func(msgLen int) (int, iter.Seq[int]) {
		nFrames := (msgLen + maxCap - 1) / maxCap

		seq := func(yield func(int) bool) {
			rem := msgLen
			for rem > 0 {
				cap := maxCap
				if rem < maxCap {
					cap = rem
				}
				if !yield(cap) {
					return
				}
				rem -= cap
			}
		}

		return nFrames, seq
	}
}

type Seg struct {
	conn io.ReadWriter
	name string
	rMsg []byte
	rBuf []byte
	nErr int
	wBuf []byte

	strategy Strategy

	PrevWriteMultiple bool
	WriteDelay        time.Duration
	Tracef            func(format string, a ...any)
}

func New(conn io.ReadWriter, size int, name string, opts ...Option) *Seg {
	s := &Seg{
		conn:       conn,
		name:       name,
		rBuf:       make([]byte, size),
		wBuf:       make([]byte, size),
		strategy:   defaultStrategy(size),
		WriteDelay: DefaultWriteDelay,
	}

	for _, opt := range opts {
		opt(s)
	}
	return s
}

const startBit byte = 1 << 7

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
			if c&startBit == 0 {
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
			if c&startBit != 0 || c != iCont {
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
	if len(msg) == 0 {
		return 0, nil
	}

	totalFrames, seq := s.strategy(len(msg))
	s.PrevWriteMultiple = totalFrames > 1

	msgPos := 0
	i := 0
	for dataCap := range seq {
		frameLen := dataCap + 1
		b := s.wBuf[:frameLen]

		var event string
		if totalFrames == 1 {
			b[0] = startBit
			event = "single"
		} else if i == 0 {
			b[0] = byte(totalFrames-1) | startBit
			event = "start"
		} else {
			b[0] = byte(i)
			event = "cont"
		}

		copy(b[1:], msg[msgPos:msgPos+dataCap])
		_, err = s.conn.Write(b)
		s.trace("<-", event, b)
		if err != nil {
			return nMsg, err
		}

		msgPos += dataCap
		nMsg += dataCap

		if s.WriteDelay != 0 && i < totalFrames-1 {
			time.Sleep(s.WriteDelay)
		}
		i++
	}

	return nMsg, nil
}

func (s *Seg) trace(dir, event string, frame []byte) {
	if s.Tracef == nil {
		return
	}
	s.Tracef("%s seg/%s %s % x\n", dir, s.name, event, frame)
}
