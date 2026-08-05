// The access view: the header, the staged state model, the mode control, the reachability
// matrix, and the flow overview.
//
// The view draws no rule row. Issue #151 adds it, and it writes into the staged state
// model that this file holds.
//
// The flow overview reuses the renderer of the overview topology in topology.js. It reads
// the staged rule set, therefore an edit reaches the picture before the daemon holds it.
//
// The model holds three things: the rule set that the daemon reported, the staged rule
// set, and the difference between them. The staged count is the size of the difference.
// A staged edit changes the model alone. Only the apply of issue #152 sends a rule set,
// which FR-editor-23 states.
//
// The mode control is the one mutating request of this view. It sends the rule set of the
// daemon with the new mode, therefore a staged rule reaches no host until the operator
// applies it.
//
// Every function above the drawing section is pure, or it takes its transport as an
// argument, so internal/ui/jstest asserts it under Node with no browser and no network.

import { ACCESS_ROUTE, registerView, requestJSON } from "./app.js";
import { buildTopology, textEquivalentMarkup, topologySVGMarkup } from "./topology.js";

/** The identifier of the element that holds the text equivalent of the flow overview.
 *  It differs from the identifier of the overview topology, because the console keeps the
 *  markup of a view that it does not show. */
export const FLOW_TEXT_ID = "access-flow-text";

/** The mode in which the daemon drops the packet that no rule allows. */
export const MODE_ENFORCE = "enforce";

/** The mode in which the daemon logs the packet that no rule allows and drops nothing. */
export const MODE_OBSERVE = "observe";

/** The command that shows what the daemon stops in enforce mode. FR-editor-32. */
export const OBSERVE_LOG_COMMAND = "journalctl -u hydrascale | grep hydrascale-would-deny";

/** ruleKey names one path. A path holds at most one rule, therefore the pair is the key. */
function ruleKey(rule) {
  // The separator is the space character, which no endpoint name holds: a tailnet
  // identifier matches the pattern of the daemon, and the two other names are host and
  // internet.
  return `${rule.from} ${rule.to}`;
}

/** normalizeRule returns one rule with a port list, because the daemon reports none for
 *  a rule that allows every port. */
function normalizeRule(rule) {
  return { from: rule.from, to: rule.to, ports: (rule.ports || []).slice() };
}

/** samePorts reports whether two rules hold the same ports in the same order. */
function samePorts(one, other) {
  const a = one.ports || [];
  const b = other.ports || [];
  return a.length === b.length && a.every((port, at) => port === b[at]);
}

/**
 * createAccessState returns the staged state model of the access view.
 *
 * options.request sends one request. It takes the route, the method, and the body, and it
 * rejects with the message that the daemon stated. A test replaces it.
 *
 * The model opens no request of its own. The poll layer gives it the answer of
 * GET /api/access through setBase.
 */
export function createAccessState(options = {}) {
  const request = options.request || ((route, method, body) => requestJSON(route, method, body));

  let base = { mode: MODE_ENFORCE, rules: [], nodes: [] };
  let staged = [];

  function difference() {
    const baseByKey = new Map(base.rules.map((rule) => [ruleKey(rule), rule]));
    const stagedByKey = new Map(staged.map((rule) => [ruleKey(rule), rule]));

    const added = [];
    const changed = [];
    for (const [key, rule] of stagedByKey) {
      const before = baseByKey.get(key);
      if (!before) {
        added.push(rule);
      } else if (!samePorts(before, rule)) {
        changed.push(rule);
      }
    }
    const removed = base.rules.filter((rule) => !stagedByKey.has(ruleKey(rule)));
    return { added, removed, changed };
  }

  function count() {
    const { added, removed, changed } = difference();
    return added.length + removed.length + changed.length;
  }

  return {
    /** base returns the mode, the rule set, and the node list that the daemon reported. */
    base() {
      return { mode: base.mode, rules: base.rules.map(normalizeRule), nodes: base.nodes.slice() };
    },

    /** rules returns the staged rule set, which the editor of the operator writes. */
    rules() {
      return staged.map(normalizeRule);
    },

    /**
     * setBase takes one answer of GET /api/access.
     *
     * setBase replaces the staged rule set when the operator staged no edit, so that the
     * view shows what the daemon holds now. It keeps every staged edit otherwise, because
     * the console applies no edit and removes no edit on its own.
     */
    setBase(body) {
      const wasClean = count() === 0;
      base = {
        mode: (body && body.mode) || MODE_ENFORCE,
        rules: ((body && body.rules) || []).map(normalizeRule),
        nodes: (body && body.nodes) || [],
      };
      if (wasClean) {
        staged = base.rules.map(normalizeRule);
      }
    },

    /** setRules replaces the staged rule set. It sends nothing. FR-editor-23. */
    setRules(rules) {
      staged = rules.map(normalizeRule);
    },

    /** discard returns the staged rule set to the rule set of the daemon. FR-editor-27. */
    discard() {
      staged = base.rules.map(normalizeRule);
    },

    /** difference returns the added rules, the removed rules, and the changed rules. */
    difference,

    /** count returns the number of staged edits. FR-editor-24. */
    count,

    /** send runs one request through the transport that the model holds. */
    send(route, method, body) {
      return request(route, method, body);
    },
  };
}

