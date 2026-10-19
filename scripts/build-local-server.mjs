// Builds the Go local server binary. Prerequisites: the editor bundle must
// exist in apps/editor/dist-local (the build:local pnpm script produces it).
// The script copies the bundle into apps/local-server/web, stamps the build
// from git/package metadata, and compiles the server.
//
// Usage:
//   node scripts/build-local-server.mjs              # current host, tray
//   node scripts/build-local-server.mjs --notray     # current host, no tray
//   node scripts/build-local-server.mjs --release    # cross-platform matrix
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
copyBundle();
rmSync(distDir, { recursive: true, force: true });

if (release) {
  // cgo system-tray builds must run on their own platform or with a C
  // cross-toolchain; everything else ships as a pure-Go console build.
  goBuild("linux", "amd64", "notray", "analog-canvas-local-linux-amd64-notray", false);
  goBuild("linux", "arm64", "notray", "analog-canvas-local-linux-arm64-notray", false);
  goBuild("windows", "amd64", "notray", "analog-canvas-local-windows-amd64-notray.exe", false);
  goBuild("darwin", "amd64", "notray", "analog-canvas-local-darwin-amd64-notray", false);
  goBuild("darwin", "arm64", "notray", "analog-canvas-local-darwin-arm64-notray", false);
  if (hasTool("x86_64-w64-mingw32-gcc")) {
    goBuild("windows", "amd64", "", "analog-canvas-local-windows-amd64.exe", true);
  } else {
    console.log(
      "skipped windows/amd64 tray build: x86_64-w64-mingw32-gcc is not installed " +
        "(build it on Windows, or install mingw-w64)",
    );
  }
  const host = currentPlatform();
  if (host.goos === "linux" && host.goarch === "amd64") {
    goBuild("linux", "amd64", "", "analog-canvas-local-linux-amd64", true);
  } else {
    console.log(`skipped tray build for ${host.goos}/${host.goarch}: build it natively on that platform`);
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
