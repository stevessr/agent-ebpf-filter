export class BpfTsCompileError extends Error {
  constructor(
    message: string,
    readonly file?: string,
    readonly line?: number,
    readonly column?: number,
  ) {
    super(message);
    this.name = "BpfTsCompileError";
  }

  format() {
    const location =
      this.file && this.line && this.column
        ? `${this.file}:${this.line}:${this.column}: `
        : this.file
          ? `${this.file}: `
          : "";
    return `${location}${this.message}`;
  }
}
