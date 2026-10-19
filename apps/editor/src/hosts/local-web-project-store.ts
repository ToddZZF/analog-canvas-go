// The Go local server has no native file dialog, so File → Open Project…
// uses the browser's own picker and imports the chosen file as an
// independent library project through the content-based open route.
import { nativeProjectRequest as post } from "./native-project-request";
import {
  binding,
  createNativeProjectStore,
  type NativeFileBinding,
  type NativeProjectStore,
} from "./native-project-store";

type OpenOutcome = Awaited<ReturnType<NativeProjectStore["open"]>>;

const openJournalPrefix = "icm.local.open";

function pickProjectFile(): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".icproj.json,.icproj,application/json";
    let settled = false;
    const finish = (file: File | null) => {
      if (settled) return;
      settled = true;
      resolve(file);
    };
    input.addEventListener("change", () =>
      finish(input.files?.[0] ?? null),
    );
    input.addEventListener("cancel", () => finish(null));
    input.click();
  });
}

function asOutcome(body: Record<string, unknown>): OpenOutcome {
  if (body.status === "cancelled") return { status: "cancelled" };
  const file = body.file as unknown;
  if (body.status === "opened" && binding(file) && typeof body.text === "string")
    return { status: "opened", file: file as NativeFileBinding, text: body.text };
  return {
    status: body.status === "conflict" ? "conflict" : "failed",
    message:
      typeof body.message === "string" ? body.message : "File operation failed",
  };
}

async function openThroughPicker(): Promise<OpenOutcome> {
  const file = await pickProjectFile();
  if (!file) return { status: "cancelled" };
  const text = await file.text();
  // A lost response must not duplicate the import on retry.
  const journalKey = `${openJournalPrefix}:${file.name}:${file.size}:${file.lastModified}`;
  const creationKey = localStorage.getItem(journalKey) ?? crypto.randomUUID();
  localStorage.setItem(journalKey, creationKey);
  try {
    const outcome = asOutcome(
      await post("open", { text, name: file.name, creationKey }),
    );
    if (outcome.status === "opened") localStorage.removeItem(journalKey);
    return outcome;
  } catch (error) {
    return {
      status: "failed",
      message:
        error instanceof Error ? error.message : "Invalid file response",
    };
  }
}

export function createLocalWebProjectStore(): NativeProjectStore {
  const store = createNativeProjectStore();
  return {
    ...store,
    async open(recentId) {
      return recentId ? store.open(recentId) : openThroughPicker();
    },
  };
}
