// The topology builder is a pure function of the poll payload, so these tests read its
// output exactly and need no browser. internal/ui/static/overview.js holds every line that
// needs a document, and it holds no arithmetic.
//
// The Go test TestTheConsoleJavaScriptTestsPass starts this file.
import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_SOCKET_PATH,
  ROW_GAP,
  buildTopology,
  errorSentences,
  lastReconcileAt,
  newestEvents,
  reachabilityOf,
  reconcilerState,
  sinceWords,
  textEquivalentMarkup,
  topologySVGMarkup,
} from "../static/topology.js";

// statusWith builds one GET /api/status body. The Go type reconciler.TailnetState carries
// no JSON tag on its first four fields, so the keys are the Go field names.
function statusWith(tailnets = {}) {
  const desired = {};
  const actual = {};
  for (const [id, state] of Object.entries(tailnets)) {
    desired[id] = { id };
    actual[id] = {
      ID: id,
      NsName: `ns-${id}`,
      NsExists: true,
      DaemonHealthy: state.healthy !== false,
      measured_reachability: state.reach === undefined ? null : state.reach,
    };
  }
  return {
    server_version: "1.0.0",
    desired,
    actual,
    error_states: {},
    paused_states: {},
    failure_counts: {},
    last_errors: {},
  };
}

// accessWith builds one GET /api/access body. buildAccessResponse appends the host node
// and the internet node after the tailnet nodes.
function accessWith(tailnets, rules) {
  const nodes = tailnets.map((tn) => ({
    id: tn.id,
    kind: "tailnet",
    peers: tn.peers,
    veth: tn.veth,
  }));
  nodes.push({ id: "host", kind: "host" }, { id: "internet", kind: "internet" });
  return { mode: "enforce", rules, nodes };
}

// three is the shape of the mockup: three tailnets, five allowed paths, no tailnet that
// reaches another tailnet.
function three() {
  const status = statusWith({
    jbones: { reach: { state: "reachable", target: "1.1.1.1", checked_at: "2026-08-05T14:02:19Z", detail: "" } },
    homelab: { reach: { state: "unreachable", target: "1.1.1.1", checked_at: "2026-08-05T14:02:19Z", detail: "no answer inside 250ms" } },
    "corp-prod": { reach: { state: "not_probed", checked_at: "2026-08-05T14:02:19Z", detail: "the namespace does not exist" } },
  });
  const access = accessWith(
    [
      { id: "jbones", peers: 6, veth: "10.99.0.2" },
      { id: "homelab", peers: 3, veth: "10.99.0.6" },
      { id: "corp-prod", peers: 5, veth: "10.99.0.10" },
    ],
    [
      { from: "jbones", to: "internet", ports: [] },
      { from: "jbones", to: "host", ports: ["tcp/22"] },
      { from: "homelab", to: "internet", ports: [] },
      { from: "homelab", to: "host", ports: [] },
      { from: "corp-prod", to: "internet", ports: [] },
    ],
  );
  return buildTopology(status, access);
}

test("the topology places every tailnet on the left and the host and the internet on the right", () => {
  const model = three();

  const left = model.nodes.filter((node) => node.column === "left");
  const right = model.nodes.filter((node) => node.column === "right");

  assert.deepEqual(
    left.map((node) => node.id),
    ["jbones", "homelab", "corp-prod"],
  );
  assert.deepEqual(
    right.map((node) => node.id).sort(),
    ["host", "internet"],
  );

  for (const node of left) {
    for (const other of right) {
      assert.ok(
        node.x + node.w <= other.x,
        `${node.id} is not left of ${other.id}`,
      );
    }
  }

  // The left column stacks in the declared order, one row apart.
  assert.equal(left[1].y - left[0].y, ROW_GAP);
  assert.equal(left[2].y - left[1].y, ROW_GAP);
});

