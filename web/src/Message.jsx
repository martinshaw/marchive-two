import { useEffect, useRef, useState } from "react";
import { fileLabel, itemKindLabel, shortWhen } from "./format.js";
import { TweetText } from "./TweetText.jsx";

export function useMessage(itemId, revision) {
  const [data, setData] = useState(null);
  const [fileName, setFileName] = useState("");
  const [note, setNote] = useState("");

  useEffect(() => {
    if (!itemId) {
      setData(null);
      setFileName("");
      setNote("");
      return undefined;
    }
    let cancel = false;
    setData((current) => (current && String(current.id) === String(itemId) ? current : null));
    setNote("Loading…");
    fetch(`/api/items/${itemId}/files`, { cache: "no-store", headers: { Accept: "application/json" } })
      .then((response) => {
        if (!response.ok) throw new Error("missing");
        return response.json();
      })
      .then((next) => {
        if (cancel) return;
        setData(next);
        setNote("");
        setFileName((current) => {
          if (current && (next.files || []).some((file) => file.name === current)) return current;
          return (next.files || [])[0]?.name || "";
        });
      })
      .catch(() => {
        if (!cancel) {
          setData(null);
          setNote("Could not open this item.");
        }
      });
    return () => { cancel = true; };
  }, [itemId, revision]);

  return { data, fileName, setFileName, note };
}

export function Message({ host, message, titleFallback }) {
  const { data, fileName, setFileName, note } = message;
  const file = (data?.files || []).find((item) => item.name === fileName);
  let title = titleFallback;
  if (data) title = data.title || "Untitled";
  else if (note && note !== "Loading…" && !note.startsWith("Select")) title = titleFallback;
  const when = data ? shortWhen(data.capturedAt) : "";
  const bits = data ? [data.sourceName, itemKindLabel(data.kind), when].filter(Boolean) : [];

  return (
    <article className={host === "capture" ? "message message-dock" : "message"} id={host === "capture" ? "capture-message" : "reader-message"}>
      <header className="message-head">
        <h2 className="message-title">{title}</h2>
        <p className="message-meta">
          {bits.join(" · ")}
          {data?.url ? <>{bits.length ? " · " : ""}<a href={data.url}>{data.url}</a></> : null}
        </p>
      </header>
      <div className="message-files" hidden={!data?.files?.length}>
        {(data?.files || []).map((item) => (
          <button
            key={item.name}
            type="button"
            className={item.name === fileName ? "file-tab is-on" : "file-tab"}
            onClick={() => setFileName(item.name)}
          >
            {fileLabel(item)}
          </button>
        ))}
      </div>
      <div className="message-body">
        <MessageBody host={host} data={data} file={file} note={note} />
      </div>
    </article>
  );
}

function MessageBody({ host, data, file, note }) {
  const [text, setText] = useState("");
  const loadedFor = useRef("");
  const fileKey = data && file ? `${data.id}:${file.name}` : "";
  useEffect(() => {
    if (!data || !file || file.kind === "image" || file.kind === "audio" || file.kind === "video" || file.kind === "html" || file.bytes > 1500000) {
      loadedFor.current = "";
      setText("");
      return undefined;
    }
    let cancel = false;
    const key = `${data.id}:${file.name}`;
    setText("Loading…");
    const url = `/api/items/${data.id}/files/${encodeURIComponent(file.name)}`;
    fetch(url, { cache: "no-store" })
      .then((response) => {
        if (!response.ok) throw new Error("missing");
        return response.text();
      })
      .then((body) => {
        if (cancel) return;
        loadedFor.current = key;
        setText(body);
      })
      .catch(() => {
        if (cancel) return;
        loadedFor.current = key;
        setText("Could not read this file.");
      });
    return () => { cancel = true; };
  }, [data?.id, file?.name, file?.kind, file?.bytes]);
  const shown = loadedFor.current === fileKey ? text : "Loading…";

  if (!data) return <p className="empty">{note || "Select an item to read its files."}</p>;
  if (!file) return <p className="empty">{note || "This item has no files."}</p>;
  const url = `/api/items/${data.id}/files/${encodeURIComponent(file.name)}`;
  if (file.kind === "image") return <img className="message-shot" alt={file.name} src={url} />;
  if (file.kind === "audio") return <audio className="message-audio" controls src={url} />;
  if (file.kind === "video") return <video className="message-video" controls src={url} />;
  if (file.kind === "html") return <iframe className="message-frame" sandbox="" title={file.name} src={url} />;
  if (file.bytes > 1500000) return <p className="empty">This file is too large to show here.</p>;
  if (host === "reader" && data.kind === "tweet" && file.name === "text.txt") {
    return <TweetText data={data} text={shown} />;
  }
  return <pre className="message-text">{shown}</pre>;
}
