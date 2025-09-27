package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/yasushi-saito/zlibng"
	"github.com/yeka/zip"
	"github.com/zeebo/xxh3"
)

const (
	zlibngMethod uint16 = 99
	xxh3ExtraID  uint16 = 0xCAFE
)

func main() {
	flag.Parse()
	srcDir := flag.Arg(0)
	outFile := flag.Arg(1)
	fmt.Printf("Source Directory: %s \n", srcDir)
	fmt.Printf("Output File: %s \n", outFile)

	// Resolve to absolute path to avoid ambiguity
	absOutFile, err := filepath.Abs(outFile)
	if err != nil {
		log.Fatalf("Failed to resolve absolute path for output file: %v", err)
	}
	outFile = absOutFile

	profileFileName := "cpu.prof"
	// Profiling setup
	cpuProf, err := os.Create(profileFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer cpuProf.Close()
	pprof.StartCPUProfile(cpuProf)
	defer pprof.StopCPUProfile()

	start := time.Now()

	// Create output directory if it doesn't exist
	if mkdirErr := os.MkdirAll(filepath.Dir(outFile), os.ModePerm); mkdirErr != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	// Create output zip file
	out, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("Failed to create zip file: %v", err)
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	zip.RegisterCompressor(zlibngMethod, func(w io.Writer) (io.WriteCloser, error) {
		return zlibng.NewWriter(w, zlibng.Opts{Level: 9, MemLevel: 9})
	})

	totalUncompressed := int64(0)
	totalCompressed := int64(0)

	// Walk the directory
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Open source file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Add file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		// Create zip entry. The local header is written here, without the xxh3 extra field.
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// We will compute the hash as we copy the file to the zip writer.
		hasher := xxh3.New()
		multiWriter := io.MultiWriter(writer, hasher)

		written, err := io.Copy(multiWriter, srcFile)
		if err != nil {
			return err
		}
		totalUncompressed += written

		// Adding xxhash3 to the extra field
		hash := hasher.Sum64()
		extra := make([]byte, 12)
		binary.LittleEndian.PutUint16(extra[0:2], xxh3ExtraID) // Header ID
		binary.LittleEndian.PutUint16(extra[2:4], 8)           // Data size
		binary.LittleEndian.PutUint64(extra[4:12], hash)       // Hash
		header.Extra = extra

		// Zip doesn’t expose compressed size directly,
		// but we can check file offset in output.
		offset, _ := out.Seek(0, io.SeekCurrent)
		totalCompressed = offset

		fmt.Printf("Added %s (%d bytes) \n", relPath, written)
		return nil
	})
	if err != nil {
		log.Fatalf("Walk error: %v", err)
	}

	// Finish
	if err := zipWriter.Close(); err != nil {
		log.Fatalf("Failed to finalize zip: %v", err)
	}
	if err := out.Close(); err != nil {
		log.Fatalf("Failed to close output: %v", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("\nFinished in %s\n", elapsed)
	fmt.Printf("Uncompressed size: %d MiB\n", totalUncompressed/1024/1024)
	fmt.Printf("Compressed size:   %d MiB\n", totalCompressed/1024/1024)
	fmt.Printf("Throughput:        %.2f MB/s\n",
		float64(totalUncompressed)/1024.0/1024.0/elapsed.Seconds())

	// Print memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Memory usage: %.2f MB\n", float64(m.Alloc)/1024.0/1024.0)
}
