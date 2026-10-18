import { useEffect, useState } from "react";
import { shortWhen } from "./format.js";

const COLORS = ["#0a64ff", "#1f8a4c", "#e07a12", "#9b4dca", "#d92b2b", "#6e6e73"];

export function useCloud(url, enabled, revision) {
  const [pack, setPack] = useState(null);
  const [note, setNote] = useState("");
  useEffect(() => {
    if (!enabled || !url) {
      setPack(null);
      setNote("");
      return undefined;
    }
    let cancel = false;
    setNote("Loading words…");
    fetch(url, { cache: "no-store", headers: { Accept: "application/json" } })
      .then((response) => {
        if (!response.ok) throw new Error("missing");
        return response.json();
      })
      .then((next) => {
        if (cancel) return;
        setPack({ url, data: next });
        setNote("");
      })
      .catch(() => {
        if (!cancel) setNote("Could not load the word index.");
      });
    return () => { cancel = true; };
  }, [url, enabled, revision]);
  const current = pack && pack.url === url ? pack.data : null;
  return { data: current, note: current ? "" : note };
}

export function Cloud({ data, note, missing, query, focus, onPick }) {
  let words = ((data && data.words) || []).slice(0, 80);
  if (onPick && query) {
    const q = query.trim().toLowerCase();
    if (q) words = words.filter((word) => word.word.toLowerCase().includes(q));
  }
  if (!words.length) {
    return <p className="empty cloud-empty">{note || missing}</p>;
  }
  const max = words[0].count || 1;
  return (
    <div className="cloud">
      {words.map((item, index) => {
        const tone = index < 5 ? " tone-2" : index > 24 ? " tone-4" : "";
        const on = onPick && focus === item.word ? " is-on" : "";
        return (
          <button
            key={item.word}
            type="button"
            className={`cloud-word${tone}${on}`}
            style={{ fontSize: `${Math.round(13 + Math.sqrt(item.count / max) * 24)}px` }}
            title={`${item.word} · ${item.count}`}
            onClick={() => onPick?.(item.word)}
          >
            {item.word}
          </button>
        );
      })}
    </div>
  );
}

export function WordChart({ data, focus, onPick }) {
  const captures = (data && data.captures) || [];
  const series = focus ? [focus] : ((data && data.words) || []).slice(0, 6).map((word) => word.word);
  if (!captures.length || !series.length) {
    return <p className="empty cloud-empty">Word frequencies appear here after a capture saves a word index.</p>;
  }
  const width = 640;
  const height = 220;
  const left = 36;
  const right = 12;
  const top = 16;
  const bottom = 32;
  let max = 1;
  captures.forEach((capture) => {
    series.forEach((word) => {
      const count = (capture.counts && capture.counts[word]) || 0;
      if (count > max) max = count;
    });
  });
  const xAt = (index) => {
    if (captures.length === 1) return left + (width - left - right) / 2;
    return left + ((width - left - right) * index) / (captures.length - 1);
  };
  const yAt = (count) => top + (height - top - bottom) * (1 - count / max);
  const step = captures.length > 6 ? Math.ceil(captures.length / 6) : 1;
  return (
    <>
      <div className="words-legend">
        {series.map((word, index) => (
          <button
            key={word}
            type="button"
            style={{ color: COLORS[index % COLORS.length] }}
            className={focus === word ? "is-on" : undefined}
            onClick={() => onPick(word)}
          >
            {word}
          </button>
        ))}
      </div>
      <svg className="words-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Word frequencies over captures">
        <text x="4" y={top + 4} fill="#8e8e93" fontSize="11">{max}</text>
        {series.map((word, seriesIndex) => {
          const color = COLORS[seriesIndex % COLORS.length];
          const points = captures.map((capture, index) => `${xAt(index)},${yAt((capture.counts && capture.counts[word]) || 0)}`).join(" ");
          return (
            <g key={word}>
              <polyline fill="none" stroke={color} strokeWidth={focus ? "2.5" : "1.8"} strokeLinejoin="round" strokeLinecap="round" points={points} />
              {captures.map((capture, index) => (
                <circle
                  key={`${word}-${capture.runId}`}
                  cx={xAt(index)}
                  cy={yAt((capture.counts && capture.counts[word]) || 0)}
                  r={captures.length > 24 ? 2 : 3}
                  fill={color}
                />
              ))}
            </g>
          );
        })}
        {captures.map((capture, index) => {
          if (index % step !== 0 && index !== captures.length - 1) return null;
          return (
            <text key={capture.runId} x={xAt(index)} y={height - 8} textAnchor="middle" fill="#6e6e73" fontSize="11">
              {shortWhen(capture.at)}
            </text>
          );
        })}
      </svg>
    </>
  );
}
