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
	openEditor := xembedAddItem("Open Editor", "Open the editor in your browser")
	go func() {
		for range openEditor {
			_ = openInBrowser(config.url)
		}
	}()
	openProjects := xembedAddItem("Open Projects Folder", config.projects)
	go func() {
		for range openProjects {
			_ = openInFileManager(config.projects)
		}
	}()
	C.xembed_tray_menu_separator()
	about := xembedAddItem("About", "Analog Canvas "+config.version)
	go func() {
		for range about {
			_ = openInBrowser(externalTargets["repository"])
		}
	}()
	C.xembed_tray_menu_separator()
	quit := xembedAddItem("Quit", "Stop the local server")
	go func() {
		for range quit {
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

// xembedAddItem appends one menu entry and returns its activation channel.
func xembedAddItem(label string, tooltip string) chan struct{} {
	clicked := make(chan struct{})
	cLabel := C.CString(label)
	cTooltip := C.CString(tooltip)
	C.xembed_tray_menu_item(C.int(len(xembedMenuClicks)), cLabel, cTooltip)
	C.free(unsafe.Pointer(cLabel))
	C.free(unsafe.Pointer(cTooltip))
	xembedMenuClicks = append(xembedMenuClicks, clicked)
	return clicked
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
