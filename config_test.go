package main

import (
	"testing"
)

func TestIsValidAuthType_S3(t *testing.T) {
	tests := []struct {
		name     string
		authType string
		expected bool
	}{
		{"empty string (defaults to AWS_DEFAULT)", "", true},
		{"AWS_DEFAULT", AuthTypeAWSDefault, true},
		{"AWS_CONFIG_FILE without profile", AuthTypeAWSConfigFile, true},
		{"AWS_CONFIG_FILE with profile", "AWS_CONFIG_FILE[production]", true},
		{"AWS_CONFIG_FILE with empty profile", "AWS_CONFIG_FILE[]", false},
		{"S3_ACCESS_KEYS with key and secret", "S3_ACCESS_KEYS[AKIAIOSFODNN7EXAMPLE:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY]", true},
		{"S3_ACCESS_KEYS with session token", "S3_ACCESS_KEYS[AKIAIOSFODNN7EXAMPLE:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY:token123]", true},
		{"S3_ACCESS_KEYS missing key", "S3_ACCESS_KEYS[:secret]", false},
		{"S3_ACCESS_KEYS missing secret", "S3_ACCESS_KEYS[key:]", false},
		{"S3_ACCESS_KEYS too many parts", "S3_ACCESS_KEYS[key:secret:token:extra]", false},
		{"S3_ACCESS_KEYS without brackets", "S3_ACCESS_KEYS", false},
		{"OCI auth type on S3 should fail", AuthTypeOCIConfigFile, false},
		{"unsupported auth type", "UNKNOWN_TYPE", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAuthType("s3", tt.authType)
			if got != tt.expected {
				t.Errorf("isValidAuthType(\"s3\", %q) = %v, want %v", tt.authType, got, tt.expected)
			}
		})
	}
}

func TestIsValidAuthType_OCI(t *testing.T) {
	tests := []struct {
		name     string
		authType string
		expected bool
	}{
		{"OCI_CONFIG_FILE without profile", AuthTypeOCIConfigFile, true},
		{"OCI_CONFIG_FILE with profile", "OCI_CONFIG_FILE[DEFAULT]", true},
		{"OCI_CONFIG_FILE with empty profile", "OCI_CONFIG_FILE[]", false},
		{"OKE_WORKLOAD_IDENTITY", AuthTypeOKEWorkloadIdentity, true},
		{"INSTANCE_PRINCIPAL", AuthTypeInstancePrincipal, true},
		{"empty string on OCI should fail", "", false},
		{"S3 auth type on OCI should fail", AuthTypeAWSDefault, false},
		{"invalid auth type", "INVALID", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAuthType("oci", tt.authType)
			if got != tt.expected {
				t.Errorf("isValidAuthType(\"oci\", %q) = %v, want %v", tt.authType, got, tt.expected)
			}
		})
	}
}
