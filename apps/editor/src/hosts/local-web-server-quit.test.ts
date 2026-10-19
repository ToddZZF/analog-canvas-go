import { describe, expect, it, vi } from "vitest";

import {
  LOCAL_SERVER_STOPPED_ID,
  quitLocalServer,
  showLocalServerStopped,
  type LocalServerQuitEnvironment,
  type StoppedScreenDocument,
} from "./local-web-server-quit";

/** A fake page: records confirms, fetches and the stopped screen. */
function page({ confirm = true, status = 200 } = {}) {
  const events: string[] = [];
  const document_: StoppedScreenDocument = {
    createElement: (tag) => {
      events.push(`create ${tag}`);
      return {
        id: "",
        textContent: "",
        append: (...children: unknown[]) => {
          events.push(`compose ${children.length}`);
        },
      };
    },
    getElementById: (id) => {
      events.push(`lookup ${id}`);
      return null;
    },
    body: {
      append: (...children: unknown[]) => {
        events.push(`append ${children.length}`);
      },
    },
  };
  const environment: LocalServerQuitEnvironment = {
    confirm: (message) => {
      events.push(`confirm:${message}`);
      return confirm;
    },
    fetch: vi.fn(async (input: RequestInfo | URL) => {
      events.push(`fetch ${String(input)}`);
      return new Response("{}", { status });
    }) as unknown as typeof fetch,
    showStoppedScreen: showLocalServerStopped,
    document: document_,
  };
  return { events, environment };
}

describe("the File menu's Quit Local Server", () => {
  it("confirms, quits over HTTP and pins the stopped screen", async () => {
    const { events, environment } = page();
    await quitLocalServer(environment);
    expect(events).toEqual([
      "confirm:Quit the local server? Every open editor tab stops, and unsaved changes are not written to disk.",
      "fetch /desktop/quit",
      `lookup ${LOCAL_SERVER_STOPPED_ID}`,
      "create div",
      "create h1",
      "create p",
      "compose 2",
      "append 1",
    ]);
  });

  it("does not touch the server when the confirmation is declined", async () => {
    const { events, environment } = page({ confirm: false });
    await quitLocalServer(environment);
    expect(events).toEqual([
      "confirm:Quit the local server? Every open editor tab stops, and unsaved changes are not written to disk.",
    ]);
  });

  it("reports a failing server and leaves the page alone", async () => {
    const { events, environment } = page({ status: 500 });
    await expect(quitLocalServer(environment)).rejects.toThrow(
      "The local server could not quit (500)",
    );
    expect(events).not.toContain("append 1");
  });

  it("shows the stopped notice only once per page", () => {
    const { events, environment } = page();
    const present: StoppedScreenDocument = {
      ...environment.document,
      getElementById: (id) => {
        events.push(`lookup ${id}`);
        return { id };
      },
    };
    showLocalServerStopped(present);
    expect(events).toEqual([`lookup ${LOCAL_SERVER_STOPPED_ID}`]);
  });
});
