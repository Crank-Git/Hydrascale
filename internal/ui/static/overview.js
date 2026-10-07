// The overview view: the verdict, the board, the topology, and the notable events.
//
// This file holds every line that needs a document. board.js and topology.js hold the
// models, the layout, and the serializer, and internal/ui/jstest asserts those under Node.
//
// The view reads the snapshot that the poll layer gives it. It opens no request of its
// own and it starts no timer, therefore the console keeps one data source. See
// FR-console-15.
//
// The accent belongs to one thing in this view: the selection. A selected row names its
// tailnet in the accent, and the topology draws the paths of that tailnet in the accent,
// which FR-console-23 requires. In the empty state there is no selection, therefore the
// add action takes the accent instead.

import { VIEWS, registerView } from "./app.js";
import { COLUMNS, buildBoard, cellMap, cellText, changedCells, notableEvents, verdictOf } from "./board.js";
import {
  TEXT_EQUIVALENT_ID,
  buildTopology,
  errorSentences,
  lastReconcileAt,
  pathListMarkup,
  reconcilerState,
  textEquivalentMarkup,
  topologySVGMarkup,
} from "./topology.js";

/** The count of notable events that the overview shows. The activity view shows the rest. */
const NOTABLE_EVENTS = 5;

/** selected holds the node that the operator chose, and null when the operator chose none. */
let selected = null;

/** shown holds the text of every board cell of the last draw, for the turnover. */
let shown = new Map();

/**
 * focusArea names the part of the view that held the keyboard focus before a redraw:
 * "board", "flow", or null. A redraw replaces every element, so the view reads the focus
 * before it replaces them and gives it back to the selected row or node after.
 */
let focusArea = null;

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

/** plural states a count and its noun, with the noun in the right number. */
function plural(n, noun) {
  return n === 1 ? `${n} ${noun}` : `${n} ${noun}s`;
}

/**
 * tierClass marks a cell of a column that a narrow screen hides: "mid" for a column that
 * a phone hides, and "opt" for a column that a tablet hides as well.
 */
function tierClass(cell, column) {
  if (column.tier === "tablet") {
    cell.classList.add("mid");
  } else if (column.tier === "desktop") {
    cell.classList.add("opt");
  }
}

/** detailRow states the cells of one row that a phone hides, as a description list. */
function detailRow(row) {
  const line = element("tr", "board-detail");
  const cell = element("td");
  cell.colSpan = COLUMNS.length;
  const list = element("dl", "kv-list");
  for (const column of COLUMNS.filter((entry) => entry.tier !== "phone")) {
    const pair = element("div", "kv");
    pair.append(element("dt", undefined, column.head));
    pair.append(element("dd", "mono", cellText(row, column.key)));
    list.append(pair);
  }
  cell.append(list);
  line.append(cell);
  return line;
}

/** choose selects a tailnet, or clears the selection when the operator chooses it again. */
function choose(id, redraw) {
  selected = selected === id ? null : id;
  redraw();
}

/**
 * value writes the text of one cell. A changed text turns over once, one character after
 * the next, as a flap board does. Each character carries its index, and app.css turns it
 * over after a delay of that index times --stagger-flip.
 */
function value(text, turn) {
  const node = element("span", "val");
  if (!turn) {
    node.textContent = text;
    return node;
  }
  node.classList.add("turn");
  node.setAttribute("aria-label", text);
  const characters = [...text];
  characters.forEach((character, index) => {
    const flap = element("span", "flap", character);
    flap.setAttribute("aria-hidden", "true");
    flap.style.setProperty("--i", String(index));
    node.append(flap);
  });
  // A character that turned over stays a box of its own, and the browser renders it with
  // other antialiasing than the plain text beside it. When the last character lands, the
  // cell becomes plain text again. Under reduced motion no animation runs, so no
  // animationend event arrives and the cell keeps the characters, which match at rest.
  if (node.lastChild) {
    node.lastChild.addEventListener("animationend", () => {
      node.classList.remove("turn");
      node.removeAttribute("aria-label");
      node.textContent = text;
    });
  }
  return node;
}

