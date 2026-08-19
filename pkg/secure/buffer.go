// Package secure provides best-effort protection for sensitive byte buffers:
// optional mlock/VirtualLock to reduce swap risk, and explicit zeroing.
package secure

import (
	"fmt"
	"runtime"
)

// Buffer holds sensitive bytes that should be zeroed after use.
type Buffer struct {
	data   []byte
	locked bool
}

// New allocates an unlocked buffer of size n (zero-filled).
func New(n int) *Buffer {
	b := &Buffer{data: make([]byte, n)}
	runtime.SetFinalizer(b, (*Buffer).Zero)
	return b
}

// Clone copies src into a new Buffer. src is not modified.
func Clone(src []byte) *Buffer {
	b := New(len(src))
	copy(b.data, src)
	return b
}

// Consume copies src into a new Buffer and zeroes src.
func Consume(src []byte) *Buffer {
	b := Clone(src)
	Zero(src)
	return b
}

// Bytes returns the underlying slice. Callers must not retain it past Zero.
func (b *Buffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data
}

// Len returns the buffer length.
func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}
	return len(b.data)
}

// Lock attempts to pin the buffer in RAM (mlock / VirtualLock). Best-effort.
func (b *Buffer) Lock() error {
	if b == nil || len(b.data) == 0 || b.locked {
		return nil
	}
	if err := mlock(b.data); err != nil {
		return fmt.Errorf("secure: lock: %w", err)
	}
	b.locked = true
	return nil
}

// Zero overwrites the buffer with zeros and unlocks if locked.
func (b *Buffer) Zero() {
	if b == nil {
		return
	}
	Zero(b.data)
	if b.locked {
		_ = munlock(b.data)
		b.locked = false
	}
	runtime.SetFinalizer(b, nil)
}

// Zero overwrites every byte of b with 0.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
