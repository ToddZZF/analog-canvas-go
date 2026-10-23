/**
 * The Project store for a note application.
 *
 * The editor's own file bridge is built for a desktop shell that owns the
 * disk. A note application owns the disk too, but it owns it in another
 * process, so this store speaks the bridge protocol instead of the file
 * system: it opens the one Project the note application hands over, and it
 * asks the note application to write that Project back.
 */
import type { CircuitProject } from "@icm/model";
import { serializeProject } from "@icm/project-protocol";

import type {
  NativeFileBinding,
  NativeProjectStore,
  NativeSaveOutcome,
} from "./native-project-store";
import type { ObsidianOpenFile } from "./obsidian-bridge";

/** The one Project a note frame owns at a time. */
const PROJECT_FILE_ID = "obsidian-project";

export interface ObsidianProjectStore extends NativeProjectStore {
  /**
   * The Project the note application asked to open. Resolves once the note
   * application has handed over its text.
   */
  readonly file: Promise<ObsidianOpenFile>;
}

export interface ObsidianProjectStoreOptions {
  /** The Project the note application handed over. */
  readonly file: Promise<ObsidianOpenFile>;
  /** Persists the Project text; resolves when the note application wrote it. */
  readonly write: (file: ObsidianOpenFile, text: string) => Promise<void>;
}

function bindingFor(file: ObsidianOpenFile): NativeFileBinding {
  const name = file.path.split(/[\\/]/u).pop() ?? file.path;
  return {
    id: PROJECT_FILE_ID,
    revision: 1,
    name,
    path: file.path,
  };
}

export function createObsidianProjectStore(
  options: ObsidianProjectStoreOptions,
): ObsidianProjectStore {
  let launched = false;
  return {
    file: options.file,
    // A note frame owns one drawing: it takes over the editor's tab instead
    // of joining the blank circuit next to it.
    singleFile: true,
    async nextLaunch() {
      // The note application opens exactly one Project per frame, so the
      // launch request is spent once.
      if (launched) return null;
      await options.file;
      launched = true;
      return PROJECT_FILE_ID;
    },
    async open() {
      try {
        const file = await options.file;
        return { status: "opened", file: bindingFor(file), text: file.text };
      } catch (error) {
        return {
          status: "failed",
          message:
            error instanceof Error
              ? error.message
              : "The note application did not hand over a Project",
        };
      }
    },
    async recent() {
      return [];
    },
    async forget() {
      // A note frame owns no recent list: the note application keeps it.
    },
    async release() {
      // Nothing is held open; the note application owns the file.
    },
    async save(
      project: CircuitProject,
      file: NativeFileBinding | null,
    ): Promise<NativeSaveOutcome> {
      try {
        // A note frame writes the drawing the note application handed over.
        // An editor that is no longer bound to a file has no drawing to
        // write: writing the hand-over path anyway would destroy the
        // drawing the note still points at.
        if (file === null) {
          return {
            status: "failed",
            message:
              "The drawing is no longer bound to a file, so it was not written",
          };
        }
        const target = file;
        const opened = await options.file;
        await options.write(
          { ...opened, path: target.path },
          serializeProject(project),
        );
        return { status: "saved", file: target };
      } catch (error) {
        return {
          status: "failed",
          message:
            error instanceof Error
              ? error.message
              : "The Project was not saved",
        };
      }
    },
  };
}