/** drawVerdict draws the one line that answers the glance. */
function drawVerdict(section, rows, model, status) {
  const verdict = verdictOf(rows);
  const line = element("div", "verdict");
  line.setAttribute("role", "status");

  const statement = element("p", "verdict-text");
  statement.append(element("span", `dot ${verdict.tone}`));
  statement.append(element("span", undefined, verdict.sentence));
  for (const id of verdict.faults) {
    statement.append(element("span", "mono verdict-fault", id));
  }
  line.append(statement);

  // Each count is one item, so a narrow line wraps between two counts and never inside
  // one. The access mode and the time of the last tick show on a narrow screen only,
  // where the title block of the rail is hidden; the tick tells a fresh page from a
  // stale one.
  const reconciler = reconcilerState(status);
  const counts = element("p", "verdict-counts mono");
  const items = [
    plural(model.counts.tailnets, "tailnet"),
    plural(model.counts.peers, "peer"),
    plural(model.counts.paths, "allowed path"),
    reconciler.word,
  ];
  for (const item of items) {
    counts.append(element("span", "count", item));
  }
  const access = status.access && status.access.mode ? `access ${status.access.mode}` : "no access mode yet";
  counts.append(element("span", "count count-mode", access));
  const tick = lastReconcileAt(status.events);
  counts.append(element("span", "count count-tick", tick === null ? "no tick yet" : `tick ${clockTime(tick)}`));
  line.append(counts);
  section.append(line);
}

/**
 * drawBoard draws one row per tailnet in fixed columns. A row is a control: a pointer or
 * the Enter key selects it, and the arrow keys move between rows.
 */
function drawBoard(section, rows, redraw) {
  const changed = changedCells(shown, rows);
  shown = cellMap(rows);

  const table = element("table", "board");
  const caption = element("caption", "sr", "One row per tailnet. A row with a fault sorts first. Select a row to draw the paths of its tailnet in the topology.");
  table.append(caption);

  const head = element("thead");
  const headRow = element("tr");
  for (const column of COLUMNS) {
    const cell = element("th", column.numeric ? "num" : undefined, column.head);
    cell.scope = "col";
    tierClass(cell, column);
    headRow.append(cell);
  }
  head.append(headRow);
  table.append(head);

  const body = element("tbody");
  for (const row of rows) {
    const line = element("tr");
    line.tabIndex = 0;
    line.dataset.tailnet = row.id;
    line.setAttribute("aria-selected", row.id === selected ? "true" : "false");

    for (const column of COLUMNS) {
      const text = cellText(row, column.key);
      const turn = changed.has(`${row.id}/${column.key}`);
      const cell = element(column.key === "id" ? "th" : "td", "mono");
      if (column.key === "id") {
        cell.scope = "row";
      }
      if (column.numeric) {
        cell.classList.add("num");
      }
      tierClass(cell, column);
      const state = row[column.key];
      if (typeof state === "object") {
        cell.classList.add("state");
        cell.append(element("span", `dot ${state.tone}`));
      }
      if (text === "none") {
        cell.classList.add("absent");
      }
      cell.append(value(text, turn));
      line.append(cell);
    }

    line.addEventListener("click", () => choose(row.id, redraw));
    body.append(line);
    // A phone hides the columns of the tablet tier and the desktop tier, so the selected
    // row opens one detail row that states them. A wider screen hides the detail row.
    if (row.id === selected) {
      body.append(detailRow(row));
    }
    line.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        choose(row.id, redraw);
        return;
      }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const step = (node) => (event.key === "ArrowDown" ? node.nextElementSibling : node.previousElementSibling);
        let next = step(line);
        while (next && next.classList.contains("board-detail")) {
          next = step(next);
        }
        if (next) {
          next.focus();
        }
      }
    });
  }
  table.append(body);

  const frame = element("div", "board-frame");
  frame.append(table);
  section.append(frame);

  const chosen = body.querySelector(`tr[data-tailnet="${CSS.escape(selected || "")}"]`);
  if (chosen && focusArea === "board") {
    chosen.focus();
  }
}

/**
 * drawTopology draws the picture and its text equivalent.
 *
 * The serializer returns SVG source, so the function writes it once and then binds the
 * handlers on the groups that it wrote. A selection redraws the source alone, and the node
 * that the operator chose keeps the focus.
 */
