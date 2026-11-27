// SPDX-License-Identifier: Apache-2.0

package ptyx

// ring keeps the last size bytes written to it. It is not safe for
// concurrent use; Process guards it with its mutex.
type ring struct {
	buf  []byte
	size int
}

func newRing(size int) *ring {
	return &ring{buf: make([]byte, 0, size), size: size}
}

func (r *ring) Write(p []byte) {
	if len(p) >= r.size {
		r.buf = append(r.buf[:0], p[len(p)-r.size:]...)
		return
	}
	if over := len(r.buf) + len(p) - r.size; over > 0 {
		r.buf = append(r.buf[:0], r.buf[over:]...)
	}
	r.buf = append(r.buf, p...)
}

// Bytes returns a copy of the kept bytes.
func (r *ring) Bytes() []byte {
	return append([]byte(nil), r.buf...)
}
