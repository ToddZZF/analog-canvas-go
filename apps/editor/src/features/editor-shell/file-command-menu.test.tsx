import { createRef } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { FileCommandMenu } from "./file-command-menu";

describe("FileCommandMenu", () => {
  it("presents one Cloud Save protocol and explicit local interchange", () => {
    const markup = renderToStaticMarkup(
      <FileCommandMenu
        canRevert
        hasRecoverySessions
        checkAndSave={{ enabled: true, execute: vi.fn() }}
        projectInputRef={createRef<HTMLInputElement>()}
        onNewProject={vi.fn()}
        onSave={vi.fn()}
        onImportProject={vi.fn()}
        onImportSpice={vi.fn()}
        onExportProject={vi.fn()}
        onExportSvg={vi.fn()}
        onExportRaster={vi.fn()}
        onExportNetlist={vi.fn()}
        onRevert={vi.fn()}
        onOpenRecovery={vi.fn()}
        onOpenInfo={vi.fn()}
      />,
    );

    expect(markup.split(">New Project<")).toHaveLength(2);
    // Project Info comes first; Cloud Projects are listed elsewhere.
    expect(markup.indexOf(">Project Info<")).toBeGreaterThan(0);
    expect(markup.indexOf(">Project Info<")).toBeLessThan(
      markup.indexOf(">New Project<"),
    );
    expect(markup).not.toContain("Save as Cloud Copy");
    expect(markup).not.toContain("Cloud Projects (");
    expect(markup).not.toContain("file-cloud-project-list");
    expect(markup).toContain(">Import<");
    expect(markup).toContain("Project File…");
    expect(markup).toContain("SPICE / SCS…");
    expect(markup).toContain("Cadence SPICE (`!` globals)…");
    expect(markup).toContain('data-testid="cadence-spice-files"');
    expect(markup).toContain(">Export<");
    expect(markup).toContain("Drawing as SVG");
    // The Export group saves the netlist the Netlist panel shows, named by
    // its format.
    expect(markup).toContain('aria-label="Export Netlist"');
    expect(markup).toContain(">Netlist…<");
    expect(markup).toContain("Recover Unsaved Work…");
    expect(markup).not.toContain("Quit Local Server");
    expect(markup).not.toContain("Refresh app");
    expect(markup).not.toContain("Copy SPICE netlist");
    expect(markup).not.toContain("Copy Spectre netlist");
    expect(markup).not.toContain("Download Backup");
    expect(markup).not.toContain("Previous Project");
    expect(markup).not.toContain("cloud snapshot");
  });

  it("offers Quit Local Server only where a quit handler exists", () => {
    const base = {
      canRevert: false,
      hasRecoverySessions: false,
      checkAndSave: { enabled: true, execute: vi.fn() },
      projectInputRef: createRef<HTMLInputElement>(),
      onNewProject: vi.fn(),
      onSave: vi.fn(),
      onImportProject: vi.fn(),
      onImportSpice: vi.fn(),
      onExportProject: vi.fn(),
      onExportSvg: vi.fn(),
      onExportRaster: vi.fn(),
      onExportNetlist: vi.fn(),
      onRevert: vi.fn(),
      onOpenRecovery: vi.fn(),
    };
    expect(renderToStaticMarkup(<FileCommandMenu {...base} />)).not.toContain(
      "Quit Local Server",
    );
    const quit = vi.fn();
    const markup = renderToStaticMarkup(
      <FileCommandMenu {...base} onQuitServer={quit} />,
    );
    expect(markup).toContain('data-testid="quit-local-server"');
    expect(markup).toContain(">Quit Local Server<");
  });

  it("offers no second Project to a host that owns one drawing", () => {
    const base = {
      canRevert: false,
      hasRecoverySessions: false,
      checkAndSave: { enabled: true, execute: vi.fn() },
      projectInputRef: createRef<HTMLInputElement>(),
      onNewProject: vi.fn(),
      onSave: vi.fn(),
      onImportProject: vi.fn(),
      onImportSpice: vi.fn(),
      onExportProject: vi.fn(),
      onExportSvg: vi.fn(),
      onExportRaster: vi.fn(),
      onExportNetlist: vi.fn(),
      onRevert: vi.fn(),
      onOpenRecovery: vi.fn(),
    };
    const markup = renderToStaticMarkup(
      <FileCommandMenu {...base} replaceDrawing={false} />,
    );
    // A new or an imported Project would replace the drawing the host owns,
    // and there is no second file to put it in.
    expect(markup).not.toContain(">New Project<");
    expect(markup).not.toContain(">Import<");
    expect(markup).not.toContain('data-testid="project-file"');
    expect(markup).not.toContain('data-testid="spice-files"');
    // What the host's own drawing needs stays.
    expect(markup).toContain(">Save<");
    expect(markup).toContain(">Export<");
  });
});
