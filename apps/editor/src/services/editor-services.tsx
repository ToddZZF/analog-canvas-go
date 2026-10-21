import {
  createContext,
  useContext,
  type ReactNode,
  type ComponentType,
} from "react";
import type { NativeProjectHomeProps } from "../hosts/native-project-home";
import type { CircuitProject } from "@icm/model";
import type { SessionUser } from "../components/account";
import type {
  CloudProjectBinding,
  CloudProjectListOutcome,
  CloudProjectOpenOutcome,
  CloudProjectSaveOutcome,
  deleteCloudProject,
} from "../features/editor-shell/cloud-projects";
import type { EditorExportDelivery } from "../hosts/export-delivery";
import type { FileCommandMenuProps } from "../features/editor-shell/file-command-menu";
import type { NativeProjectStore } from "../hosts/native-project-store";
import type { ProjectWorkspace } from "../document/project-workspace";
import type { NativeExternalTarget } from "../hosts/native-external-links";
import type { createProjectSnapshotSerializer } from "../document/project-snapshot-serializer";
import type { HostPersistence } from "../document/host-persistence";

/** Current Cloud contract, including revisions/conflicts; not a native store. */
export interface CloudProjectStore {
  readonly kind: "cloud";
  readonly limit: number;
  list(): Promise<CloudProjectListOutcome>;
  open(projectId: string): Promise<CloudProjectOpenOutcome>;
  save(
    project: CircuitProject,
    binding: CloudProjectBinding | null,
    galleryEntryId?: string,
  ): Promise<CloudProjectSaveOutcome>;
  delete(projectId: string): ReturnType<typeof deleteCloudProject>;
}

/** First composition seam. Other online features still own their dependencies. */
export interface EditorServices {
  /** Shared by local recovery and Cloud transport; never a second file format. */
  readonly projectSerializer?: ReturnType<
    typeof createProjectSnapshotSerializer
  >;
  readonly restoreWorkspace?: (
    workspace: ProjectWorkspace | null,
  ) => Promise<{ workspace: ProjectWorkspace | null; notice?: string }>;
  readonly identity: { getSessionUser(): Promise<SessionUser | null> } | null;
  readonly projectStore: CloudProjectStore | null;
  readonly nativeProjectStore?: NativeProjectStore;
  readonly NativeFileCommands?: FileCommandMenuProps["NativeFileCommands"];
  readonly NativeProjectHome?: ComponentType<NativeProjectHomeProps>;
  readonly NativeHeaderActions?: ComponentType;
  readonly openExternalLink?: (target: NativeExternalTarget) => Promise<void>;
  /**
   * Quits the host server after in-page confirmation; only the local-web
   * host sets it, where a panel may not host the tray icon. The desktop
   * shell quits through its own window instead.
   */
  readonly quitServer?: () => Promise<void>;
  readonly capabilities: {
    readonly community: boolean;
    readonly localComponents?: boolean;
    readonly agent: boolean;
    readonly simulation: boolean;
    readonly externalLinks: boolean;
    /**
     * Whether the editor may bring a window's tabs back when it starts.
     *
     * A host that opens exactly one drawing at a time — a note application,
     * for instance — has nothing to bring back: every frame starts on the
     * drawing it was handed. Turning this off also keeps the editor from
     * writing tabs the host would never read.
     */
    readonly workspaceRestore?: boolean;
    /**
     * Whether the editor may show a different Project than the one it was
     * given.
     *
     * A host that owns one drawing — a note application, for instance —
     * cannot hand the editor a second one: there is no second file to put it
     * in. The editor therefore offers neither a new nor an imported Project.
     * Absent, the editor may replace its drawing.
     */
    readonly replaceDrawing?: boolean;
    /**
     * How the host writes the drawing, if it does at all.
     *
     * A host that owns the drawing in a file of its own — a note application,
     * for instance — writes it without the person asking, exactly as it
     * writes a note. A host that says nothing keeps the editor's own save.
     */
    readonly hostPersistence?: HostPersistence;
  };
  readonly exportDelivery: EditorExportDelivery;
}

const EditorServicesContext = createContext<EditorServices | null>(null);

export function EditorServicesProvider({
  services,
  children,
}: {
  services: EditorServices;
  children: ReactNode;
}) {
  return (
    <EditorServicesContext.Provider value={services}>
      {children}
    </EditorServicesContext.Provider>
  );
}

export function useEditorServices(): EditorServices {
  const services = useContext(EditorServicesContext);
  if (!services)
    throw new Error("Editor services must be supplied by the host");
  return services;
}
