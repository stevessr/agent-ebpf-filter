import { copyFile, mkdir, readdir, readFile, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";

const source = fileURLToPath(new URL("../node_modules/@lobehub/icons-static-svg/icons/", import.meta.url));
const target = fileURLToPath(new URL("../public/brand-icons/", import.meta.url));
const catalog = JSON.parse(await readFile(new URL("../src/utils/harnessIconCatalog.json", import.meta.url), "utf8"));

if (!existsSync(source)) {
  throw new Error("Lobe Icons static assets are missing. Install frontend dependencies first (bun install).");
}

// Only ship the harness logos we use; never copy the entire icon collection.
await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });
const available = new Set(await readdir(source));
let count = 0;
for (const [id, { assets }] of Object.entries(catalog)) {
  const asset = assets.find((candidate) => available.has(candidate));
  if (!asset) {
    console.warn(`[lobe-icons] No official icon for ${id}; UI will use the generic fallback.`);
    continue;
  }
  await copyFile(`${source}/${asset}`, `${target}/${id}.svg`);
  count++;
}
console.log(`[lobe-icons] Prepared ${count} self-hosted harness logos.`);
