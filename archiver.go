package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func CreateZipArchive(srcDir string, writer io.Writer, encryptionType, password string, ignoreFile string) error {

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
	fmt.Printf("Creating zip archive for %s\n", srcDir)
	totalUncompressed := int64(0)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if ignorer != nil && ignorer.MatchesPath(relPath) {
			fmt.Printf("Ignoring %s\n", relPath)
			return nil
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		var entryWriter io.Writer
		if password != "" {
			var encMethod zip.EncryptionMethod
			switch encryptionType {
			case EncryptionTypeZipCrypto:
				encMethod = zip.StandardEncryption
			case EncryptionTypeAES128:
				encMethod = zip.AES128Encryption
			case EncryptionTypeAES192:
				encMethod = zip.AES192Encryption
			case EncryptionTypeAES256:
				encMethod = zip.AES256Encryption
			default:
				encMethod = zip.StandardEncryption
			}
			entryWriter, err = zipWriter.Encrypt(relPath, password, encMethod)
		} else {
			header := &zip.FileHeader{
				Name:   relPath,
				Method: zip.Deflate,
			}
			entryWriter, err = zipWriter.CreateHeader(header)
		}
		if err != nil {
			return err
		}

		written, err := io.Copy(entryWriter, srcFile)
		if err != nil {
			return err
		}
		totalUncompressed += written

		fmt.Printf("Added %s (%d bytes)\n", relPath, written)
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk error: %v", err)
	}

	fmt.Printf("\nTotal uncompressed size: %d MiB\n", totalUncompressed/1024/1024)
	fmt.Printf("Total compressed size: %d MiB\n", cw.total/1024/1024)

	return nil
}
