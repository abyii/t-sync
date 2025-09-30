package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
)

// this holds all the command line config you can pass to t-sync
type Config struct {
	Source           string
	Destination      *url.URL
	AuthType         string
	MaxPartsInMemory int
	MinPartSize      int
}

// DestDetails holds parsed details from the destination URL.
type DestDetails struct {
	Provider  string
	Bucket    string
	Namespace string
	Key       string
}

const (
	AuthTypeOCIConfigFile       = "OCI_CONFIG_FILE"
	AuthTypeOKEWorkloadIdentity = "OKE_WORKLOAD_IDENTITY"
	AuthTypeInstancePrincipal   = "INSTANCE_PRINCIPAL"
)

// isValidAuthType checks whether the provided auth-type string matches
// one of the supported patterns:
//   - OCI_CONFIG_FILE or OCI_CONFIG_FILE[PROFILE]
//   - OKE_WORKLOAD_IDENTITY
//   - INSTANCE_PRINCIPAL
func isValidAuthType(authType string) bool {
	switch {
	case strings.HasPrefix(authType, AuthTypeOCIConfigFile):
		if authType == AuthTypeOCIConfigFile {
			return true
		}
		if strings.HasPrefix(authType, AuthTypeOCIConfigFile+"[") && strings.HasSuffix(authType, "]") {
			return true
		}
	case authType == AuthTypeOKEWorkloadIdentity:
		return true
	case authType == AuthTypeInstancePrincipal:
		return true
	}
	return false
}

func ParseDestURL(dest *url.URL) (*DestDetails, error) {
	details := &DestDetails{
		Provider: dest.Scheme,
		Key:      strings.TrimLeft(dest.Path, "/"),
	}

	switch dest.Scheme {
	case "oci":
		if dest.User != nil {
			details.Namespace = dest.User.Username()
		} else {
			return nil, errors.New("OCI destination requires namespace in the format oci://namespace@bucket/key")
		}
		details.Bucket = dest.Host
	// to be implemented later
	// case "s3":
	// 	details.Bucket = dest.Host
	case "file":
		details.Key = strings.TrimLeft(dest.Path, "/")
	default:
		return nil, fmt.Errorf("unsupported destination provider: %s", dest.Scheme)
	}

	return details, nil
}

func ParseFlags() (*Config, error) {
	cfg := &Config{}
	var destStr string

	flag.StringVar(&cfg.Source, "s", "", "Source directory to zip.")
	flag.StringVar(&destStr, "d", "", "Destination URI (e.g., file:///path/to/file.zip, oci://namespace@bucket/key).")
	flag.StringVar(&cfg.AuthType, "auth-type", "", "Authentication type (e.g., OCI_CONFIG_FILE, OKE_WORKLOAD_IDENTITY, INSTANCE_PRINCIPAL).")
	flag.IntVar(&cfg.MaxPartsInMemory, "max-parts-in-memory", 10, "Maximum number of parts to hold in memory before applying backpressure.")
	flag.IntVar(&cfg.MinPartSize, "min-part-size-mb", 10, "Minimum part size in MB for multipart uploads.")

	flag.Parse()

	if cfg.Source == "" || destStr == "" {
		flag.Usage()
		return nil, errors.New("source and destination are required")
	}

	if cfg.MaxPartsInMemory <= 0 {
		flag.Usage()
		return nil, fmt.Errorf("max-parts-in-memory must be greater than 0")
	}

	if cfg.MinPartSize < 5 {
		flag.Usage()
		return nil, fmt.Errorf("min-part-size-mb must be greater than 5")
	}

	destURL, err := url.Parse(destStr)
	if err != nil {
		return nil, fmt.Errorf("invalid destination URI: %v", err)
	}

	if destURL.Scheme == "oci" {
		if ok := isValidAuthType(cfg.AuthType); !ok {
			flag.Usage()
			return nil, fmt.Errorf("unsupported auth-type for oci: %s", cfg.AuthType)
		}
	}
	cfg.Destination = destURL
	cfg.MinPartSize = cfg.MinPartSize * 1024 * 1024 // Convert to bytes

	return cfg, nil
}
