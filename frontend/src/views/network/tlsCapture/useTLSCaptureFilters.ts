import { computed, type Ref } from "vue";

import type {
  TLSIgnoreRule,
  TLSLibraryStatus,
  TLSPlaintextEvent,
} from "../../../types/tls";
import type { TLSCaptureSummaryStats } from "./types";
import {
  evaluateTLSFilter,
  isTLSDisplayEvent,
  isTLSRequestEvent,
  isTLSResponseEvent,
  matchesTLSIgnoreRule,
} from "./utils";

interface UseTLSCaptureFiltersOptions {
  events: Ref<TLSPlaintextEvent[]>;
  libraries: Ref<TLSLibraryStatus[]>;
  ignoreRules: Ref<TLSIgnoreRule[]>;
  searchQuery: Ref<string>;
  commFilter: Ref<string>;
  hostFilter: Ref<string>;
  selectedLib: Ref<string>;
  selectedDirection: Ref<string>;
  sslFilterExpr: Ref<string>;
  ignoreFilter: Ref<string>;
}

export const useTLSCaptureFilters = ({
  events,
  libraries,
  ignoreRules,
  searchQuery,
  commFilter,
  hostFilter,
  selectedLib,
  selectedDirection,
  sslFilterExpr,
  ignoreFilter,
}: UseTLSCaptureFiltersOptions) => {
  const filteredEvents = computed(() => {
    let list = events.value.filter(isTLSDisplayEvent);

    if (selectedLib.value !== "all") {
      list = list.filter(
        (event) =>
          (event.lib || "").toLowerCase() === selectedLib.value.toLowerCase(),
      );
    }
    if (selectedDirection.value !== "all") {
      list = list.filter(
        (event) =>
          (event.direction || "").toLowerCase() ===
          selectedDirection.value.toLowerCase(),
      );
    }
    if (commFilter.value.trim()) {
      const query = commFilter.value.trim().toLowerCase();
      list = list.filter((event) =>
        (event.comm || "").toLowerCase().includes(query),
      );
    }
    if (hostFilter.value.trim()) {
      const query = hostFilter.value.trim().toLowerCase();
      list = list.filter((event) =>
        (event.host || "").toLowerCase().includes(query),
      );
    }
    if (searchQuery.value.trim()) {
      const query = searchQuery.value.trim().toLowerCase();
      list = list.filter((event) =>
        [
          event.method,
          event.url,
          event.host,
          String(event.status || ""),
          event.body,
          JSON.stringify(event.headers || {}),
        ].some((value) => (value || "").toLowerCase().includes(query)),
      );
    }
    if (sslFilterExpr.value.trim()) {
      list = list.filter((event) =>
        evaluateTLSFilter(sslFilterExpr.value.trim(), event),
      );
    }
    if (ignoreFilter.value.trim()) {
      const patterns = ignoreFilter.value
        .split(",")
        .map((value) => value.trim().toLowerCase())
        .filter(Boolean);
      list = list.filter((event) => {
        const fields = [
          event.comm,
          event.host,
          event.url,
          event.method,
          String(event.status || ""),
          event.lib,
        ];
        return !patterns.some((pattern) =>
          fields.some(
            (value) => value && value.toLowerCase().includes(pattern),
          ),
        );
      });
    }

    const activeIgnoreRules = ignoreRules.value.filter((rule) => rule.enabled);
    if (activeIgnoreRules.length) {
      list = list.filter(
        (event) =>
          !activeIgnoreRules.some((rule) =>
            matchesTLSIgnoreRule(rule, event),
          ),
      );
    }

    return list;
  });

  const summaryStats = computed<TLSCaptureSummaryStats>(() => {
    const list = filteredEvents.value;
    return {
      total: list.length,
      sends: list.filter(isTLSRequestEvent).length,
      recvs: list.filter(isTLSResponseEvent).length,
      withBody: list.filter((event) => Number(event.body_size || 0) > 0).length,
      http: list.filter(
        (event) =>
          event.type === "http_request" || event.type === "http_response",
      ).length,
      sse: list.filter((event) => event.type === "sse_message").length,
      llm: list.filter((event) => event.prompt_digest || event.vendor).length,
      redacted: list.filter(
        (event) => event.redaction_state === "sanitized",
      ).length,
      attachedLibs: libraries.value.filter((library) => library.attached).length,
      handshakes: list.filter((event) => event.is_handshake).length,
      httpRequests: list.filter(
        (event) => event.data_type === "http_request",
      ).length,
      jsonData: list.filter((event) => event.data_type === "json").length,
      sseData: list.filter((event) => event.data_type === "sse").length,
    };
  });

  return { filteredEvents, summaryStats };
};