function drawTopology(parent, model, redraw) {
  const frame = element("section", "frame");
  frame.setAttribute("aria-labelledby", "topology-heading");

  const head = element("div", "frame-head");
  const heading = element("h2", "frame-title", "Topology");
  heading.id = "topology-heading";
  head.append(heading);
  head.append(element("span", "frame-meta mono", plural(model.counts.paths, "allowed path")));
  frame.append(head);

  // The two serializers escape every value that the daemon reported, and a test asserts
  // that. The console builds no markup from a daemon value anywhere else.
  const figure = element("div", "flow-wrap");
  figure.innerHTML = topologySVGMarkup(model, selected);
  frame.append(figure);

  // A phone shows the path list in place of the picture, which is too small there to
  // read or to press. A wider screen hides the list.
  const list = element("div", "path-wrap");
  list.innerHTML = pathListMarkup(model, selected);
  frame.append(list);

  const text = element("div", "sr");
  text.id = TEXT_EQUIVALENT_ID;
  text.innerHTML = textEquivalentMarkup(model);
  frame.append(text);

  frame.append(
    element("p", "note", "Select a tailnet to draw its paths alone. A path that no rule allows has no line."),
  );
  parent.append(frame);

  for (const button of list.querySelectorAll("button[data-node]")) {
    const id = button.dataset.node;
    button.addEventListener("click", () => choose(id, redraw));
  }

  for (const group of figure.querySelectorAll("g.node")) {
    const id = group.dataset.node;
    group.addEventListener("click", () => choose(id, redraw));
    group.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        choose(id, redraw);
      }
    });
  }

  const chosen = figure.querySelector(`g.node[data-node="${CSS.escape(selected || "")}"]`);
  if (chosen && focusArea === "flow") {
    chosen.focus();
  }
  const pressed = list.querySelector(`button[data-node="${CSS.escape(selected || "")}"]`);
  if (pressed && focusArea === "paths") {
    pressed.focus();
  }
}

/** drawEvents draws the newest events that are not routine reconcile ticks. */
function drawEvents(parent, events) {
  const frame = element("section", "frame");
  frame.setAttribute("aria-labelledby", "events-heading");

  const head = element("div", "frame-head");
  const heading = element("h2", "frame-title", "Events");
  heading.id = "events-heading";
  head.append(heading);
  const all = element("a", "frame-link", "Activity");
  all.href = "#/activity";
  head.append(all);
  frame.append(head);

  const notable = notableEvents(events, NOTABLE_EVENTS);
  if (notable.events.length === 0) {
    const quiet = element("p", "note");
    quiet.append(document.createTextNode("No event other than a routine reconcile tick in the last "));
    quiet.append(element("span", "mono", String(notable.scanned)));
    quiet.append(document.createTextNode(" events. The Activity view lists every tick."));
    frame.append(quiet);
    parent.append(frame);
    return;
  }

  const list = element("div", "events");
  for (const event of notable.events) {
    const row = element("div", "ev ev-stack");
    row.append(element("time", "mono", clockTime(Date.parse(event.Time))));
    row.append(element("span", "ev-kind mono", event.Type));
    const message = event.TailnetID
      ? `${event.TailnetID}: ${event.Message}`
      : event.Message;
    row.append(element("p", undefined, message));
    list.append(row);
  }
  frame.append(list);
  parent.append(frame);
}

/** drawLoading draws one quiet line until the first poll returns. */
function drawLoading(section) {
  const frame = element("div", "frame");
  frame.append(element("p", "note", "The first poll has not returned."));
  section.append(frame);
}

/** drawError names the console address and the socket path that the daemon opens. */
function drawError(section, snapshot) {
  const frame = element("div", "frame");
  for (const sentence of errorSentences(window.location.host, snapshot.error)) {
    frame.append(element("p", "note", sentence));
  }
  section.append(frame);
}

/** drawEmpty states that no tailnet is configured and it shows the add action. */
function drawEmpty(section) {
  const frame = element("div", "frame empty");
  // The shell holds the one empty sentence of every view, so the console states it once.
  const view = VIEWS.find((entry) => entry.id === "overview");
  frame.append(element("p", undefined, view.empty));
  const row = element("div", "row");
  const add = element("a", "btn primary", "Add tailnet");
  add.href = "#/namespaces";
  row.append(add);
  frame.append(row);
  section.append(frame);
}

/**
 * draw draws the overview from one poll snapshot.
 *
 * snapshot.status holds the merged body of the poll: the fields of GET /api/status, the
 * field access_model from GET /api/access, the field events from GET /api/events, and the
 * field policy from GET /api/policy. The shell draws the stale marker and the time of the
 * last success in the poll banner, so this view draws the last known state and adds no
 * second marker.
 */
function draw(section, snapshot) {
  const active = document.activeElement;
  focusArea = null;
  if (active && section.contains(active)) {
    focusArea = active.closest("tbody") ? "board" : active.closest(".flow-wrap") ? "flow" : active.closest(".path-wrap") ? "paths" : null;
  }
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
  const redraw = () => draw(section, snapshot);

  if (model.counts.tailnets === 0) {
    drawEmpty(section);
    return;
  }

  if (selected !== null && !model.nodes.some((node) => node.id === selected)) {
    selected = null;
  }

  const rows = buildBoard(status, model);
  drawVerdict(section, rows, model, status);
  drawBoard(section, rows, redraw);

  const lower = element("div", "lower");
  section.append(lower);
  drawTopology(lower, model, redraw);
  drawEvents(lower, events);
}

registerView("overview", draw);
