//go:build linux

package authprivacy

import (
	"os"

	"golang.org/x/sys/unix"
)

func syncAuthorizationCheckpointData(f *os.File) error {
	return unix.Fdatasync(int(f.Fd()))
}
