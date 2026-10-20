// Builds the Go local server binary. Prerequisites: the editor bundle must
// exist in apps/editor/dist-local (the build:local pnpm script produces it).
// The script copies the bundle into apps/local-server/web, stamps the build
// from git/package metadata, and compiles the server.
//
// Usage:
//   node scripts/build-local-server.mjs                    # current host, tray
//   node scripts/build-local-server.mjs --notray           # current host, no tray
//   node scripts/build-local-server.mjs --release          # cross-platform matrix
//   node scripts/build-local-server.mjs --tray-only goos/goarch[,goos/goarch…]
//                                                          # named tray builds only
import { execFileSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const serverDir = join(root, "apps", "local-server");
const distLocal = join(root, "apps", "editor", "dist-local");
const webDir = join(serverDir, "web");
const distDir = join(serverDir, "dist");

function run(command, arguments_, options = {}) {
  execFileSync(command, arguments_, {
    cwd: root,
    stdio: options.quiet ? ["ignore", "ignore", "inherit"] : "inherit",
    ...options,
  });
}

function gitOutput(arguments_) {
  try {
    return execFileSync("git", arguments_, { cwd: root, encoding: "utf8" }).trim();
  } catch {
    return "";
  }
}

function copyBundle() {
  if (!existsSync(join(distLocal, "index.html"))) {
    throw new Error(
      `Missing editor bundle at ${distLocal}. Run: pnpm build:local (or the vite step) first.`,
    );
  }
  mkdirSync(webDir, { recursive: true });
  for (const entry of readdirSync(webDir)) {
    if (entry !== "README.md") rmSync(join(webDir, entry), { recursive: true, force: true });
  }
  cpSync(distLocal, webDir, { recursive: true });
}

function buildFlags() {
  const require = createRequire(import.meta.url);
  const { version } = require(join(root, "package.json"));
  const commit = gitOutput(["rev-parse", "HEAD"]) || "unknown";
  const dirty = gitOutput(["status", "--porcelain"]).length > 0;
  return [
    "-ldflags",
    [
      `-X main.version=${version}`,
      `-X main.commit=${commit}`,
      `-X main.dirty=${dirty}`,
    ].join(" "),
    "-trimpath",
  ];
}

function goBuild(goos, goarch, tags, outputName, cgo) {
  const output = join(distDir, outputName);
  mkdirSync(distDir, { recursive: true });
  const arguments_ = ["build", ...buildFlags()];
  if (tags) arguments_.push("-tags", tags);
  arguments_.push("-o", output, ".");
  run("go", arguments_, {
    cwd: serverDir,
    env: {
      ...process.env,
      CGO_ENABLED: cgo ? "1" : "0",
      GOOS: goos,
      GOARCH: goarch,
    },
  });
  console.log(`built ${outputName}`);
}

function currentPlatform() {
  const goos = process.platform === "win32" ? "windows" : process.platform;
  const goarch = process.arch === "x64" ? "amd64" : process.arch === "arm64" ? "arm64" : process.arch;
  return { goos, goarch };
}

function trayTargetName(goos, goarch) {
  return `analog-canvas-local-${goos}-${goarch}${goos === "windows" ? ".exe" : ""}`;
}

// Tray builds run cgo, so a target needs its platform toolchain on this
// host: a same-arch Linux host with pkg-config gtk+-3.0, Windows through
// mingw-w64, and macOS on any Mac — Xcode's clang carries the x86_64
// slice, so Apple Silicon cross-builds the Intel tray. Returns the reason
// a target cannot build here, or "" when it can.
function trayToolchain(goos, goarch) {
  const host = currentPlatform();
  if (goos === "linux") {
    if (host.goos !== "linux" || host.goarch !== goarch) {
      return "linux tray builds run on a same-arch Linux host with gtk3-devel (pkg-config gtk+-3.0)";
    }
  } else if (goos === "windows") {
    if (!hasTool("x86_64-w64-mingw32-gcc")) {
      return "x86_64-w64-mingw32-gcc is not installed (build it on Windows, or install mingw-w64)";
    }
  } else if (goos === "darwin") {
    if (host.goos !== "darwin") {
      return "darwin tray builds run on a Mac";
    }
  }
  return "";
}

function hasTool(name) {
  try {
    execFileSync(name, ["--version"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

const release = process.argv.includes("--release");
const notrayOnly = process.argv.includes("--notray");
const trayOnlyIndex = process.argv.indexOf("--tray-only");
const knownTrayTargets = ["linux/amd64", "linux/arm64", "windows/amd64", "darwin/amd64", "darwin/arm64"];
copyBundle();
rmSync(distDir, { recursive: true, force: true });

if (trayOnlyIndex !== -1) {
  // Explicit tray requests must build: an unavailable toolchain is an
  // error here, not a skip, so CI runners never publish a partial set.
  const targets = (process.argv[trayOnlyIndex + 1] ?? "").split(",").filter(Boolean);
  if (targets.length === 0) {
    throw new Error("--tray-only needs goos/goarch targets, e.g. --tray-only darwin/amd64,darwin/arm64");
  }
  for (const target of targets) {
    if (!knownTrayTargets.includes(target)) {
      throw new Error(`unsupported tray target "${target}"; known: ${knownTrayTargets.join(", ")}`);
    }
    const [goos, goarch] = target.split("/");
    const reason = trayToolchain(goos, goarch);
    if (reason) {
      throw new Error(`cannot build tray for ${target} on this host: ${reason}`);
    }
    goBuild(goos, goarch, "", trayTargetName(goos, goarch), true);
  }
} else if (release) {
  // cgo system-tray builds must run on their own platform or with a C
  // cross-toolchain; everything else ships as a pure-Go console build.
  goBuild("linux", "amd64", "notray", "analog-canvas-local-linux-amd64-notray", false);
  goBuild("linux", "arm64", "notray", "analog-canvas-local-linux-arm64-notray", false);
  goBuild("windows", "amd64", "notray", "analog-canvas-local-windows-amd64-notray.exe", false);
  goBuild("darwin", "amd64", "notray", "analog-canvas-local-darwin-amd64-notray", false);
  goBuild("darwin", "arm64", "notray", "analog-canvas-local-darwin-arm64-notray", false);
  const windowsReason = trayToolchain("windows", "amd64");
  if (windowsReason) {
    console.log(`skipped windows/amd64 tray build: ${windowsReason}`);
  } else {
    goBuild("windows", "amd64", "", trayTargetName("windows", "amd64"), true);
  }
  const linuxReason = trayToolchain("linux", "amd64");
  if (linuxReason) {
    console.log(`skipped linux/amd64 tray build: ${linuxReason}`);
  } else {
    goBuild("linux", "amd64", "", trayTargetName("linux", "amd64"), true);
  }
} else {
  const { goos, goarch } = currentPlatform();
  const suffix = goos === "windows" ? ".exe" : "";
  if (notrayOnly) {
    goBuild(goos, goarch, "notray", `analog-canvas-local${suffix}`, false);
  } else {
    goBuild(goos, goarch, "", `analog-canvas-local${suffix}`, true);
  }
}
