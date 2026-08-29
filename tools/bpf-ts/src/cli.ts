#!/usr/bin/env bun

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { BpfTsCompileError } from "./diagnostics";
import { compileBpfTs } from "./compiler";

interface CliOptions {
  input: string;
  out: string;
  manifest?: string;
}

function usage(): never {
  console.error("usage: bpf-ts <input.ts> --out <program.bpf.c> [--manifest <program.json>]");
  process.exit(2);
}

function parseArgs(argv: string[]): CliOptions {
  const args = argv.slice(2);
  if (args.length < 3) usage();
  const input = args[0];
  let out = "";
  let manifest: string | undefined;
  for (let index = 1; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--out") {
      out = args[++index] ?? "";
      continue;
    }
    if (arg === "--manifest") {
      manifest = args[++index] ?? "";
      continue;
    }
    usage();
  }
  if (!input || !out || manifest === "") usage();
  return { input, out, manifest };
}

async function main() {
  const options = parseArgs(process.argv);
  const inputPath = resolve(options.input);
  const outputPath = resolve(options.out);
  const source = await readFile(inputPath, "utf8");
  const compilation = compileBpfTs(source, options.input);
  await mkdir(dirname(outputPath), { recursive: true });
  await writeFile(outputPath, compilation.cSource, "utf8");
  if (options.manifest) {
    const manifestPath = resolve(options.manifest);
    await mkdir(dirname(manifestPath), { recursive: true });
    await writeFile(manifestPath, `${JSON.stringify(compilation.manifest, null, 2)}\n`, "utf8");
  }
}

main().catch((error: unknown) => {
  if (error instanceof BpfTsCompileError) {
    console.error(error.format());
  } else if (error instanceof Error) {
    console.error(error.stack || error.message);
  } else {
    console.error(String(error));
  }
  process.exit(1);
});
