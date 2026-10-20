//go:build !notray && !(linux || freebsd || netbsd || openbsd)

package main

// The XEmbed fallback only exists where X11 panels host legacy icons; the
// native platforms run the StatusNotifier backend exclusively.

func runXEmbedTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	return false
}
