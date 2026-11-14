//go:build windows

package kubernetes

import "os"

// checkDirOwner is a no-op on Windows. The os.FileMode.Perm bits there are
// synthesized from the read-only attribute, so the unix group/world-writable
// check is meaningless, and the file owner is not exposed as a uid.
// ensurePrivateCacheDir still creates the directory 0700 and rejects a
// symlinked path.
func checkDirOwner(os.FileInfo, string) error {
	return nil
}
