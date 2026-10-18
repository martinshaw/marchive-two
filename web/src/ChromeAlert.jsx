import { useEffect, useState } from "react";

function mb(n) {
  if (!n || n < 0) return "0 MB";
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function ChromeAlert({ open, onClose, onStatus }) {
  const [data, setData] = useState({
    phase: "idle",
    message: "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it.",
  });

  useEffect(() => {
    if (!open) return undefined;
    let stop = false;
    let timer = 0;
    async function poll() {
      try {
        const response = await fetch("/api/chrome", { cache: "no-store" });
        if (!response.ok || stop) return;
        const next = await response.json();
        if (stop) return;
        setData(next);
        const phase = next.phase || "idle";
        if (phase === "downloading" || phase === "failed" || phase === "done" || phase === "cancelled" || phase === "installing") {
          onStatus(next.error || next.message || "");
        }
        if (phase === "downloading" || phase === "installing") timer = window.setTimeout(poll, 400);
      } catch {
        /* Keep the dialog up and try again. */
      }
    }
    poll();
    return () => {
      stop = true;
      window.clearTimeout(timer);
    };
  }, [open, data.phase, onStatus]);

  if (!open) return null;
  const phase = data.phase || "idle";
  const working = phase === "downloading" || phase === "installing";
  const known = data.total > 0 && phase === "downloading";
  const busy = phase === "installing" || (working && !known);
  const title = phase === "done" ? "Google Chrome is installed"
    : phase === "failed" ? "Google Chrome could not be installed"
      : phase === "downloading" ? "Downloading Google Chrome"
        : phase === "installing" ? "Installing Google Chrome"
          : "Google Chrome is required";
  let bar = "0%";
  if (busy) bar = "";
  else if (phase === "done") bar = "100%";
  else if (data.total > 0) bar = `${Math.max(0, Math.min(100, (data.received / data.total) * 100))}%`;
  let label = "";
  if (phase === "downloading" && data.total > 0) label = `Downloading… ${mb(data.received)} of ${mb(data.total)}`;
  else if (working) label = data.message || "";
  const okLabel = phase === "failed" ? "Try Again" : "OK";

  async function install() {
    setData({ phase: "downloading", received: 0, total: 0, message: "Downloading Google Chrome…" });
    const response = await fetch("/api/chrome/install", { method: "POST" });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      setData({ phase: "failed", error: body.error || "Could not start the install." });
      return;
    }
    setData(await response.json());
  }

  return (
    <div className="alert-layer">
      <div className="alert" role="dialog" aria-modal="true" aria-labelledby="chrome-alert-title">
        <div className="alert-main">
          <h2 id="chrome-alert-title">{title}</h2>
          <p>{data.error || data.message || "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it."}</p>
          <div className={`progress${busy ? " is-busy" : ""}`} hidden={!(working || phase === "done")}>
            <div className="progress-bar" style={bar ? { width: bar } : undefined} />
          </div>
          <p className="progress-label">{label}</p>
          <div className="alert-actions">
            <button type="button" hidden={working || phase === "done"} onClick={onClose}>Not Now</button>
            <button type="button" className="push" hidden={working || phase === "done" || phase === "failed"} onClick={install}>Install</button>
            <button
              type="button"
              hidden={!working}
              onClick={async () => {
                await fetch("/api/chrome/install/cancel", { method: "POST" });
                const response = await fetch("/api/chrome", { cache: "no-store" });
                if (response.ok) setData(await response.json());
              }}
            >
              Cancel
            </button>
            <button
              type="button"
              className="push"
              hidden={phase !== "done" && phase !== "failed" && phase !== "cancelled"}
              onClick={() => {
                if (phase === "done") {
                  window.location.reload();
                  return;
                }
                setData({ phase: "idle", message: "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it." });
              }}
            >
              {okLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
