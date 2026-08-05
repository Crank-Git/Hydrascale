// The console has no build step and no package manager, so these tests run on the source
// file that the browser loads. The Go test TestTheConsoleJavaScriptTestsPass starts them.
//
// panels.js holds the model and the serializer of the DNS view, the activity view, and
// the settings view. Every function there is pure, therefore this file asserts the drawn
// result exactly, with no browser and no network.
import assert from "node:assert/strict";
import test from "node:test";

import {
  activityMarkup,
  activityRows,
  dnsMarkup,
  dnsModel,
  settingsMarkup,
  settingsModel,
} from "../static/panels.js";

// dnsBody is one answer of GET /api/dns, as internal/api/types.go declares it.
function dnsBody(overrides = {}) {
  return {
    mode: "unified",
    bind_address: "127.0.0.53:5354",
    upstreams: ["1.1.1.1:53", "9.9.9.9:53"],
    allow_unprotected: false,
    host_resolv_path: "/etc/resolv.conf",
    host_resolv_sha256: "9f2c4d1a7b30",
    host_resolv_changed_at: "",
    namespaces: [
      { id: "jbones", protected: true, error: "" },
      { id: "homelab", protected: true, error: "" },
    ],
    ...overrides,
  };
}

// statusBody is one answer of GET /api/status, with the fields that the settings view
// reads.
function statusBody(overrides = {}) {
  return {
    server_version: "1.0.0",
    config_path: "/etc/hydrascale/config.yaml",
    socket_path: "/var/lib/hydrascale/api.sock",
    console_address: "127.0.0.1:9443",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// The DNS view
// ---------------------------------------------------------------------------

test("the DNS view shows the resolver mode, the bind address, and every upstream", () => {
  // FR-console-34.
  const model = dnsModel(dnsBody());
  assert.equal(model.mode, "unified");
  assert.equal(model.bindAddress, "127.0.0.53:5354");
  assert.deepEqual(model.upstreams, ["1.1.1.1:53", "9.9.9.9:53"]);

  const markup = dnsMarkup(model);
  assert.match(markup, /unified/);
  assert.match(markup, /127\.0\.0\.53:5354/);
  assert.match(markup, /1\.1\.1\.1:53/);
  assert.match(markup, /9\.9\.9\.9:53/);
});

test("the DNS view shows one row per namespace with the protected state", () => {
  const model = dnsModel(dnsBody());
  assert.equal(model.namespaces.length, 2);
  assert.deepEqual(model.namespaces[0], {
    id: "jbones",
    tone: "ok",
    word: "protected",
    error: "",
  });

  const markup = dnsMarkup(model);
  assert.equal((markup.match(/class="nsrow"/g) || []).length, 2);
  assert.match(markup, /<span class="dot ok"><\/span>protected/);
});

test("the DNS view states an unprotected namespace as a critical dot and its reason", () => {
  const body = dnsBody({
    namespaces: [{ id: "corp", protected: false, error: "overlay /etc failed: invalid argument" }],
  });
  const model = dnsModel(body);
  assert.deepEqual(model.namespaces[0], {
    id: "corp",
    tone: "crit",
    word: "unprotected",
    error: "overlay /etc failed: invalid argument",
  });
  assert.match(dnsMarkup(model), /overlay \/etc failed: invalid argument/);
});

test("the key dns.allow_unprotected takes an unprotected namespace out of the error state", () => {
  // Issue #76 settled FR-dns-5 against FR-dns-6: the operator who opted out of protection
  // reads a warning and no error. See the changelog of docs/specs/spec.md.
  const body = dnsBody({
    allow_unprotected: true,
    namespaces: [{ id: "corp", protected: false, error: "overlay /etc failed: invalid argument" }],
  });
  const model = dnsModel(body);
  assert.equal(model.allowUnprotected, true);
  assert.equal(model.namespaces[0].tone, "warn");
  assert.equal(model.namespaces[0].word, "unprotected");

  const markup = dnsMarkup(model);
  assert.match(markup, /dns\.allow_unprotected/);
  assert.doesNotMatch(markup, /class="dot crit"/);
});

test("the DNS view shows the checksum of the host file and the time of the last change", () => {
  const model = dnsModel(dnsBody({ host_resolv_changed_at: "2026-08-05T13:12:44Z" }));
  assert.equal(model.checksum, "9f2c4d1a7b30");
  assert.equal(model.changedAt, "2026-08-05T13:12:44Z");
  assert.equal(model.hostPath, "/etc/resolv.conf");

  const markup = dnsMarkup(model);
  assert.match(markup, /9f2c4d1a7b30/);
  assert.match(markup, /13:12:44/);
  assert.match(markup, /\/etc\/resolv\.conf/);
});

test("the DNS view shows a warning when the host resolv.conf file changed", () => {
  // FR-console-35. The daemon reports the change and it repairs nothing, so the warning
  // states that.
  const changed = dnsModel(dnsBody({ host_resolv_changed_at: "2026-08-05T13:12:44Z" }));
  assert.equal(changed.changed, true);
  const markup = dnsMarkup(changed);
  assert.match(markup, /class="alert warn"/);
  assert.match(markup, /changed/);

  const steady = dnsModel(dnsBody());
  assert.equal(steady.changed, false);
  assert.doesNotMatch(dnsMarkup(steady), /class="alert warn"/);
});

test("the DNS view states an empty state when the daemon reports no namespace", () => {
  const model = dnsModel(dnsBody({ namespaces: [], upstreams: [] }));
  const markup = dnsMarkup(model);
  assert.match(markup, /The daemon runs no namespace\./);
  assert.match(markup, /The forwarder reports no upstream\./);
});

test("the DNS view states an empty state when the daemon answers no DNS route", () => {
  const model = dnsModel(null);
  assert.equal(model.ready, false);
  assert.match(dnsMarkup(model), /The daemon reports no DNS state yet\./);
});

test("the DNS view escapes every value that the daemon reports", () => {
  // The console has no authentication, therefore an unescaped value that the daemon
  // reports is script injection into the console. See SA-5.
  const hostile = '<img src=x onerror="alert(1)">';
  const model = dnsModel(
    dnsBody({
      mode: hostile,
      bind_address: hostile,
      upstreams: [hostile],
      host_resolv_path: hostile,
      host_resolv_sha256: hostile,
      host_resolv_changed_at: hostile,
      namespaces: [{ id: hostile, protected: false, error: hostile }],
    }),
  );
  const markup = dnsMarkup(model);
  assert.doesNotMatch(markup, /<img/);
  assert.doesNotMatch(markup, /onerror=/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

// ---------------------------------------------------------------------------
// The activity view
// ---------------------------------------------------------------------------

// events returns four events in the order that the daemon appends them.
function events() {
  return [
    {
      Time: "2026-08-05T13:10:01Z",
      Type: "access.applied",
      TailnetID: "",
      Message: "wrote 11 rules into HYDRASCALE-FWD",
    },
    {
      Time: "2026-08-05T13:11:02Z",
      Type: "dns.unprotected",
      TailnetID: "corp",
      Message: "overlay /etc failed: invalid argument",
    },
    {
      Time: "2026-08-05T13:12:03Z",
      Type: "policy.pushed",
      TailnetID: "jbones",
      Message: "the control server accepted the policy document",
    },
    {
      Time: "2026-08-05T13:13:04Z",
      Type: "console.request",
      TailnetID: "",
      Message: "PUT /api/access on the console listener",
    },
  ];
}

test("the activity view lists every event newest first", () => {
  // FR-console-36.
  const rows = activityRows(events());
  assert.deepEqual(
    rows.map((row) => row.kind),
    ["console.request", "policy.pushed", "dns.unprotected", "access.applied"],
  );
});

test("the activity view states the time, the kind, the tailnet, and the message", () => {
  const rows = activityRows(events());
  assert.deepEqual(rows[1], {
    date: "2026-08-05",
    time: "13:12:03",
    kind: "policy.pushed",
    tailnet: "jbones",
    message: "the control server accepted the policy document",
  });

  const markup = activityMarkup(rows);
  assert.match(markup, /13:12:03/);
  assert.match(markup, /2026-08-05/);
  assert.match(markup, /policy\.pushed/);
  assert.match(markup, /jbones/);
  assert.match(markup, /the control server accepted the policy document/);
});

test("the activity view shows the four event kinds of version 1.0", () => {
  const markup = activityMarkup(activityRows(events()));
  for (const kind of ["access.applied", "dns.unprotected", "policy.pushed", "console.request"]) {
    assert.match(markup, new RegExp(kind.replace(".", "\\.")));
  }
});

test("the activity view keeps a time that no parser reads", () => {
  // The console shows no invented data, so a time that the console cannot read reaches
  // the operator as the daemon wrote it.
  const rows = activityRows([{ Time: "not a time", Type: "access.applied", Message: "m" }]);
  assert.equal(rows[0].date, "");
  assert.equal(rows[0].time, "not a time");
});

test("the activity view states an empty state when the daemon reports no event", () => {
  assert.match(activityMarkup(activityRows([])), /The daemon reports no event\./);
  assert.match(activityMarkup(activityRows(null)), /The daemon reports no event\./);
});

test("the activity view escapes every value that the daemon reports", () => {
  const hostile = '<img src=x onerror="alert(1)">';
  const rows = activityRows([
    { Time: hostile, Type: hostile, TailnetID: hostile, Message: hostile },
  ]);
  const markup = activityMarkup(rows);
  assert.doesNotMatch(markup, /<img/);
  assert.doesNotMatch(markup, /onerror=/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

// ---------------------------------------------------------------------------
// The settings view
// ---------------------------------------------------------------------------

test("the settings view shows the paths, the console address, the interval, and the version", () => {
  // FR-console-37.
  const model = settingsModel(statusBody(), 5000, "127.0.0.1:9443");
  assert.deepEqual(
    model.rows.map((row) => [row.label, row.value]),
    [
      ["configuration file", "/etc/hydrascale/config.yaml"],
      ["control socket", "/var/lib/hydrascale/api.sock"],
      ["console address", "127.0.0.1:9443"],
      ["poll interval", "5s"],
      ["version", "1.0.0"],
    ],
  );

  const markup = settingsMarkup(model);
  assert.match(markup, /\/etc\/hydrascale\/config\.yaml/);
  assert.match(markup, /\/var\/lib\/hydrascale\/api\.sock/);
  assert.match(markup, /127\.0\.0\.1:9443/);
  assert.match(markup, /5s/);
  assert.match(markup, /1\.0\.0/);
});

test("the settings view states that the console has no authentication", () => {
  // FR-console-38. The section "The console has no authentication" of docs/specs/spec.md
  // records the accepted risk SA-5.
  const markup = settingsMarkup(settingsModel(statusBody(), 5000, "127.0.0.1:9443"));
  assert.match(markup, /no authentication/);
  assert.match(markup, /Any local account on this host/);
});

test("the settings view links the activity view, which holds the event list", () => {
  // The fourth console control of docs/specs/spec.md is the event list of every mutating
  // console request.
  assert.match(settingsMarkup(settingsModel(statusBody(), 5000, "")), /href="#\/activity"/);
});

test("the settings view states a value that the daemon does not report", () => {
  const model = settingsModel({ server_version: "" }, 5000, "");
  for (const row of model.rows) {
    if (row.label === "poll interval") {
      continue;
    }
    assert.equal(row.reported, false, `${row.label} reports a value that no poll returned`);
  }
  const markup = settingsMarkup(model);
  assert.match(markup, /not reported/);
});

test("the settings view states an empty state when no poll returned", () => {
  assert.match(settingsMarkup(settingsModel(null, 5000, "")), /The daemon reports no path yet\./);
});

test("the settings view reads the console address of the browser when the daemon reports none", () => {
  const model = settingsModel(statusBody({ console_address: "" }), 5000, "127.0.0.1:9443");
  const address = model.rows.find((row) => row.label === "console address");
  assert.equal(address.value, "127.0.0.1:9443");
  assert.equal(address.reported, true);
});

test("the settings view never shows a credential", () => {
  // SA-1 was an auth key in the body of GET /api/status. config.Tailnet carries the tag
  // json:"-" on AuthKey now, and this test states the second rule: the settings view
  // renders no field of the status body other than the five that it names.
  const key = "tskey-auth-kNotARealKey-000000000000";
  const status = statusBody({
    desired: { corp: { id: "corp", auth_key: key } },
    secrets: { headscale_api_key: key, oauth_client_secret: key },
  });
  const markup = settingsMarkup(settingsModel(status, 5000, "127.0.0.1:9443"));

  assert.match(JSON.stringify(status), /tskey-/, "the payload holds no key, so this test proves nothing");
  assert.doesNotMatch(markup, /tskey-/);
  assert.doesNotMatch(markup, /auth_key/);
  assert.doesNotMatch(markup, /headscale_api_key/);
  assert.doesNotMatch(markup, /oauth_client_secret/);
});

test("the settings view escapes every value that the daemon reports", () => {
  const hostile = '<img src=x onerror="alert(1)">';
  const model = settingsModel(
    statusBody({ config_path: hostile, socket_path: hostile, console_address: hostile, server_version: hostile }),
    5000,
    hostile,
  );
  const markup = settingsMarkup(model);
  assert.doesNotMatch(markup, /<img/);
  assert.doesNotMatch(markup, /onerror=/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});
