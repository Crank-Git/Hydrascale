// The namespace view builds its rows, its panel and its removal dialog from the poll
// payload with pure functions, so these tests assert the whole output without a browser.
// The Go test TestTheConsoleJavaScriptTestsPass starts them.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  DETAIL_TTL_MS,
  IDENTIFIER_PATTERN,
  IDENTIFIER_RULE,
  buildPanel,
  buildRemovalDialog,
  buildRows,
  createAddFlow,
  planDetailRequests,
  validateIdentifier,
} from "../static/namespaces.js";

// statusOf builds a status payload of GET /api/status. The keys are the keys that the
// daemon writes: config.Tailnet and reconciler.TailnetState carry no JSON tag except
// AuthKey and MeasuredReachability.
function statusOf(tailnets) {
  const desired = {};
  const actual = {};
  const errorStates = {};
  const pausedStates = {};
  const lastErrors = {};
  for (const tailnet of tailnets) {
    desired[tailnet.id] = {
      ID: tailnet.id,
      ControlURL: tailnet.controlURL || "",
      ExitNode: tailnet.exitNode || "",
      HostAccess: tailnet.hostAccess === undefined ? null : tailnet.hostAccess,
    };
    actual[tailnet.id] = {
      ID: tailnet.id,
      NsName: `ns-${tailnet.id}`,
      NsExists: tailnet.nsExists !== false,
      DaemonHealthy: tailnet.healthy !== false,
      Routes: [],
      measured_reachability: tailnet.reachability || null,
    };
    if (tailnet.error) {
      errorStates[tailnet.id] = true;
      lastErrors[tailnet.id] = tailnet.error;
    }
    if (tailnet.paused) {
      pausedStates[tailnet.id] = true;
    }
  }
  return {
    server_version: "1.0.0",
    desired,
    actual,
    error_states: errorStates,
    paused_states: pausedStates,
    failure_counts: {},
    last_errors: lastErrors,
  };
}

// detailOf builds a payload of GET /api/tailnet/{id}/detail.
function detailOf(fields = {}) {
  return {
    tailscale_ips: fields.ips || [],
    magic_dns_name: fields.magicDNS || "",
    peer_count: fields.peers ? fields.peers.length : 0,
    online_peers: 0,
    peers: fields.peers || [],
    backend_state: fields.backendState || "Running",
    login_url: fields.loginURL || "",
    error: fields.error || "",
  };
}

test("the list shows a state dot, the identifier, the peer count and the address", () => {
  const status = statusOf([{ id: "jbones", reachability: { state: "reachable" } }]);
  const details = {
    jbones: detailOf({
      ips: ["100.83.14.7"],
      peers: [
        { host_name: "mars", tailscale_ips: ["100.83.2.11"] },
        { host_name: "brain", tailscale_ips: ["100.83.7.4"] },
      ],
    }),
  };

  const rows = buildRows(status, details, {});
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "jbones");
  assert.deepEqual(rows[0].state, { word: "healthy", tone: "ok" });
  assert.equal(rows[0].peers, "2 peers");
  assert.equal(rows[0].address, "100.83.14.7");
  assert.equal(rows[0].namespace, "ns-jbones");
});

test("the list states the reachability and the health as two states", () => {
  // Issue #172 found two tailnets that reported healthy while neither reached anything,
  // so the view never merges the two answers into one word.
  const status = statusOf([
    { id: "alpha", reachability: { state: "unreachable", detail: "no answer" } },
  ]);
  const rows = buildRows(status, {}, {});
  assert.deepEqual(rows[0].state, { word: "healthy", tone: "ok" });
  assert.deepEqual(rows[0].reachability, { word: "unreachable", tone: "crit" });
});

test("a tailnet with no measurement states that the daemon probed nothing", () => {
  const rows = buildRows(statusOf([{ id: "alpha" }]), {}, {});
  assert.deepEqual(rows[0].reachability, { word: "not probed", tone: "" });
});

