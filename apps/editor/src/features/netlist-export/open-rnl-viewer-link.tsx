import { useState } from "react";
import { openRnlViewer, RNL_VIEWER_URL } from "./rnl-viewer";

export function OpenRnlViewerLink({
  file,
}: {
  file: { name: string; text: string } | null;
}) {
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <>
      {" · "}
      <a
        className="rnl-about-link"
        href={RNL_VIEWER_URL}
        aria-disabled={!file || opening}
        aria-label="Open RNL Viewer (opens in another tab)"
        onClick={(event) => {
          event.preventDefault();
          if (!file || opening) return;
          setError(null);
          setOpening(true);
          void openRnlViewer(file)
            .catch((reason: unknown) =>
              setError(
                reason instanceof Error ? reason.message : String(reason),
              ),
            )
            .finally(() => setOpening(false));
        }}
      >
        Open RNL Viewer
      </a>
      {error ? <span role="alert"> {error}</span> : null}
    </>
  );
}