/**
 * headerModel returns the header of the view: the mode, the staged count, and the
 * controls.
 *
 * mode is the mode that the daemon reported and count is the number of staged edits.
 * The accent belongs to one thing per view, and this view gives it to the apply action.
 * Issue #152 gives the apply control and the discard control their behaviour.
 */
export function headerModel(mode, count) {
  const staged = count === 1 ? "1 staged" : `${count} staged`;
  return {
    mode: {
      word: mode === MODE_OBSERVE ? MODE_OBSERVE : MODE_ENFORCE,
      tone: mode === MODE_OBSERVE ? "warn" : "ok",
    },
    staged,
    controls: [
      { id: "mode", label: "Change the mode", kind: "button", accent: false, disabled: false },
      { id: "discard", label: "Discard", kind: "button", accent: false, disabled: count === 0 },
      { id: "apply", label: "Apply", kind: "button", accent: true, disabled: count === 0 },
    ],
  };
}

/**
 * observeStatement returns the statement of observe mode, and null in enforce mode.
 *
 * FR-editor-32 states that the view names the log command that shows what the daemon
 * stops in enforce mode.
 */
export function observeStatement(mode) {
  if (mode !== MODE_OBSERVE) {
    return null;
  }
  return {
    sentence: "The daemon denies nothing in this mode. It logs the packet that no rule allows.",
    lead: "Read what the daemon stops in enforce mode with:",
    command: OBSERVE_LOG_COMMAND,
  };
}

/**
 * modeChange returns the dialog of the mode control. FR-editor-33.
 *
 * mode is the mode that the daemon holds now. The dialog states what the change does, and
 * the change to enforce carries the warning first, because a connection stops at once.
 */
export function modeChange(mode) {
  if (mode === MODE_OBSERVE) {
    return {
      next: MODE_ENFORCE,
      heading: "Change the mode to enforce",
      sentences: [
        "Warning: a connection that no rule allows stops at once.",
        "In enforce mode the daemon drops the packet that no rule allows.",
        "The rule set does not change, and the console sends no staged edit.",
      ],
      confirmLabel: "Change to enforce",
    };
  }
  return {
    next: MODE_OBSERVE,
    heading: "Change the mode to observe",
    sentences: [
      "In observe mode the daemon drops no packet.",
      "The daemon logs the packet that no rule allows, and it stops nothing.",
      "The rule set does not change, and the console sends no staged edit.",
    ],
    confirmLabel: "Change to observe",
  };
}

/**
 * sendModeChange sends the new mode with PUT /api/access.
 *
 * state is the model, and next is the mode to apply. sendModeChange sends the rule set of
 * the daemon rather than the staged rule set, therefore a staged edit reaches no host
 * before the operator applies it. See FR-editor-23.
 * sendModeChange rejects with the message that the daemon stated.
 */
export function sendModeChange(state, next) {
  return state.send(ACCESS_ROUTE, "PUT", { mode: next, rules: state.base().rules });
}

/**
 * emptyStatement returns the empty state of the view, and null for a rule set that holds
 * one rule.
 *
 * A host that declares no tailnet states the tailnet as the first step. A host that holds
 * no rule states that nothing reaches anything, and it names the matrix.
 */
