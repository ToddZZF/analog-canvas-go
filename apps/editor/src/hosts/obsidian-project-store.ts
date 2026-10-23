/**
 * The Project store for a note application.
 *
 * The editor's own file bridge is built for a desktop shell that owns the
 * disk. A note application owns the disk too, but it owns it in another
 * process, so this store speaks the bridge protocol instead of the file
 * system: it opens the Project the note application hands over, and it asks
 * the note application to write that Project back.
 *
 * A frame is handed its drawing again whenever the pane shows another
 * drawing, so the store serves the drawing that was named last and owes one
 * launch per hand-over.
 */
import type { CircuitProject } from "@icm/model";
import { serializeProject } from "@icm/project-protocol";

import type {
  NativeFileBinding,
  NativeProjectStore,
  NativeSaveOutcome,
} from "./native-project-store";
import type { ObsidianOpenFile } from "./obsidian-bridge";

/** The Project a note frame owns at a time. */
const PROJECT_FILE_ID = "obsidian-project";

export interface ObsidianProjectStoreOptions {
  /** The drawing the note application last handed over. */
  readonly file: () => ObsidianOpenFile | undefined;
  /**
   * Persists the Project text and resolves to the file the note application
   * wrote. The note application owns the name: a drawing that was renamed
   * while it was open comes back under the name it has now.
   */
  readonly write: (
    file: ObsidianOpenFile,
    text: string,
  ) => Promise<ObsidianOpenFile>;
}

export type ObsidianProjectStore = NativeProjectStore;

function nameOf(path: string): string {
  return path.split(/[\\/]/u).pop() ?? path;
}

function bindingFor(file: ObsidianOpenFile): NativeFileBinding {
  return {
    // Each drawing keeps an identity of its own. A name every drawing shared
    // would let one drawing stand in for the next one the pane shows.
    id: `${PROJECT_FILE_ID}:${file.path}`,
    revision: 1,
    name: nameOf(file.path),
    path: file.path,
  };
}

export function createObsidianProjectStore(
  options: ObsidianProjectStoreOptions,
): ObsidianProjectStore {
  /** The hand-over whose drawing the editor was given, if any. */
  let served: ObsidianOpenFile | undefined;
  return {
    // A note frame owns one drawing: it takes over the editor's tab instead
    // of joining the blank circuit next to it.
    singleFile: true,
    async nextLaunch() {
      // The frame owes a launch while the note application has named a
      // drawing the editor has not been given yet. Handing the drawing over
      // again — another drawing, or the same one under a new name — owes
      // the next launch.
      const file = options.file();
      if (file === undefined || file === served) return null;
      return `${PROJECT_FILE_ID}:${file.path}`;
    },
    async open() {
      const file = options.file();
      if (file === undefined)
        return {
          status: "failed",
          message: "The note application did not hand over a Project",
        };
      served = file;
      return { status: "opened", file: bindingFor(file), text: file.text };
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
        const opened = options.file();
        const written = await options.write(
          { ...(opened ?? { text: "" }), path: target.path },
          serializeProject(project),
        );
        // The name the note application wrote the drawing under is the name
        // it has now: the editor stays bound to the drawing through a
        // rename instead of asking for the name it was opened under.
        return { status: "saved", file: bindingFor(written) };
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
