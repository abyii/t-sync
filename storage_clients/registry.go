package storage_clients

import (
    "fmt"
)

// UploaderFactory is a function that creates a new uploader.
type UploaderFactory func(bucket, object, authType, namespace string) (interface{}, error)

var (
    registry = make(map[string]UploaderFactory)
)

// RegisterUploader registers a new uploader factory for a provider.
func RegisterUploader(provider string, factory UploaderFactory) {
    registry[provider] = factory
}

// GetUploader returns an uploader for the given provider.
func GetUploader(provider, bucket, object, authType, namespace string) (interface{}, error) {
    if factory, ok := registry[provider]; ok {
        return factory(bucket, object, authType, namespace)
    }
    return nil, fmt.Errorf("provider '%s' is not supported in this build", provider)
}
