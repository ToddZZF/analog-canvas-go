//go:build !notray

package main

import "testing"

func TestChooseTrayBackend(t *testing.T) {
	cases := []struct {
		name string
		goos string
		sni  bool
		x11  bool
		want trayBackend
	}{
		{"SNI host wins on Linux", "linux", true, true, trayStatusNotifier},
		{"SNI host needs no X11", "linux", true, false, trayStatusNotifier},
		{"XEmbed fallback without an SNI host", "linux", false, true, trayXEmbed},
		{"No host and no X11 stays console", "linux", false, false, trayNone},
		{"FreeBSD follows the same rule", "freebsd", false, true, trayXEmbed},
		{"Windows is always StatusNotifier", "windows", false, false, trayStatusNotifier},
		{"macOS is always StatusNotifier", "darwin", false, false, trayStatusNotifier},
	}
	for _, c := range cases {
		if got := chooseTrayBackend(c.goos, c.sni, c.x11); got != c.want {
			t.Errorf("%s: backend %v, want %v", c.name, got, c.want)
		}
	}
}
