package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/abyii/zip-xxh3"
)

type countingWriter struct {
    writer io.Writer
    total  int64
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
    n, err = cw.writer.Write(p)
    cw.total += int64(n)
    return
}

func openFileWithRetry(path string) (*os.File, error) {
    var file *os.File
    var err error
    for i := 0; i < 3; i++ {
        file, err = os.Open(path)
        if err == nil {
            return file, nil
        }
        time.Sleep(time.Duration(1<<i) * time.Second) // Exponential backoff: 1s, 2s, 4s
    }
    return nil, err
}

func CreateZipArchive(srcDir string, writer io.Writer, compressionLevel int, password string, ignoreFile string) error {

    var ignorer IgnoreParser
    if ignoreFile != "" {
        gi, err := CompileIgnoreFile(ignoreFile)
        if err != nil {
            return fmt.Errorf("failed to compile ignore file: %v", err)
        }
        ignorer = gi
    }

    cw := &countingWriter{writer: writer}
    zipWriter := zip.NewWriter(cw)

    defer zipWriter.Close()
    log.Printf("Creating zip archive for %s\n", srcDir)
    totalUncompressed := int64(0)

    err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        relPath, err := filepath.Rel(srcDir, path)
        if err != nil {
            return err
        }

		checkPath := relPath 
		if info.IsDir() {
			checkPath += "/" // for directories, we should check with a trailing slash
		}

		if ignorer != nil && ignorer.MatchesPath(checkPath) {
			log.Printf("Ignoring %s\n", relPath)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

        if info.IsDir() {
            if relPath == "." {
                return nil
            }
            
            dirName := filepath.ToSlash(relPath)
            if len(dirName) > 0 && dirName[len(dirName)-1] != '/' {
                dirName += "/"
            }

            _, err := zipWriter.Create(dirName, zip.Store, 0, zip.NoEncryption, "")
            if err != nil {
                log.Printf("Failed to create zip entry for directory %s: %v\n", dirName, err)
                return err
            }
            log.Printf("Added directory %s\n", dirName)
            return nil
        }

        if !info.Mode().IsRegular() {
            return nil
        }

        srcFile, err := openFileWithRetry(path)
        if err != nil {
            log.Printf("Failed to open file %s: %v\n", path, err)
            return err
        }
        defer srcFile.Close()

        dynamicLevel := getCompressionLevelForFile(relPath, compressionLevel)

        var entryWriter io.Writer
        if password != "" {
            entryWriter, err = zipWriter.Create(relPath, zip.Deflate, dynamicLevel, zip.StandardEncryption, password)
        } else {
            entryWriter, err = zipWriter.Create(relPath, zip.Deflate, dynamicLevel, zip.NoEncryption, "")
        }
        if err != nil {
            log.Printf("Failed to create zip entry for file %s: %v\n", relPath, err)
            return err
        }

        written, err := io.Copy(entryWriter, srcFile)
        if err != nil {
            return err
        }
        totalUncompressed += written

        log.Printf("Added %s (%d bytes)\n", relPath, written)
        return nil
    })

    if err != nil {
        return fmt.Errorf("walk error: %v", err)
    }

    log.Printf("Total uncompressed size: %d MiB\n", totalUncompressed/KiB/KiB) // Convert to MiB
    log.Printf("Total compressed size: %d MiB\n", cw.total/KiB/KiB)              // Convert to MiB

    return nil
}
