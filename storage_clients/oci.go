//go:build oci

package storage_clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

func init() {
	RegisterUploader("oci", func(bucket, object, authType, namespace string) (any, error) {
		return NewOCIUploader(namespace, bucket, object, authType)
	})
}

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
		Object:        &u.object,
	}

	var uploadID string
	err := withRetry(ctx, "initiate multipart upload", func() error {
		resp, err := u.client.CreateMultipartUpload(ctx, req)
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

func (u *OCIUploader) UploadPart(ctx context.Context, uploadID string, partNumber int, data []byte) (string, error) {
	var etag string
	err := withRetry(ctx, fmt.Sprintf("upload part %d", partNumber), func() error {
		resp, err := u.client.UploadPart(ctx, objectstorage.UploadPartRequest{
			NamespaceName:  &u.namespace,
			BucketName:     &u.bucket,
			ObjectName:     &u.object,
			UploadId:       &uploadID,
			UploadPartNum:  &partNumber,
			ContentLength:  new(int64(len(data))),
			UploadPartBody: io.NopCloser(bytes.NewReader(data)),
		})
		if err != nil {
			return err
		}
		if resp.ETag == nil {
			return fmt.Errorf("no ETag returned for part %d", partNumber)
		}
		etag = *resp.ETag
		return nil
	})
	if err != nil {
		return "", err
	}
	log.Printf("Successfully uploaded part %d with ETag: %s, %d bytes", partNumber, etag, len(data))
	return etag, nil
}

func (u *OCIUploader) Complete(ctx context.Context, uploadID string, etags map[int]string) error {
	log.Printf("Completing multipart upload %s with %d parts", uploadID, len(etags))

	partNums := make([]int, 0, len(etags))
	for partNum := range etags {
		partNums = append(partNums, partNum)
	}
	slices.Sort(partNums)

	parts := make([]objectstorage.CommitMultipartUploadPartDetails, 0, len(partNums))
	for _, partNum := range partNums {
		parts = append(parts, objectstorage.CommitMultipartUploadPartDetails{
			PartNum: new(partNum),
			Etag:    new(etags[partNum]),
		})
	}

	req := objectstorage.CommitMultipartUploadRequest{
		NamespaceName: &u.namespace,
		BucketName:    &u.bucket,
		ObjectName:    &u.object,
		UploadId:      &uploadID,
		PartsToCommit: parts,
	}

	err := withRetry(ctx, "complete multipart upload "+uploadID, func() error {
		_, err := u.client.CommitMultipartUpload(ctx, req)
		return err
	})
	if err != nil {
		return err
	}
	log.Printf("Successfully completed multipart upload %s", uploadID)
	return nil
}

func (u *OCIUploader) PutObject(ctx context.Context, data []byte) error {
	log.Printf("Putting object %s with %d bytes (simple upload)", u.object, len(data))

	err := withRetry(ctx, "put object "+u.object, func() error {
		_, err := u.client.PutObject(ctx, objectstorage.PutObjectRequest{
			NamespaceName: &u.namespace,
			BucketName:    &u.bucket,
			ObjectName:    &u.object,
			ContentLength: new(int64(len(data))),
			PutObjectBody: io.NopCloser(bytes.NewReader(data)),
		})
		return err
	})
	if err != nil {
		return err
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

	err := withRetry(ctx, "abort multipart upload "+uploadID, func() error {
		_, err := u.client.AbortMultipartUpload(ctx, req)
		// The upload no longer exists: it was already aborted or completed.
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
