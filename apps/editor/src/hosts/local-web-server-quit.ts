// The Go local server's tray icon needs a StatusNotifier/AppIndicator host;
// desktops without one (vanilla GNOME, several panels) leave the server
// without its Quit menu, so the editor's File menu quits over HTTP instead,
// on the same graceful shutdown path as the tray and Ctrl+C.
import "./local-web-server-quit.css";

export const LOCAL_SERVER_STOPPED_ID = "local-web-server-stopped";

const QUIT_MESSAGE =
  "Quit the local server? Every open editor tab stops, and unsaved changes are not written to disk.";

export interface StoppedScreenDocument {
  createElement(tag: string): {
    id: string;
    textContent: string;
    append(...children: unknown[]): void;
  };
  getElementById(id: string): unknown;
  body: { append(...children: unknown[]): void };
}

export interface LocalServerQuitEnvironment {
  confirm(message: string): boolean;
  fetch: typeof fetch;
  showStoppedScreen(document: StoppedScreenDocument): void;
  document: StoppedScreenDocument;
}

function browserEnvironment(): LocalServerQuitEnvironment {
  return {
    confirm: (message) => window.confirm(message),
    fetch: window.fetch.bind(window),
    showStoppedScreen: showLocalServerStopped,
    document: window.document as unknown as StoppedScreenDocument,
  };
}

/**
 * Confirms inside the page (a browser tab has no native dialog), quits the
 * server, and pins a stopped notice over the page, which outlives the
 * server. Throws when the server answers with an error, so the menu can
 * report it while the server is still running.
 */
export async function quitLocalServer(
  environment: LocalServerQuitEnvironment = browserEnvironment(),
): Promise<void> {
  if (!environment.confirm(QUIT_MESSAGE)) return;
  const response = await environment.fetch("/desktop/quit", {
    method: "POST",
  });
  if (!response.ok)
    throw new Error(`The local server could not quit (${response.status})`);
  environment.showStoppedScreen(environment.document);
}

/** The full-page notice shown after the server has quit. */
export function showLocalServerStopped(document_: StoppedScreenDocument): void {
  if (document_.getElementById(LOCAL_SERVER_STOPPED_ID)) return;
  const screen = document_.createElement("div");
  screen.id = LOCAL_SERVER_STOPPED_ID;
  const title = document_.createElement("h1");
  title.textContent = "Analog Canvas stopped";
  const note = document_.createElement("p");
  note.textContent =
    "The local server has quit. Close this browser tab; start the server again to keep editing.";
  screen.append(title, note);
  document_.body.append(screen);
}
