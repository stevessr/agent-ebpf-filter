/**
 * Central HTTP bootstrap for the default axios instance.
 *
 * A single request interceptor injects the current API token and cluster
 * target (see `utils/requestContext.ts`) into every outgoing request, so
 * components never need to attach auth headers manually and no global
 * `axios.defaults` mutation is required.
 */
import axios from "axios";
import { buildRequestHeaders } from "../utils/requestContext";

axios.interceptors.request.use((config) => {
  const headers = buildRequestHeaders();
  if (Object.keys(headers).length) {
    config.headers = config.headers ?? {};
    Object.assign(config.headers as Record<string, string>, headers);
  }
  return config;
});

// Re-apply stored credentials eagerly so the very first request issued
// before Vue mounts already carries them.
const applyStoredRequestContext = () => {
  const headers = buildRequestHeaders();
  if (!headers["X-API-KEY"]) return;
  axios.defaults.headers.common["X-API-KEY"] = headers["X-API-KEY"];
  axios.defaults.headers.common.Authorization = headers.Authorization;
};
applyStoredRequestContext();

export default axios;
