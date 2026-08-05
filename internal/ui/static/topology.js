// The overview topology: the model, the layout, and the serializer.
//
// Every function here is pure. It takes the poll payload and it returns a value. It
// touches no document, therefore internal/ui/jstest reads its output under Node and
// asserts the drawn result exactly. overview.js holds every line that needs a document.
//
// Epic 7 draws the same picture in the access view, so the builder takes the paths as an
// argument and it holds no knowledge of which view calls it.
//
// The drawing rules come from CLAUDE.md and from .claude/rules/console-brand.md:
// an allowed path is a dotted curve, a denied path has no line, and the topology carries
// no arrow, no icon on a node, and no label on a curve.

/** The socket path that the daemon opens. It matches api.DefaultSocketPath. */
export const DEFAULT_SOCKET_PATH = "/var/lib/hydrascale/api.sock";

/** The identifier of the element that holds the text equivalent of the topology. */
export const TEXT_EQUIVALENT_ID = "topology-text";

// The layout. The picture is a fixed grid, because a force-directed layout moves a node
// between two polls and the operator loses the node that was under the pointer.
export const WIDTH = 660;
export const NODE_W = 200;
export const NODE_H = 48;
export const ROW_GAP = 70;
export const LEFT_X = 12;
export const RIGHT_X = 448;
export const TOP = 34;

/** The distance between the internet node and the host node of the right column. */
const RIGHT_GAP = 138;

/** The horizontal distance that a curve between two nodes of one column bulges. */
const BULGE = 60;

/** The three states that internal/reach reports, as a dot tone and a lowercase word. */
const REACHABILITY = {
  reachable: { tone: "ok", word: "reachable" },
  unreachable: { tone: "crit", word: "unreachable" },
  not_probed: { tone: "warn", word: "not probed" },
};

/** The count words that a sentence of the text equivalent opens with. */
const COUNT_WORDS = [
  "No",
  "One",
  "Two",
  "Three",
  "Four",
  "Five",
  "Six",
  "Seven",
  "Eight",
  "Nine",
  "Ten",
];

/**
 * reachabilityOf returns the reachability of one namespace as a dot tone and a lowercase
 * word.
 *
 * actual is one entry of the field `actual` of GET /api/status. The daemon measures the
 * reachability with one packet and reports the result in `measured_reachability`.
 *
 * reachabilityOf reports `not probed` for a namespace that carries no measurement,
 * because the console shows no invented state. It reports `unknown` for a state that this
 * console does not know, which happens when a newer daemon serves an older console.
 */
export function reachabilityOf(actual) {
  const measured = actual && actual.measured_reachability;
  if (!measured || !measured.state) {
    return { state: "not_probed", tone: "warn", word: "not probed", detail: "" };
  }
  const known = REACHABILITY[measured.state];
  const detail = measured.detail || "";
  if (!known) {
    return { state: measured.state, tone: "warn", word: "unknown", detail };
  }
  return { state: measured.state, tone: known.tone, word: known.word, detail };
}

/**
 * reconcilerState returns the state of the reconciler as a dot tone and a lowercase word.
 *
 * status is the body of GET /api/status. A tailnet in the error state makes the whole
 * state `error`. A declared tailnet whose namespace is absent, or whose daemon is not
 * healthy, makes the state `reconciling`. Every other case is `converged`.
 */
export function reconcilerState(status) {
  if (!status || !status.desired) {
    return { tone: "warn", word: "no state yet" };
  }
  const errors = status.error_states || {};
  const actual = status.actual || {};
  for (const id of Object.keys(status.desired)) {
    if (errors[id]) {
      return { tone: "crit", word: "error" };
    }
  }
  for (const id of Object.keys(status.desired)) {
    const state = actual[id];
    if (!state || !state.NsExists || !state.DaemonHealthy) {
      return { tone: "warn", word: "reconciling" };
    }
  }
  return { tone: "ok", word: "converged" };
}

/**
 * lastReconcileAt returns the time of the newest `reconcile_complete` event, in
 * milliseconds, and it returns null when the event list holds none.
 *
 * events is the field `events` of GET /api/events. The reconciler records one
 * `reconcile_complete` event at the end of every tick, so the newest one is the last tick.
 */
export function lastReconcileAt(events) {
  let newest = null;
  for (const event of events || []) {
    if (!event || event.Type !== "reconcile_complete") {
      continue;
    }
    const at = Date.parse(event.Time);
    if (!Number.isNaN(at) && (newest === null || at > newest)) {
      newest = at;
    }
  }
  return newest;
}

/**
 * sinceWords states the time between at and now as the operator reads it.
 * at is a time in milliseconds, and it is null when the daemon reports no tick.
 */
