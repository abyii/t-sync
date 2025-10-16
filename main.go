package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

func main() {
	cfg, err := ParseFlags()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	fmt.Printf("Source Directory: %s\n", cfg.Source)
	fmt.Printf("Destination: %s\n", cfg.Destination)

	// this is for cpu profiling. dev only
	// profileFileName := "cpu.prof"
	// cpuProf, err := os.Create(profileFileName)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer cpuProf.Close()
	// if profErr := pprof.StartCPUProfile(cpuProf); profErr != nil {
	// 	log.Fatal("could not start CPU profile: ", profErr)
	// }
	// defer pprof.StopCPUProfile()

	start := time.Now()

	var writer io.Writer
	var closer io.Closer
	var uploadWg sync.WaitGroup
	var uploadErr error

	destDetails, err := ParseDestURL(cfg.Destination)
	if err != nil {
		log.Fatalf("Invalid destination: %v", err)
	}

	if destDetails.Provider == "file" {
		absOutFile, err := filepath.Abs(destDetails.Key)
		fmt.Printf("Output File: %s\n", absOutFile)
		if err != nil {
			log.Fatalf("failed to resolve absolute path for output file: %v", err)
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(absOutFile), os.ModePerm); mkdirErr != nil {
			log.Fatalf("failed to create output directory: %v", mkdirErr)
		}
		outFile, err := os.Create(absOutFile)
		if err != nil {
			log.Fatalf("failed to create zip file: %v", err)
		}
		writer = outFile
		closer = outFile
	} else {
		uploader, err := NewUploader(destDetails, cfg.AuthType)
		if err != nil {
			log.Fatalf("Failed to create uploader: %v", err)
		}

		partChan := make(chan Part, cfg.MaxPartsInMemory)
		channelWriter := NewChannelWriter(partChan, cfg.MinPartSize)
		writer = channelWriter
		closer = channelWriter

		uploadWg.Add(1)
		go func() {
			uploadErr = uploadToObjectStorage(context.Background(), uploader, partChan, &uploadWg, cfg.MaxPartsInMemory)
		}()
	}

	if err := CreateZipArchive(cfg.Source, writer, cfg.EncryptionType, cfg.Password, cfg.IgnoreFile); err != nil {
		log.Fatalf("Failed to create zip archive: %v", err)
	}

	if err := closer.Close(); err != nil {
		log.Fatalf("Failed to close writer: %v", err)
	}

	uploadWg.Wait()

	// After the uploader has definitely finished, check the error.
	if uploadErr != nil {
		log.Fatalf("Upload failed: %v", uploadErr)
	}

	elapsed := time.Since(start)
	fmt.Printf("\nFinished in %s\n", elapsed)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Memory usage: %.2f MB\n", float64(m.Alloc)/1024.0/1024.0)
}
