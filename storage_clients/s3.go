package storage_clients

import (
	"context"
	"fmt"
	"time"
)

// S3MockUploader is a mock S3 uploader.
type S3MockUploader struct{}

func (u *S3MockUploader) PutObject(ctx context.Context, data []byte) error {
	fmt.Printf("S3 Mock: Putting object with %d bytes (simple upload)\n", len(data))
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (u *S3MockUploader) Initiate(ctx context.Context) (string, error) {
	fmt.Println("S3 Mock: Initiate multipart upload")
	return "mock-s3-upload-id", nil
}
func (u *S3MockUploader) UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (string, error) {
	fmt.Printf("S3 Mock: Uploading part %d, size %d\n", partNumber, len(data))
	time.Sleep(100 * time.Millisecond)
	return fmt.Sprintf("s3-etag-%d", partNumber), nil
}
func (u *S3MockUploader) Complete(ctx context.Context, uploadID string, etags map[int]string) error {
	fmt.Printf("S3 Mock: Completing multipart upload %s with %d parts\n", uploadID, len(etags))
	return nil
}
func (u *S3MockUploader) Abort(ctx context.Context, uploadID string) error {
	fmt.Printf("S3 Mock: Aborting multipart upload %s\n", uploadID)
	return nil
}
