//go:build !windows

package kubernetes

import (
	"fmt"
	"os"
	"syscall"
)

// checkDirOwner fails closed on a CA cache directory that another local user
// could have pre-created: group or world writable modes, or an owner other
// than the current user. World-readable is tolerated: the CA files themselves
// are 0600, and read access cannot swap a trust anchor.
func checkDirOwner(fi os.FileInfo, dir string) error {
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("CA cache directory %s is group or world writable (mode %#o)", dir, perm)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("CA cache directory %s is owned by uid %d, not the current user", dir, st.Uid)
	}
	return nil
}
