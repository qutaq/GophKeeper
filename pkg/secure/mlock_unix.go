//go:build unix

package secure

import "golang.org/x/sys/unix"

func mlock(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Mlock(b)
}

func munlock(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Munlock(b)
}
