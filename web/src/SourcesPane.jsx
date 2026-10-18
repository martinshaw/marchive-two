import { Grid } from "./Grid.jsx";

export function SourcesPane({ view, query, sourceRows, visibleSources, orderRef, take, marks, scope, picked, setScheduleIds, openEdit }) {
  return (
    <section className={view === "sources" ? "pane is-on" : "pane"}>
      <div className="table-scroll">
        <Grid
          columns={[{ label: "Name", className: "col-name" }, { label: "Kind" }, { label: "Address" }, { label: "Every" }, { label: "Next" }, { label: "New", className: "num" }, { label: "Folder" }, { label: "State" }]}
          rows={sourceRows}
          visible={view === "sources"}
          empty={visibleSources.length ? "" : (query.trim() ? "No sources match this filter." : "No sources in this section.")}
          onOrder={(keys) => { orderRef.current.sources = keys; }}
          onActivate={(row, dbl, event, ordered) => {
            if (dbl) {
              const id = row.row.schedule.id;
              marks.current.sources = { anchor: id, lead: id };
              scope.current = "sources";
              picked.current.sources = [id];
              setScheduleIds([id]);
              openEdit(row.row);
              return;
            }
            take("sources", ordered, row.row.schedule.id, event);
          }}
        />
      </div>
    </section>
  );
}
