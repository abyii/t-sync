package storage_clients

import (
	"errors"
	"io"
	"testing"
)

// A transport goroutine still draining the body must not observe the slice
// once revoke has returned; run with -race to catch any overlap.
func TestPartBodyRevokeFencesLingeringReader(t *testing.T) {
	for range 200 {
		data := make([]byte, 1<<16)
		body := newPartBody(data)
		done := make(chan error, 1)
		go func() {
			buf := make([]byte, 512)
			for {
				if _, err := body.Read(buf); err != nil {
					done <- err
					return
				}
				if _, err := body.Seek(0, io.SeekStart); err != nil {
					done <- err
					return
				}
			}
		}()
		body.revoke()
		for i := range data { // the caller reuses the buffer
			data[i] = 0xff
		}
		if err := <-done; !errors.Is(err, errBodyRevoked) {
			t.Fatalf("lingering reader ended with %v, want errBodyRevoked", err)
		}
	}
}
