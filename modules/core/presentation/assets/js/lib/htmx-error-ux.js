// HTMX error UX helpers
// - Allows swapping HTML error fragments (e.g. 422 forms, 403 unauthorized)
// - Shows a fallback toast for error responses when no server toast is provided
(function () {
  function toastLabels() {
    const ds = (document.body && document.body.dataset) || {};
    const errorTitle = String(ds.toastErrorTitle || "Error").trim();
    const defaultError = String(ds.toastDefaultError || "Request failed, please try again").trim();
    const networkError = String(ds.toastNetworkError || defaultError).trim();
    return { errorTitle, defaultError, networkError };
  }

  function dispatchToast(variant, title, message) {
    if (!window.dispatchEvent) {
      return;
    }
    window.dispatchEvent(
      new CustomEvent("notify", {
        detail: { variant, title, message },
      }),
    );
  }

  function normalizeMessage(value) {
    return String(value || "")
      .replace(/\s+/g, " ")
      .trim();
  }

  function looksLikeHTML(text) {
    const trimmed = String(text || "").trim();
    if (!trimmed) {
      return false;
    }
    return trimmed.startsWith("<") || trimmed.includes("<div") || trimmed.includes("<section") || trimmed.includes("<html");
  }

  function hasToastTrigger(xhr) {
    if (!xhr || !xhr.getResponseHeader) {
      return false;
    }
    const header = xhr.getResponseHeader("HX-Trigger");
    if (!header) {
      return false;
    }
    const raw = String(header).trim();
    if (!raw) {
      return false;
    }

    if (raw === "notify" || raw === "showErrorToast") {
      return true;
    }
    if (raw.includes('"notify"') || raw.includes('"showErrorToast"')) {
      return true;
    }

    const events = raw.split(",").map((part) => part.trim());
    return events.includes("notify") || events.includes("showErrorToast");
  }

  function safeJSONParse(text) {
    try {
      return JSON.parse(text);
    } catch (e) {
      return null;
    }
  }

  function extractAPIError(xhr) {
    if (!xhr) {
      return null;
    }
    const text = typeof xhr.responseText === "string" ? xhr.responseText : "";
    const contentType = xhr.getResponseHeader ? xhr.getResponseHeader("Content-Type") || "" : "";
    if (!text) {
      return null;
    }

    const seemsJSON = contentType.toLowerCase().includes("application/json") || text.trim().startsWith("{");
    if (!seemsJSON) {
      return null;
    }

    const parsed = safeJSONParse(text);
    if (!parsed || typeof parsed !== "object") {
      return null;
    }

    const code = normalizeMessage(parsed.code || parsed.Code || "");
    const message = normalizeMessage(parsed.message || parsed.Message || "");
    if (!code && !message) {
      return null;
    }
    return { code, message };
  }

  function tryFormatFrozenWindow(text) {
    const raw = String(text || "");
    const match = raw.match(/affected_at=([^\s]+)\s+is before cutoff=([^\s]+)/i);
    if (!match) {
      return null;
    }

    const affectedAtRaw = match[1];
    const cutoffRaw = match[2];
    const affectedAtDate = new Date(affectedAtRaw);
    const cutoffDate = new Date(cutoffRaw);
    const affectedAt = Number.isNaN(affectedAtDate.getTime()) ? affectedAtRaw : affectedAtDate.toISOString().slice(0, 10);
    const cutoff = Number.isNaN(cutoffDate.getTime()) ? cutoffRaw : cutoffDate.toISOString().slice(0, 10);

    const lang = String((document.documentElement && document.documentElement.lang) || "");
    const isZh = lang.toLowerCase().startsWith("zh");
    if (isZh) {
      return `所选日期（${affectedAt}）早于冻结截止日（${cutoff}），请调整日期后重试。`;
    }
    return `Selected date (${affectedAt}) is before freeze cutoff (${cutoff}). Please choose a later date.`;
  }

  function shouldSwapHTMLOnError(evt) {
    const xhr = evt && evt.detail && evt.detail.xhr;
    if (!xhr || typeof xhr.status !== "number") {
      return false;
    }
    if (xhr.status < 400) {
      return false;
    }
    if (!xhr.getResponseHeader) {
      return false;
    }
    const contentType = (xhr.getResponseHeader("Content-Type") || "").toLowerCase();
    if (!contentType.includes("text/html")) {
      return false;
    }
    const text = xhr.responseText || "";
    return !!String(text).trim();
  }

  function shouldSkipToast(evt) {
    const elt = evt && evt.detail && evt.detail.elt;
    if (!elt || !elt.closest) {
      return false;
    }
    return !!elt.closest('[data-hx-silent-errors="true"]');
  }

  document.addEventListener("htmx:beforeSwap", function (evt) {
    if (!evt || !evt.detail) {
      return;
    }
    if (shouldSwapHTMLOnError(evt)) {
      evt.detail.shouldSwap = true;
    }
  });

  document.addEventListener("htmx:responseError", function (evt) {
    if (shouldSkipToast(evt)) {
      return;
    }

    const xhr = evt && evt.detail && evt.detail.xhr;
    if (!xhr) {
      return;
    }
    if (hasToastTrigger(xhr)) {
      return;
    }

    const labels = toastLabels();
    const contentType = xhr.getResponseHeader ? (xhr.getResponseHeader("Content-Type") || "").toLowerCase() : "";
    const text = xhr.responseText || "";
    if (contentType.includes("text/html") || looksLikeHTML(text)) {
      return;
    }

    const status = xhr.status || 0;
    const apiErr = extractAPIError(xhr);

    let title = labels.errorTitle;
    let message = "";
    if (apiErr) {
      title = apiErr.code || title;
      message = apiErr.message || labels.defaultError;
    } else {
      message = tryFormatFrozenWindow(text) || normalizeMessage(text) || labels.defaultError;
      if (status >= 400) {
        title = `${title} (${status})`;
      }
    }

    if (message.length > 300) {
      message = message.slice(0, 297) + "...";
    }

    dispatchToast("error", title, message);
  });

  document.addEventListener("htmx:sendError", function (evt) {
    if (shouldSkipToast(evt)) {
      return;
    }
    const labels = toastLabels();
    dispatchToast("error", labels.errorTitle, labels.networkError || labels.defaultError);
  });

  document.addEventListener("htmx:timeout", function (evt) {
    if (shouldSkipToast(evt)) {
      return;
    }
    const labels = toastLabels();
    dispatchToast("error", labels.errorTitle, labels.networkError || labels.defaultError);
  });
})();

