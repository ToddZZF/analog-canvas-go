/**
 * How a host that owns the drawing writes it.
 *
 * A host that keeps the drawing in a file of its own — a note application,
 * for instance — writes it without the person asking, exactly as it writes a
 * note. The editor asks the host how eagerly that should happen and tells the
 * host when the drawing holds changes that are not on disk.
 *
 * A host that says nothing keeps the editor's own explicit save.
 */

/** When a host writes the drawing after the last edit. */
export type AutoSaveMode = "off" | "immediately" | "delayed";

export interface HostPersistence {
  /**
   * The host's save preference, read whenever a save is due.
   *
   * "off" keeps the editor's explicit save, "immediately" writes the drawing
   * shortly after the last edit, "delayed" waits longer for the drawing to
   * settle.
   */
  autoSave(): AutoSaveMode;
  /**
   * The drawing holds changes that are not on disk, or no longer does.
   *
   * The host uses this to show that the drawing wants to be saved.
   */
  onDirtyChange(dirty: boolean): void;
}
