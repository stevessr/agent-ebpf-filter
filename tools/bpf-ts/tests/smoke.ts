import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { compileBpfTs } from "../src/compiler";

for (const file of ["exec.ts", "tls-write.ts"]) {
  const path = resolve(import.meta.dir, "../examples", file);
  const source = await readFile(path, "utf8");
  const output = compileBpfTs(source, file);
  if (!output.cSource.includes('char LICENSE[] SEC("license")')) {
    throw new Error(`${file}: generated C is missing a license section`);
  }
}
