// The console has no build step and no package manager, so these tests run on the source
// file that the browser loads. The Go test TestTheConsoleJavaScriptTestsPass starts them.
//
// panels.js holds the model and the serializer of the DNS view, the activity view, and
// the settings view. Every function there is pure, therefore this file asserts the drawn
// result exactly, with no browser and no network.
import assert from "node:assert/strict";
import test from "node:test";

import {
  ACTIVITY_ROW_LIMIT,
  activityMarkup,
  activityModel,
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
  assert.doesNotMatch(markup, /onerror="/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("the DNS view maps the split DNS of each tailnet", () => {
  // FR-split-9. The model reads split_dns, and it holds the domains and the conflict.
  const model = dnsModel(
    dnsBody({
      split_dns: [
        { tailnet: "alpha", domains: ["acme.example.com", "zeta.example.com"], conflict: "" },
        { tailnet: "beta", domains: [], conflict: "the split DNS domain acme.example.com of beta is claimed by alpha" },
      ],
    }),
  );
  assert.equal(model.splitDNS.length, 2);
  assert.deepEqual(model.splitDNS[0], {
    tailnet: "alpha",
    domains: ["acme.example.com", "zeta.example.com"],
    conflict: "",
  });
  assert.deepEqual(model.splitDNS[1], {
    tailnet: "beta",
    domains: [],
    conflict: "the split DNS domain acme.example.com of beta is claimed by alpha",
  });

  // A body before the first poll holds no split DNS.
  assert.deepEqual(dnsModel(null).splitDNS, []);
  assert.deepEqual(dnsModel(dnsBody({ split_dns: null })).splitDNS, []);
});

test("the DNS view draws the split DNS card with one row per domain owner", () => {
  const model = dnsModel(
    dnsBody({
      split_dns: [
        { tailnet: "alpha", domains: ["acme.example.com"], conflict: "" },
        { tailnet: "beta", domains: ["zeta.example.com"], conflict: "" },
      ],
    }),
  );
  const markup = dnsMarkup(model);
  assert.match(markup, /<h2 class="frame-title">Split DNS<\/h2>/);
  assert.match(markup, /<span class="id mono">alpha<\/span>/);
  assert.match(markup, /<span class="id mono">beta<\/span>/);
  assert.match(markup, /acme\.example\.com/);
  assert.match(markup, /zeta\.example\.com/);
});

test("the DNS view draws an empty note when no tailnet reports a split domain", () => {
  const model = dnsModel(dnsBody({ split_dns: [] }));
  const markup = dnsMarkup(model);
  assert.match(markup, /No tailnet reports a split DNS domain\./);
  assert.doesNotMatch(markup, /class="domains mono"/);
});

test("the DNS view draws a conflict as a critical alert that names both tailnets", () => {
  const model = dnsModel(
    dnsBody({
      split_dns: [
        { tailnet: "alpha", domains: ["shared.example.com"], conflict: "" },
        {
          tailnet: "beta",
          domains: [],
          conflict: "the split DNS domain shared.example.com of beta is claimed by alpha",
        },
      ],
    }),
  );
  const markup = dnsMarkup(model);
  assert.match(markup, /class="alert crit"/);
  assert.match(markup, /shared\.example\.com of beta is claimed by alpha/);
});

