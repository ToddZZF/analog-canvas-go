//go:build !notray

package main

import (
	"strings"
	"testing"
)

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

func TestMenuProjectNameFitsOneRow(t *testing.T) {
	if got := menuProjectName("Amplifier"); got != "Amplifier" {
		t.Fatalf("menuProjectName = %q", got)
	}
	got := menuProjectName(strings.Repeat("名", 80))
	runes := []rune(got)
	if len(runes) != 64 || !strings.HasSuffix(got, "…") {
		t.Fatalf("menuProjectName kept %d runes without an ellipsis suffix", len(runes))
	}
}

func TestRecentSlotsPublishThenRead(t *testing.T) {
	slots := recentSlots{}
	if id := slots.at(0); id != "" {
		t.Fatalf("empty slots answered %q", id)
	}
	slots.publish([]string{"first", "second"})
	if slots.at(0) != "first" || slots.at(1) != "second" || slots.at(2) != "" {
		t.Fatalf("slots = %q, %q, %q", slots.at(0), slots.at(1), slots.at(2))
	}
}
