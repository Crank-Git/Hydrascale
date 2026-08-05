// The overview view: the statistics row, the topology, and the recent activity.
//
// This file holds every line that needs a document. topology.js holds the model, the
// layout, and the serializer, and internal/ui/jstest asserts those under Node.
//
// The view reads the snapshot that the poll layer gives it. It opens no request of its
// own and it starts no timer, therefore the console keeps one data source. See
// FR-console-15.
//
// The accent belongs to one thing in this view. In the populated state it is the paths of
// the selected node, which FR-console-23 requires. In the empty state there is no
// topology, therefore the add action takes the accent instead.

import { VIEWS, registerView } from "./app.js";
import {
  TEXT_EQUIVALENT_ID,
  buildTopology,
  errorSentences,
  lastReconcileAt,
  newestEvents,
  reconcilerState,
  sinceWords,
  textEquivalentMarkup,
  topologySVGMarkup,
} from "./topology.js";

/** The count of events that the overview shows. The activity view shows the rest. */
const RECENT_EVENTS = 5;

/** selected holds the node that the operator chose, and null when the operator chose none. */
let selected = null;

/** element builds one element with a class and a text. */
function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

/** clockTime states a time of day as the operator reads it. */
function clockTime(at) {
  const time = new Date(at);
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(time.getHours())}:${pad(time.getMinutes())}:${pad(time.getSeconds())}`;
}

/** statCard draws one cell of the statistics row. A value is a machine value. */
function statCard(label, value) {
  const card = element("div", "stat");
  card.append(element("span", "label", label));
  card.append(element("div", "stat-value mono", value));
  return card;
}

/** stateCard draws one cell whose value is a state: a coloured dot and a lowercase word. */
function stateCard(label, state) {
  const card = element("div", "stat");
  card.append(element("span", "label", label));
  const value = element("div", "stat-value state");
  value.append(element("span", `dot ${state.tone}`));
  value.append(element("span", undefined, state.word));
  card.append(value);
  return card;
}

/** drawStatistics draws the four counts of FR-console-18. */
function drawStatistics(section, model, status, events, now) {
  const row = element("div", "stats");
  row.append(statCard("Tailnets", String(model.counts.tailnets)));
  row.append(statCard("Peers", String(model.counts.peers)));
  row.append(stateCard("Reconciler", reconcilerState(status)));
  row.append(statCard("Last tick", sinceWords(now, lastReconcileAt(events))));
  section.append(row);
}

/**
 * drawTopology draws the picture and its text equivalent.
 *
 * The serializer returns SVG source, so the function writes it once and then binds the
 * handlers on the groups that it wrote. A selection redraws the source alone, and the node
 * that the operator chose keeps the focus.
 */
function drawTopology(section, model, redraw) {
  const card = element("section", "card");
  card.setAttribute("aria-labelledby", "topology-heading");

  const head = element("div", "card-head");
  const heading = element("span", "label", "Topology");
  heading.id = "topology-heading";
  head.append(heading);
  head.append(element("span", "label", model.summary.replace(/\.$/, "").toLowerCase()));
  card.append(head);

  // The two serializers escape every value that the daemon reported, and a test asserts
  // that. The console builds no markup from a daemon value anywhere else.
  const figure = element("div", "flow-wrap");
  figure.innerHTML = topologySVGMarkup(model, selected);
  card.append(figure);

  const text = element("div", "sr");
  text.id = TEXT_EQUIVALENT_ID;
  text.innerHTML = textEquivalentMarkup(model);
  card.append(text);

  card.append(
    element(
      "p",
      "note",
      "Select a node to draw its paths alone. A path that no rule allows has no line.",
    ),
  );
  section.append(card);

  for (const group of figure.querySelectorAll("g.node")) {
    const id = group.dataset.node;
    const choose = () => {
      selected = selected === id ? null : id;
      redraw();
    };
    group.addEventListener("click", choose);
    group.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        choose();
      }
    });
  }

  const chosen = figure.querySelector(`g.node[data-node="${CSS.escape(selected || "")}"]`);
  if (chosen) {
    chosen.focus();
  }
}

/** drawActivity draws the five newest events. */
function drawActivity(section, events) {
  const card = element("section", "card");
  card.append(element("span", "label", "Recent activity"));

  const recent = newestEvents(events, RECENT_EVENTS);
  if (recent.length === 0) {
    card.append(
      element(
        "p",
        "note",
        "The daemon reports no event. An event arrives when the reconciler creates a namespace, connects a tailnet, or writes the access rules.",
      ),
    );
    section.append(card);
    return;
  }

  const list = element("div", "events");
  for (const event of recent) {
    const row = element("div", "ev");
    row.append(element("time", "mono", clockTime(Date.parse(event.Time))));
    row.append(element("span", "ev-kind mono", event.Type));
    const message = event.TailnetID
      ? `${event.TailnetID}: ${event.Message}`
      : event.Message;
    row.append(element("p", undefined, message));
    list.append(row);
  }
  card.append(list);
  section.append(card);
}

/** drawLoading draws the shell and one quiet placeholder row. */
function drawLoading(section) {
  const card = element("div", "card");
  card.append(element("span", "label", "Loading"));
  card.append(element("p", "note", "The first poll has not returned."));
  section.append(card);
}

/** drawError names the console address and the socket path that the daemon opens. */
function drawError(section, snapshot) {
  const card = element("div", "card");
  card.append(element("span", "label", "Unreachable"));
  for (const sentence of errorSentences(window.location.host, snapshot.error)) {
    card.append(element("p", "note", sentence));
  }
  section.append(card);
}

/** drawEmpty states that no tailnet is configured and it shows the add action. */
function drawEmpty(section) {
  const card = element("div", "card empty");
  card.append(element("span", "label", "Empty"));
  // The shell holds the one empty sentence of every view, so the console states it once.
  const view = VIEWS.find((entry) => entry.id === "overview");
  card.append(element("p", undefined, view.empty));
  const row = element("div", "row");
  const add = element("a", "btn primary", "Add tailnet");
  add.href = "#/namespaces";
  row.append(add);
  card.append(row);
  card.append(
    element(
      "p",
      "note",
      "The add flow arrives with the namespace view of issue #144. The command hydrascale init writes a configuration file now.",
    ),
  );
  section.append(card);
}

/**
 * draw draws the overview from one poll snapshot.
 *
 * snapshot.status holds the merged body of the poll: the fields of GET /api/status, the
 * field access_model from GET /api/access, and the field events from GET /api/events.
 * The shell draws the stale marker and the time of the last success in the poll banner, so
 * this view draws the last known state and adds no second marker.
 */
function draw(section, snapshot) {
  section.replaceChildren();

  if (snapshot.loading) {
    drawLoading(section);
    return;
  }
  if (!snapshot.status) {
    drawError(section, snapshot);
    return;
  }

  const status = snapshot.status;
  const events = status.events || [];
  const model = buildTopology(status, status.access_model);

  if (model.counts.tailnets === 0) {
    drawEmpty(section);
    drawActivity(section, events);
    return;
  }

  if (selected !== null && !model.nodes.some((node) => node.id === selected)) {
    selected = null;
  }

  drawStatistics(section, model, status, events, Date.now());

  const grid = element("div", "grid");
  section.append(grid);
  drawTopology(grid, model, () => draw(section, snapshot));
  drawActivity(grid, events);
}

registerView("overview", draw);