test("the DNS view escapes a hostile split DNS domain", () => {
  // SA-19 and SA-5: the domain reaches the page only as escaped text.
  const hostile = '<img src=x onerror="alert(1)">';
  const model = dnsModel(
    dnsBody({
      split_dns: [{ tailnet: hostile, domains: [hostile], conflict: hostile }],
    }),
  );
  const markup = dnsMarkup(model);
  assert.doesNotMatch(markup, /<img/);
  assert.doesNotMatch(markup, /onerror="/);
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

// tick returns the routine events of one reconcile tick, as the daemon appends them.
function tick(time, actions) {
  const list = [{ Time: time, Type: "reconcile_start", TailnetID: "", Message: "" }];
  list.push({ Time: time, Type: "reconcile_apply", TailnetID: "", Message: `${actions.length} actions` });
  for (const [tailnet, action] of actions) {
    list.push({ Time: time, Type: "action_ok", TailnetID: tailnet, Message: action });
  }
  list.push({ Time: time, Type: "reconcile_complete", TailnetID: "", Message: `applied ${actions.length} actions` });
  return list;
}

function rowCount(markup) {
  return (markup.match(/<tr class="log-row/g) || []).length;
}

test("the activity view lists every notable event newest first", () => {
  // FR-console-36.
  const model = activityModel(events());
  assert.deepEqual(
    model.rows.map((row) => row.event.Type),
    ["console.request", "policy.pushed", "dns.unprotected", "access.applied"],
  );
});

test("the activity view states the time, the tailnet, the kind, and the message", () => {
  const markup = activityMarkup(activityModel(events()));
  assert.match(markup, /13:12:03/);
  assert.match(markup, /2026-08-05/);
  assert.match(markup, /policy\.pushed/);
  assert.match(markup, /jbones/);
  assert.match(markup, /the control server accepted the policy document/);
});

test("the activity view states each date once, as a header of its day", () => {
  const log = [
    { Time: "2026-08-04T23:59:00Z", Type: "access.applied", TailnetID: "", Message: "a" },
    { Time: "2026-08-05T00:01:00Z", Type: "access.applied", TailnetID: "", Message: "b" },
    { Time: "2026-08-05T00:02:00Z", Type: "access.applied", TailnetID: "", Message: "c" },
  ];
  const markup = activityMarkup(activityModel(log));
  assert.equal((markup.match(/class="day"/g) || []).length, 2);
  assert.equal((markup.match(/>2026-08-05</g) || []).length, 1);
});

test("the activity view hides the reconcile ticks by default and folds each tick into one row", () => {
  // A tick records four kinds of routine event, and an incident hides between them.
  const log = [
    ...tick("2026-08-05T13:00:00Z", [["alpha", "sync_routes"], ["beta", "sync_routes"]]),
    { Time: "2026-08-05T13:00:05Z", Type: "access.jump_displaced", TailnetID: "", Message: "the jump rule of INPUT is at position 2" },
    ...tick("2026-08-05T13:00:10Z", [["alpha", "sync_host_access"]]),
  ];

  const quiet = activityModel(log);
  assert.equal(quiet.notable, 1);
  assert.deepEqual(quiet.rows.map((row) => row.kind), ["event"]);

  const every = activityModel(log, { ticks: true });
  assert.deepEqual(every.rows.map((row) => row.kind), ["tick", "event", "tick"]);
  assert.equal(every.rows[0].message, "applied 1 actions");
  assert.deepEqual(every.rows[2].actions, [
    { tailnet: "alpha", action: "sync_routes" },
    { tailnet: "beta", action: "sync_routes" },
  ]);

  const markup = activityMarkup(every);
  assert.equal(rowCount(markup), 3);
  assert.match(markup, /reconcile tick/);
  assert.match(markup, /alpha: sync_routes · beta: sync_routes/);
  assert.doesNotMatch(markup, /reconcile_start/);
});

test("the tailnet filter keeps the events and the tick actions of one tailnet", () => {
  const log = [
    ...tick("2026-08-05T13:00:00Z", [["alpha", "sync_routes"], ["beta", "sync_routes"]]),
    { Time: "2026-08-05T13:00:05Z", Type: "policy.pushed", TailnetID: "beta", Message: "pushed" },
    { Time: "2026-08-05T13:00:06Z", Type: "policy.pushed", TailnetID: "alpha", Message: "pushed" },
  ];
  const model = activityModel(log, { ticks: true, tailnet: "beta" });
  assert.deepEqual(model.tailnets, ["alpha", "beta"]);
  assert.deepEqual(model.rows.map((row) => row.kind), ["event", "tick"]);
  assert.equal(model.rows[0].event.TailnetID, "beta");
  assert.deepEqual(model.rows[1].actions, [{ tailnet: "beta", action: "sync_routes" }]);
});

test("the activity view states the routine log when no event is notable", () => {
  const markup = activityMarkup(activityModel(tick("2026-08-05T13:00:00Z", [["alpha", "sync_routes"]])));
  assert.match(markup, /No event other than a routine reconcile tick/);
  assert.match(markup, /Select Every event to show the ticks\./);
});

test("the activity view keeps a time that no parser reads", () => {
  // The console shows no invented data, so a time that the console cannot read reaches
  // the operator as the daemon wrote it.
  const markup = activityMarkup(activityModel([{ Time: "not a time", Type: "access.applied", Message: "m" }]));
  assert.match(markup, />not a time</);
});

test("the activity view states an empty state when the daemon reports no event", () => {
  assert.match(activityMarkup(activityModel([])), /The daemon reports no event\./);
  assert.match(activityMarkup(activityModel(null)), /The daemon reports no event\./);
});

test("the activity view escapes every value that the daemon reports", () => {
  const hostile = '<img src=x onerror="alert(1)">';
  const markup = activityMarkup(activityModel([{ Time: hostile, Type: hostile, TailnetID: hostile, Message: hostile }]));
  assert.doesNotMatch(markup, /<img/);
  assert.doesNotMatch(markup, /onerror="/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("the activity view names a tailnet whose credential the control server rejected above the table", () => {
  // Issue #287. The activity log stays a record of real daemon events, so the credential
  // fault is an alert above the table, not a synthetic event row.
  const markup = activityMarkup(activityModel(events()), [
    { id: "havoc", kind: "tailscale", credential_state: "rejected", reason: "rejected" },
  ]);
  assert.match(markup, /rejected the policy credential of havoc/);
  assert.match(markup, /href="#\/policy"/);
  assert.ok(markup.indexOf("rejected the policy credential") < markup.indexOf('<table class="log"'));
});

test("the activity view draws no alert for an absent credential, which is optional", () => {
  const markup = activityMarkup(activityModel(events()), [
    { id: "havoc", kind: "tailscale", credential_state: "absent", reason: "no credential" },
    { id: "jbones", kind: "tailscale", credential_state: "usable" },
  ]);
  assert.doesNotMatch(markup, /class="alert/);
});

// longLog returns count events, oldest first, as the daemon reports them. The message of
// each event names its index, so a test can name which events reached the markup.
function longLog(count) {
  const log = [];
  for (let i = 0; i < count; i += 1) {
    log.push({ Time: "2026-08-05T13:00:00Z", Type: "access.applied", TailnetID: "jbones", Message: `event ${i}` });
  }
  return log;
}

test("the activity view draws no more rows than the row limit, and keeps the newest", () => {
  // Issue #355. The daemon holds 1000 events, and every one reached the page as a row.
  const total = ACTIVITY_ROW_LIMIT + 150;
  const markup = activityMarkup(activityModel(longLog(total)));
  assert.equal(rowCount(markup), ACTIVITY_ROW_LIMIT);
  assert.match(markup, new RegExp(`event ${total - 1}<`));
  assert.match(markup, new RegExp(`event ${total - ACTIVITY_ROW_LIMIT}<`));
  assert.doesNotMatch(markup, new RegExp(`event ${total - ACTIVITY_ROW_LIMIT - 1}<`));
  assert.match(markup, new RegExp(`The view draws the newest <span class="mono">${ACTIVITY_ROW_LIMIT}</span> rows of <span class="mono">${total}</span>\\.`));
});

test("the activity view draws every row when the log holds no more than the row limit", () => {
  const markup = activityMarkup(activityModel(longLog(ACTIVITY_ROW_LIMIT)));
  assert.equal(rowCount(markup), ACTIVITY_ROW_LIMIT);
  assert.doesNotMatch(markup, /The view draws the newest/);
});

test("the activity view escapes the tailnet identifiers of the credential alert", () => {
  const hostile = '<img src=x onerror="alert(1)">';
  const markup = activityMarkup(activityModel([]), [
    { id: hostile, kind: "tailscale", credential_state: "rejected", reason: hostile },
  ]);
  assert.doesNotMatch(markup, /<img/);
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
  assert.doesNotMatch(markup, /onerror="/);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});
