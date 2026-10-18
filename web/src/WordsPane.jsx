import { Cloud, WordChart } from "./Words.jsx";
import { WordToggles } from "./CapturePane.jsx";

export function WordsPane({ view, timeline, query, focus, setFocus, hideCommon, hideURL, toggleCommon, toggleURL }) {
  return (
    <section className={view === "words" ? "pane is-on" : "pane"}>
      <div className="words-layout">
        <div className="cloud-card">
          <div className="cloud-head">
            <div className="split-label">{timeline.data?.wordCount ? `All captures · ${timeline.data.wordCount} words` : "All captures"}</div>
            <WordToggles hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
          </div>
          <Cloud
            data={timeline.data}
            note={timeline.note}
            missing="No word index yet."
            query={query}
            focus={focus}
            onPick={(word) => setFocus((current) => (current === word ? "" : word))}
          />
        </div>
        <div className="chart-card">
          <div className="split-label">Over time</div>
          <WordChart data={timeline.data} focus={focus} onPick={(word) => setFocus((current) => (current === word ? "" : word))} />
        </div>
      </div>
    </section>
  );
}
