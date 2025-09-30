package main

import (
	"bytes"
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
type ObjectStorageUploader interface {
	Initiate(ctx context.Context) (uploadID string, err error)
	UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (etag string, err error)
	Complete(ctx context.Context, uploadID string, etags map[int]string) error
	Abort(ctx context.Context, uploadID string) error

	PutObject(ctx context.Context, data []byte) error
}

// NewUploader is a factory function that returns an uploader based on the provider.
func NewUploader(details *DestDetails, authType string) (ObjectStorageUploader, error) {
	switch details.Provider {
	case "oci":
		// Here you would initialize your real OCI uploader with credentials from auth
		log.Printf("Using OCI uploader for namespace '%s', bucket '%s'", details.Namespace, details.Bucket)
		return storage_clients.NewOCIUploader(details.Namespace, details.Bucket, details.Key, authType)
	case "s3":
		// Here you would initialize your real S3 uploader
		log.Printf("Using S3 uploader for bucket '%s'", details.Bucket)
		return &storage_clients.S3MockUploader{}, nil // Using a mock for now
	default:
		return nil, fmt.Errorf("no uploader available for provider: %s", details.Provider)
	}
}

// channelWriter is an io.Writer that writes to a channel of parts.
type channelWriter struct {
	partChan    chan<- Part
	buffer      *bytes.Buffer
	minPartSize int
	partNumber  int
}

// NewChannelWriter creates a new channelWriter.
func NewChannelWriter(partChan chan<- Part, minPartSize int) *channelWriter {
	return &channelWriter{
		partChan:    partChan,
		buffer:      &bytes.Buffer{},
		minPartSize: minPartSize,
		partNumber:  1,
	}
}

// Write implements the io.Writer interface.
func (cw *channelWriter) Write(p []byte) (n int, err error) {
	cw.buffer.Write(p)
	for cw.buffer.Len() >= cw.minPartSize {
		partData := make([]byte, cw.minPartSize)
		_, err := cw.buffer.Read(partData)
		if err != nil {
			return 0, err // Should not happen
		}

		cw.partChan <- Part{
			Number: cw.partNumber,
			Data:   partData,
		}
		cw.partNumber++
	}
	return len(p), nil
}

// Close flushes any remaining data in the buffer as the last part.
func (cw *channelWriter) Close() error {
	if cw.buffer.Len() > 0 {
		partData := cw.buffer.Bytes()
		cw.partChan <- Part{
			Number: cw.partNumber,
			Data:   partData,
		}
	}
	close(cw.partChan)
	return nil
}

func uploadToObjectStorage(parentCtx context.Context, uploader ObjectStorageUploader, partChan <-chan Part, uploadWg *sync.WaitGroup, concurrency int) error {
	defer uploadWg.Done()

	// Create a new context that can be cancelled if an error occurs
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel() // Ensure cancel is called to free resources

	// Read the first part to decide on the upload strategy
	part1, ok := <-partChan
	if !ok {
		log.Println("No data to upload.")
		return nil
	}

	// Check for a second part to determine if multipart is needed
	part2, ok := <-partChan
	if !ok {
		// Only one part exists, so use a simple upload
		log.Printf("Total size is %d bytes. Using simple upload.", len(part1.Data))
		if err := uploader.PutObject(ctx, part1.Data); err != nil {
			return fmt.Errorf("failed to put object: %v", err)
		}
		fmt.Println("Upload completed successfully.")
		return nil
	}

	// If we're here, we have at least two parts, so we do a multipart upload
	log.Println("Data size is large, using multipart upload.")
	uploadID, err := uploader.Initiate(ctx)
	if err != nil {
		return fmt.Errorf("failed to initiate multipart upload: %v", err)
	}

	var etags = make(map[int]string)
	var uploadErr error
	var mu sync.Mutex
	var workerWg sync.WaitGroup

	if concurrency <= 0 {
		concurrency = 10 // Default to 10 if a non-positive value is provided
	}
	sem := make(chan struct{}, concurrency)

	// uploadPart is a closure that handles uploading a single part
	uploadPart := func(part Part) {
		defer workerWg.Done()

		// Check for cancellation before proceeding
		if ctx.Err() != nil {
			return
		}

		etag, err := uploader.UploadPart(ctx, uploadID, part.Number, part.Data)
		if err != nil {
			mu.Lock()
			if uploadErr == nil { // Record the first error
				uploadErr = fmt.Errorf("failed to upload part %d: %v", part.Number, err)
				log.Println(uploadErr)
				cancel() // Cancel the context for all other workers
			}
			mu.Unlock()
			return
		}

		mu.Lock()
		// Check for duplicate part numbers
		if _, exists := etags[part.Number]; exists {
			if uploadErr == nil {
				uploadErr = fmt.Errorf("duplicate part number detected: %d", part.Number)
				log.Println(uploadErr)
				cancel()
			}
		} else {
			etags[part.Number] = etag
		}
		mu.Unlock()
	}

	// Goroutine launcher that respects the semaphore
	runWorker := func(part Part) {
		sem <- struct{}{} // Acquire semaphore
		go func(p Part) {
			defer func() { <-sem }() // Release semaphore
			uploadPart(p)
		}(part)
	}

	// Start processing the first two parts
	workerWg.Add(2)
	runWorker(part1)
	runWorker(part2)

	// Process the rest of the parts from the channel
	for part := range partChan {
		// Check for cancellation before starting a new worker
		if ctx.Err() != nil {
			break
		}
		workerWg.Add(1)
		runWorker(part)
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
