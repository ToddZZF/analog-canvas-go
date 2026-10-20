//go:build !notray

package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
)

// The tray keeps the server a desktop citizen: the editor is one click away,
// recent projects reopen straight from the menu, the project folder opens in
// the system file manager, and quitting stops the HTTP server cleanly. Built
// without cgo via `-tags notray`, which replaces this file with a stub.

//go:embed assets/tray.png assets/tray.ico
var trayAssets embed.FS

// trayIcon returns the app icon the panel shows. Both files are the editor
// bundle's own web icon (apps/editor/public/icon-192.png); regenerate them
// together when that artwork changes, nothing keeps the copies in sync.
func trayIcon() []byte {
	name := "assets/tray.png"
	if runtime.GOOS == "windows" {
		// Shell_NotifyIcon loads icons through LoadImage, which reads ICO
		// files only; every other platform takes the PNG.
		name = "assets/tray.ico"
	}
	icon, err := trayAssets.ReadFile(name)
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

// trayRecentCount bounds the recent section; the recent index itself keeps 20.
const trayRecentCount = 5

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
		addSniRecentSection(config)
		systray.AddSeparator()
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

// menuProjectName fits a project name into one menu row.
func menuProjectName(name string) string {
	const limit = 64
	runes := []rune(name)
	if len(runes) <= limit {
		return name
	}
	return string(runes[:limit-1]) + "…"
}

// recentSlots maps the fixed menu rows to the recent ids they currently
// open. Refresh rewrites labels first and publishes ids last, so a click
// that slips in mid-refresh still opens the project whose label it read.
type recentSlots struct {
	mutex sync.Mutex
	ids   []string
}

func (slots *recentSlots) publish(ids []string) {
	slots.mutex.Lock()
	defer slots.mutex.Unlock()
	slots.ids = ids
}

func (slots *recentSlots) at(index int) string {
	slots.mutex.Lock()
	defer slots.mutex.Unlock()
	if index < 0 || index >= len(slots.ids) {
		return ""
	}
	return slots.ids[index]
}

// openRecentProject queues the deep link, then opens the editor page that
// consumes it through the next-launch handshake.
func openRecentProject(config trayConfiguration, slots *recentSlots, slot int) {
	id := slots.at(slot)
	if id == "" {
		return
	}
	config.queue(id)
	_ = openInBrowser(config.url)
}

// watchRecentSlot routes one recent row's activations to the deep link;
// each backend calls it once per row.
func watchRecentSlot(config trayConfiguration, slots *recentSlots, slot int, clicked <-chan struct{}) {
	go func() {
		for range clicked {
			openRecentProject(config, slots, slot)
		}
	}()
}

// recentSlotIds lays the projects into the fixed slot count; slots past the
// end stay empty and render hidden.
func recentSlotIds(projects []trayProject) []string {
	ids := make([]string, trayRecentCount)
	for index, project := range projects {
		ids[index] = project.ID
	}
	return ids
}

// watchRecent refreshes the section once now and again after every
// recent-list change; the file handler coalesces save bursts into one
// signal. Backends call it only once their rows exist.
func watchRecent(config trayConfiguration, refresh func()) {
	go func() {
		refresh()
		for range config.changed() {
			refresh()
		}
	}()
}

// sniRecentSection owns the recent rows of the StatusNotifier menu: a
// disabled heading plus trayRecentCount item slots shown or hidden as the
// recent list changes.
type sniRecentSection struct {
	config  trayConfiguration
	slots   recentSlots
	heading *systray.MenuItem
	items   []*systray.MenuItem
}

// addSniRecentSection inserts the section at the menu's top, where the most
// recent projects sit above the fixed actions, and keeps it live.
func addSniRecentSection(config trayConfiguration) {
	section := &sniRecentSection{config: config}
	section.heading = systray.AddMenuItem("Recent Projects", "Projects opened recently")
	section.heading.Disable()
	section.heading.Hide()
	for range trayRecentCount {
		item := systray.AddMenuItem("", "")
		item.Hide()
		section.items = append(section.items, item)
	}
	for slot, item := range section.items {
		watchRecentSlot(config, &section.slots, slot, item.ClickedCh)
	}
	watchRecent(config, section.refresh)
}

func (section *sniRecentSection) refresh() {
	projects := section.config.recent(trayRecentCount)
	for index, item := range section.items {
		if index < len(projects) {
			item.SetTitle(menuProjectName(projects[index].Name))
			item.SetTooltip(projects[index].Name)
			item.Show()
		} else {
			item.Hide()
		}
	}
	if len(projects) > 0 {
		section.heading.Show()
	} else {
		section.heading.Hide()
	}
	section.slots.publish(recentSlotIds(projects))
}
