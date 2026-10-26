package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"t-sync/storage_clients"
)

// Part represents a chunk of the file to be uploaded.
type Part struct {
	Number int
	Data   []byte
}

// ObjectStorageUploader defines the interface for a multipart upload.
//
// UploadPart and PutObject must not read or retain data after they return,
// whatever the outcome: the caller recycles the buffer for later parts.
type ObjectStorageUploader interface {
	Initiate(ctx context.Context) (uploadID string, err error)
	UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (etag string, err error)
	Complete(ctx context.Context, uploadID string, etags map[int]string) error
	Abort(ctx context.Context, uploadID string) error

	PutObject(ctx context.Context, data []byte) error
}

// NewUploader is a factory function that returns an uploader based on the provider.
func NewUploader(details *DestDetails, authType string) (ObjectStorageUploader, error) {
	uploader, err := storage_clients.GetUploader(details.Provider, details.Bucket, details.Key, authType, details.Namespace)
	if err != nil {
		return nil, err
	}

	// Type assertion to the interface
	if u, ok := uploader.(ObjectStorageUploader); ok {
		return u, nil
	}

	return nil, fmt.Errorf("internal error: registered uploader for '%s' does not implement ObjectStorageUploader interface", details.Provider)
}

// partPool recycles part buffers once their upload finishes, so a long archive
// does not allocate a fresh part-sized buffer for every part.
var partPool sync.Pool // holds *[]byte

func getPartBuffer(size int) []byte {
	if v := partPool.Get(); v != nil {
		if b := *v.(*[]byte); cap(b) >= size {
			return b[:0]
		}
	}
	return make([]byte, 0, size)
}

// releasePartBuffer returns a part buffer to the pool. The caller must not use
// the buffer afterwards.
func releasePartBuffer(b []byte) {
	b = b[:0]
	partPool.Put(&b)
}

// channelWriter is an io.Writer that slices its input into fixed-size parts
// and sends them on a channel. Every part except the last is exactly partSize
// bytes; the last part (sent by Close) is shorter and non-empty.
type channelWriter struct {
	ctx        context.Context
	partChan   chan<- Part
	buf        []byte
	partSize   int
	partNumber int
	closed     bool
}

// NewChannelWriter creates a new channelWriter. Writes fail with the
// cancellation cause of ctx once it is done, so a failed consumer can never
// leave the producer blocked on a full channel.
func NewChannelWriter(ctx context.Context, partChan chan<- Part, partSize int) *channelWriter {
	return &channelWriter{
		ctx:        ctx,
		partChan:   partChan,
		partSize:   partSize,
		partNumber: 1,
	}
}

// Write implements the io.Writer interface.
func (cw *channelWriter) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		if cw.buf == nil {
			cw.buf = getPartBuffer(cw.partSize)
		}
		k := copy(cw.buf[len(cw.buf):cw.partSize], p)
		cw.buf = cw.buf[:len(cw.buf)+k]
		p = p[k:]
		n += k
		if len(cw.buf) == cw.partSize {
			if err := cw.send(); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

func (cw *channelWriter) send() error {
	select {
	case cw.partChan <- Part{Number: cw.partNumber, Data: cw.buf}:
		cw.partNumber++
		cw.buf = nil
		return nil
	case <-cw.ctx.Done():
		return context.Cause(cw.ctx)
	}
}

// Close flushes any remaining data in the buffer as the last part and closes the channel.
func (cw *channelWriter) Close() error {
	if cw.closed {
		return nil
	}
	cw.closed = true
	defer close(cw.partChan)
	if len(cw.buf) > 0 {
		return cw.send()
	}
	return nil
}

func uploadToObjectStorage(parentCtx context.Context, uploader ObjectStorageUploader, partChan <-chan Part, partSize int, concurrency int) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	part1, ok := <-partChan
	if !ok {
		log.Println("No data to upload.")
		return nil
	}

	if len(part1.Data) < partSize {
		log.Printf("Total size is %d bytes. Using simple upload.", len(part1.Data))
		err := uploader.PutObject(ctx, part1.Data)
		releasePartBuffer(part1.Data)
		if err != nil {
			return fmt.Errorf("failed to put object: %v", err)
		}
		if _, more := <-partChan; more {
			return fmt.Errorf("internal error: received data after the final part")
		}
		fmt.Println("Upload completed successfully.")
		return nil
	}

	log.Println("Data size is large, using multipart upload.")
	uploadID, err := uploader.Initiate(ctx)
	if err != nil {
		releasePartBuffer(part1.Data)
		return fmt.Errorf("failed to initiate multipart upload: %v", err)
	}

	etags := make(map[int]string)
	var uploadErr error
	var mu sync.Mutex
	var workerWg sync.WaitGroup

	sem := make(chan struct{}, concurrency)

	// uploadPart is a closure that handles uploading a single part
	uploadPart := func(part Part) {
		// Check for cancellation before proceeding
		if ctx.Err() != nil {
			releasePartBuffer(part.Data)
			return
		}

		etag, err := uploader.UploadPart(ctx, uploadID, part.Number, part.Data)
		releasePartBuffer(part.Data)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if uploadErr == nil { // Record the first error
				uploadErr = fmt.Errorf("failed to upload part %d: %v", part.Number, err)
				log.Println(uploadErr)
				cancel() // Cancel the context for all other workers
			}
			return
		}
		if _, exists := etags[part.Number]; exists {
			if uploadErr == nil {
				uploadErr = fmt.Errorf("duplicate part number detected: %d", part.Number)
				log.Println(uploadErr)
				cancel()
			}
			return
		}
		etags[part.Number] = etag
	}

	// startWorker uploads part in a new goroutine; the caller must hold a semaphore slot.
	startWorker := func(part Part) {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			defer func() { <-sem }() // Release semaphore
			uploadPart(part)
		}()
	}

	sem <- struct{}{}
	startWorker(part1)
	// Take a worker slot before receiving the next part, so a part is only
	// pulled off the (unbuffered) channel once it can be uploaded. Parts in
	// memory are thus bounded by concurrency + the one the producer is filling.
	for {
		sem <- struct{}{}
		part, ok := <-partChan
		if !ok {
			<-sem
			break
		}
		// Stop consuming on failure; the caller cancels the producer once we return.
		if ctx.Err() != nil {
			<-sem
			releasePartBuffer(part.Data)
			break
		}
		startWorker(part)
	}

	workerWg.Wait()

	if uploadErr != nil {
		log.Printf("An error occurred during upload, aborting: %v", uploadErr)
		if abortErr := uploader.Abort(parentCtx, uploadID); abortErr != nil { // Use parentCtx for abort
			log.Printf("failed to abort upload: %v", abortErr)
		}
		return uploadErr
	}

	if err := uploader.Complete(ctx, uploadID, etags); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %v", err)
	}

	fmt.Println("Upload completed successfully.")
	return nil
}
