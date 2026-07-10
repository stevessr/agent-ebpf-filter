import { createHighlighterCore } from "@shikijs/core";
import { createJavaScriptRegexEngine } from "@shikijs/engine-javascript";
import githubDark from "@shikijs/themes/github-dark";

const languageLoaders = {
  bash: () => import("@shikijs/langs/bash"),
  c: () => import("@shikijs/langs/c"),
  cpp: () => import("@shikijs/langs/cpp"),
  css: () => import("@shikijs/langs/css"),
  go: () => import("@shikijs/langs/go"),
  html: () => import("@shikijs/langs/html"),
  java: () => import("@shikijs/langs/java"),
  javascript: () => import("@shikijs/langs/javascript"),
  json: () => import("@shikijs/langs/json"),
  markdown: () => import("@shikijs/langs/markdown"),
  python: () => import("@shikijs/langs/python"),
  rust: () => import("@shikijs/langs/rust"),
  sql: () => import("@shikijs/langs/sql"),
  typescript: () => import("@shikijs/langs/typescript"),
  yaml: () => import("@shikijs/langs/yaml"),
} as const;

type PreviewLanguage = keyof typeof languageLoaders;
type Highlighter = Awaited<ReturnType<typeof createHighlighterCore>>;

const languageAliases: Readonly<Record<string, PreviewLanguage>> = {
  bash: "bash",
  c: "c",
  cc: "cpp",
  cjs: "javascript",
  cpp: "cpp",
  css: "css",
  cts: "typescript",
  cxx: "cpp",
  go: "go",
  golang: "go",
  h: "c",
  hpp: "cpp",
  html: "html",
  java: "java",
  javascript: "javascript",
  js: "javascript",
  json: "json",
  markdown: "markdown",
  md: "markdown",
  mjs: "javascript",
  mts: "typescript",
  py: "python",
  python: "python",
  rs: "rust",
  rust: "rust",
  sh: "bash",
  shell: "bash",
  shellscript: "bash",
  sql: "sql",
  ts: "typescript",
  tsx: "typescript",
  typescript: "typescript",
  yaml: "yaml",
  yml: "yaml",
  zsh: "bash",
};

let highlighterPromise: Promise<Highlighter> | null = null;
const languageLoadPromises = new Map<PreviewLanguage, Promise<void>>();

const getHighlighter = () => {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighterCore({
      themes: [githubDark],
      langs: [],
      engine: createJavaScriptRegexEngine(),
    }).catch((error) => {
      highlighterPromise = null;
      throw error;
    });
  }
  return highlighterPromise;
};

export const normalizePreviewLanguage = (
  language?: string,
): PreviewLanguage | "text" => {
  const normalized = (language || "").trim().toLowerCase().replace(/^\./, "");
  return languageAliases[normalized] || "text";
};

const ensureLanguageLoaded = async (
  highlighter: Highlighter,
  language: PreviewLanguage,
) => {
  if (highlighter.getLoadedLanguages().includes(language)) return;

  let loadPromise = languageLoadPromises.get(language);
  if (!loadPromise) {
    loadPromise = languageLoaders[language]()
      .then(({ default: grammar }) => highlighter.loadLanguage(grammar))
      .catch((error) => {
        languageLoadPromises.delete(language);
        throw error;
      });
    languageLoadPromises.set(language, loadPromise);
  }
  await loadPromise;
};

export const highlightPreviewCode = async (
  content: string,
  language?: string,
) => {
  const normalizedLanguage = normalizePreviewLanguage(language);
  const highlighter = await getHighlighter();
  if (normalizedLanguage !== "text") {
    await ensureLanguageLoaded(highlighter, normalizedLanguage);
  }
  return highlighter.codeToHtml(content, {
    lang: normalizedLanguage,
    theme: "github-dark",
  });
};
