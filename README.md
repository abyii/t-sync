WSL (Linux AMD 64):
    comp1 (1023 Mib):
        Deflate: 
            time: 26.54034546s, throughput: 38.57 MB/s, mem: 2.76 MB, finalsize: 124 MiB
            time: 24.78674412s, throughput: 41.30 MB/s, mem: 2.76 MB, finalsize: 124 MiB
            time: 26.82243443s, throughput: 38.16 MB/s, mem: 2.76 MB, finalsize: 124 MiB
        Zlib (4kills/go-zlib): 
            time: 25.79606939s, throughput: 39.68 MB/s, mem: 3.56 MB, finalsize: 123 MiB - unzip not working
            time: 25.74878374s, throughput: 39.75 MB/s, mem: 1.66 MB, finalsize: 123 MiB - unzip not working
            time: 33.64031561s, throughput: 30.43 MB/s, mem: 2.99 MB, finalsize: 123 MiB - unzip not working
        ZlibNG (yasushi-saito/zlibng): 
            time: 27.76268016s, throughput: 36.87 MB/s, mem: 2.39 MB, finalsize: 123 MiB - unzip not working
            time: 20.05417290s, throughput: 51.04 MB/s, mem: 3.67 MB, finalsize: 123 MiB - unzip not working
            time: 19.57272619s, throughput: 52.30 MB/s, mem: 1.89 MB, finalsize: 123 MiB - unzip not working
Windows:
    comp1 (1023 Mib):
        Deflate: 
            time: 10.0975571s, throughput: 101.37 MB/s, mem: 2.82 MB, finalsize: 124 MiB
            time: 9.18836830s, throughput: 111.40 MB/s, mem: 2.82 MB, finalsize: 124 MiB
            time: 9.59423760s, throughput: 106.69 MB/s, mem: 2.82 MB, finalsize: 124 MiB
        Zlib (4kills/go-zlib): 
            time: 16.58832960s, throughput: 61.70 MB/s, mem: 3.66 MB, finalsize: 123 MiB - unzip not working
            time: 15.82086170s, throughput: 64.70 MB/s, mem: 1.41 MB, finalsize: 123 MiB - unzip not working
            time: 15.94093750s, throughput: 64.21 MB/s, mem: 3.27 MB, finalsize: 123 MiB - unzip not working
        ZlibNG (yasushi-saito/zlibng): 
            NA - works only on linux amd64
Oracle Linux 2ocpu, 5gb, 12.5% burst, E4 flex:
    comp1 (1023 Mib):
        Deflate: 
            time: 12.22800772s, throughput: 83.71 MB/s, mem: 2.68 MB, finalsize: 124 MiB
            time: 13.05952484s, throughput: 78.38 MB/s, mem: 2.68 MB, finalsize: 124 MiB
            time: 12.92900490s, throughput: 79.17 MB/s, mem: 2.68 MB, finalsize: 124 MiB
        Zlib (4kills/go-zlib): 
            time: 22.11141464s, throughput: 46.29 MB/s, mem: 1.93 MB, finalsize: 124 MiB - unzip not working
        ZlibNG (yasushi-saito/zlibng): 
            time: 13.39734423s, throughput: 76.40 MB/s, mem: 2.31 MB, finalsize: 123 MiB - unzip not working (`DCL implode' method not supported)

Winner: Deflate - both for speed, compatibility and ease-of-use


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