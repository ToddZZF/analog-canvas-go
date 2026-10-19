//go:build notray

package main

// The pure-Go build without a system tray: headless servers, SSH sessions
// and CGO-free cross compilation use the console wait instead.

func trayIcon() []byte { return nil }

func traySupported() bool { return false }

func runTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	return false
}
