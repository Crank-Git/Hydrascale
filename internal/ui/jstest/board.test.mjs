// The board of the overview: the rows, the verdict, the turnover, and the notable events.
//
// The Go test TestTheConsoleJavaScriptTestsPass starts this file.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  COLUMNS,
  ROUTINE,
  buildBoard,
  cellMap,
  changedCells,
  notableEvents,
  verdictOf,
} from "../static/board.js";
import { buildTopology } from "../static/topology.js";

/** poll builds the merged poll body of two or more tailnets. */
function poll(tailnets) {
  const desired = {};
  const actual = {};
  const nodes = [];
  const policy = [];
  for (const [id, state] of Object.entries(tailnets)) {
    desired[id] = { ID: id, ExitNode: state.exit || "", HostAccess: true };
    actual[id] = {
      ID: id,
      NsName: `ns-${id}`,
      NsExists: state.ns !== false,
      DaemonHealthy: state.healthy !== false,
      measured_reachability: { state: state.reach || "reachable", target: "1.1.1.1" },
    };
    nodes.push({ id, kind: "tailnet", peers: state.peers || 0, veth: "10.200.0.2" });
    policy.push({ id, kind: "tailscale", credential_state: state.credential || "usable" });
  }
  nodes.push({ id: "host", kind: "host" }, { id: "internet", kind: "internet" });
  const status = { desired, actual, error_states: {}, paused_states: {}, policy };
  const access = { mode: "enforce", rules: [], nodes };
  return { status, model: buildTopology(status, access) };
}

test("a row with a fault sorts above every row without one, and equal rows sort by identifier", () => {
  const { status, model } = poll({
    gamma: { peers: 5 },
    beta: { peers: 4, reach: "unreachable" },
    alpha: { peers: 3 },
  });
  const rows = buildBoard(status, model);
  assert.deepEqual(rows.map((row) => row.id), ["beta", "alpha", "gamma"]);
  assert.equal(rows[0].reach.word, "unreachable");
  assert.equal(rows[0].reach.tone, "crit");
});

test("an absent credential is a quiet state and not a fault, and a rejected credential is a fault", () => {
  // A credential is optional: a tailnet without one runs and reaches its peers. A red dot
  // on the board therefore always means a fault.
  const { status, model } = poll({
    alpha: { credential: "absent" },
    beta: { credential: "rejected" },
  });
  const rows = buildBoard(status, model);
  const alpha = rows.find((row) => row.id === "alpha");
  const beta = rows.find((row) => row.id === "beta");
  assert.deepEqual(alpha.policy, { tone: "", word: "no credential" });
  assert.equal(alpha.rank, 0);
  assert.deepEqual(beta.policy, { tone: "crit", word: "credential rejected" });
  assert.equal(beta.rank, 2);
});

test("the state words match the namespace view", () => {
  const { status, model } = poll({
    alpha: {},
    beta: { healthy: false },
    gamma: { ns: false },
  });
  const words = Object.fromEntries(buildBoard(status, model).map((row) => [row.id, row.state.word]));
  assert.deepEqual(words, { alpha: "healthy", beta: "not healthy", gamma: "no namespace" });
});

test("the verdict states every tailnet as healthy when no row holds a fault", () => {
  const { status, model } = poll({ alpha: {}, beta: {} });
  assert.deepEqual(verdictOf(buildBoard(status, model)), {
    tone: "ok",
    sentence: "2 of 2 tailnets are healthy and reachable.",
    faults: [],
  });
});

test("the verdict names each faulted tailnet and takes the worst tone", () => {
  const { status, model } = poll({ alpha: {}, beta: { ns: false }, gamma: { reach: "unreachable" } });
  const verdict = verdictOf(buildBoard(status, model));
  assert.equal(verdict.tone, "crit");
  assert.equal(verdict.sentence, "2 of 3 tailnets have a fault:");
  assert.deepEqual([...verdict.faults].sort(), ["beta", "gamma"]);
});

test("the first draw turns over nothing, and a later draw turns over only the changed cells", () => {
  // The operator ruled on 2026-10-07 that a cell turns over when its value changes, and
  // never on load or on a poll that changed nothing.
  const first = poll({ alpha: { peers: 3 }, beta: { peers: 4 } });
  const rows = buildBoard(first.status, first.model);
  assert.equal(changedCells(new Map(), rows).size, 0);

  const shown = cellMap(rows);
  assert.equal(changedCells(shown, rows).size, 0, "an unchanged poll turns over nothing");

  const second = poll({ alpha: { peers: 3 }, beta: { peers: 9, reach: "unreachable" } });
  const changed = changedCells(shown, buildBoard(second.status, second.model));
  assert.deepEqual([...changed].sort(), ["beta/peers", "beta/reach"]);
});

test("a new row turns over nothing", () => {
  const first = poll({ alpha: {} });
  const shown = cellMap(buildBoard(first.status, first.model));
  const second = poll({ alpha: {}, beta: {} });
  assert.equal(changedCells(shown, buildBoard(second.status, second.model)).size, 0);
});

test("the notable events leave out the routine events of a reconcile tick", () => {
  const events = [
    { Time: "2026-10-07T10:00:00Z", Type: "reconcile_start", Message: "" },
    { Time: "2026-10-07T10:00:01Z", Type: "access.jump_displaced", Message: "the jump rule of INPUT is at position 2" },
    { Time: "2026-10-07T10:00:02Z", Type: "action_ok", TailnetID: "alpha", Message: "sync_routes" },
    { Time: "2026-10-07T10:00:03Z", Type: "reconcile_complete", Message: "applied 4 actions" },
  ];
  const notable = notableEvents(events, 5);
  assert.equal(notable.scanned, 4);
  assert.deepEqual(notable.events.map((event) => event.Type), ["access.jump_displaced"]);
  for (const type of ["reconcile_start", "reconcile_apply", "reconcile_complete", "action_ok"]) {
    assert.ok(ROUTINE.has(type), `${type} is routine`);
  }
});

test("a phone keeps the tailnet, the state, the reachability, and the peers, and a tablet adds the paths and the policy", () => {
  const phone = COLUMNS.filter((column) => column.tier === "phone").map((column) => column.key);
  assert.deepEqual(phone, ["id", "state", "reach", "peers"]);
  const tablet = COLUMNS.filter((column) => column.tier === "tablet").map((column) => column.key);
  assert.deepEqual(tablet, ["paths", "policy"]);
});

test("app.css hides the optional columns on a phone and turns over no cell under reduced motion", async () => {
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /@media \(max-width:900px\)\{[^@]*\.board \.opt\{display:none\}/);
  assert.match(style, /@media \(max-width:600px\)\{[^@]*\.board \.mid\{display:none\}/);
  assert.match(style, /@media \(prefers-reduced-motion: reduce\)\{\s*\.flap\{animation:none\}/);

  const motion = await readFile(new URL("../static/brand/tokens/motion.css", import.meta.url), "utf8");
  assert.match(motion, /prefers-reduced-motion[\s\S]*--dur-flip:0ms/);
});
