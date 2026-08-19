//go:build !unix && !windows

package secure

func mlock([]byte) error   { return nil }
func munlock([]byte) error { return nil }
