package main

import (
	"os/exec"
	"runtime"
)

// The server shares the user's desktop session, so it hosts the system
// integrations a browser page cannot: revealing folders and opening links.
func openWith(arguments ...string) error {
	command := exec.Command(arguments[0], arguments[1:]...)
	return command.Start()
}

func openInBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return openWith("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return openWith("open", url)
	default:
		return openWith("xdg-open", url)
	}
}

func openInFileManager(path string) error {
	switch runtime.GOOS {
	case "windows":
		return openWith("explorer", path)
	case "darwin":
		return openWith("open", path)
	default:
		return openWith("xdg-open", path)
	}
}