test("the not-authenticated row shows a warning dot", () => {
  const status = statusOf([{ id: "lab" }]);
  const details = { lab: detailOf({ backendState: "NeedsLogin", loginURL: "x" }) };
  const rows = buildRows(status, details, {});
  assert.deepEqual(rows[0].state, { word: "not authenticated", tone: "warn" });
});

test("the error row shows a critical dot", () => {
  const status = statusOf([{ id: "alpha", error: "the namespace does not exist" }]);
  const rows = buildRows(status, {}, {});
  assert.deepEqual(rows[0].state, { word: "error", tone: "crit" });
});

test("a row with no usable policy credential shows an additional state, and the health word does not change", () => {
  // Issue #287. Local reachability and upstream policy are two independent systems, so
  // the credential problem is a second line, not a replacement of the health word.
  // Issue #347. The word is short, because the row draws it in the dot and word slot. The
  // reason travels beside the word and the row draws no reason.
  const status = statusOf([{ id: "havoc", reachability: { state: "reachable" } }]);
  status.policy = [
    { id: "havoc", kind: "tailscale", credential_state: "absent", reason: "the tailnet \"havoc\" has no Tailscale OAuth credential" },
  ];
  const rows = buildRows(status, {}, {});
  assert.deepEqual(rows[0].state, { word: "healthy", tone: "ok" });
  assert.deepEqual(rows[0].reachability, { word: "reachable", tone: "ok" });
  assert.deepEqual(rows[0].credential, {
    tone: "crit",
    word: "no credential",
    reason: "the tailnet \"havoc\" has no Tailscale OAuth credential",
  });
});

test("a row of a tailnet the control server rejects states the rejection in two words", () => {
  // Issue #347. The word matches the word of the policy view for the same state.
  const status = statusOf([{ id: "havoc" }]);
  status.policy = [
    { id: "havoc", kind: "tailscale", credential_state: "rejected", reason: "the control server takes the credential for no request" },
  ];
  const rows = buildRows(status, {}, {});
  assert.deepEqual(rows[0].credential, {
    tone: "crit",
    word: "credential rejected",
    reason: "the control server takes the credential for no request",
  });
});

test("a row with a usable policy credential shows no additional state", () => {
  const status = statusOf([{ id: "jbones" }]);
  status.policy = [{ id: "jbones", kind: "tailscale", credential_state: "usable" }];
  const rows = buildRows(status, {}, {});
  assert.equal(rows[0].credential, null);
});

test("a row of a tailnet the poll holds no policy entry for shows no additional state", () => {
  const status = statusOf([{ id: "jbones" }]);
  status.policy = [];
  const rows = buildRows(status, {}, {});
  assert.equal(rows[0].credential, null);
});

test("the removing row is muted and its actions are disabled", () => {
  const status = statusOf([{ id: "alpha" }, { id: "beta" }]);
  const rows = buildRows(status, {}, { removing: ["alpha"] });
  const alpha = rows.find((row) => row.id === "alpha");
  const beta = rows.find((row) => row.id === "beta");

  assert.equal(alpha.muted, true);
  assert.equal(alpha.actionsDisabled, true);
  assert.deepEqual(alpha.state, { word: "removing", tone: "" });
  assert.equal(beta.muted, false);
  assert.equal(beta.actionsDisabled, false);
});

test("the panel opens on a selection and it closes when the selection clears", () => {
  const status = statusOf([{ id: "alpha" }]);
  assert.notEqual(buildPanel(status, {}, [], "alpha"), null);
  assert.equal(buildPanel(status, {}, [], null), null);
  assert.equal(buildPanel(status, {}, [], "gone"), null);
});

