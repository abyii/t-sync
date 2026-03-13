# t-sync

A CLI tool to Zip directly to Object Storage.

### Setup & Build

Running the build commands, or even the Run command from t-sync directory (with `go run .`), will automatically download dependencies and create the build.
To download/reconcile go package dependencies as a separate step, You can simply run: `go mod tidy`

#### Build Commands
```
// for Linux x86:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../bin/t-sync
upx ../bin/t-sync
```

```
// for Linux arm64:
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o ../bin/t-sync
upx ../bin/t-sync
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

