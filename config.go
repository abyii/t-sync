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
	MinPartSize      int // in bytes
	Password         string
	EncryptionType   string
}

// DestDetails holds parsed details from the destination URL.
type DestDetails struct {
	Provider  string
	Bucket    string
	Namespace string
	Key       string
}

const (
	// auth types
	AuthTypeOCIConfigFile       = "OCI_CONFIG_FILE"
	AuthTypeOKEWorkloadIdentity = "OKE_WORKLOAD_IDENTITY"
	AuthTypeInstancePrincipal   = "INSTANCE_PRINCIPAL"

	// encryption types
	EncryptionTypeZipCrypto = "zipcrypto"
	EncryptionTypeAES128    = "aes128"
	EncryptionTypeAES192    = "aes192"
	EncryptionTypeAES256    = "aes256"
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

	// source and destination configuration
	flag.StringVar(&cfg.Source, "s", "", "Source directory to zip.")
	flag.StringVar(&destStr, "d", "", "Destination URI (e.g., file:///path/to/file.zip, oci://namespace@bucket/key).")

	// Auth incase of object storage
	flag.StringVar(&cfg.AuthType, "auth-type", "", "Authentication type (e.g., OCI_CONFIG_FILE, OKE_WORKLOAD_IDENTITY, INSTANCE_PRINCIPAL).")

	// multipart upload config
	flag.IntVar(&cfg.MaxPartsInMemory, "max-parts-in-memory", 10, "Maximum number of parts to hold in memory before applying backpressure. (Default: 10)")
	flag.IntVar(&cfg.MinPartSize, "min-part-size-mb", 10, "Minimum part size in MB for multipart uploads. (Default: 10)")

	// password when zip encryption is enabled
	flag.StringVar(&cfg.Password, "password", "", "Password for encrypting the zip file.")
	flag.StringVar(&cfg.EncryptionType, "encryption-type", EncryptionTypeZipCrypto, "Encryption type (e.g., zipcrypto, aes128, aes192, aes256). (Default: zipcrypto)")

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

	if cfg.Password != "" && !(cfg.EncryptionType == EncryptionTypeZipCrypto || cfg.EncryptionType == EncryptionTypeAES128 || cfg.EncryptionType == EncryptionTypeAES192 || cfg.EncryptionType == EncryptionTypeAES256) {
		flag.Usage()
		return nil, fmt.Errorf("unsupported encryption-type: %s", cfg.EncryptionType)
	}
	if cfg.Password == "" && cfg.EncryptionType != "ZipCrypto" {
		flag.Usage()
		return nil, fmt.Errorf("password is required when encryption-type is specified")
	}

	cfg.Destination = destURL
	cfg.MinPartSize = cfg.MinPartSize * 1024 * 1024 // Convert to bytes

	return cfg, nil
}
