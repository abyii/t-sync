package main

import (
	"fmt"
	"os"
)

func exitWithErrorCode(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
