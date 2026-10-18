import { itemKindLabel } from "./format.js";
import { Message } from "./Message.jsx";
import { heldMessage } from "./CapturePane.jsx";

export function ReaderPane({
  view, groups, shownReader, readerIds, query, readerList, take, oneReader, readerMessage, readerHold,
  sources, kinds, source, kind, read, sort, group, filtered,
  onSource, onKind, onRead, onSort, onGroup, onClear,
}) {
  const selected = new Set(readerIds);
  return (
    <section className={view === "reader" ? "pane is-on" : "pane"}>
      <div className="reader">
        <div className="reader-list">
          <div className="reader-tools">
            <select aria-label="Source" value={source} onChange={(event) => onSource(event.target.value)}>
              <option value="">All sources</option>
              {sources.map((name) => <option key={name} value={name}>{name}</option>)}
            </select>
            <select aria-label="Kind" value={kind} onChange={(event) => onKind(event.target.value)}>
              <option value="">All kinds</option>
              {kinds.map((id) => <option key={id} value={id}>{itemKindLabel(id)}</option>)}
            </select>
            <select aria-label="Read" value={read} onChange={(event) => onRead(event.target.value)}>
              <option value="all">All items</option>
              <option value="unread">Unread</option>
              <option value="read">Read</option>
            </select>
            <select aria-label="Sort" value={sort} onChange={(event) => onSort(event.target.value)}>
              <option value="newest">Newest</option>
              <option value="oldest">Oldest</option>
              <option value="title">Title</option>
            </select>
            <select aria-label="Group" value={group} onChange={(event) => onGroup(event.target.value)}>
              <option value="none">No groups</option>
              <option value="source">Group by source</option>
            </select>
            <button type="button" disabled={!filtered} onClick={onClear}>Clear filters</button>
          </div>
          <div className="reader-scroll" ref={readerList}>
            {shownReader.length ? groups.map((section) => (
              <div key={section.key}>
                {section.label ? <p className="reader-group">{section.label}</p> : null}
                {section.items.map((item) => (
                  <button
                    type="button"
                    key={item.id}
                    data-key={item.id}
                    className={["reader-row", selected.has(item.id) ? "is-selected" : "", item.read ? "" : "is-unread"].filter(Boolean).join(" ")}
                    onMouseDown={(event) => { if (event.shiftKey || event.metaKey) event.preventDefault(); }}
                    onClick={(event) => {
                      if (event.ctrlKey && !event.metaKey) return;
                      if (event.shiftKey || event.metaKey) event.preventDefault();
                      take("reader", shownReader.map((row) => row.id), item.id, event);
                    }}
                  >
                    <span className="primary">{item.title}</span>
                    <span className="sub">{[item.source, itemKindLabel(item.kind), item.when].filter(Boolean).join(" · ")}</span>
                  </button>
                ))}
              </div>
            )) : <p className="empty reader-empty">{filtered || query.trim() ? "No items match these filters." : "No capture items yet."}</p>}
          </div>
        </div>
        <Message host="reader" message={heldMessage(oneReader ? readerMessage : { data: null, fileName: "", setFileName: () => {}, note: "" }, readerHold)} titleFallback="Reader" />
      </div>
    </section>
  );
}
