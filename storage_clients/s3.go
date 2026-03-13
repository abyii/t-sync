package storage_clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3Uploader handles multipart uploads to S3.
type S3Uploader struct {
	client *s3.S3
	bucket string
	object string
}

// NewS3Uploader creates a new S3Uploader.
func NewS3Uploader(bucket, object, authType string) (*S3Uploader, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if object == "" {
		return nil, fmt.Errorf("object name is required")
	}

	var creds *credentials.Credentials
	if strings.HasPrefix(authType, "S3_ACCESS_KEYS[") && strings.HasSuffix(authType, "]") {
		keysStr := authType[len("S3_ACCESS_KEYS[") : len(authType)-1]
		parts := strings.SplitN(keysStr, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid S3_ACCESS_KEYS format, expected ACCESS_KEY:SECRET_KEY")
		}
		creds = credentials.NewStaticCredentials(parts[0], parts[1], "")
		log.Printf("Using explicit S3 access keys")
	} else {
		return nil, fmt.Errorf("only S3_ACCESS_KEYS authentication is supported for S3")
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = "ap-south-1"
		}
	}

	awsCfg := aws.NewConfig().WithRegion(region).WithCredentials(creds)

	sess, err := session.NewSession(awsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	client := s3.New(sess)
	log.Printf("S3 client created successfully for bucket: %s, object: %s, region: %s", bucket, object, region)

	return &S3Uploader{
		client: client,
		bucket: bucket,
		object: object,
	}, nil
}

func (u *S3Uploader) Initiate(ctx context.Context) (string, error) {
	log.Printf("Initiating multipart upload for bucket: %s, object: %s", u.bucket, u.object)
	req := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(u.object),
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := u.client.CreateMultipartUploadWithContext(ctx, req)
		if err == nil {
			uploadID := *resp.UploadId
			log.Printf("Successfully initiated multipart upload with ID: %s", uploadID)
			return uploadID, nil
		}

		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("Attempt %d failed to initiate multipart upload: %v. Retrying in %v...", attempt, lastErr, backoff)

			select {
			case <-ctx.Done():
				return "", fmt.Errorf("context cancelled while waiting to retry initiate: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	return "", fmt.Errorf("failed to initiate multipart upload after 3 attempts: %v", lastErr)
}

func (u *S3Uploader) UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req := &s3.UploadPartInput{
			Bucket:        aws.String(u.bucket),
			Key:           aws.String(u.object),
			UploadId:      aws.String(uploadID),
			PartNumber:    aws.Int64(int64(partNumber)),
			ContentLength: aws.Int64(int64(len(data))),
			Body:          bytes.NewReader(data),
		}

		resp, err := u.client.UploadPartWithContext(ctx, req)
		if err == nil {
			if resp.ETag != nil {
				log.Printf("Successfully uploaded part %d with ETag: %s, %d bytes", partNumber, *resp.ETag, len(data))
				return *resp.ETag, nil
			}
			lastErr = fmt.Errorf("no ETag returned for part %d", partNumber)
		} else {
			lastErr = err
		}

		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("Attempt %d failed for part %d: %v. Retrying in %v...", attempt, partNumber, lastErr, backoff)

			select {
			case <-ctx.Done():
				return "", fmt.Errorf("context cancelled while waiting to retry upload part %d: %v", partNumber, ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	return "", fmt.Errorf("failed to upload part %d after 3 attempts: %v", partNumber, lastErr)
}

func (u *S3Uploader) Complete(ctx context.Context, uploadID string, etags map[int]string) error {
	log.Printf("Completing multipart upload %s with %d parts", uploadID, len(etags))

	var partNums []int
	for partNum := range etags {
		partNums = append(partNums, partNum)
	}
	sort.Ints(partNums)

	var completedParts []*s3.CompletedPart
	for _, partNum := range partNums {
		completedParts = append(completedParts, &s3.CompletedPart{
			PartNumber: aws.Int64(int64(partNum)),
			ETag:       aws.String(etags[partNum]),
		})
	}

	req := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(u.bucket),
		Key:      aws.String(u.object),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := u.client.CompleteMultipartUploadWithContext(ctx, req)
		if err == nil {
			log.Printf("Successfully completed multipart upload %s", uploadID)
			return nil
		}

		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("Attempt %d failed to complete multipart upload %s: %v. Retrying in %v...", attempt, uploadID, lastErr, backoff)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting to retry complete: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	return fmt.Errorf("failed to complete multipart upload after 3 attempts: %v", lastErr)
}

func (u *S3Uploader) PutObject(ctx context.Context, data []byte) error {
	log.Printf("Putting object %s with %d bytes (simple upload)", u.object, len(data))

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req := &s3.PutObjectInput{
			Bucket:        aws.String(u.bucket),
			Key:           aws.String(u.object),
			ContentLength: aws.Int64(int64(len(data))),
			Body:          bytes.NewReader(data),
		}

		_, err := u.client.PutObjectWithContext(ctx, req)
		if err == nil {
			log.Printf("Successfully put object %s", u.object)
			return nil
		}

		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("Attempt %d failed to put object %s: %v. Retrying in %v...", attempt, u.object, lastErr, backoff)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting to retry put object: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	return fmt.Errorf("failed to put object after 3 attempts: %v", lastErr)
}

func (u *S3Uploader) Abort(ctx context.Context, uploadID string) error {
	log.Printf("Aborting multipart upload %s", uploadID)

	req := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(u.bucket),
		Key:      aws.String(u.object),
		UploadId: aws.String(uploadID),
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := u.client.AbortMultipartUploadWithContext(ctx, req)
		if err == nil {
			log.Printf("Successfully aborted multipart upload %s", uploadID)
			return nil
		}

		if strings.Contains(err.Error(), "NoSuchUpload") || strings.Contains(err.Error(), "404") {
			return nil
		}

		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("Attempt %d failed to abort multipart upload %s: %v. Retrying in %v...", attempt, uploadID, lastErr, backoff)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting to retry abort: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	return fmt.Errorf("failed to abort multipart upload after 3 attempts: %v", lastErr)
}

// GetObjectRange retrieves a specific byte range from an object in S3
// This can be used to emulate seek operations by reading specific parts of an object
func (u *S3Uploader) GetObjectRange(ctx context.Context, startByte, endByte int64) ([]byte, error) {
	if startByte < 0 {
		return nil, fmt.Errorf("start byte must be non-negative")
	}
	if endByte < startByte {
		return nil, fmt.Errorf("end byte must be greater than or equal to start byte")
	}

	rangeHeader := fmt.Sprintf("bytes=%d-%d", startByte, endByte)
	log.Printf("Getting object range: %s for object %s", rangeHeader, u.object)

	req := &s3.GetObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(u.object),
		Range:  aws.String(rangeHeader),
	}

	resp, err := u.client.GetObjectWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get object range: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object content: %v", err)
	}

	log.Printf("Successfully retrieved %d bytes from object range", len(data))
	return data, nil
}