test("the topology draws one dotted curve for each allowed path and no line for a denied path", () => {
  const model = three();

  assert.equal(model.paths.length, 5, "the rule set holds five rules");
  assert.deepEqual(
    model.paths.map((path) => path.id),
    [
      "jbones->internet",
      "jbones->host",
      "homelab->internet",
      "homelab->host",
      "corp-prod->internet",
    ],
  );

  // corp-prod reaches the internet and nothing else. The model holds no path to the host
  // and no path to another tailnet, because absence is the denial.
  const denied = model.paths.filter(
    (path) => path.id === "corp-prod->host" || path.from === path.to,
  );
  assert.equal(denied.length, 0);

  const markup = topologySVGMarkup(model, null);
  assert.equal(
    (markup.match(/<path /g) || []).length,
    5,
    "the rendered topology holds one path element per allowed path",
  );
  assert.ok(
    !markup.includes("corp-prod->host"),
    "the rendered topology names a denied path",
  );

  // The dotted curve is the brand: a 2px dash, a 6px gap, a 1.4px stroke. app.css sets
  // those on the class, so the markup carries the class and no inline stroke.
  for (const element of markup.match(/<path [^>]*>/g)) {
    assert.match(element, /class="edge/);
    assert.match(element, / d="M[^"]*C[^"]*"/, "an allowed path is a curve");
    assert.ok(!element.includes("stroke="), "a path states its stroke in app.css");
  }
});

test("the topology draws no arrowhead, no node icon, and no edge label", () => {
  const markup = topologySVGMarkup(three(), "jbones");

  for (const forbidden of [
    "<marker",
    "marker-end",
    "marker-start",
    "marker-mid",
    "<image",
    "<use",
    "<polygon",
    "<textPath",
    "xlink:href",
  ]) {
    assert.ok(!markup.includes(forbidden), `the topology holds ${forbidden}`);
  }

  // Every text element belongs to a node group. A text element outside a group is an
  // edge label, and FR-console-22 forbids one.
  const groups = markup.match(/<g class="node"[\s\S]*?<\/g>/g) || [];
  assert.equal(groups.length, 5, "the topology holds five node groups");
  const inGroups = groups.join("").match(/<text /g) || [];
  const total = markup.match(/<text /g) || [];
  assert.equal(total.length, inGroups.length, "a text element sits outside a node group");
});

test("a selected node draws its own paths and mutes every other path", () => {
  const model = three();

  const resting = topologySVGMarkup(model, null);
  assert.ok(!resting.includes("edge sel"), "no node is selected, so no path is accented");
  assert.ok(!resting.includes("edge muted"), "no node is selected, so no path is muted");

  const selected = topologySVGMarkup(model, "homelab");
  const classes = (selected.match(/<path class="([^"]*)"/g) || []).map((element) =>
    element.replace(/^<path class="/, "").replace(/"$/, ""),
  );
  assert.deepEqual(classes, [
    "edge muted", // jbones->internet
    "edge muted", // jbones->host
    "edge sel", //   homelab->internet
    "edge sel", //   homelab->host
    "edge muted", // corp-prod->internet
  ]);

  assert.match(selected, /data-node="homelab"[^>]*aria-pressed="true"/);
  assert.match(selected, /data-node="jbones"[^>]*aria-pressed="false"/);

  // The internet node owns two of the five paths, because two rules name it.
  const internet = topologySVGMarkup(model, "internet");
  const accented = (internet.match(/edge sel/g) || []).length;
  assert.equal(accented, 3);
});

test("the text equivalent lists every allowed path as a sentence", () => {
  const model = three();

  assert.deepEqual(model.sentences, [
    "jbones reaches internet.",
    "jbones reaches host on tcp/22.",
    "homelab reaches internet.",
    "homelab reaches host.",
    "corp-prod reaches internet.",
  ]);
  assert.equal(model.summary, "Five allowed paths.");
  assert.equal(model.absence, "No tailnet reaches another tailnet.");

  const markup = textEquivalentMarkup(model);
  assert.match(markup, /<ul>/, "the text equivalent is a list");
  assert.equal((markup.match(/<li>/g) || []).length, 5);
  for (const sentence of model.sentences) {
    assert.ok(markup.includes(`<li>${sentence}</li>`), `the list holds no ${sentence}`);
  }
  assert.ok(markup.includes(model.summary));
  assert.ok(markup.includes(model.absence));

  // The svg names the text equivalent, so a screen reader reads it in place of the curves.
  const svg = topologySVGMarkup(model, null);
  assert.match(svg, /role="img"/);
  assert.match(svg, /aria-describedby="topology-text"/);
});

test("the text equivalent states the absence of a path only when no tailnet reaches another", () => {
  const status = statusWith({ jbones: {}, homelab: {} });
  const access = accessWith(
    [
      { id: "jbones", peers: 1, veth: "10.99.0.2" },
      { id: "homelab", peers: 0, veth: "10.99.0.6" },
    ],
    [{ from: "jbones", to: "homelab", ports: [] }],
  );
  const model = buildTopology(status, access);

  assert.deepEqual(model.sentences, ["jbones reaches homelab."]);
  assert.equal(model.summary, "One allowed path.");
  assert.equal(model.absence, "");

  // Both endpoints sit in the left column, so the curve leaves and returns on the left.
  assert.equal(model.paths.length, 1);
  assert.match(model.paths[0].d, /^M12,/, "a same-column curve starts on the left edge");
});

test("an empty rule set draws every node and no path", () => {
  const status = statusWith({ jbones: {} });
  const model = buildTopology(status, accessWith([{ id: "jbones", peers: 0, veth: "10.99.0.2" }], []));

  assert.equal(model.paths.length, 0);
  assert.equal(model.summary, "No allowed path.");
  assert.deepEqual(model.sentences, []);
  assert.equal(model.nodes.length, 3);

  const markup = topologySVGMarkup(model, null);
  assert.equal((markup.match(/<path /g) || []).length, 0);
});

