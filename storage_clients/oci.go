package storage_clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// OCIUploader handles multipart uploads to OCI Object Storage.
type OCIUploader struct {
	client    objectstorage.ObjectStorageClient
	namespace string
	bucket    string
	object    string
}

// NewOCIUploader creates a new OCIUploader.
func NewOCIUploader(namespace, bucket, object, authType string) (*OCIUploader, error) {
	// Validate required parameters
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if object == "" {
		return nil, fmt.Errorf("object name is required")
	}

	var provider common.ConfigurationProvider
	var err error

	if strings.HasPrefix(authType, "OCI_CONFIG_FILE") {
		// Handle OCI_CONFIG_FILE or OCI_CONFIG_FILE(PROFILE)
		profile := "DEFAULT" // default profile
		if strings.Contains(authType, "[") && strings.Contains(authType, "]") {
			start := strings.Index(authType, "[")
			end := strings.Index(authType, "]")
			if start != -1 && end != -1 && end > start {
				profile = authType[start+1 : end]
			}
		}
		log.Printf("Using OCI config file with profile: %s", profile)
		provider = common.CustomProfileConfigProvider("~/.oci/config", profile)
	} else if authType == "OKE_WORKLOAD_IDENTITY" {
		// Handle OKE Workload Identity
		log.Printf("Using OKE Workload Identity authentication")
		provider, err = auth.OkeWorkloadIdentityConfigurationProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to create OKE workload identity provider: %v", err)
		}
	} else if authType == "INSTANCE_PRINCIPAL" {
		// Handle Instance Principal
		log.Printf("Using Instance Principal authentication")
		provider, err = auth.InstancePrincipalConfigurationProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to create instance principal provider: %v", err)
		}
	} else {
		// Default to config file provider
		log.Printf("Using default OCI config file authentication")
		provider = common.DefaultConfigProvider()
	}

	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI object storage client: %v", err)
	}

	log.Printf("OCI client created successfully for namespace: %s, bucket: %s, object: %s", namespace, bucket, object)

	return &OCIUploader{
		client:    client,
		namespace: namespace,
		bucket:    bucket,
		object:    object,
	}, nil
}

func (u *OCIUploader) Initiate(ctx context.Context) (string, error) {
	log.Printf("Initiating multipart upload for namespace: %s, bucket: %s, object: %s", u.namespace, u.bucket, u.object)

	req := objectstorage.CreateMultipartUploadRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		CreateMultipartUploadDetails: objectstorage.CreateMultipartUploadDetails{
			Object: &u.object,
		},
	}

	resp, err := u.client.CreateMultipartUpload(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to initiate multipart upload: %v", err)
	}

	uploadID := *resp.UploadId
	log.Printf("Successfully initiated multipart upload with ID: %s", uploadID)
	return uploadID, nil
}

func (u *OCIUploader) UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (string, error) {
	log.Printf("Uploading part %d with %d bytes", partNumber, len(data))

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req := objectstorage.UploadPartRequest{
			NamespaceName:  &u.namespace,
			BucketName:     &u.bucket,
			ObjectName:     &u.object,
			UploadId:       &uploadID,
			UploadPartNum:  &partNumber,
			ContentLength:  common.Int64(int64(len(data))),
			UploadPartBody: io.NopCloser(bytes.NewReader(data)),
		}

		resp, err := u.client.UploadPart(ctx, req)
		if err == nil {
			// Return the ETag from the response
			if resp.ETag != nil {
				log.Printf("Successfully uploaded part %d with ETag: %s", partNumber, *resp.ETag)
				return *resp.ETag, nil
			}
			lastErr = fmt.Errorf("no ETag returned for part %d", partNumber)
			// Fall through to retry logic, as this is unexpected
		} else {
			lastErr = err
		}

		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s, 4s
			log.Printf("Attempt %d failed for part %d: %v. Retrying in %v...", attempt, partNumber, lastErr, backoff)
			time.Sleep(backoff)
		}
	}

	return "", fmt.Errorf("failed to upload part %d after 3 attempts: %v", partNumber, lastErr)
}

func (u *OCIUploader) Complete(ctx context.Context, uploadID string, etags map[int]string) error {
	log.Printf("Completing multipart upload %s with %d parts", uploadID, len(etags))

	parts := make([]objectstorage.CommitMultipartUploadPartDetails, 0, len(etags))
	for partNum, etag := range etags {
		parts = append(parts, objectstorage.CommitMultipartUploadPartDetails{
			PartNum: &partNum,
			Etag:    &etag,
		})
	}

	req := objectstorage.CommitMultipartUploadRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		ObjectName:    &u.object,
		UploadId:      &uploadID,
		CommitMultipartUploadDetails: objectstorage.CommitMultipartUploadDetails{
			PartsToCommit: parts,
		},
	}

	_, err := u.client.CommitMultipartUpload(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %v", err)
	}

	log.Printf("Successfully completed multipart upload %s", uploadID)
	return nil
}

func (u *OCIUploader) PutObject(ctx context.Context, data []byte) error {
	log.Printf("Putting object %s with %d bytes (simple upload)", u.object, len(data))

	req := objectstorage.PutObjectRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		ObjectName:    &u.object,
		ContentLength: common.Int64(int64(len(data))),
		PutObjectBody: io.NopCloser(bytes.NewReader(data)),
	}

	_, err := u.client.PutObject(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to put object: %v", err)
	}

	log.Printf("Successfully put object %s", u.object)
	return nil
}

func (u *OCIUploader) Abort(ctx context.Context, uploadID string) error {
	log.Printf("Aborting multipart upload %s", uploadID)

	req := objectstorage.AbortMultipartUploadRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		ObjectName:    &u.object,
		UploadId:      &uploadID,
	}

	_, err := u.client.AbortMultipartUpload(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %v", err)
	}

	log.Printf("Successfully aborted multipart upload %s", uploadID)
	return nil
}

// GetObjectRange retrieves a specific byte range from an object in OCI Object Storage
// This can be used to emulate seek operations by reading specific parts of an object
func (u *OCIUploader) GetObjectRange(ctx context.Context, startByte, endByte int64) ([]byte, error) {
	if startByte < 0 {
		return nil, fmt.Errorf("start byte must be non-negative")
	}
	if endByte < startByte {
		return nil, fmt.Errorf("end byte must be greater than or equal to start byte")
	}

	rangeHeader := fmt.Sprintf("bytes=%d-%d", startByte, endByte)
	log.Printf("Getting object range: %s for object %s", rangeHeader, u.object)

	req := objectstorage.GetObjectRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		ObjectName:    &u.object,
		Range:         &rangeHeader,
	}

	resp, err := u.client.GetObject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get object range: %v", err)
	}
	defer resp.Content.Close()

	data, err := io.ReadAll(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to read object content: %v", err)
	}

	log.Printf("Successfully retrieved %d bytes from object range", len(data))
	return data, nil
}
