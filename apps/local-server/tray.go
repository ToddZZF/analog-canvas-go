//go:build !notray

package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
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

// trayBackend selects how the icon is hosted on the desktop. The
// StatusNotifier protocol is preferred; XEmbed is the fallback for panels
// whose build hosts only legacy icons (several EL9 panel builds do).
type trayBackend int

const (
	trayStatusNotifier trayBackend = iota
	trayXEmbed
	trayNone
)

// chooseTrayBackend is the pure decision behind runTrayBlocking.
func chooseTrayBackend(goos string, sniHost bool, x11Display bool) trayBackend {
	switch goos {
	case "linux", "freebsd", "netbsd", "openbsd":
		switch {
		case sniHost:
			return trayStatusNotifier
		case x11Display:
			return trayXEmbed
		default:
			return trayNone
		}
	default:
		return trayStatusNotifier
	}
}

// sniHostPresent reports whether a StatusNotifierWatcher is on the session
// bus. Set by tests; the real probe lives in probeSNIHost.
var sniHostPresent = probeSNIHost

// probeSNIHost asks the session bus whether any panel registered the
// StatusNotifier host. Errors mean "no": fyne.io/systray gives no failure
// signal, so an absent host must be detected before choosing the backend.
func probeSNIHost() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	err = conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.NameHasOwner", dbus.Flags(0),
			500*time.Millisecond, "org.kde.StatusNotifierWatcher").Store(&owned)
	return err == nil && owned
}

// runTrayBlocking owns the main goroutine until the user quits; it returns
// false when no backend could take the icon. Each backend logs once its
// icon is live, because neither protocol reports display failures later.
func runTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	backend := chooseTrayBackend(runtime.GOOS, sniHostPresent(), os.Getenv("DISPLAY") != "")
	switch backend {
	case trayStatusNotifier:
		return runSniTrayBlocking(config, interrupt)
	case trayXEmbed:
		return runXEmbedTrayBlocking(config, interrupt)
	default:
		return false
	}
}

// runSniTrayBlocking hosts the icon through fyne.io/systray's
// StatusNotifier implementation, the native protocol of KDE, Unity and
// GNOME with the AppIndicator extension.
func runSniTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	finished := make(chan struct{})
	started := make(chan struct{})
	onReady := func() {
		defer func() { close(started) }()
		log.Printf("system tray active (StatusNotifier); use its menu to quit")
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
