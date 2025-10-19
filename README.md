# t-sync

A CLI tool to Zip directly to Object Storage.

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

### Recommended Build Command:

```
// for Linux x86:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"
upx t-sync
```

```
// for Linux arm64:
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w"
upx t-sync
```

------------------------------------------------------

# tests on 2ocpu, 5gb, 50gb boot, 12.5% burst:

Total uncompressed size: 1023 MiB
Total compressed size: 124 MiB
Finished in 12.510350757s
Memory usage: 29.13 MB

Total uncompressed size: 2047 MiB
Total compressed size: 249 MiB
Finished in 30.515451534s
Memory usage: 49.53 MB

Total uncompressed size: 4094 MiB
Total compressed size: 498 MiB
Finished in 1m17.718494774s
Memory usage: 29.38 MB

after filling the boot to the brim,
Filesystem                  Size  Used Avail Use% Mounted on
devtmpfs                    2.3G     0  2.3G   0% /dev
tmpfs                       2.3G     0  2.3G   0% /dev/shm
tmpfs                       2.3G   92M  2.2G   4% /run
tmpfs                       2.3G     0  2.3G   0% /sys/fs/cgroup
/dev/mapper/ocivolume-root   39G   39G  570M  99% /
/dev/mapper/ocivolume-oled   10G  418M  9.6G   5% /var/oled
/dev/sda2                  1014M  717M  298M  71% /boot
/dev/sda1                   100M  6.0M   94M   6% /boot/efi

[root@test-instance-z sample_data_size1]# cp sample_data sample_data20 -r
cp: error writing 'sample_data20/sample_data/TranMgr.1800': No space left on device

Total uncompressed size: 19781 MiB
Total compressed size: 2402 MiB
Finished in 6m42.871349533s
Memory usage: 30.14 MB

Limiting cpu consumption to 50% by using systemd-run:

systemd-run --scope -p CPUQuota=50% t-sync -s "../sample_data_size1" -d "oci://bmcx0flrsnis@test-bucket-for-poc/output_cpu_limit.zip" -auth-type OCI_CONFIG_FILE

Total uncompressed size: 19781 MiB
Total compressed size: 2402 MiB
Finished in 8m20.937375689s
Memory usage: 30.04 MB