test("a tailnet reports its reachability as a coloured dot and a lowercase word", () => {
  assert.deepEqual(reachabilityOf({ measured_reachability: { state: "reachable", detail: "" } }), {
    state: "reachable",
    tone: "ok",
    word: "reachable",
    detail: "",
  });
  assert.deepEqual(
    reachabilityOf({ measured_reachability: { state: "unreachable", detail: "no answer inside 250ms" } }),
    { state: "unreachable", tone: "crit", word: "unreachable", detail: "no answer inside 250ms" },
  );
  assert.deepEqual(
    reachabilityOf({ measured_reachability: { state: "not_probed", detail: "the namespace does not exist" } }),
    { state: "not_probed", tone: "warn", word: "not probed", detail: "the namespace does not exist" },
  );

  // A daemon that reports no measurement gets no invented state.
  assert.equal(reachabilityOf({}).word, "not probed");
  assert.equal(reachabilityOf(null).word, "not probed");
  assert.equal(reachabilityOf({ measured_reachability: { state: "flapping" } }).word, "unknown");

  for (const entry of ["reachable", "unreachable", "not probed", "unknown"]) {
    assert.equal(entry, entry.toLowerCase(), "a state word is lowercase");
  }
});

test("the statistics row counts the tailnets and the peers of the poll payload", () => {
  const model = three();
  assert.equal(model.counts.tailnets, 3);
  assert.equal(model.counts.peers, 14);
  assert.equal(model.counts.paths, 5);
});

test("the reconciler state is a coloured dot and a lowercase word", () => {
  const converged = statusWith({ jbones: {}, homelab: {} });
  assert.deepEqual(reconcilerState(converged), { tone: "ok", word: "converged" });

  const working = statusWith({ jbones: {}, homelab: { healthy: false } });
  assert.deepEqual(reconcilerState(working), { tone: "warn", word: "reconciling" });

  const failed = statusWith({ jbones: {} });
  failed.error_states = { jbones: true };
  assert.deepEqual(reconcilerState(failed), { tone: "crit", word: "error" });

  const missing = statusWith({ jbones: {} });
  delete missing.actual.jbones;
  assert.deepEqual(reconcilerState(missing), { tone: "warn", word: "reconciling" });

  assert.deepEqual(reconcilerState(null), { tone: "warn", word: "no state yet" });
});

test("the time since the last reconcile comes from the newest reconcile_complete event", () => {
  const events = [
    { Time: "2026-08-05T14:02:00Z", Type: "reconcile_complete", TailnetID: "", Message: "no changes needed" },
    { Time: "2026-08-05T14:02:04Z", Type: "reconcile_start", TailnetID: "", Message: "" },
    { Time: "2026-08-05T14:02:05Z", Type: "reconcile_complete", TailnetID: "", Message: "applied 2 actions" },
    { Time: "2026-08-05T14:02:06Z", Type: "access.written", TailnetID: "", Message: "11 rules" },
  ];
  assert.equal(lastReconcileAt(events), Date.parse("2026-08-05T14:02:05Z"));
  assert.equal(lastReconcileAt([]), null);
  assert.equal(lastReconcileAt(null), null);

  const at = Date.parse("2026-08-05T14:02:05Z");
  assert.equal(sinceWords(at + 4000, at), "4s ago");
  assert.equal(sinceWords(at + 125000, at), "2m ago");
  assert.equal(sinceWords(at + 7300000, at), "2h ago");
  assert.equal(sinceWords(at, null), "no tick yet");
});

test("the recent activity holds the five newest events, newest first", () => {
  const events = [];
  for (let n = 0; n < 9; n++) {
    events.push({
      Time: `2026-08-05T14:0${n}:00Z`,
      Type: "reconcile_complete",
      TailnetID: "",
      Message: `tick ${n}`,
    });
  }

  const newest = newestEvents(events, 5);
  assert.equal(newest.length, 5);
  assert.deepEqual(
    newest.map((event) => event.Message),
    ["tick 8", "tick 7", "tick 6", "tick 5", "tick 4"],
  );

  assert.deepEqual(newestEvents([], 5), []);
  assert.deepEqual(newestEvents(null, 5), []);
});

test("the error state names the socket path and the console address", () => {
  assert.equal(DEFAULT_SOCKET_PATH, "/var/lib/hydrascale/api.sock");

  const sentences = errorSentences("127.0.0.1:9443", "connection refused");
  const joined = sentences.join(" ");
  assert.ok(joined.includes("127.0.0.1:9443"), "the error state names no console address");
  assert.ok(joined.includes(DEFAULT_SOCKET_PATH), "the error state names no socket path");
  assert.ok(joined.includes("connection refused"), "the error state states no reason");
  for (const sentence of sentences) {
    assert.ok(sentence.endsWith("."), `${sentence} is not a sentence`);
  }
});

test("the topology escapes every value that the daemon reports", () => {
  const status = statusWith({ "a<b": {} });
  const access = accessWith([{ id: "a<b", peers: 0, veth: "10.99.0.2" }], []);
  const markup = topologySVGMarkup(buildTopology(status, access), null);

  assert.ok(!markup.includes("a<b"), "the topology writes a raw angle bracket");
  assert.ok(markup.includes("a&lt;b"));
});