export function emptyStatement(body) {
  const nodes = (body && body.nodes) || [];
  const rules = (body && body.rules) || [];
  if (nodes.filter((node) => node.kind === "tailnet").length === 0) {
    return {
      label: "Empty",
      sentence: "No tailnet is configured. Add a tailnet, and this view shows the paths that the host allows.",
    };
  }
  if (rules.length === 0) {
    return {
      label: "Empty",
      sentence: "No rule exists, therefore nothing reaches anything. Click a square in the matrix to allow a path.",
    };
  }
  return null;
}

/** The side of one square in pixels. FR-editor-15 states the corner radius; app.css holds
 *  it as the radius token of the brand. */
export const SQUARE_SIDE = 34;

/** The side of one square in pixels on a dense host. */
export const SQUARE_SIDE_DENSE = 28;

/** The number of tailnets from which the matrix draws the smaller square. */
const DENSE_TAILNET_COUNT = 12;

/** square returns one square of the matrix. allowed holds the key of every allowed path. */
function square(from, to, allowed) {
  // The daemon rejects a rule where from equals to, therefore the diagonal takes no rule
  // and it accepts no click. See FR-access-11 and FR-editor-10.
  const inert = from === to;
  const on = !inert && allowed.has(`${from} ${to}`);
  let label = `${from} to ${to}, no rule`;
  if (inert) {
    label = `${from} to ${to}, not applicable`;
  } else if (on) {
    label = `${from} to ${to}, allowed`;
  }
  return { from, to, allowed: on, inert, kind: "button", disabled: inert, label };
}

/**
 * matrixModel returns the grid of squares of the reachability matrix.
 *
 * base is the answer of the daemon, which carries the node list. staged is the staged rule
 * set, therefore the matrix shows the edit of the operator before the daemon holds it.
 * A source is a tailnet or the host, and a destination is a tailnet, the host, or the
 * internet, which features/05-reachability-model.md states. FR-editor-7.
 *
 * matrixModel carries no port, because the ports live in the rule list. FR-editor-14.
 */
export function matrixModel(base, staged) {
  const nodes = (base && base.nodes) || [];
  const tailnets = nodes.filter((node) => node.kind === "tailnet").map((node) => node.id);
  const holdsHost = nodes.some((node) => node.kind === "host");
  const holdsInternet = nodes.some((node) => node.kind === "internet");

  const sources = holdsHost ? [...tailnets, "host"] : [...tailnets];
  const destinations = [...tailnets];
  if (holdsHost) {
    destinations.push("host");
  }
  if (holdsInternet) {
    destinations.push("internet");
  }

  const allowed = new Set((staged || []).map(ruleKey));
  return {
    side: tailnets.length >= DENSE_TAILNET_COUNT ? SQUARE_SIDE_DENSE : SQUARE_SIDE,
    sources,
    destinations,
    rows: sources.map((source) => ({
      source,
      squares: destinations.map((destination) => square(source, destination, allowed)),
    })),
  };
}

/**
 * toggleSquare returns the staged rule set that one click on a square produces.
 *
 * rules is the staged rule set, and the pair from and to names the path. A click on an
 * empty square adds one rule that allows every port, and a click on a filled square
 * removes every rule of that path. See FR-editor-11 and FR-editor-12.
 * toggleSquare returns the rule set unchanged for the diagonal. FR-editor-10.
 */
export function toggleSquare(rules, from, to) {
  const staged = rules.map(normalizeRule);
  if (from === to) {
    return staged;
  }
  const key = `${from} ${to}`;
  const kept = staged.filter((rule) => ruleKey(rule) !== key);
  if (kept.length < staged.length) {
    return kept;
  }
  return [...staged, { from, to, ports: [] }];
}

/**
 * hoverMarks returns the two labels that the square under the pointer marks.
 *
 * The mark names the row label and the column label, and the view draws no third element.
 * FR-editor-13.
 */
export function hoverMarks(from, to) {
  return { row: from, column: to };
}

// ---------------------------------------------------------------------------
// The flow overview
// ---------------------------------------------------------------------------

