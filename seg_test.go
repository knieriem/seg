package seg_test

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/knieriem/seg" // Replace with your actual package import path
)

// packetPipe simulates a packet-oriented datagram bus preserving discrete frame boundaries.
type packetPipe struct {
	packets chan []byte
}

func newPacketPipe(bufferSize int) *packetPipe {
	return &packetPipe{
		packets: make(chan []byte, bufferSize),
	}
}

func (p *packetPipe) Write(b []byte) (int, error) {
	pkt := make([]byte, len(b))
	copy(pkt, b)
	p.packets <- pkt
	return len(b), nil
}

func (p *packetPipe) Read(b []byte) (int, error) {
	pkt, ok := <-p.packets
	if !ok {
		return 0, io.EOF
	}
	copy(b, pkt)
	return len(pkt), nil
}

// generateTestBuffer creates a deterministic byte sequence of specified length.
func generateTestBuffer(length int) []byte {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = byte((i*31 + 17) % 256) // Non-repeating deterministic pattern
	}
	return buf
}

// TestDefaultCANStrategy_AllLengths tests all payload lengths from 1 to 250 bytes
// against the default CAN 2.0 (7-byte chunking) strategy.
func TestDefaultCANStrategy_AllLengths(t *testing.T) {
	const maxLen = 250
	masterPayload := generateTestBuffer(maxLen)

	for msgLen := 1; msgLen <= maxLen; msgLen++ {
		original := masterPayload[:msgLen]

		// 250 frames buffer is plenty to prevent blocking
		pipe := newPacketPipe(250)
		sender := seg.New(pipe, 8, "senderCAN")
		receiver := seg.New(pipe, 8, "receiverCAN")

		var wg sync.WaitGroup
		var received []byte
		var readErr error

		wg.Add(1)
		go func() {
			defer wg.Done()
			received, readErr = receiver.ReadMsg()
		}()

		nWritten, err := sender.Write(original)
		if err != nil {
			t.Fatalf("[Len %d] Write failed: %v", msgLen, err)
		}
		if nWritten != len(original) {
			t.Fatalf("[Len %d] Expected %d bytes written, got %d", msgLen, len(original), nWritten)
		}

		wg.Wait()

		if readErr != nil {
			t.Fatalf("[Len %d] ReadMsg failed: %v", msgLen, readErr)
		}

		if !bytes.Equal(original, received) {
			t.Fatalf("[Len %d] CAN 2.0 payload mismatch!\nGot len:  %d\nWant len: %d", msgLen, len(received), len(original))
		}
	}
}

// TestCANFDStrategy_AllLengths tests all payload lengths from 1 to 500 bytes
// against the CAN FD (discrete step, zero-padding) strategy.
func TestCANFDStrategy_AllLengths(t *testing.T) {
	const maxLen = 500
	masterPayload := generateTestBuffer(maxLen)

	for msgLen := 1; msgLen <= maxLen; msgLen++ {
		original := masterPayload[:msgLen]

		pipe := newPacketPipe(500)
		maxSeg := 64
		sender := seg.New(pipe, maxSeg, "senderFD", seg.WithStrategy(seg.CANFDStrategy(maxSeg)))
		receiver := seg.New(pipe, maxSeg, "receiverFD", seg.WithStrategy(seg.CANFDStrategy(maxSeg)))

		var wg sync.WaitGroup
		var received []byte
		var readErr error

		wg.Add(1)
		go func() {
			defer wg.Done()
			received, readErr = receiver.ReadMsg()
		}()

		nWritten, err := sender.Write(original)
		if err != nil {
			t.Fatalf("[Len %d] CAN FD Write failed: %v", msgLen, err)
		}
		if nWritten != len(original) {
			t.Fatalf("[Len %d] Expected %d bytes written, got %d", msgLen, len(original), nWritten)
		}

		wg.Wait()

		if readErr != nil {
			t.Fatalf("[Len %d] CAN FD ReadMsg failed: %v", msgLen, readErr)
		}

		if !bytes.Equal(original, received) {
			t.Fatalf("[Len %d] CAN FD payload mismatch!\nGot len:  %d\nWant len: %d", msgLen, len(received), len(original))
		}
	}
}
