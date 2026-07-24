//go:build !linux

package services

import "os"

// fileOwnerGroup is a no-op off Linux (owner/group come from a syscall stat).
func fileOwnerGroup(os.FileInfo) (string, string) { return "", "" }
