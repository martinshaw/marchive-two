import { useEffect, useState } from "react";
import { tweetAvatar, tweetIdentity, tweetMedia, tweetParts, tweetStamp } from "./tweet.js";

export function TweetText({ data, text }) {
  const [meta, setMeta] = useState(null);
  const body = text === "Loading…" ? "" : text;

  useEffect(() => {
    if (!(data.files || []).some((file) => file.name === "metadata.json")) {
      setMeta(null);
      return undefined;
    }
    let cancel = false;
    fetch(`/api/items/${data.id}/files/metadata.json`, { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((next) => { if (!cancel) setMeta(next); })
      .catch(() => { if (!cancel) setMeta(null); });
    return () => { cancel = true; };
  }, [data.id, data.files]);

  const who = tweetIdentity({
    title: data.title,
    text: body,
    url: meta?.url || data.url,
    authors: meta?.authors,
    handle: meta?.handle,
  });
  const avatar = tweetAvatar(who.handle || who.name);
  const media = tweetMedia(data.files);
  const images = media.filter((file) => file.kind === "image");
  const rest = media.filter((file) => file.kind !== "image");
  const when = tweetStamp(meta?.capturedAt || data.capturedAt);
  const fileURL = (name) => `/api/items/${data.id}/files/${encodeURIComponent(name)}`;

  return (
    <article className="tweet">
      <header className="tweet-top">
        <div className="tweet-avatar" style={{ background: avatar.color }} aria-hidden="true">{avatar.letter}</div>
        <div className="tweet-id">
          <strong className="tweet-name">{who.name}</strong>
          {who.handle ? <span className="tweet-handle">{who.handle}</span> : null}
        </div>
        <XMark />
      </header>
      <div className="tweet-copy">
        {text === "Loading…" ? <span className="tweet-wait">Loading…</span> : <TweetCopy text={body} />}
      </div>
      {images.length ? (
        <div className={`tweet-shots is-${Math.min(images.length, 4)}`}>
          {images.slice(0, 4).map((file) => <img key={file.name} alt="" src={fileURL(file.name)} />)}
        </div>
      ) : null}
      {rest.map((file) => (
        file.kind === "video"
          ? <video key={file.name} className="tweet-video" controls src={fileURL(file.name)} />
          : <audio key={file.name} className="tweet-audio" controls src={fileURL(file.name)} />
      ))}
      {when ? <p className="tweet-time">{when}</p> : null}
      <div className="tweet-bar" aria-hidden="true">
        <ReplyIcon />
        <RepostIcon />
        <LikeIcon />
        <ViewsIcon />
        <BookmarkIcon />
        <ShareIcon />
      </div>
    </article>
  );
}

function TweetCopy({ text }) {
  return tweetParts(text).map((part, index) => {
    if (part.type === "link") {
      return <a key={index} className="tweet-link" href={part.value} target="_blank" rel="noreferrer">{part.value}</a>;
    }
    if (part.type === "mention" || part.type === "tag") {
      return <span key={index} className="tweet-link">{part.value}</span>;
    }
    return <span key={index}>{part.value}</span>;
  });
}

function XMark() {
  return (
    <svg className="tweet-logo" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="currentColor" d="M14.7 10.3 22.2 2h-1.8l-6.5 7.2L8.8 2H2.2l7.9 11.1L2.2 22h1.8l6.9-7.6L15.2 22h6.6l-7.1-11.7Zm-2.4 2.7-.8-1.1L4.6 3.3h2.7l5.1 7.1.8 1.1 6.7 9.2h-2.7l-5.4-7.7Z" />
    </svg>
  );
}

function ReplyIcon() {
  return <svg viewBox="0 0 24 24"><path d="M6 12.2a6 6 0 1 1 2.2 4.6L6 18.5l.7-2.6A6 6 0 0 1 6 12.2Z" /></svg>;
}

function RepostIcon() {
  return <svg viewBox="0 0 24 24"><path d="M7 8h9.5L14 5.5M16.5 8 14 10.5M17 16H7.5L10 18.5M7.5 16 10 13.5" /></svg>;
}

function LikeIcon() {
  return <svg viewBox="0 0 24 24"><path d="M12 19.2s-6.2-3.9-8.2-7.4C2.2 9.2 3 6.2 5.6 5.4c1.6-.5 3.2.1 4.2 1.5L12 9.2l2.2-2.3c1-1.4 2.6-2 4.2-1.5 2.6.8 3.4 3.8 1.8 6.4-2 3.5-8.2 7.4-8.2 7.4Z" /></svg>;
}

function ViewsIcon() {
  return <svg viewBox="0 0 24 24"><path d="M5 16v4M10 12v8M15 8v12M20 13v7" /></svg>;
}

function BookmarkIcon() {
  return <svg viewBox="0 0 24 24"><path d="M7 4.5h10v15l-5-3-5 3v-15Z" /></svg>;
}

function ShareIcon() {
  return <svg viewBox="0 0 24 24"><path d="M12 15.5V5M8.2 8.6 12 4.8l3.8 3.8M6 19h12" /></svg>;
}
