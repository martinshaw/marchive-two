import { send } from "./api.js";
import { Grid } from "./Grid.jsx";
import { Message } from "./Message.jsx";
import { Split } from "./Split.jsx";
import { Cloud } from "./Words.jsx";

export function heldMessage(message, hold) {
  if (!hold) return message;
  return { data: null, fileName: "", setFileName: () => {}, note: hold };
}

export function WordToggles({ hideCommon, hideURL, toggleCommon, toggleURL }) {
  return (
    <div className="cloud-toggles">
      <label className="common-toggle"><input type="checkbox" className="hide-common" checked={hideCommon} onChange={(event) => toggleCommon(event.target.checked)} /> Hide common words</label>
      <label className="common-toggle"><input type="checkbox" className="hide-url" checked={hideURL} onChange={(event) => toggleURL(event.target.checked)} /> Hide URL words</label>
    </div>
  );
}

function CaptureCloud({ cloud, run, hold, hideCommon, hideURL, toggleCommon, toggleURL }) {
  return (
    <div className="cloud-card detail-cloud">
      <div className="cloud-head">
        <div className="split-label">{run && cloud.data?.title ? `This capture · ${cloud.data.title}` : "This capture"}</div>
        <WordToggles hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
      </div>
      <Cloud data={run && !hold ? cloud.data : null} note={run && !hold ? cloud.note : ""} missing={hold || (run ? "This capture has no word index." : "Select a capture to see its words.")} />
    </div>
  );
}

function ItemCloud({ cloud, item, hold, toggles, hideCommon, hideURL, toggleCommon, toggleURL }) {
  return (
    <div className="cloud-card detail-cloud">
      <div className="cloud-head">
        <div className="split-label">{item && cloud.data?.title ? `This item · ${cloud.data.title}` : "This item"}</div>
        {toggles ? <WordToggles hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} /> : null}
      </div>
      <Cloud data={item && !hold ? cloud.data : null} note={item && !hold ? cloud.note : ""} missing={hold || (item ? "This item has no word index." : "Select an item to see its words.")} />
    </div>
  );
}

export function CapturePane({
  view, compact, query, visibleRuns, runRows, itemRows, itemsEmpty,
  orderRef, take, marks, scope, picked, setItemIds, act,
  detailTab, setDetailTab, oneItem, message, itemHold, captureCloud, oneRun, captureHold,
  itemCloud, hideCommon, hideURL, toggleCommon, toggleURL,
}) {
  const captures = (
    <>
      <div className="split-label">Captures</div>
      <div className="table-scroll runs-scroll">
        <Grid
          columns={[{ label: "Status" }, { label: "Source", className: "col-name" }, { label: "Kind" }, { label: "When" }, { label: "Note" }, { label: "Items", className: "num" }]}
          rows={runRows}
          visible={view === "captures"}
          empty={visibleRuns.length ? "" : (query.trim() ? "No captures match this filter." : "No captures in this section.")}
          onOrder={(keys) => { orderRef.current.runs = keys; }}
          onActivate={(row, dbl, event, ordered) => {
            if (dbl) return;
            take("runs", ordered, row.run.id, event);
          }}
        />
      </div>
    </>
  );
  const items = (
    <>
      <div className="split-label">Items</div>
      <div className="table-scroll items-scroll">
        <Grid
          columns={[{ label: "Status" }, { label: "Title", className: "col-name" }, { label: "Kind" }, { label: "Address" }, { label: "Error" }]}
          rows={itemRows}
          visible={view === "captures"}
          empty={itemsEmpty}
          onOrder={(keys) => { orderRef.current.items = keys; }}
          onActivate={(row, dbl, event, ordered) => {
            if (dbl) {
              const id = row.item.id;
              marks.current.items = { anchor: id, lead: id };
              scope.current = "items";
              picked.current.items = [id];
              setItemIds([id]);
              if (row.item.downloadLocation) act(() => send("POST", "/api/open", { path: row.item.downloadLocation }));
              return;
            }
            take("items", ordered, row.item.id, event);
          }}
        />
      </div>
    </>
  );
  const details = heldMessage(oneItem ? message : { data: null, fileName: "", setFileName: () => {}, note: "" }, itemHold);

  return (
    <section className={view === "captures" ? (compact ? "pane is-on captures-pane is-compact" : "pane is-on captures-pane") : "pane captures-pane"}>
      {compact ? (
        <Split storageKey="marchive-captures-compact" sizes={[34, 33, 33]} min={96}>
          {captures}
          {items}
          <div className="item-details">
            <div className="detail-bar">
              <div className="split-label">Item details</div>
              <div className="detail-switch" role="tablist" aria-label="Item details">
                <button type="button" role="tab" aria-selected={detailTab === "message"} className={detailTab === "message" ? "file-tab is-on" : "file-tab"} onClick={() => setDetailTab("message")}>Item Details</button>
                <button type="button" role="tab" aria-selected={detailTab === "capture"} className={detailTab === "capture" ? "file-tab is-on" : "file-tab"} onClick={() => setDetailTab("capture")}>Words · This capture</button>
                <button type="button" role="tab" aria-selected={detailTab === "item"} className={detailTab === "item" ? "file-tab is-on" : "file-tab"} onClick={() => setDetailTab("item")}>Words · This item</button>
              </div>
            </div>
            {detailTab === "message" ? <Message host="capture" message={details} titleFallback="Item Details" /> : null}
            {detailTab === "capture" ? (
              <CaptureCloud cloud={captureCloud} run={oneRun} hold={captureHold} hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
            ) : null}
            {detailTab === "item" ? (
              <ItemCloud cloud={itemCloud} item={oneItem} hold={itemHold} toggles hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
            ) : null}
          </div>
        </Split>
      ) : (
        <Split storageKey="marchive-captures-tall" sizes={[34, 22, 26, 18]} min={96}>
          {captures}
          {items}
          <Message host="capture" message={details} titleFallback="Item Details" />
          <Split storageKey="marchive-captures-words" direction="row" sizes={[50, 50]} min={160}>
            <CaptureCloud cloud={captureCloud} run={oneRun} hold={captureHold} hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
            <ItemCloud cloud={itemCloud} item={oneItem} hold={itemHold} />
          </Split>
        </Split>
      )}
    </section>
  );
}
