//go:build !notray

package main

import (
	"embed"
	"fmt"
	"os"
	"runtime"

	"fyne.io/systray"
)

// The tray keeps the server a desktop citizen: the editor is one click away,
// the project folder opens in the system file manager, and quitting stops
// the HTTP server cleanly. Built without cgo via `-tags notray`, which
// replaces this file with a stub.

//go:embed assets/tray.png
var trayAssets embed.FS

func trayIcon() []byte {
	icon, err := trayAssets.ReadFile("assets/tray.png")
	if err != nil {
		return nil
	}
	return icon
}

// traySupported reports whether a desktop session exists to host the icon.
// GNOME needs the AppIndicator extension; without any session bus the
// console fallback runs instead.
func traySupported() bool {
	switch runtime.GOOS {
	case "linux", "freebsd", "netbsd", "openbsd":
		return os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" &&
			(os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "")
	default:
		return true
	}
}

// runTrayBlocking owns the main goroutine until the user quits; macOS
// requires the tray loop to run there. Whether a session can host the icon
// is decided by traySupported() before this runs; systray.Run itself gives
// no failure signal, so an icon that never appears leaves the server
// running until Quit or a signal.
func runTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	finished := make(chan struct{})
	started := make(chan struct{})
	onReady := func() {
		defer func() { close(started) }()
		systray.SetIcon(trayIcon())
		systray.SetTitle(config.title)
		systray.SetTooltip(config.tooltip)
		openEditor := systray.AddMenuItem("Open Editor", "Open the editor in your browser")
		go func() {
			for range openEditor.ClickedCh {
				_ = openInBrowser(config.url)
			}
		}()
		openProjects := systray.AddMenuItem("Open Projects Folder", config.projects)
		go func() {
			for range openProjects.ClickedCh {
				_ = openInFileManager(config.projects)
			}
		}()
		systray.AddSeparator()
		about := systray.AddMenuItem("About", fmt.Sprintf("Analog Canvas %s", config.version))
		go func() {
			for range about.ClickedCh {
				_ = openInBrowser(externalTargets["repository"])
			}
		}()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop the local server")
		go func() {
			for range quit.ClickedCh {
				systray.Quit()
			}
		}()
	}
	onExit := func() {
		select {
		case <-finished:
		default:
			close(finished)
		}
	}
	go func() {
		<-started
		select {
		case <-interrupt:
			systray.Quit()
		case <-finished:
		}
	}()
	systray.Run(onReady, onExit)
	// Let the quit path win any race with a concurrent interrupt signal.
	return true
}
