import { copyFile, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";

const source = fileURLToPath(new URL("../node_modules/@lobehub/icons-static-svg/icons/", import.meta.url));
const target = fileURLToPath(new URL("../public/brand-icons/", import.meta.url));
const catalog = JSON.parse(await readFile(new URL("../src/utils/harnessIconCatalog.json", import.meta.url), "utf8"));

if (!existsSync(source)) {
  throw new Error("Lobe Icons static assets are missing. Install frontend dependencies first (bun install).");
}

// Build a local copy of each brand icon; no third-party CDN fetch on page load.
// A neutral monogram is not a vendor logo, and prevents broken/404 icons when
// Lobe Icons does not yet provide a specific harness (e.g. Augment, ZCode).
const neutralMonogram = (id) => {
  const letters = id.toUpperCase().slice(0, 2);
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48" role="img" aria-label="Generic harness icon">
<rect x="2" y="2" width="44" height="44" rx="12" fill="#E5E7EB"/>
<text x="24" y="30" text-anchor="middle" font-family="sans-serif" font-weight="700" font-size="19" fill="#374151">${letters}</text>
</svg>\n`;
};

await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });
const available = new Set(await readdir(source));
const fallbackIds = [];
for (const [id, { assets }] of Object.entries(catalog)) {
  if (!/^[a-z0-9-]+$/.test(id) || !Array.isArray(assets)) {
    throw new Error(`Invalid harness icon catalog entry: ${id}`);
  }
  const asset = assets.find((candidate) => /^[a-z0-9-]+\.svg$/.test(candidate) && available.has(candidate));
  if (asset) {
    await copyFile(`${source}/${asset}`, `${target}/${id}.svg`);
  } else {
    fallbackIds.push(id);
    await writeFile(`${target}/${id}.svg`, neutralMonogram(id));
  }
}
console.log(`[lobe-icons] Prepared ${Object.keys(catalog).length - fallbackIds.length} official logos and ${fallbackIds.length} neutral monograms.`);
if (fallbackIds.length) console.log(`[lobe-icons] No official logo in this release: ${fallbackIds.join(", ")}`);
