import { message as antdMessage } from "ant-design-vue";

// Backend rejections carry the operator-facing text under `response.data.error`;
// anything else falls back to the axios message, then to a caller-supplied default.
export const backendError = (cause: unknown, fallback: string): string => {
  if (cause && typeof cause === "object" && "response" in cause) {
    const response: unknown = cause.response;
    if (response && typeof response === "object" && "data" in response) {
      const data: unknown = response.data;
      if (
        data &&
        typeof data === "object" &&
        "error" in data &&
        typeof data.error === "string" &&
        data.error
      ) {
        return data.error;
      }
    }
  }
  if (cause instanceof Error && cause.message) return cause.message;
  return fallback;
};

// antd's imperative message mounts a holder element into `document` the first
// time it is used, so it is only callable in a DOM environment. Outside one
// (bun tests, prerender) the notice is dropped rather than crashing the caller.
// Every mutation these toasts confirm also renders its authoritative state on
// screen, so a dropped toast degrades feedback without hiding a result.
export const renewToast = {
  success: (text: string) => {
    if (typeof document === "undefined") return;
    antdMessage.success(text);
  },
};
