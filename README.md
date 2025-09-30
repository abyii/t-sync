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