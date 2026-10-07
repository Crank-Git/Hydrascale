// The board of the overview: one row per tailnet in fixed columns, and the verdict above
// it.
//
// Every function here is pure. It takes the poll payload and it returns a value, and it
// touches no document, therefore internal/ui/jstest reads its output under Node.
// overview.js holds every line that needs a document.
//
// The board answers one question first: is every tailnet healthy and reachable, and if
// not, which one is not. A row with a fault therefore sorts above every row without one.

import { reachabilityOf } from "./topology.js";

/**
 * The columns of the board, in order. `key` names the field of a row, `head` is the
 * column head, and `numeric` aligns the value to the right edge of the cell. `tier`
 * states the narrowest screen that keeps the column: "phone" keeps it everywhere,
 * "tablet" keeps it from 601 pixels, and "desktop" keeps it from 901 pixels. The tailnet
 * column is the row header.
 */
export const COLUMNS = [
  { key: "id", head: "Tailnet", tier: "phone" },
  { key: "state", head: "State", tier: "phone" },
  { key: "reach", head: "Reachability", tier: "phone" },
  { key: "target", head: "Probe", tier: "desktop" },
  { key: "peers", head: "Peers", numeric: true, tier: "phone" },
  { key: "paths", head: "Paths", numeric: true, tier: "tablet" },
  { key: "exit", head: "Exit node", tier: "desktop" },
  { key: "host", head: "Host access", tier: "desktop" },
  { key: "policy", head: "Policy", tier: "tablet" },
];

/**
 * byFaultThenID orders two rows: a row with a fault first, then by identifier. The
 * namespace view orders by identifier, so a healthy host lists its tailnets in the same
 * order in both views.
 */
export function byFaultThenID(a, b) {
  return b.rank - a.rank || a.id.localeCompare(b.id);
}

/** The marker of a value that the daemon does not report. */
const ABSENT = "none";

/** The rank of a tone. A row takes the rank of its worst cell. */
const RANK = { crit: 2, warn: 1 };

/**
 * stateOf returns the reconciler state of one tailnet as a dot tone and a lowercase word.
 * The words match the namespace view, so one state reads the same in both views.
 */
export function stateOf(status, id) {
  if (status.error_states && status.error_states[id]) {
    return { tone: "crit", word: "error" };
  }
  if (status.paused_states && status.paused_states[id]) {
    return { tone: "", word: "paused" };
  }
  const actual = status.actual && status.actual[id];
  if (!actual || !actual.NsExists) {
    return { tone: "warn", word: "no namespace" };
  }
  if (!actual.DaemonHealthy) {
    return { tone: "crit", word: "not healthy" };
  }
  return { tone: "ok", word: "healthy" };
}

/**
 * policyOf returns the upstream policy credential state of one tailnet.
 *
 * A credential is optional: a tailnet without one runs and reaches its peers, and only the
 * policy view needs it. An absent credential is therefore a quiet state with no tone, so
 * that a red dot on the board always means a fault. A credential that the control server
 * rejected is a fault.
 */
export function policyOf(status, id) {
  const entries = (status && status.policy) || [];
  const entry = entries.find((tailnet) => tailnet.id === id);
  if (!entry) {
    return { tone: "", word: "not read yet" };
  }
  if (entry.credential_state === "usable") {
    return { tone: "ok", word: "usable" };
  }
  if (entry.credential_state === "rejected") {
    return { tone: "crit", word: "credential rejected" };
  }
  return { tone: "", word: "no credential" };
}

/** hostAccessWord states the host access of a tailnet. A null value follows the file. */
function hostAccessWord(desired) {
  if (!desired || desired.HostAccess === null || desired.HostAccess === undefined) {
    return "default";
  }
  return desired.HostAccess ? "on" : "off";
}

/**
 * buildBoard returns one row per tailnet of the topology model, faults first, then by
 * identifier.
 *
 * status is the merged body of the poll and model comes from buildTopology, which holds
 * the peer count and the path count of every tailnet. A row moves only when its rank
 * changes.
 */
export function buildBoard(status, model) {
  const desired = (status && status.desired) || {};
  const actual = (status && status.actual) || {};
  const rows = model.nodes
    .filter((node) => node.kind === "tailnet")
    .map((node, order) => {
      const state = stateOf(status, node.id);
      const reach = reachabilityOf(actual[node.id]);
      const measured = actual[node.id] && actual[node.id].measured_reachability;
      const policy = policyOf(status, node.id);
      const rank = Math.max(RANK[state.tone] || 0, RANK[reach.tone] || 0, RANK[policy.tone] || 0);
      return {
        id: node.id,
        order,
        rank,
        state,
        reach: { tone: reach.tone, word: reach.word },
        target: (measured && measured.target) || ABSENT,
        peers: String(node.peers),
        paths: String(node.paths),
        exit: (desired[node.id] && desired[node.id].ExitNode) || ABSENT,
        host: hostAccessWord(desired[node.id]),
        policy,
      };
    });
  rows.sort(byFaultThenID);
  return rows;
}

/** cellText returns the text of one cell, which the turnover compares between polls. */
export function cellText(row, key) {
  const value = row[key];
  return typeof value === "object" ? value.word : value;
}

/**
 * verdictOf returns the one line that answers the glance: a dot tone, the sentence, and
 * the faulted tailnets. The counts of the line are the sums of the board columns.
 */
export function verdictOf(rows) {
  const faults = rows.filter((row) => row.rank > 0);
  const total = rows.length;
  if (faults.length === 0) {
    const sentence = total === 1
      ? "1 of 1 tailnet is healthy and reachable."
      : `${total} of ${total} tailnets are healthy and reachable.`;
    return { tone: "ok", sentence, faults: [] };
  }
  const tone = faults.some((row) => row.rank === RANK.crit) ? "crit" : "warn";
  const sentence = faults.length === 1
    ? `1 of ${total} tailnets has a fault:`
    : `${faults.length} of ${total} tailnets have a fault:`;
  return { tone, sentence, faults: faults.map((row) => row.id) };
}

/**
 * changedCells returns the cell keys whose text differs from the previous draw.
 *
 * previous maps a key of the form `<tailnet>/<column>` to the text that the last draw
 * showed. A key that the previous draw did not hold is not a change, so the first draw
 * and a new row turn over nothing. See the motion rule of .claude/rules/console-brand.md.
 */
export function changedCells(previous, rows) {
  const changed = new Set();
  for (const row of rows) {
    for (const column of COLUMNS) {
      const key = `${row.id}/${column.key}`;
      const text = cellText(row, column.key);
      if (previous.has(key) && previous.get(key) !== text) {
        changed.add(key);
      }
    }
  }
  return changed;
}

/** cellMap returns the text of every cell, keyed as changedCells reads it. */
export function cellMap(rows) {
  const cells = new Map();
  for (const row of rows) {
    for (const column of COLUMNS) {
      cells.set(`${row.id}/${column.key}`, cellText(row, column.key));
    }
  }
  return cells;
}

/** ROUTINE names the event types that every reconcile tick records. */
export const ROUTINE = new Set(["reconcile_start", "reconcile_apply", "reconcile_complete", "action_ok"]);

/**
 * notableEvents returns the count newest events that are not routine, newest first, and
 * the count of events that the log holds. A tick records four routine events, and an
 * incident hides between them when the overview lists them all.
 */
export function notableEvents(events, count) {
  const all = events || [];
  const notable = all.filter((event) => event && !ROUTINE.has(event.Type));
  return { events: notable.slice(-count).reverse(), scanned: all.length };
}