export function sinceWords(now, at) {
  if (at === null || at === undefined) {
    return "no tick yet";
  }
  const seconds = Math.max(0, Math.floor((now - at) / 1000));
  if (seconds < 60) {
    return `${seconds}s ago`;
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m ago`;
  }
  return `${Math.floor(seconds / 3600)}h ago`;
}

/** newestEvents returns the count newest events of the log, newest first. */
export function newestEvents(events, count) {
  return (events || []).slice(-count).reverse();
}

/**
 * errorSentences states what the operator reads when the daemon answers no poll.
 * host is the address that the browser reached, and reason is the message that the poll
 * layer holds.
 */
export function errorSentences(host, reason) {
  return [
    `The daemon does not answer on ${host}.`,
    `The daemon serves the console on that address and it serves the same routes on the socket ${DEFAULT_SOCKET_PATH}.`,
    `Start the daemon, or read its log. The last attempt reported: ${reason}.`,
  ];
}

/** countWord states a small count as a word, and a large count as digits. */
function countWord(n) {
  return n < COUNT_WORDS.length ? COUNT_WORDS[n] : String(n);
}

/** plural states a count and its noun, with the noun in the right number. */
function plural(n, noun) {
  return n === 1 ? `${n} ${noun}` : `${n} ${noun}s`;
}

/**
 * buildTopology returns the whole drawn model of the overview topology.
 *
 * status is the body of GET /api/status and access is the body of GET /api/access. The
 * rule set of the access body holds allow rules alone, therefore every rule becomes one
 * curve and a pair of nodes with no rule between them gets no curve at all.
 *
 * buildTopology is pure: the same two bodies always produce the same model.
 */
export function buildTopology(status, access) {
  const reported = (access && access.nodes) || [];
  const tailnets = reported.filter((node) => node.kind === "tailnet");

  // The host and the internet are two constants of the local rule model, so the topology
  // draws both even when the access body names neither.
  const right = [
    reported.find((node) => node.kind === "internet") || { id: "internet", kind: "internet" },
    reported.find((node) => node.kind === "host") || { id: "host", kind: "host" },
  ];

  const leftSpan = tailnets.length > 0 ? (tailnets.length - 1) * ROW_GAP + NODE_H : 0;
  const rightSpan = RIGHT_GAP + NODE_H;
  const span = Math.max(leftSpan, rightSpan);
  const leftTop = TOP + Math.round((span - leftSpan) / 2);
  const rightTop = TOP + Math.round((span - rightSpan) / 2);

  const actual = (status && status.actual) || {};
  const nodes = [];
  tailnets.forEach((node, index) => {
    const reach = reachabilityOf(actual[node.id]);
    nodes.push({
      id: node.id,
      kind: "tailnet",
      column: "left",
      x: LEFT_X,
      y: leftTop + index * ROW_GAP,
      w: NODE_W,
      h: NODE_H,
      peers: node.peers || 0,
      veth: node.veth || "",
      tone: reach.tone,
      word: reach.word,
      detail: reach.detail,
    });
  });
  right.forEach((node, index) => {
    nodes.push({
      id: node.id,
      kind: node.kind,
      column: "right",
      x: RIGHT_X,
      y: rightTop + index * RIGHT_GAP,
      w: NODE_W,
      h: NODE_H,
      peers: 0,
      veth: "",
      tone: "",
      word: "",
      detail: "",
    });
  });

  const byID = new Map(nodes.map((node) => [node.id, node]));
  const paths = [];
  for (const rule of (access && access.rules) || []) {
    const from = byID.get(rule.from);
    const to = byID.get(rule.to);
    if (!from || !to) {
      continue;
    }
    paths.push({
      id: `${rule.from}->${rule.to}`,
      from: rule.from,
      to: rule.to,
      ports: rule.ports || [],
      d: curve(from, to),
    });
  }

  // A node states how many paths it owns, because the count is the one number that the
  // picture cannot show. It is a value on the node and never a label on a curve.
  for (const node of nodes) {
    const owned = paths.filter((path) => path.from === node.id || path.to === node.id).length;
    node.paths = owned;
    node.line = node.kind === "tailnet"
      ? `${node.word} · ${plural(node.peers, "peer")}`
      : plural(owned, "path");
  }

  const sentences = paths.map((path) =>
    path.ports.length > 0
      ? `${path.from} reaches ${path.to} on ${path.ports.join(", ")}.`
      : `${path.from} reaches ${path.to}.`,
  );

  const between = paths.some(
    (path) => byID.get(path.from).kind === "tailnet" && byID.get(path.to).kind === "tailnet",
  );

  return {
    width: WIDTH,
    height: TOP + span + TOP,
    nodes,
    paths,
    counts: {
      tailnets: tailnets.length,
      peers: tailnets.reduce((sum, node) => sum + (node.peers || 0), 0),
      paths: paths.length,
    },
    sentences,
    summary: `${countWord(paths.length)} allowed ${paths.length > 1 ? "paths" : "path"}.`,
    absence: tailnets.length > 1 && !between ? "No tailnet reaches another tailnet." : "",
  };
}

/**
 * curve returns the path data of one allowed path between two nodes.
 *
 * A path between the two columns leaves the facing edge of each node. A path between two
 * nodes of one column leaves the outer edge of both and bulges away from the picture, so
 * that it crosses no node.
 */
function curve(from, to) {
  const sy = from.y + from.h / 2;
  const ty = to.y + to.h / 2;

  if (from.column === to.column) {
    const outward = from.column === "left" ? from.x - BULGE : from.x + from.w + BULGE;
    const edge = from.column === "left" ? from.x : from.x + from.w;
    return `M${edge},${sy} C${outward},${sy} ${outward},${ty} ${edge},${ty}`;
  }

  const sx = from.column === "left" ? from.x + from.w : from.x;
  const tx = to.column === "left" ? to.x + to.w : to.x;
  const mid = Math.round((sx + tx) / 2);
  return `M${sx},${sy} C${mid},${sy} ${mid},${ty} ${tx},${ty}`;
}

/** esc states a value that the daemon reported as text that an XML parser accepts. */
function esc(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * topologySVGMarkup returns the whole picture as SVG source.
 *
 * model comes from buildTopology and selected holds the identifier of the selected node,
 * or null. When the operator selects a node, the paths of that node take the accent and
 * every other path goes quiet. See FR-console-23.
 *
 * The element carries no arrow, no icon on a node, and no text on a curve. Every text
 * element sits inside a node group. See FR-console-22.
 */
export function topologySVGMarkup(model, selected) {
  const parts = [];
  parts.push(
    `<svg class="flow" viewBox="0 0 ${model.width} ${model.height}" role="img"` +
      ` aria-label="The topology of every tailnet, the host, and the internet."` +
      ` aria-describedby="${TEXT_EQUIVALENT_ID}">`,
  );

  for (const path of model.paths) {
    let className = "edge";
    if (selected !== null && selected !== undefined) {
      className = path.from === selected || path.to === selected ? "edge sel" : "edge muted";
    }
    parts.push(`<path class="${className}" d="${path.d}"></path>`);
  }

  for (const node of model.nodes) {
    const pressed = node.id === selected ? "true" : "false";
    const label = node.kind === "tailnet"
      ? `${node.id}, ${node.word}, ${plural(node.peers, "peer")}, ${plural(node.paths, "allowed path")}.`
      : `${node.id}, ${plural(node.paths, "allowed path")}.`;
    parts.push(
      `<g class="node" data-node="${esc(node.id)}" tabindex="0" role="button"` +
        ` aria-pressed="${pressed}" aria-label="${esc(label)}">`,
    );
    parts.push(
      `<rect x="${node.x}" y="${node.y}" width="${node.w}" height="${node.h}" rx="12"></rect>`,
    );
    parts.push(`<text class="n" x="${node.x + 18}" y="${node.y + 21}">${esc(node.id)}</text>`);
    if (node.kind === "tailnet") {
      parts.push(
        `<circle class="dot ${node.tone}" cx="${node.x + 22}" cy="${node.y + 34}" r="4"></circle>`,
      );
      parts.push(`<text class="s" x="${node.x + 32}" y="${node.y + 38}">${esc(node.line)}</text>`);
    } else {
      parts.push(`<text class="s" x="${node.x + 18}" y="${node.y + 38}">${esc(node.line)}</text>`);
    }
    parts.push("</g>");
  }

  parts.push("</svg>");
  return parts.join("");
}

/**
 * textEquivalentMarkup returns the text that a screen reader reads in place of the curves.
 *
 * A dotted curve carries no meaning to a screen reader, so the list states every allowed
 * path as one sentence, and it states the absence that the picture shows by drawing
 * nothing. See FR-console-24.
 */
export function textEquivalentMarkup(model) {
  const parts = [`<p>${esc(model.summary)}</p>`];
  if (model.sentences.length > 0) {
    parts.push("<ul>");
    for (const sentence of model.sentences) {
      parts.push(`<li>${esc(sentence)}</li>`);
    }
    parts.push("</ul>");
  }
  if (model.absence !== "") {
    parts.push(`<p>${esc(model.absence)}</p>`);
  }
  return parts.join("");
}
