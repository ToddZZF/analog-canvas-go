import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { parseProject } from "@icm/project-protocol";
import rc from "../../examples/simulation-rc.icproj.json";

import { RNL_INTRO_URL } from "../../components/site-resource-links";
import { AboutRnlLink, RnlNetlistView } from "./rnl-netlist-view";

describe("the About RNL link", () => {
  it("opens the introduction to RNL in a new tab", () => {
    expect(RNL_INTRO_URL).toBe("https://rich-netlist.github.io/rnl/en/");
    const markup = renderToStaticMarkup(<AboutRnlLink />);
    expect(markup).toContain(`href="${RNL_INTRO_URL}"`);
    expect(markup).toContain('target="_blank"');
    expect(markup).toContain('rel="noopener noreferrer"');
    expect(markup).toContain(">About RNL<");
    expect(markup).not.toContain("data-native-external-link");
  });

  it("goes through the desktop app's fixed list of links when it has one", () => {
    const markup = renderToStaticMarkup(
      <AboutRnlLink
        externalLinksEnabled={false}
        onOpenExternalLink={() => {}}
      />,
    );
    expect(markup).toContain('data-native-external-link="rnl"');
    expect(markup).toContain(">About RNL<");
  });

  it("is left out where the build links nowhere outside itself", () => {
    expect(
      renderToStaticMarkup(<AboutRnlLink externalLinksEnabled={false} />),
    ).toBe("");
  });
});

it("waits for the current export and omits the Viewer action from desktop", () => {
  const props = {
    project: parseProject(JSON.stringify(rc)),
    format: "spice",
    namingProfile: "native",
  } as const;
  const web = renderToStaticMarkup(<RnlNetlistView {...props} />);
  expect(web).toContain("Open RNL Viewer");
  expect(web).toContain('aria-disabled="true"');
  const desktop = renderToStaticMarkup(
    <RnlNetlistView
      {...props}
      externalLinksEnabled={false}
      onOpenExternalLink={() => {}}
    />,
  );
  expect(desktop).not.toContain("Open RNL Viewer");
  expect(desktop).toContain('data-native-external-link="rnl"');
});