/**
 * flowModel returns the drawn model of the flow overview.
 *
 * status is the body of GET /api/status, which carries the reachability of each tailnet.
 * nodes is the node list of GET /api/access and rules is the staged rule set. The rule set
 * holds allow rules alone, therefore a pair of nodes with no rule between them gets no
 * curve at all: the absence of a line is the whole statement. See FR-editor-4.
 *
 * flowModel places the tailnets on the left and the destinations on the right, which
 * FR-editor-1 states. It is pure.
 */
export function flowModel(status, nodes, rules) {
  return buildTopology(status, { nodes: nodes || [], rules: rules || [] });
}

/**
 * flowMarkup returns the picture as SVG source.
 *
 * selected holds the identifier of the source that the operator chose, or null. The paths
 * that start at that source take the accent and every other path goes quiet, which
 * FR-editor-5 states. One source draws at a time.
 *
 * The serializer escapes every value that the daemon reported, so a hostile identifier
 * reaches the page as text.
 */
export function flowMarkup(model, selected) {
  return topologySVGMarkup(model, selected, { bySource: true });
}

/** flowTextMarkup returns the text that a screen reader reads in place of the curves. */
export function flowTextMarkup(model) {
  return textEquivalentMarkup(model);
}

/**
 * flowSelection returns the source that one click leaves selected.
 *
 * current is the source that the picture holds now and id is the node that the operator
 * chose. A second click on the same source returns the picture to the resting state, and a
 * node that the model does not hold selects nothing.
 */
export function flowSelection(model, current, id) {
  if (!model.nodes.some((node) => node.id === id)) {
    return null;
  }
  return current === id ? null : id;
}

/**
 * flowCaption states which source the picture draws.
 *
 * label carries the sans typeface and id carries the mono typeface, because the identifier
 * belongs to the machine. sentence states what the operator reads under the picture.
 */
export function flowCaption(model, selected) {
  if (selected === null || selected === undefined) {
    return {
      label: "every source",
      id: "",
      sentence: "Select a node to draw the paths that start at it. A path that no rule allows has no line.",
    };
  }
  const owned = model.paths.filter((path) => path.from === selected).length;
  return {
    label: "source",
    id: selected,
    sentence: owned === 0 ? "No path starts at this node." : "Every other path is muted.",
  };
}

// ---------------------------------------------------------------------------
// The drawing. Everything below this line needs a document.
// ---------------------------------------------------------------------------

const state = createAccessState();

let dialog = null; // null, or {sending, error}
let redraw = () => {};

/** source holds the source that the picture draws, and null when the operator chose none. */
let source = null;

/** takeFocus is true after a click or a key selected a source. The poll redraws the view
 *  every few seconds, and a redraw that always moves the focus takes it from the operator. */
let takeFocus = false;

/** el builds one element with a class and a text. */
function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

/** chip builds one chip. A machine value takes the mono typeface. */
function chip(text, dotTone) {
  const node = el("span", "chip");
  if (dotTone) {
    node.append(el("span", `dot ${dotTone}`));
  }
  node.append(el("span", "mono", text));
  return node;
}

/** control builds one button of the header from its model. */
function control(model, onClick) {
  const button = el("button", model.accent ? "btn primary" : "btn", model.label);
  button.type = "button";
  button.disabled = model.disabled;
  button.addEventListener("click", onClick);
  return button;
}

/** drawHeader draws the mode, the staged count, and the three controls. */
function drawHeader(base) {
  const header = headerModel(base.mode, state.count());
  const row = el("div", "ac-head");

  const states = el("div", "ac-states");
  states.append(chip(header.mode.word, header.mode.tone));
  states.append(chip(header.staged));
  row.append(states);

  const acts = el("div", "ac-acts");
  for (const model of header.controls) {
    // Issue #152 gives the apply action and the discard action their behaviour. Both stay
    // disabled while the console holds no staged edit, which is every state of this issue.
    const onClick = model.id === "mode" ? openModeDialog : () => {};
    acts.append(control(model, onClick));
  }
  row.append(acts);
  return row;
}

