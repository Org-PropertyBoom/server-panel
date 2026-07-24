//go:build linux

package services

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// fileOwnerGroup resolves a file's owning user + group names from its stat (Linux).
// Falls back to numeric ids when a name lookup fails.
func fileOwnerGroup(info os.FileInfo) (string, string) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}
	owner := strconv.FormatUint(uint64(st.Uid), 10)
	group := strconv.FormatUint(uint64(st.Gid), 10)
	if u, err := user.LookupId(owner); err == nil {
		owner = u.Username
	}
	if g, err := user.LookupGroupId(group); err == nil {
		group = g.Name
	}
	return owner, group
}
