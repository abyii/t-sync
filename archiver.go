package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// openFileWithRetry retries transient open failures (e.g. a file briefly locked
// by another process) with 1s and 2s backoff. Missing or permission-denied
// files fail immediately.
func openFileWithRetry(path string) (*os.File, error) {
	const attempts = 3
	var err error
	for i := range attempts {
		var file *os.File
		file, err = os.Open(path)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, err
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(1<<i) * time.Second)
		}
	}
	return nil, err
}

func CreateZipArchive(srcDir string, writer io.Writer, compressionLevel int, password string, ignoreFile string) (err error) {

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

	// Close writes the central directory; without it the archive is unreadable.
	defer func() {
		if closeErr := zipWriter.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to finalize zip archive: %w", closeErr)
		}
	}()
	log.Printf("Creating zip archive for %s\n", srcDir)
	totalUncompressed := int64(0)

	copyBuf := make([]byte, 32*KiB)

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		zipPath := filepath.ToSlash(relPath)

		checkPath := zipPath
		if d.IsDir() {
			checkPath += "/" // for directories, we should check with a trailing slash
		}

		if ignorer != nil && ignorer.MatchesPath(checkPath) {
			log.Printf("Ignoring %s\n", relPath)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			dirName := zipPath + "/"
			if _, err := zipWriter.Create(dirName, zip.Store, 0, zip.NoEncryption, ""); err != nil {
				log.Printf("Failed to create zip entry for directory %s: %v\n", dirName, err)
				return err
			}
			log.Printf("Added directory %s\n", dirName)
			return nil
		}

		if !d.Type().IsRegular() {
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
			entryWriter, err = zipWriter.Create(zipPath, zip.Deflate, dynamicLevel, zip.StandardEncryption, password)
		} else {
			entryWriter, err = zipWriter.Create(zipPath, zip.Deflate, dynamicLevel, zip.NoEncryption, "")
		}
		if err != nil {
			log.Printf("Failed to create zip entry for file %s: %v\n", zipPath, err)
			return err
		}

		written, err := io.CopyBuffer(entryWriter, struct{ io.Reader }{srcFile}, copyBuf)
		if err != nil {
			return err
		}
		totalUncompressed += written

		log.Printf("Added %s (%d bytes)\n", zipPath, written)
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk error: %v", err)
	}

	log.Printf("Total uncompressed size: %d MiB\n", totalUncompressed/KiB/KiB) // Convert to MiB
	log.Printf("Total compressed size: %d MiB\n", cw.total/KiB/KiB)            // Convert to MiB

	return nil
}
