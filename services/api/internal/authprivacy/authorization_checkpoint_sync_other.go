//go:build !linux

package authprivacy

import "os"

// Platforms without the Linux data-only durability call retain full fsync.
func syncAuthorizationCheckpointData(f *os.File) error {
	return f.Sync()
}