test("the panel states the peers, the magicdns name, the control server and the host access", () => {
  const status = statusOf([
    { id: "alpha", controlURL: "hs.example.net", hostAccess: true },
  ]);
  const details = {
    alpha: detailOf({
      ips: ["100.83.14.7"],
      magicDNS: "phobos.alpha.ts.net",
      peers: [{ host_name: "mars", tailscale_ips: ["100.83.2.11"] }],
    }),
  };
  const events = [
    { Time: "2026-08-05T10:00:00Z", Type: "namespace.created", TailnetID: "alpha", Message: "created ns-alpha" },
    { Time: "2026-08-05T10:00:01Z", Type: "dns.refreshed", TailnetID: "beta", Message: "refreshed" },
  ];

  const panel = buildPanel(status, details, events, "alpha");
  const value = (label) => panel.fields.find((field) => field.label === label).value;

  assert.equal(value("namespace"), "ns-alpha");
  assert.equal(value("address"), "100.83.14.7");
  assert.equal(value("magicdns"), "phobos.alpha.ts.net");
  assert.equal(value("control server"), "hs.example.net");
  assert.equal(value("host access"), "on");
  assert.equal(panel.peers.length, 1);
  assert.equal(panel.peers[0].name, "mars");

  // The panel shows the events of this tailnet only.
  assert.equal(panel.events.length, 1);
  assert.equal(panel.events[0].kind, "namespace.created");
});

test("the panel states the credential problem beside the health and it keeps the reason out of the control server row", () => {
  // Issue #287 put the reason in the control server row. Issue #347 took it out again: the
  // row holds one short value, and a sentence in it overflowed the panel. The reason
  // travels as its own field, which the panel draws in a note that wraps.
  const status = statusOf([{ id: "havoc" }]);
  status.policy = [
    { id: "havoc", kind: "tailscale", credential_state: "absent", reason: "the tailnet \"havoc\" has no Tailscale OAuth credential" },
  ];
  const panel = buildPanel(status, {}, [], "havoc");
  assert.deepEqual(panel.credential, {
    tone: "crit",
    word: "no credential",
    reason: "the tailnet \"havoc\" has no Tailscale OAuth credential",
  });
  const value = (label) => panel.fields.find((field) => field.label === label).value;
  assert.equal(value("control server"), "—");
});

test("the panel keeps the declared control server in its row when the credential is absent", () => {
  // Issue #347. The row states the control server of the file, and the credential problem
  // never replaces it.
  const status = statusOf([{ id: "havoc", controlURL: "https://hs.example.net" }]);
  status.policy = [
    { id: "havoc", kind: "tailscale", credential_state: "absent", reason: "the tailnet \"havoc\" has no Tailscale OAuth credential" },
  ];
  const panel = buildPanel(status, {}, [], "havoc");
  const value = (label) => panel.fields.find((field) => field.label === label).value;
  assert.equal(value("control server"), "https://hs.example.net");
});

test("app.css breaks the credential reason inside the panel", async () => {
  // Issue #347. The reason names HYDRASCALE_TS_CLIENT_SECRET_HAVOC, which no space breaks,
  // so the paragraph needs the rule to stay inside the panel.
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ns-reason\{overflow-wrap:anywhere\}/);
});

test("app.css keeps the address of a row inside the row", async () => {
  // Issue #354. The row is a flex line, and the address is its last item. The address
  // holds no space that breaks it below its own width, so a line that is too short
  // draws the address outside the row, where the panel paints over it. The row
  // therefore wraps, and the address ends in an ellipsis when even its own line is too
  // short.
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");

  const row = style.match(/\.ns-row\{([^}]*)\}/);
  assert.ok(row, "app.css holds no rule .ns-row");
  assert.match(row[1], /flex-wrap:\s*wrap/);

  const address = style.match(/\.ns-addr\{([^}]*)\}/);
  assert.ok(address, "app.css holds no rule .ns-addr");
  assert.match(address[1], /min-width:\s*0/);
  assert.match(address[1], /overflow:\s*hidden/);
  assert.match(address[1], /text-overflow:\s*ellipsis/);
  assert.match(address[1], /white-space:\s*nowrap/);
});

test("the panel keeps the em dash of the control server row when no policy entry names the tailnet", () => {
  const status = statusOf([{ id: "jbones" }]);
  const panel = buildPanel(status, {}, [], "jbones");
  assert.equal(panel.credential, null);
  const value = (label) => panel.fields.find((field) => field.label === label).value;
  assert.equal(value("control server"), "—");
});