/** drawObserve draws the statement of observe mode and the log command. */
function drawObserve(statement) {
  const card = el("div", "card ac-observe");
  const alert = el("div", "alert");
  alert.append(el("span", "dot warn"));
  const text = el("div");
  text.append(el("p", undefined, statement.sentence));
  text.append(el("p", undefined, statement.lead));
  alert.append(text);
  card.append(alert);
  card.append(el("code", "ns-cmds mono", statement.command));
  return card;
}

/** drawEmpty draws the empty state of the view. */
function drawEmpty(statement) {
  const card = el("div", "card empty");
  card.append(el("span", "label", statement.label));
  card.append(el("p", undefined, statement.sentence));
  return card;
}

/**
 * drawFlow draws the flow overview and its text equivalent.
 *
 * The serializer returns SVG source, so the function writes it once and then binds the
 * handlers on the groups that it wrote. The picture reads the staged rule set, therefore a
 * staged edit draws its curve before the operator applies it.
 */
function drawFlow(status, nodes) {
  const model = flowModel(status, nodes, state.rules());
  if (source !== null && !model.nodes.some((node) => node.id === source)) {
    // The daemon removed the tailnet that the operator had selected.
    source = null;
  }
  const caption = flowCaption(model, source);

  const card = el("section", "card ac-flow");
  card.setAttribute("aria-labelledby", "ac-flow-heading");

  const head = el("div", "card-head");
  const heading = el("span", "label", "Allowed paths");
  heading.id = "ac-flow-heading";
  head.append(heading);
  const mark = el("div", "ac-flow-cap");
  mark.append(el("span", "label", caption.label));
  if (caption.id) {
    mark.append(el("span", "mono", caption.id));
  }
  head.append(mark);
  card.append(head);

  // The serializer escapes every value that the daemon reported, and a test asserts that.
  const figure = el("div", "flow-wrap");
  figure.innerHTML = flowMarkup(model, source);
  const picture = figure.querySelector("svg.flow");
  if (picture) {
    picture.setAttribute("aria-label", "The paths that the staged rule set allows.");
    picture.setAttribute("aria-describedby", FLOW_TEXT_ID);
  }
  card.append(figure);

  const text = el("div", "sr");
  text.id = FLOW_TEXT_ID;
  text.innerHTML = flowTextMarkup(model);
  card.append(text);

  card.append(el("p", "note", caption.sentence));

  for (const group of figure.querySelectorAll("g.node")) {
    const id = group.dataset.node;
    const choose = () => {
      source = flowSelection(model, source, id);
      takeFocus = true;
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

  if (takeFocus && source !== null) {
    const chosen = figure.querySelector(`g.node[data-node="${CSS.escape(source)}"]`);
    if (chosen) {
      chosen.focus();
    }
  }
  takeFocus = false;
  return card;
}

/** stageSquare stages the edit of one square and it draws the view again. */
function stageSquare(from, to) {
  state.setRules(toggleSquare(state.rules(), from, to));
  redraw();
}

/**
 * drawMatrix draws the grid of squares.
 *
 * The square is a button, therefore it reaches focus with the tab key and the browser
 * gives it the space key and the enter key. The pointer and the keyboard both mark the row
 * label and the column label, and the mark changes the class of the two labels alone.
 */
function drawMatrix(model) {
  const card = el("div", "card ac-matrix");
  card.append(el("span", "label", "Reachability"));
  card.append(el("p", "note", "A filled square means that a rule allows the path. The ports live in the rule list."));

  const table = el("table", "ac-mtx");
  table.style.setProperty("--ac-square", `${model.side}px`);

  const headRow = el("tr");
  headRow.append(el("th"));
  const columnLabels = model.destinations.map((destination) => {
    const cell = el("th", "col", destination);
    cell.scope = "col";
    headRow.append(cell);
    return cell;
  });
  const head = el("thead");
  head.append(headRow);
  table.append(head);

  const body = el("tbody");
  for (const row of model.rows) {
    const line = el("tr");
    const rowLabel = el("th", undefined, row.source);
    rowLabel.scope = "row";
    line.append(rowLabel);

    row.squares.forEach((one, column) => {
      const holder = el("td");
      const button = el("button", one.allowed ? "ac-square on" : "ac-square");
      button.type = "button";
      button.setAttribute("aria-label", one.label);
      if (one.inert) {
        button.className = "ac-square inert";
        button.disabled = true;
      } else {
        const marks = hoverMarks(row.source, one.to);
        const mark = (on) => {
          rowLabel.classList.toggle("hot", on && marks.row === row.source);
          columnLabels[column].classList.toggle("hot", on && marks.column === one.to);
        };
        button.addEventListener("mouseenter", () => mark(true));
        button.addEventListener("mouseleave", () => mark(false));
        button.addEventListener("focus", () => mark(true));
        button.addEventListener("blur", () => mark(false));
        button.addEventListener("click", () => stageSquare(one.from, one.to));
      }
      holder.append(button);
      line.append(holder);
    });
    body.append(line);
  }
  table.append(body);

  card.append(table);
  return card;
}

/** openModeDialog opens the dialog that states what the mode change does. */
function openModeDialog() {
  dialog = { sending: false, error: null };
  redraw();
}

/** closeModeDialog closes the dialog and keeps the mode that the daemon holds. */
function closeModeDialog() {
  dialog = null;
  redraw();
}

/** confirmModeChange sends the new mode and closes the dialog on a success. */
function confirmModeChange(next) {
  dialog = { sending: true, error: null };
  redraw();
  sendModeChange(state, next)
    .then((body) => {
      state.setBase(body);
      dialog = null;
      redraw();
    })
    .catch((err) => {
      dialog = { sending: false, error: err && err.message ? err.message : String(err) };
      redraw();
    });
}

/** drawModeDialog draws the dialog of the mode control. FR-editor-33. */
function drawModeDialog(mode) {
  const change = modeChange(mode);
  const box = el("div", "ac-dialog");
  box.setAttribute("role", "dialog");
  box.setAttribute("aria-modal", "true");
  box.setAttribute("aria-label", change.heading);
  box.append(el("h3", undefined, change.heading));
  for (const sentence of change.sentences) {
    box.append(el("p", "note", sentence));
  }
  if (dialog.error) {
    box.append(el("p", "ns-error", "The daemon refused the change."));
    box.append(el("code", "ns-cmds mono", dialog.error));
  }

  const acts = el("div", "ns-acts ns-dialog-acts");
  const cancel = el("button", "btn", "Cancel");
  cancel.type = "button";
  cancel.addEventListener("click", closeModeDialog);
  acts.append(cancel);

  const confirm = el("button", "btn", dialog.sending ? "The daemon applies the change" : change.confirmLabel);
  confirm.type = "button";
  confirm.disabled = dialog.sending;
  confirm.addEventListener("click", () => confirmModeChange(change.next));
  acts.append(confirm);

  box.append(acts);
  return box;
}

/**
 * draw draws the access view from one poll snapshot.
 *
 * snapshot.status.access_model holds the answer of GET /api/access, which the poll layer
 * reads on every tick. The view opens no request of its own and it starts no timer.
 */
function draw(section, snapshot) {
  redraw = () => draw(section, snapshot);
  section.replaceChildren();

  if (snapshot.loading) {
    const card = el("div", "card");
    card.append(el("span", "label", "Loading"));
    card.append(el("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  const body = snapshot.status && snapshot.status.access_model;
  if (body) {
    state.setBase(body);
  }
  const base = state.base();

  section.append(drawHeader(base));

  const observe = observeStatement(base.mode);
  if (observe) {
    section.append(drawObserve(observe));
  }

  // The empty state reads the staged rule set, because the view draws that rule set. A
  // view that reads the rule set of the daemon states that no rule exists while the
  // picture already holds a staged curve.
  const empty = emptyStatement({ nodes: base.nodes, rules: state.rules() });
  if (empty) {
    section.append(drawEmpty(empty));
  } else {
    section.append(drawFlow(snapshot.status, base.nodes));
  }

  // A host that declares no tailnet and no host node produces no row, so the empty
  // statement stands alone and the view draws no grid with no square.
  const matrix = matrixModel(base, state.rules());
  if (matrix.rows.length > 0) {
    section.append(drawMatrix(matrix));
  }

  if (dialog) {
    const scrim = el("div", "ac-scrim");
    scrim.append(drawModeDialog(base.mode));
    section.append(scrim);
  }
}

registerView("access", draw);
