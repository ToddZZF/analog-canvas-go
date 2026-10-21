/**
 * Copies the built note bundle into the note application.
 *
 * The bundle is built outside of the application tree and copied in, so that
 * the application ships with the editor it embeds. Stale files from an
 * earlier build are removed, so that the application never carries two
 * versions of the same chunk.
 */
import { cp, readdir, rm, stat } from "node:fs/promises";
import { join, relative, resolve } from "node:path";

const [source, target] = process.argv.slice(2);
if (!source || !target) {
  console.error(
    "Usage: node scripts/sync-circuit-bundle.mjs <source> <target>",
  );
  process.exit(1);
}

const from = resolve(source);
const to = resolve(target);

/** Every file below the given directory, as paths relative to it. */
async function filesBelow(directory, prefix = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  const found = [];
  for (const entry of entries) {
    const child = join(directory, entry.name);
    const name = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
    if (entry.isDirectory()) {
      found.push(...(await filesBelow(child, name)));
    } else {
      found.push(name);
    }
  }
  return found;
}

await cp(from, to, { recursive: true });

const wanted = new Set(await filesBelow(from));
const stale = (await filesBelow(to)).filter((name) => !wanted.has(name));

for (const name of stale) {
  const file = join(to, name);
  await stat(file);
  await rm(file);
  console.log(`removed stale ${name}`);
}

console.log(
  `${wanted.size} files copied, ${stale.length} stale files removed -> ${relative(process.cwd(), to)}`,
);