test("the panel lists two hundred peers in a scrolling region and it states the count", () => {
  const peers = [];
  for (let index = 0; index < 200; index += 1) {
    peers.push({ host_name: `peer-${index}`, tailscale_ips: [`100.83.1.${index % 250}`] });
  }
  const status = statusOf([{ id: "alpha" }]);
  const panel = buildPanel(status, { alpha: detailOf({ peers }) }, [], "alpha");

  assert.equal(panel.peerCount, 200);
  assert.equal(panel.peerLabel, "200 peers");
  assert.equal(panel.peers.length, 200);
  assert.equal(panel.scrolls, true);
});

test("the panel of a tailnet that is not authenticated states the login url", () => {
  const status = statusOf([{ id: "lab" }]);
  const details = {
    lab: detailOf({ backendState: "NeedsLogin", loginURL: "hs.example.net/register/nodekey:0000" }),
  };
  const panel = buildPanel(status, details, [], "lab");
  assert.equal(panel.loginURL, "hs.example.net/register/nodekey:0000");
});

test("the panel of a tailnet in error states the last error", () => {
  const status = statusOf([{ id: "alpha", error: "the veth pair is absent" }]);
  const panel = buildPanel(status, {}, [], "alpha");
  assert.equal(panel.lastError, "the veth pair is absent");
});

test("the panel offers disconnect for a running tailnet and connect for a stopped one", () => {
  const running = buildPanel(statusOf([{ id: "alpha" }]), {}, [], "alpha");
  assert.equal(running.actions.disconnect, true);
  assert.equal(running.actions.connect, false);

  const stopped = buildPanel(statusOf([{ id: "alpha", healthy: false }]), {}, [], "alpha");
  assert.equal(stopped.actions.connect, true);
  assert.equal(stopped.actions.disconnect, false);
});

test("the identifier rule of the console is the rule of the daemon", () => {
  assert.equal(IDENTIFIER_PATTERN.source, "^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$");
  for (const id of ["alpha", "corp-prod", "a.b_c-1", "9", "a".repeat(63)]) {
    assert.equal(validateIdentifier(id), null, `the console refused ${id}`);
  }
  for (const id of ["My Net", "-lead", ".dot", "a/b", "a".repeat(64), "../../tmp/x"]) {
    assert.equal(validateIdentifier(id), IDENTIFIER_RULE, `the console accepted ${id}`);
  }
  assert.equal(validateIdentifier(""), "an id is required");
});

test("the add flow rejects the identifier My Net before it sends a request", async () => {
  const sent = [];
  const flow = createAddFlow({ request: async (route, method, body) => { sent.push({ route, method, body }); return {}; } });

  flow.setField("id", "My Net");
  flow.setField("authKey", "tskey-auth-kNotARealKey-000000000000");
  const result = await flow.submit();

  assert.equal(result.ok, false);
  assert.equal(result.errors.id, IDENTIFIER_RULE);
  assert.deepEqual(sent, [], "the add flow sent a request for an identifier that the daemon refuses");
});

test("the add flow sends the five fields to the add route", async () => {
  const sent = [];
  const flow = createAddFlow({ request: async (route, method, body) => { sent.push({ route, method, body }); return {}; } });

  flow.setField("id", "alpha");
  flow.setField("authKey", "tskey-auth-kNotARealKey-000000000000");
  flow.setField("controlURL", "hs.example.net");
  flow.setField("exitNode", "100.83.2.11");
  flow.setField("hostAccess", true);

  const result = await flow.submit();
  assert.equal(result.ok, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].route, "/api/tailnet/add");
  assert.equal(sent[0].method, "POST");
  assert.deepEqual(sent[0].body, {
    id: "alpha",
    auth_key: "tskey-auth-kNotARealKey-000000000000",
    control_url: "hs.example.net",
    exit_node: "100.83.2.11",
    host_access: true,
  });
});

