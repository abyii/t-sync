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
Exit codes stay in the portable 0-255 range; they loosely mirror HTTP status codes (`4x` = caller error, `5x` = failure).

| Code | Name | Meaning |
|---|---|---|
| 0 | — | Success |
| 40 | `ExitCodeInvalidParameters` | Bad flags, invalid destination, source is not a directory, or the output file/directory cannot be created |
| 41 | `ExitCodeAuthenticationFailed` | Reserved; not currently emitted (auth errors surface as 52 or 53) |
| 44 | `ExitCodeSourceDirNotFound` | Source directory does not exist |
| 50 | `ExitCodeInternalCodeError` | Failed to close the output file or the upload channel writer |
| 52 | `ExitCodeUploadFailed` | Upload to object storage failed (multipart upload is aborted) |
| 53 | `ExitCodeUploaderClientFailed` | Object storage client could not be initialised |
| 54 | `ExitCodeZipArchiverFailed` | Failed to create the zip archive |



### Limiting CPU Usage.

Zipping/Deflate is a CPU-intensive operation. To limit the CPU usage, you can use the `CPUQuota` option with `systemd-run`.

```
systemd-run --scope -p CPUQuota=50% t-sync -s "../sample_data_size1" -d "oci://bmcx0flrsnis@test-bucket-for-poc/output_cpu_limit.zip" -auth-type OCI_CONFIG_FILE
```

### Limiting Memory Usage.

Part data in memory is at most `(max-parts-in-memory + 1) x part size` (10 parts of 10 MiB by default). On top of that the Go GC keeps headroom set by `GOGC` (default 100). On memory-constrained hosts, set `GOGC=25`: in a 37 GiB OCI backup on a 4-core host this cut typical RSS from ~100 MB to ~65 MB with no change in run time.

```
GOGC=25 t-sync -s "./data" -d "oci://namespace@bucket/backup.zip" -auth-type OKE_WORKLOAD_IDENTITY
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
* *(default when `-auth-type` is omitted)*: Uses the OCI SDK default configuration provider: `DEFAULT` profile of `~/.oci/config` (or the file named by the `OCI_CONFIG_FILE` env var), then `~/.obmcs/config`, then `TF_VAR_*` environment variables.
* **`OCI_CONFIG_FILE`** or **`OCI_CONFIG_FILE[PROFILE]`**: Uses `~/.oci/config` profile (default: `DEFAULT`).
* **`INSTANCE_PRINCIPAL`**: Uses OCI Compute instance principal identity.
* **`OKE_WORKLOAD_IDENTITY`**: Uses Oracle Container Engine for Kubernetes (OKE) workload identity.


