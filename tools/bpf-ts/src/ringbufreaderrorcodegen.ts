import { ringbufReadErrorMapName } from "./ringbufreaderrors";
import type { ProgramIR } from "./ir";

function leadingWhitespace(line: string): string {
  return line.slice(0, line.length - line.trimStart().length);
}

function braceDelta(line: string): number {
  let delta = 0;
  for (const char of line) {
    if (char === "{") delta++;
    else if (char === "}") delta--;
  }
  return delta;
}

// The core C emitter already suppresses compact records when
// bpf_probe_read_user() fails. This pass makes that suppression observable by
// adding a failure-only per-CPU counter without changing the success hot path.
export function instrumentCompactRingbufReadErrors(program: ProgramIR, cSource: string): string {
  const ringbufs = program.maps.filter((map) => map.kind === "ringbuf");
  const readErrorMaps = new Set(
    program.maps
      .filter((map) => map.kind === "percpu_array")
      .map((map) => map.name),
  );
  const trackedRingbufs = ringbufs.filter((ringbuf) =>
    readErrorMaps.has(ringbufReadErrorMapName(ringbuf.name)),
  );
  if (trackedRingbufs.length === 0) return cSource;

  const lines = cSource.split("\n");
  const readIf = /^\s*if \((__bpf_ts_read_rc_\d+) == 0\) \{$/;

  for (let index = 0; index < lines.length; index++) {
    const match = lines[index].match(readIf);
    if (!match) continue;

    let depth = braceDelta(lines[index]);
    let closing = -1;
    let ringbufName = "";
    for (let cursor = index + 1; cursor < lines.length; cursor++) {
      for (const ringbuf of trackedRingbufs) {
        if (lines[cursor].includes(`bpf_ringbuf_output(&${ringbuf.name}, `)) {
          ringbufName = ringbuf.name;
        }
      }
      depth += braceDelta(lines[cursor]);
      if (depth === 0) {
        closing = cursor;
        break;
      }
    }
    if (closing < 0 || ringbufName === "") continue;

    const indent = leadingWhitespace(lines[closing]);
    lines.splice(
      closing,
      1,
      `${indent}} else {`,
      `${indent}  __bpf_ts_note_read_error_${ringbufName}();`,
      `${indent}}`,
    );
    index = closing + 2;
  }

  const helpers: string[] = [];
  for (const ringbuf of trackedRingbufs) {
    const mapName = ringbufReadErrorMapName(ringbuf.name);
    helpers.push(
      `static __always_inline void __bpf_ts_note_read_error_${ringbuf.name}(void) {`,
      "  __u32 key = 0;",
      `  __u64 *counter = bpf_map_lookup_elem(&${mapName}, &key);`,
      "  if (counter) (*counter)++;",
      "}",
      "",
    );
  }

  const output = lines.join("\n");
  const firstProbe = output.indexOf("// bpf-ts attach:");
  if (firstProbe < 0) return output;
  return `${output.slice(0, firstProbe)}${helpers.join("\n")}\n${output.slice(firstProbe)}`;
}
