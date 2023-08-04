package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/knieriem/hash/crc16"
	"github.com/knieriem/modbus"
	"github.com/knieriem/seg"
	"github.com/knieriem/serframe"
)

const (
	cmdGwCatch = 1
)

var (
	trace            = flag.Bool("D", false, "trace SEG messages")
	fakeMultipleAcks = flag.Bool("multi-acks", false, "fake multiple catch ACKs")
	frameSize        = flag.Int("frame-size", 8, "size of one SEG frame")

	crctab = crc16.MakeTable(crc16.IBMCRC)

	canDev = flag.String("can", "", "use the specified can device")
)

var fakeMultiAcks bool

func main() {
	var c io.ReadWriter
	flag.Parse()

	f, err := runArgs()
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	if *canDev != "" {
		c, err = openCAN(*canDev)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		c = &conn{os.Stdin, os.Stdout}
	}

	tm := seg.New(c, *frameSize, "can")
	if *trace {
		tm.Tracef = func(format string, a ...interface{}) {
			fmt.Fprintf(os.Stderr, format, a...)
		}
	}

	stream := serframe.NewStream(f,
		serframe.WithInternalBufSize(512),
	)

	go func() {
		buf := make([]byte, 1024)
		for {
			stream.StartReception(buf)
			data, err := stream.ReadFrame(context.TODO(),
				serframe.WithInitialTimeout(3*time.Second),
				serframe.WithInterByteTimeout(30*time.Millisecond),
			)
			if err != nil {
				if err == modbus.ErrTimeout {
					continue
				}
				log.Fatal(err)
			}
			_, err = tm.Write(data[:len(data)-2])
			if fakeMultiAcks {
				for i := 0; i < 3; i++ {
					time.Sleep(50 * time.Millisecond)
					_, err = tm.Write(data[:len(data)-2])
				}
				fakeMultiAcks = false
			}
			if err != nil {
				log.Fatal(err)
			}
		}

	}()
	for {
		buf, err := tm.ReadMsg()
		if err != nil {
			log.Fatal(err)
		}
		crc := crc16.Checksum(buf, crctab)
		buf = append(buf, byte(crc&0xFF), byte((crc>>8)&0xFF))
		if buf[2] == cmdGwCatch {
			if *fakeMultipleAcks {
				fakeMultiAcks = true
			}
		}
		_, err = f.Write(buf)
		if err != nil {
			log.Fatal(err)
		}
	}
}

type conn struct {
	io.Reader
	io.WriteCloser
}

func runArgs() (f io.ReadWriteCloser, err error) {
	cmd := exec.Command(flag.Arg(0), flag.Args()[1:]...)

	w, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	r, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return
	}

	f = &conn{r, w}
	return
}
