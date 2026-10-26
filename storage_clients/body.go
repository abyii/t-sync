package storage_clients

import (
	"bytes"
	"errors"
	"sync"
)

var errBodyRevoked = errors.New("request body read after the request completed")

// partBody is a request body over a recycled part buffer. net/http may keep
// reading a body after RoundTrip returns (e.g. after an early error response),
// so revoke fences off any such late reader before the buffer is reused.
type partBody struct {
	mu sync.Mutex
	r  *bytes.Reader // nil once revoked
}

func newPartBody(data []byte) *partBody { return &partBody{r: bytes.NewReader(data)} }

func (b *partBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.r == nil {
		return 0, errBodyRevoked
	}
	return b.r.Read(p)
}

// Seek lets the SDKs measure the body and rewind it for their own retries.
func (b *partBody) Seek(offset int64, whence int) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.r == nil {
		return 0, errBodyRevoked
	}
	return b.r.Seek(offset, whence)
}

func (b *partBody) Close() error { return nil }

// revoke waits for an in-flight Read to finish, then fails all later ones.
func (b *partBody) revoke() {
	b.mu.Lock()
	b.r = nil
	b.mu.Unlock()
}
