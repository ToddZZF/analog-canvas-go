//go:build !notray && (linux || freebsd || netbsd || openbsd)

package main

import (
	"log"
	"os"
	"runtime"
	"unsafe"
)

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>

int xembed_tray_init(void);
int xembed_tray_set_icon_file(const char *path);
void xembed_tray_set_tooltip(const char *text);
void xembed_tray_menu_item(int id, const char *label, const char *tooltip);
void xembed_tray_menu_header(int id, const char *label);
void xembed_tray_menu_item_update(int id, const char *label, const char *tooltip, int visible);
void xembed_tray_menu_separator(void);
void xembed_tray_run(void);
void xembed_tray_quit(void);
*/
import "C"

// xembedMenuClicks receives one send per activation, in menu order. The
// slice is fully populated before the GTK loop starts, and the exported
// callback only reads it, so no locking is needed.
var xembedMenuClicks []chan struct{}

//export xembedMenuSelected
func xembedMenuSelected(id C.int) {
	if int(id) >= 0 && int(id) < len(xembedMenuClicks) {
		xembedMenuClicks[id] <- struct{}{}
	}
}

// runXEmbedTrayBlocking hosts the icon through GTK's status icon — the
// legacy XEmbed tray protocol — for panels whose build hosts only legacy
// icons. The X11 session existence is decided by the backend selector; a
// panel without any tray manager still cannot show the icon, which no
// protocol change here can fix.
func runXEmbedTrayBlocking(config trayConfiguration, interrupt <-chan struct{}) bool {
	// Every GTK call must land on one OS thread, including the loop below.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if C.xembed_tray_init() == 0 {
		return false
	}
	if icon := trayIcon(); len(icon) == 0 {
		return false
	} else if !writeXEmbedIcon(icon) {
		log.Printf("the tray icon asset could not be loaded; staying in console mode")
		return false
	}
	cTip := C.CString(config.tooltip)
	C.xembed_tray_set_tooltip(cTip)
	C.free(unsafe.Pointer(cTip))

	newXEmbedRecentSection(config)
	C.xembed_tray_menu_separator()
	openEditor := xembedAddItem("Open Editor", "Open the editor in your browser")
	go func() {
		for range openEditor.clicked {
			_ = openInBrowser(config.url)
		}
	}()
	openProjects := xembedAddItem("Open Projects Folder", config.projects)
	go func() {
		for range openProjects.clicked {
			_ = openInFileManager(config.projects)
		}
	}()
	C.xembed_tray_menu_separator()
	about := xembedAddItem("About", "Analog Canvas "+config.version)
	go func() {
		for range about.clicked {
			_ = openInBrowser(externalTargets["repository"])
		}
	}()
	C.xembed_tray_menu_separator()
	quit := xembedAddItem("Quit", "Stop the local server")
	go func() {
		for range quit.clicked {
			xembedTrayQuit()
		}
	}()

	finished := make(chan struct{})
	go func() {
		select {
		case <-interrupt:
			xembedTrayQuit()
		case <-finished:
		}
	}()

	log.Printf("system tray active (XEmbed); use its menu to quit")
	C.xembed_tray_run()
	close(finished)
	return true
}

func xembedTrayQuit() {
	C.xembed_tray_quit()
}

// xembedRow is one menu entry: its activation channel and the id the C side
// knows the row by.
type xembedRow struct {
	clicked chan struct{}
	id      int
}

// xembedAddItem appends one menu entry.
func xembedAddItem(label string, tooltip string) xembedRow {
	row := xembedRow{clicked: make(chan struct{}), id: len(xembedMenuClicks)}
	cLabel := C.CString(label)
	cTooltip := C.CString(tooltip)
	C.xembed_tray_menu_item(C.int(row.id), cLabel, cTooltip)
	C.free(unsafe.Pointer(cLabel))
	C.free(unsafe.Pointer(cTooltip))
	xembedMenuClicks = append(xembedMenuClicks, row.clicked)
	return row
}

// xembedAddHeaderItem appends an insensitive entry naming the section that
// follows it. Its activation channel stays empty (GTK never fires it), but
// it keeps the id numbering aligned with the C side.
func xembedAddHeaderItem(label string) xembedRow {
	row := xembedRow{clicked: make(chan struct{}), id: len(xembedMenuClicks)}
	cLabel := C.CString(label)
	C.xembed_tray_menu_header(C.int(row.id), cLabel)
	C.free(unsafe.Pointer(cLabel))
	xembedMenuClicks = append(xembedMenuClicks, row.clicked)
	return row
}

// xembedRecentSection mirrors the StatusNotifier recent section on the GTK
// menu: trayRecentCount fixed rows whose labels and visibility are
// rewritten on the GTK thread through the idle queue, never from here.
type xembedRecentSection struct {
	config trayConfiguration
	slots  recentSlots
	header xembedRow
	items  []xembedRow
}

func newXEmbedRecentSection(config trayConfiguration) {
	section := &xembedRecentSection{config: config}
	section.header = xembedAddHeaderItem("Recent Projects")
	for range trayRecentCount {
		section.items = append(section.items, xembedAddItem("", ""))
	}
	for slot, row := range section.items {
		watchRecentSlot(config, &section.slots, slot, row.clicked)
	}
	watchRecent(config, section.refresh)
}

func (section *xembedRecentSection) refresh() {
	projects := section.config.recent(trayRecentCount)
	for index, row := range section.items {
		if index < len(projects) {
			xembedUpdateItem(row.id, menuProjectName(projects[index].Name), projects[index].Name, true)
		} else {
			xembedUpdateItem(row.id, "", "", false)
		}
	}
	xembedUpdateItem(section.header.id, "Recent Projects", "", len(projects) > 0)
	section.slots.publish(recentSlotIds(projects))
}

// xembedUpdateItem asks the GTK loop to rewrite one row; safe from any
// thread, applied on the thread that runs the loop.
func xembedUpdateItem(id int, label string, tooltip string, visible bool) {
	cLabel := C.CString(label)
	defer C.free(unsafe.Pointer(cLabel))
	cTooltip := C.CString(tooltip)
	defer C.free(unsafe.Pointer(cTooltip))
	shown := 0
	if visible {
		shown = 1
	}
	C.xembed_tray_menu_item_update(C.int(id), cLabel, cTooltip, C.int(shown))
}

// writeXEmbedIcon hands the embedded PNG to GTK through a temp file, which
// gdk_pixbuf_new_from_file reads synchronously before the file disappears.
func writeXEmbedIcon(icon []byte) bool {
	tmp, err := os.CreateTemp("", "analog-canvas-tray-*.png")
	if err != nil {
		return false
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(icon); err != nil {
		_ = tmp.Close()
		return false
	}
	if err := tmp.Close(); err != nil {
		return false
	}
	cpath := C.CString(name)
	defer func() { C.free(unsafe.Pointer(cpath)) }()
	return C.xembed_tray_set_icon_file(cpath) == 1
}
