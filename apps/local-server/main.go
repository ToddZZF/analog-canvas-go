// Command analog-canvas-local serves the Analog Canvas editor and its local
// project storage from one binary, with a system tray when a desktop session
// is available. Circuits save as .icproj.json files under the data directory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Overridden at release build time with -ldflags -X.
var (
	version = "dev"
	commit  = ""
	dirty   = true
)

const defaultPort = "4173"

type trayConfiguration struct {
	url      string
	projects string
	title    string
	tooltip  string
	version  string
}

func main() {
	options := struct {
		addr        string
		dataDir     string
		projectsDir string
		noTray      bool
		noOpen      bool
	}{}
	flag.StringVar(&options.addr, "addr", "127.0.0.1:"+defaultPort, "listen address; an occupied default port falls back to a free one")
	flag.StringVar(&options.dataDir, "data-dir", "", "data directory (default ~/Analog Canvas)")
	flag.StringVar(&options.projectsDir, "projects-dir", "", "project library root (default <data-dir>/Projects)")
	flag.BoolVar(&options.noTray, "no-tray", false, "run as a plain console server without the system tray")
	flag.BoolVar(&options.noOpen, "no-open", false, "do not open the browser on startup")
	flag.Parse()

	if err := run(options); err != nil {
		log.Fatal(err)
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "Analog Canvas"
	}
	return filepath.Join(home, "Analog Canvas")
}

func run(options struct {
	addr        string
	dataDir     string
	projectsDir string
	noTray      bool
	noOpen      bool
}) error {
	dataDir := options.dataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("could not create the data directory: %w", err)
	}
	resolvedDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	stateDirectory := filepath.Join(resolvedDataDir, "state")
	projectsRoot := options.projectsDir
	if projectsRoot == "" {
		projectsRoot = filepath.Join(resolvedDataDir, "Projects")
	}
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		return fmt.Errorf("could not create the projects directory: %w", err)
	}

	listener, addr, err := listen(options.addr)
	if err != nil {
		return err
	}
	origins := []string{"http://" + addr, "http://127.0.0.1:" + portOf(addr), "http://localhost:" + portOf(addr)}
	library := newProjectLibrary(stateDirectory, projectsRoot)
	staticHandler, err := newStaticHandler()
	if err != nil {
		return fmt.Errorf("the embedded editor bundle is missing or damaged; build it with `pnpm build:local`: %w", err)
	}
	server := &localServer{
		static:     staticHandler,
		files:      newFileHandler(library, stateDirectory, origins),
		components: newComponentHandler(filepath.Join(resolvedDataDir, "components.json"), origins),
		system:     newSystemHandler(library, resolvedDataDir, version, commit, dirty, origins),
		external:   newExternalHandler(commit, origins),
	}
	httpServer := &http.Server{Handler: server}

	url := "http://" + addr
	fmt.Printf("Analog Canvas %s (commit %s", version, commitShort())
	if dirty {
		fmt.Printf(", uncommitted development build")
	}
	fmt.Printf(")\nEditor:   %s\nProjects: %s\nData:     %s\n", url, projectsRoot, resolvedDataDir)

	if !options.noOpen {
		if err := openInBrowser(url); err != nil {
			log.Printf("could not open a browser automatically; visit %s", url)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
		close(serverError)
	}()

	trayActive := false
	if !options.noTray && traySupported() {
		log.Printf("system tray active; use its menu to quit")
		trayActive = runTrayBlocking(trayConfiguration{
			url:      url,
			projects: projectsRoot,
			title:    "Analog Canvas",
			tooltip:  "Analog Canvas — " + url,
			version:  version,
		}, ctx.Done())
		if !trayActive {
			log.Printf("the system tray is unavailable; staying in console mode (Ctrl+C quits)")
		}
	}
	if !trayActive {
		select {
		case err := <-serverError:
			if err != nil {
				return fmt.Errorf("server failed: %w", err)
			}
		case <-ctx.Done():
		}
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	log.Printf("Analog Canvas stopped")
	return nil
}

// listen binds the configured address; the unoccupied default port may fall
// back to any free loopback port so double launches still work.
func listen(addr string) (net.Listener, string, error) {
	listener, err := net.Listen("tcp", addr)
	if err == nil {
		return listener, listener.Addr().String(), nil
	}
	if addr != "127.0.0.1:"+defaultPort {
		return nil, "", fmt.Errorf("could not listen on %s: %w", addr, err)
	}
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("could not listen on %s: %w", addr, err)
	}
	return listener, listener.Addr().String(), nil
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultPort
	}
	return port
}

func commitShort() string {
	if commit == "" {
		return "unknown"
	}
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
