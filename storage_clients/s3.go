//go:build s3

package storage_clients

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func init() {
	RegisterUploader("s3", func(bucket, object, authType, namespace string) (any, error) {
		return NewS3Uploader(bucket, object, authType)
	})
}

// S3Uploader handles multipart uploads to S3.
type S3Uploader struct {
	client        *s3.Client
	bucket        string
	object        string
	partChecksums sync.Map
}

// NewS3Uploader creates a new S3Uploader.
func NewS3Uploader(bucket, object, authType string) (*S3Uploader, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if object == "" {
		return nil, fmt.Errorf("object name is required")
	}

	ctx := context.Background()
	var configOpts []func(*config.LoadOptions) error

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region != "" {
		configOpts = append(configOpts, config.WithRegion(region))
	} else {
		// Enable resolving region from EC2 IMDS if not specified via environment
		configOpts = append(configOpts, config.WithEC2IMDSRegion())
	}

	switch {
	case strings.HasPrefix(authType, "S3_ACCESS_KEYS[") && strings.HasSuffix(authType, "]"):
		keysStr := authType[len("S3_ACCESS_KEYS[") : len(authType)-1]
		parts := strings.Split(keysStr, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("invalid S3_ACCESS_KEYS format, expected ACCESS_KEY:SECRET_KEY or ACCESS_KEY:SECRET_KEY:SESSION_TOKEN")
		}

		accessKey := parts[0]
		secretKey := parts[1]
		sessionToken := ""
		if len(parts) == 3 {
			sessionToken = parts[2]
			log.Printf("Using explicit S3 access keys with session token")
		} else {
			log.Printf("Using explicit S3 access keys")
		}

		configOpts = append(configOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken),
		))

	case strings.HasPrefix(authType, "AWS_CONFIG_FILE"):
		profile := "default"
		if strings.Contains(authType, "[") && strings.Contains(authType, "]") {
			start := strings.Index(authType, "[")
			end := strings.Index(authType, "]")
			if start != -1 && end != -1 && end > start {
				profile = authType[start+1 : end]
			}
		}
		log.Printf("Using AWS config file with profile: %s", profile)
		configOpts = append(configOpts, config.WithSharedConfigProfile(profile))

	case authType == "AWS_DEFAULT" || authType == "":
		log.Printf("Using default AWS credential chain (supports env, config, EKS, ECS, EC2 IMDS)")

	default:
		return nil, fmt.Errorf("unsupported S3 authentication type: %s", authType)
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %v", err)
	}

	if cfg.Region == "" {
		cfg.Region = "ap-south-1"
		log.Printf("WARNING: AWS region not found in AWS_REGION, AWS_DEFAULT_REGION, the shared config, or EC2 IMDS; falling back to %s", cfg.Region)
	}

	client := s3.NewFromConfig(cfg)
	log.Printf("S3 client created successfully for bucket: %s, object: %s, region: %s", bucket, object, cfg.Region)

	return &S3Uploader{
		client: client,
		bucket: bucket,
		object: object,
	}, nil
}

func (u *S3Uploader) Initiate(ctx context.Context) (string, error) {
	log.Printf("Initiating multipart upload for bucket: %s, object: %s", u.bucket, u.object)
	u.partChecksums.Clear()
	input := &s3.CreateMultipartUploadInput{
		Bucket:            aws.String(u.bucket),
		Key:               aws.String(u.object),
		ChecksumAlgorithm: types.ChecksumAlgorithmCrc64nvme,
	}

	var uploadID string
	err := withRetry(ctx, "initiate multipart upload", func() error {
		resp, err := u.client.CreateMultipartUpload(ctx, input)
		if err != nil {
			return err
		}
		uploadID = *resp.UploadId
		return nil
	})
	if err != nil {
		return "", err
	}
	log.Printf("Successfully initiated multipart upload with ID: %s", uploadID)
	return uploadID, nil
}

func (u *S3Uploader) UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (string, error) {
	var etag string
	err := withRetry(ctx, fmt.Sprintf("upload part %d", partNumber), func() error {
		body := newPartBody(data)
		defer body.revoke()
		resp, err := u.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:            aws.String(u.bucket),
			Key:               aws.String(u.object),
			UploadId:          aws.String(uploadID),
			PartNumber:        aws.Int32(int32(partNumber)),
			ContentLength:     aws.Int64(int64(len(data))),
			Body:              body,
			ChecksumAlgorithm: types.ChecksumAlgorithmCrc64nvme,
		})
		if err != nil {
			return err
		}
		if resp.ETag == nil {
			return fmt.Errorf("no ETag returned for part %d", partNumber)
		}
		if resp.ChecksumCRC64NVME == nil {
			return fmt.Errorf("server did not return CRC64NVME checksum for part %d", partNumber)
		}
		u.partChecksums.Store(partNumber, *resp.ChecksumCRC64NVME)
		etag = *resp.ETag
		return nil
	})
	if err != nil {
		return "", err
	}
	log.Printf("Successfully uploaded part %d with ETag: %s, %d bytes", partNumber, etag, len(data))
	return etag, nil
}

func (u *S3Uploader) Complete(ctx context.Context, uploadID string, etags map[int]string) error {
	log.Printf("Completing multipart upload %s with %d parts", uploadID, len(etags))

	partNums := make([]int, 0, len(etags))
	for partNum := range etags {
		partNums = append(partNums, partNum)
	}
	slices.Sort(partNums)

	completedParts := make([]types.CompletedPart, 0, len(partNums))
	for _, partNum := range partNums {
		cp := types.CompletedPart{
			PartNumber: aws.Int32(int32(partNum)),
			ETag:       aws.String(etags[partNum]),
		}
		if cs, ok := u.partChecksums.Load(partNum); ok {
			cp.ChecksumCRC64NVME = aws.String(cs.(string))
		}
		completedParts = append(completedParts, cp)
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(u.bucket),
		Key:      aws.String(u.object),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	err := withRetry(ctx, "complete multipart upload "+uploadID, func() error {
		_, err := u.client.CompleteMultipartUpload(ctx, input)
		return err
	})
	if err != nil {
		return err
	}
	log.Printf("Successfully completed multipart upload %s", uploadID)
	return nil
}

func (u *S3Uploader) PutObject(ctx context.Context, data []byte) error {
	log.Printf("Putting object %s with %d bytes (simple upload)", u.object, len(data))

	err := withRetry(ctx, "put object "+u.object, func() error {
		body := newPartBody(data)
		defer body.revoke()
		_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:            aws.String(u.bucket),
			Key:               aws.String(u.object),
			ContentLength:     aws.Int64(int64(len(data))),
			Body:              body,
			ChecksumAlgorithm: types.ChecksumAlgorithmCrc64nvme,
		})
		return err
	})
	if err != nil {
		return err
	}
	log.Printf("Successfully put object %s", u.object)
	return nil
}

func (u *S3Uploader) Abort(ctx context.Context, uploadID string) error {
	log.Printf("Aborting multipart upload %s", uploadID)
	u.partChecksums.Clear()

	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(u.bucket),
		Key:      aws.String(u.object),
		UploadId: aws.String(uploadID),
	}

	err := withRetry(ctx, "abort multipart upload "+uploadID, func() error {
		_, err := u.client.AbortMultipartUpload(ctx, input)
		if httpStatus(err) == http.StatusNotFound {
			log.Printf("Multipart upload %s already aborted or not found", uploadID)
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	log.Printf("Successfully aborted multipart upload %s", uploadID)
	return nil
}
