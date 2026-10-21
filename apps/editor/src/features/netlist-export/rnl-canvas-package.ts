import lock from "./rnl-canvas-package.lock.json" with { type: "json" };

export const canvasPackageLock = lock;

/** Verify original bytes before decoding, in the build, Node tests and editor. */
export async function decodeCanvasPackage(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new Uint8Array(bytes));
  const hash = Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
  if (hash !== lock.sha256)
    throw new Error(
      `Canvas package SHA-256 mismatch: expected ${lock.sha256}, received ${hash}`,
    );
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}
