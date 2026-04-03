package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// this holds all the command line config you can pass to t-sync
type Config struct {
    Source           string
    Destination      *url.URL
    CompressionLevel int
    AuthType         string
    MaxPartsInMemory int
    MinPartSize      int // in bytes
    Password         string
    IgnoreFile       string
}

// DestDetails holds parsed details from the destination URL.
type DestDetails struct {
    Provider  string
    Bucket    string
    Namespace string
    Key       string
}

var compressionLevelByExtension = map[string]int{
    ".zip":  0,
    ".gz":   0,
    ".bz2":  0,
    ".rar":  0,
    ".7z":   0,
    ".jpg":  0,
    ".jpeg": 0,
    ".png":  0,
    ".gif":  0,
    ".mp4":  0,
    ".mkv":  0,
    ".avi":  0,
    ".mov":  0,
    ".mp3":  0,
    ".flac": 0,
}

const (
    DefaultCompressionLevel = 6
    DefaultMinPartSizeInMiB = 10
    DefaultMaxPartsInMemory = 10
)

func getCompressionLevelForFile(filename string, defaultLevel int) int {
    ext := filepath.Ext(filename)
    if level, ok := compressionLevelByExtension[ext]; ok {
        return level
    }
    return defaultLevel
}

const (
    KiB = 1024

    // auth types
    AuthTypeOCIConfigFile       = "OCI_CONFIG_FILE"
    AuthTypeOKEWorkloadIdentity = "OKE_WORKLOAD_IDENTITY"
    AuthTypeInstancePrincipal   = "INSTANCE_PRINCIPAL"

    // exit codes mapped to valid 8-bit range (0-255)
    ExitCodeInvalidParameters    = 40 // Bad Parameters
    ExitCodeAuthenticationFailed = 41 // Authentication Failed
    ExitCodeSourceDirNotFound    = 44 // Source directory not found
    ExitCodeInternalCodeError    = 50 // Internal Code Error
    ExitCodeUploadFailed         = 52 // Upload Failed
    ExitCodeUploaderClientFailed = 53 // Client Failed
    ExitCodeZipArchiverFailed    = 54 // Zip Failed
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
    case "s3":
        details.Bucket = dest.Host
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
    flag.StringVar(&destStr, "d", "", "Destination URI (e.g., file:///path/to/file.zip, oci://namespace@bucket/key, s3://bucket/key).")

    // compression level: default selected is 6 for best speed vs compression ratio tradeoff.
    flag.IntVar(&cfg.CompressionLevel, "compression-level", DefaultCompressionLevel, "Compression level (0-9).")

    // Auth incase of object storage
    flag.StringVar(&cfg.AuthType, "auth-type", "", "Authentication type (e.g., OCI_CONFIG_FILE, OKE_WORKLOAD_IDENTITY, INSTANCE_PRINCIPAL, S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY] or S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY:SESSION_TOKEN]).")

    // multipart upload config
    flag.IntVar(&cfg.MaxPartsInMemory, "max-parts-in-memory", DefaultMaxPartsInMemory, "Maximum number of parts to hold in memory before applying backpressure.")
    flag.IntVar(&cfg.MinPartSize, "min-part-size-mb", DefaultMinPartSizeInMiB, "Minimum part size in MB for multipart uploads.")

    // password when zip encryption is enabled
    flag.StringVar(&cfg.Password, "password", "", "Password for encrypting the zip file.")

    // ignore file
    flag.StringVar(&cfg.IgnoreFile, "ignore-file", "", "Path to a file with .gitignore style patterns to ignore. File can be named '.tsyncignore'.")

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

    switch destURL.Scheme {
    case "oci":
        if ok := isValidAuthType(cfg.AuthType); !ok {
            flag.Usage()
            return nil, fmt.Errorf("unsupported auth-type for oci: %s", cfg.AuthType)
        }
    case "s3":
        if !strings.HasPrefix(cfg.AuthType, "S3_ACCESS_KEYS[") || !strings.HasSuffix(cfg.AuthType, "]") || !strings.Contains(cfg.AuthType, ":") {
            flag.Usage()
            return nil, fmt.Errorf("unsupported auth-type for s3: %s, expected S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY] or S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY:SESSION_TOKEN]", cfg.AuthType)
        }
    }

    cfg.Destination = destURL
    cfg.MinPartSize = cfg.MinPartSize * KiB * KiB // Convert to bytes

    return cfg, nil
}