test("the add flow never holds an auth key after the operator submits it", async () => {
  const authKey = "tskey-auth-kNotARealKey-000000000000";
  const flow = createAddFlow({ request: async () => ({}) });

  flow.setField("id", "alpha");
  flow.setField("authKey", authKey);
  await flow.submit();

  const state = flow.state();
  assert.equal(state.fields.authKey, "");
  assert.equal(
    JSON.stringify(state).includes(authKey),
    false,
    "the add flow holds the auth key after the submit",
  );
  assert.equal(JSON.stringify(state).includes("tskey"), false);
});

test("the add flow keeps the auth key after a refused request, and it never states the key", async () => {
  const authKey = "tskey-auth-kNotARealKey-000000000000";
  const flow = createAddFlow({
    request: async () => {
      throw new Error("tailnet alpha already exists");
    },
  });

  flow.setField("id", "alpha");
  flow.setField("authKey", authKey);
  const result = await flow.submit();

  assert.equal(result.ok, false);
  assert.equal(result.errors.form, "tailnet alpha already exists");
  assert.equal(flow.state().fields.authKey, "", "a refused add left the auth key in the console");
});

test("the removal dialog names every command, the rule count and the state directory", () => {
  const plan = {
    id: "homelab",
    namespace: "ns-homelab",
    host_veth: "vh4f2a91b0c3d4",
    state_dir: "/var/lib/hydrascale/state/homelab",
    rule_count: 3,
    commands: [
      "iptables -D FORWARD -i vh4f2a91b0c3d4 -j ACCEPT",
      "iptables -D FORWARD -o vh4f2a91b0c3d4 -m state --state RELATED,ESTABLISHED -j ACCEPT",
      "iptables -t nat -D POSTROUTING -s 10.200.0.2 -j MASQUERADE",
      "ip link del vh4f2a91b0c3d4",
      "ip netns del ns-homelab",
      "rm -rf /var/lib/hydrascale/state/homelab",
    ],
  };

  const dialog = buildRemovalDialog(plan);
  const joined = dialog.commands.join("\n");

  assert.equal(dialog.heading, "Remove homelab");
  assert.equal(dialog.confirmLabel, "Remove homelab");
  assert.ok(joined.includes("ns-homelab"), "the dialog names no namespace");
  assert.ok(joined.includes("vh4f2a91b0c3d4"), "the dialog names no veth device");
  assert.ok(joined.includes("/var/lib/hydrascale/state/homelab"), "the dialog names no state directory");
  assert.ok(joined.includes("iptables"), "the dialog names no iptables rule");
  assert.ok(dialog.ruleSentence.includes("3"), `the dialog states no rule count: ${dialog.ruleSentence}`);
  assert.ok(dialog.logoutSentence.includes("tailscale logout"), `the dialog names no logout command: ${dialog.logoutSentence}`);
  assert.ok(
    dialog.authorizationSentence.includes("stays authorized on the control server"),
    `the dialog states no authorization: ${dialog.authorizationSentence}`,
  );
});

test("the console asks for the detail of a tailnet once per poll interval", () => {
  // The poll layer is the one data source. The detail requests hang off it, so the
  // console starts no second timer.
  const details = {};
  assert.deepEqual(planDetailRequests(["alpha", "beta"], details, 0, DETAIL_TTL_MS), ["alpha", "beta"]);

  const fresh = { alpha: { at: 1000, value: {} }, beta: { at: 1000, value: {} } };
  assert.deepEqual(planDetailRequests(["alpha", "beta"], fresh, 2000, DETAIL_TTL_MS), []);
  assert.deepEqual(planDetailRequests(["alpha", "beta"], fresh, 1000 + DETAIL_TTL_MS, DETAIL_TTL_MS), ["alpha", "beta"]);

  // A tailnet that the operator removed asks for nothing.
  assert.deepEqual(planDetailRequests([], fresh, 100000, DETAIL_TTL_MS), []);
});
