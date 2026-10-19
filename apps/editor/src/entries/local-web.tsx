import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "../app/App";
import { EditorErrorBoundary } from "../components/editor-error-boundary";
import { createLocalWebEditorServices } from "../services/local-web-editor-services";
import "../styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("Editor root element is missing");
// Every browser tab owns its workspace; no fixed window identity is pinned.

// Same-origin links navigate in place; web links open a new browser tab.
// Fixed product links keep their private host command (the local server
// opens them in the system browser).
document.addEventListener(
  "click",
  (event) => {
    const target =
      event.target instanceof Element ? event.target.closest("a[href]") : null;
    if (!target) return;
    if (target.hasAttribute("data-native-external-link")) return;
    const url = new URL(target.getAttribute("href")!, window.location.href);
    if (url.origin === window.location.origin) return;
    if (url.protocol !== "http:" && url.protocol !== "https:") return;
    event.preventDefault();
    window.open(url.href, "_blank", "noopener");
  },
  true,
);

createRoot(root).render(
  <StrictMode>
    <EditorErrorBoundary>
      <App services={createLocalWebEditorServices()} />
    </EditorErrorBoundary>
  </StrictMode>,
);
