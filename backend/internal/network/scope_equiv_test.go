package network

import (
	"net"
	"testing"
)

func TestClassifyIPv4ScopeBytesMatchesNetPath(t *testing.T) {
	cases := []struct{ a, b, c, d byte }{
		{0, 0, 0, 0}, {0, 1, 2, 3}, {10, 0, 0, 1}, {100, 64, 0, 1}, {100, 128, 0, 1},
		{127, 0, 0, 1}, {169, 254, 0, 1}, {172, 16, 0, 1}, {172, 32, 0, 1},
		{192, 0, 0, 1}, {192, 0, 2, 1}, {192, 88, 99, 1}, {192, 168, 1, 1},
		{198, 18, 0, 1}, {198, 19, 255, 1}, {198, 51, 100, 1}, {203, 0, 113, 9},
		{224, 0, 0, 1}, {239, 255, 255, 255}, {240, 0, 0, 1}, {255, 255, 255, 255},
		{93, 184, 216, 34}, {8, 8, 8, 8}, {1, 1, 1, 1}, {100, 63, 255, 254}, {100, 127, 0, 0},
	}
	for _, c := range cases {
		ip := net.IPv4(c.a, c.b, c.c, c.d)
		want := ClassifyIPScope(ip)
		got := ClassifyIPv4ScopeBytes(c.a, c.b, c.c, c.d)
		if got != want {
			t.Fatalf("ClassifyIPv4ScopeBytes(%d,%d,%d,%d) = %v, want %v", c.a, c.b, c.c, c.d, got, want)
		}
	}
}
