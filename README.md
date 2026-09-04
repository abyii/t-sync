# t-sync

A CLI tool to Zip directly to Object Storage.

### Setup & Build

Running the build commands, or even the Run command from t-sync directory (with `go run .`), will automatically download dependencies and create the build.
To download/reconcile go package dependencies as a separate step, You can simply run: `go mod tidy`

#### Build Commands
```
// for Linux amd64:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags oci -ldflags="-s -w" -o ../bin/t-sync-oci
upx ../bin/t-sync-oci

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags s3 -ldflags="-s -w" -o ../bin/t-sync-s3
upx ../bin/t-sync-s3
```

```
// for Linux arm64:
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags oci -ldflags="-s -w" -o ../bin/t-sync-oci
upx ../bin/t-sync-oci

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags s3 -ldflags="-s -w" -o ../bin/t-sync-s3
upx ../bin/t-sync-s3
```

### Exit Codes
```
// exit codes that have similar meaning to HTTP status codes
ExitCodeInvalidParameters    = 400 // Bad Parameters
ExitCodeAuthenticationFailed = 401 // Authentication Failed with Object Storage Service
ExitCodeUploadFailed         = 502 // Upload Failed with Object Storage Service
ExitCodeUploaderClientFailed = 503 // Initialization of Uploader Client Failed with Object Storage Service
ExitCodeZipArchiverFailed    = 504 // Failed to create zip archive
ExitCodeInternalCodeError    = 500 // Internal Code Error. Problem when closing IO or Upload Channel Writer
```



### Limiting CPU Usage.

Zipping/Deflate is a CPU-intensive operation. To limit the CPU usage, you can use the `CPUQuota` option with `systemd-run`.

```
systemd-run --scope -p CPUQuota=50% t-sync -s "../sample_data_size1" -d "oci://bmcx0flrsnis@test-bucket-for-poc/output_cpu_limit.zip" -auth-type OCI_CONFIG_FILE
```

### Authentication Types

#### S3 (`s3://bucket/key`)
* **`AWS_DEFAULT`** *(default when `-auth-type` is omitted)*: Uses the standard AWS SDK credential resolution chain (environment variables $\rightarrow$ shared configuration/profile $\rightarrow$ container/web identity $\rightarrow$ EC2 IMDS / Instance Profile). Automatically resolves and auto-rotates credentials on EC2 instances:
  ```bash
  t-sync -s "./data" -d "s3://my-bucket/backup.zip"
  ```
* **`AWS_CONFIG_FILE`** or **`AWS_CONFIG_FILE[PROFILE]`**: Uses local AWS shared configuration and credentials (`~/.aws/credentials`, `~/.aws/config`), with optional named profile:
  ```bash
  t-sync -s "./data" -d "s3://my-bucket/backup.zip" -auth-type "AWS_CONFIG_FILE[production]"
  ```
* **`S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY]`** or **`S3_ACCESS_KEYS[ACCESS_KEY:SECRET_KEY:SESSION_TOKEN]`**: Explicit static keys:
  ```bash
  t-sync -s "./data" -d "s3://my-bucket/backup.zip" -auth-type "S3_ACCESS_KEYS[AKIAIOSFODNN7EXAMPLE:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY]"
  ```

#### OCI (`oci://namespace@bucket/key`)
* **`OCI_CONFIG_FILE`** or **`OCI_CONFIG_FILE[PROFILE]`**: Uses `~/.oci/config` profile (default: `DEFAULT`).
* **`INSTANCE_PRINCIPAL`**: Uses OCI Compute instance principal identity.
* **`OKE_WORKLOAD_IDENTITY`**: Uses Oracle Container Engine for Kubernetes (OKE) workload identity.


