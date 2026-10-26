package main

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeUploader struct {
	mu        sync.Mutex
	failPart  int // UploadPart fails for this part number; 0 = never
	parts     map[int][]byte
	put       []byte
	completed []int
	aborted   bool
}

func (f *fakeUploader) Initiate(context.Context) (string, error) { return "upload-1", nil }

func (f *fakeUploader) UploadPart(_ context.Context, _ string, n int, data []byte) (string, error) {
	if n == f.failPart {
		return "", errors.New("boom")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.parts == nil {
		f.parts = map[int][]byte{}
	}
	f.parts[n] = bytes.Clone(data) // snapshot: detects later writes into the part
	return "etag", nil
}

func (f *fakeUploader) Complete(_ context.Context, _ string, etags map[int]string) error {
	for n := range etags {
		f.completed = append(f.completed, n)
	}
	slices.Sort(f.completed)
	return nil
}

func (f *fakeUploader) Abort(context.Context, string) error { f.aborted = true; return nil }

func (f *fakeUploader) PutObject(_ context.Context, data []byte) error {
	f.put = bytes.Clone(data)
	return nil
}

// runPipeline wires a channelWriter to uploadToObjectStorage exactly like main.
func runPipeline(t *testing.T, u ObjectStorageUploader, payload []byte, partSize, concurrency int) (writeErr, uploadErr error) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	ch := make(chan Part)
	cw := NewChannelWriter(ctx, ch, partSize)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := uploadToObjectStorage(ctx, u, ch, partSize, concurrency); err != nil {
			uploadErr = err
			cancel(err)
		}
	}()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		// Small writes, like the zip writer's, so parts span many Write calls.
		for p := payload; len(p) > 0 && writeErr == nil; {
			n := min(len(p), 7)
			_, writeErr = cw.Write(p[:n])
			p = p[n:]
		}
		if writeErr == nil {
			writeErr = cw.Close()
		}
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("producer blocked: upload failure did not unblock the writer")
	}
	<-done
	return writeErr, uploadErr
}

func TestUploadFailureUnblocksProducer(t *testing.T) {
	u := &fakeUploader{failPart: 2}
	payload := bytes.Repeat([]byte("x"), 100*16) // 100 parts of 16 bytes, channel holds 2
	writeErr, uploadErr := runPipeline(t, u, payload, 16, 2)
	if uploadErr == nil {
		t.Fatal("expected upload error")
	}
	if writeErr == nil {
		t.Fatal("expected producer write to fail after upload failure")
	}
	if !u.aborted {
		t.Fatal("expected multipart upload to be aborted")
	}
	if u.completed != nil {
		t.Fatal("failed upload must not be completed")
	}
}

func TestMultipartReassemblesPayload(t *testing.T) {
	u := &fakeUploader{}
	payload := make([]byte, 16*5+3)
	for i := range payload {
		payload[i] = byte(i)
	}
	writeErr, uploadErr := runPipeline(t, u, payload, 16, 2)
	if writeErr != nil || uploadErr != nil {
		t.Fatalf("write=%v upload=%v", writeErr, uploadErr)
	}
	if want := []int{1, 2, 3, 4, 5, 6}; !slices.Equal(u.completed, want) {
		t.Fatalf("completed parts %v, want %v", u.completed, want)
	}
	var got []byte
	for _, n := range u.completed {
		got = append(got, u.parts[n]...)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("reassembled parts differ from payload")
	}
}

func TestSmallPayloadUsesPutObject(t *testing.T) {
	u := &fakeUploader{}
	payload := []byte("short archive")
	writeErr, uploadErr := runPipeline(t, u, payload, 16, 2)
	if writeErr != nil || uploadErr != nil {
		t.Fatalf("write=%v upload=%v", writeErr, uploadErr)
	}
	if !bytes.Equal(u.put, payload) || u.parts != nil {
		t.Fatalf("expected single PutObject, got put=%q parts=%d", u.put, len(u.parts))
	}
}

func TestExactlyOnePartUsesMultipart(t *testing.T) {
	u := &fakeUploader{}
	payload := bytes.Repeat([]byte("y"), 16)
	writeErr, uploadErr := runPipeline(t, u, payload, 16, 2)
	if writeErr != nil || uploadErr != nil {
		t.Fatalf("write=%v upload=%v", writeErr, uploadErr)
	}
	if !slices.Equal(u.completed, []int{1}) || !bytes.Equal(u.parts[1], payload) {
		t.Fatalf("expected single-part multipart upload, got completed=%v", u.completed)
	}
}

// gatedUploader blocks every UploadPart until gate is closed.
type gatedUploader struct {
	fakeUploader
	gate    chan struct{}
	started chan int
}

func (g *gatedUploader) UploadPart(ctx context.Context, id string, n int, data []byte) (string, error) {
	g.started <- n
	<-g.gate
	return g.fakeUploader.UploadPart(ctx, id, n, data)
}

func TestPartsTakenFromProducerAreBoundedByConcurrency(t *testing.T) {
	const concurrency, partSize = 2, 4
	g := &gatedUploader{gate: make(chan struct{}), started: make(chan int, 16)}
	ch := make(chan Part)
	done := make(chan error, 1)
	go func() { done <- uploadToObjectStorage(context.Background(), g, ch, partSize, concurrency) }()

	var sent atomic.Int32
	go func() {
		for n := 1; n <= 10; n++ {
			ch <- Part{Number: n, Data: bytes.Repeat([]byte{byte(n)}, partSize)}
			sent.Add(1)
		}
		close(ch)
	}()

	for range concurrency {
		<-g.started
	}
	time.Sleep(100 * time.Millisecond) // give an unbounded consumer time to over-pull
	if got := sent.Load(); got != concurrency {
		t.Fatalf("uploader took %d parts while %d uploads were blocked; want %d", got, concurrency, concurrency)
	}

	close(g.gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}; !slices.Equal(g.completed, want) {
		t.Fatalf("completed %v, want %v", g.completed, want)
	}
}

// verifyingUploader checks each part's bytes before and after a pause, so a
// buffer shared by two in-flight parts, or written to while uploading, fails.
type verifyingUploader struct {
	fakeUploader
	want     []byte
	partSize int
	bad      atomic.Int32
}

func (v *verifyingUploader) UploadPart(ctx context.Context, id string, n int, data []byte) (string, error) {
	start := (n - 1) * v.partSize
	want := v.want[start:min(start+v.partSize, len(v.want))]
	if !bytes.Equal(data, want) {
		v.bad.Add(1)
	}
	time.Sleep(time.Duration(n%7) * 100 * time.Microsecond)
	if !bytes.Equal(data, want) {
		v.bad.Add(1)
	}
	return v.fakeUploader.UploadPart(ctx, id, n, data)
}

func TestPartsNeverShareBuffers(t *testing.T) {
	const partSize = 64
	payload := make([]byte, partSize*500+17)
	for i := range payload {
		payload[i] = byte(i*31 + i>>8)
	}
	v := &verifyingUploader{want: payload, partSize: partSize}
	writeErr, uploadErr := runPipeline(t, v, payload, partSize, 8)
	if writeErr != nil || uploadErr != nil {
		t.Fatalf("write=%v upload=%v", writeErr, uploadErr)
	}
	if n := v.bad.Load(); n != 0 {
		t.Fatalf("%d part content mismatches during upload", n)
	}
	var got []byte
	for _, n := range v.completed {
		got = append(got, v.parts[n]...)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("reassembled parts differ from payload")
	}
}
