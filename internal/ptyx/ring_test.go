// SPDX-License-Identifier: Apache-2.0

package ptyx

import "testing"

func TestRing(t *testing.T) {
	r := newRing(5)
	steps := []struct{ write, want string }{
		{"ab", "ab"},
		{"cde", "abcde"},
		{"f", "bcdef"},
		{"ghijklm", "ijklm"},
		{"", "ijklm"},
		{"nopqr", "nopqr"},
	}
	for _, s := range steps {
		r.Write([]byte(s.write))
		if got := string(r.Bytes()); got != s.want {
			t.Fatalf("after %q: %q, want %q", s.write, got, s.want)
		}
	}
}

func TestRingBytesIsACopy(t *testing.T) {
	r := newRing(4)
	r.Write([]byte("abcd"))
	b := r.Bytes()
	b[0] = 'x'
	if string(r.Bytes()) != "abcd" {
		t.Error("Bytes() shares memory with the ring")
	}
}
