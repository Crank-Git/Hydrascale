// The DNS view, the activity view, and the settings view: the model and the serializer.
//
// Every function here is pure. It takes the poll payload and it returns a value. It
// touches no document, therefore internal/ui/jstest reads its output under Node and
// asserts the drawn result exactly. activity.js and settings.js hold every line that
// needs a document.
//
// One function escapes every value that the daemon reports. The console has no
// authentication, so an unescaped tailnet identifier, an unescaped event message, or an
// unescaped path is script injection into this page. See SA-5 of docs/specs/spec.md.
//
// These views hold no accent. The brand gives one accent use per view, and neither view
// holds an affirmative action, a selection, or an allowed path. See FR-console-40.

/** The three protection states of a namespace, as a dot tone and a lowercase word. */
const PROTECTED = { tone: "ok", word: "protected" };
const UNPROTECTED = { tone: "crit", word: "unprotected" };
const UNPROTECTED_ALLOWED = { tone: "warn", word: "unprotected" };

/** esc states a value that the daemon reported as text that an XML parser accepts. */
function esc(value) {
  return String(value === null || value === undefined ? "" : value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * timeParts splits an RFC 3339 time into the date and the time of day.
 *
 * The daemon writes the time, and the console shows no invented data, so a value that no
 * parser reads reaches the operator as the daemon wrote it. timeParts then returns the
 * whole value as the time and an empty date.
 */
function timeParts(value) {
  const text = value === null || value === undefined ? "" : String(value);
  const match = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})/.exec(text);
  if (!match) {
    return { date: "", time: text };
  }
  return { date: match[1], time: match[2] };
}

/** valueRow returns one row of a description list. A value is a machine value. */
function valueRow(label, value, reported = true) {
  if (!reported) {
    return `<div class="kv"><dt>${esc(label)}</dt><dd class="unset">not reported</dd></div>`;
  }
  return `<div class="kv"><dt>${esc(label)}</dt><dd class="mono">${esc(value)}</dd></div>`;
}

/** listRow returns one row whose value holds a line for each entry of values. */
function listRow(label, values) {
  const lines = values.map((value) => esc(value)).join("<br>");
  return `<div class="kv"><dt>${esc(label)}</dt><dd class="mono">${lines}</dd></div>`;
}

/** alert returns one banner of the tone warn or the tone crit. */
function alert(tone, sentences) {
  const body = sentences.map((sentence) => `<p>${esc(sentence)}</p>`).join("");
  return `<div class="alert ${tone}"><span class="dot ${tone}"></span><div>${body}</div></div>`;
}

// ---------------------------------------------------------------------------
// The DNS view
// ---------------------------------------------------------------------------

/**
 * dnsModel returns the drawn model of the DNS view.
 *
 * body is the body of GET /api/dns. It is null before the first poll returns, and the
 * model then reports ready false.
 *
 * A namespace whose overlay mount failed is an error state only when the configuration
 * key dns.allow_unprotected is false. Issue #76 settled that conflict between FR-dns-5
 * and FR-dns-6, so the model reads the key rather than the protected field alone.
 *
 * dnsModel is pure: the same body always produces the same model.
 */
export function dnsModel(body) {
  if (!body) {
    return {
      ready: false,
      mode: "",
      bindAddress: "",
      upstreams: [],
      allowUnprotected: false,
      hostPath: "",
      checksum: "",
      changedAt: "",
      changed: false,
      namespaces: [],
      splitDNS: [],
    };
  }

  const allowUnprotected = body.allow_unprotected === true;
  const namespaces = (body.namespaces || []).map((entry) => {
    let state = PROTECTED;
    if (entry.protected !== true) {
      state = allowUnprotected ? UNPROTECTED_ALLOWED : UNPROTECTED;
    }
    return {
      id: entry.id || "",
      tone: state.tone,
      word: state.word,
      error: entry.error || "",
    };
  });

  return {
    ready: true,
    mode: body.mode || "",
    bindAddress: body.bind_address || "",
    upstreams: body.upstreams || [],
    allowUnprotected,
    hostPath: body.host_resolv_path || "",
    checksum: body.host_resolv_sha256 || "",
    changedAt: body.host_resolv_changed_at || "",
    changed: (body.host_resolv_changed_at || "") !== "",
    namespaces,
    splitDNS: (body.split_dns || []).map((entry) => ({
      tailnet: entry.tailnet || "",
      domains: entry.domains || [],
      conflict: entry.conflict || "",
    })),
  };
}

/**
 * dnsMarkup returns the whole DNS view as markup.
 *
 * model comes from dnsModel. Every value passes through esc, therefore no value that the
 * daemon reports reaches the page as markup. FR-console-34 and FR-console-35.
 */
export function dnsMarkup(model) {
  const parts = [];

  if (!model.ready) {
    parts.push(
      '<section class="frame empty">' +
        "<p class=\"note\">The daemon reports no DNS state yet. This view shows the resolver mode, the bind address, the upstream servers, and the protected state of every namespace.</p></section>",
    );
    return parts.join("");
  }

  // FR-console-35. The daemon reports a change to the host file and it repairs no file,
  // so the warning states the fact and names the file.
  if (model.changed) {
    const at = timeParts(model.changedAt);
    parts.push(
      alert("warn", [
        `The host file changed at ${at.time}.`,
        "The daemon reports the change and it writes no host file. Read the file, repair it, and the daemon reports the new checksum on the next tick.",
      ]),
    );
  }

  const unprotected = model.namespaces.filter((entry) => entry.word === "unprotected");
  if (unprotected.length > 0 && !model.allowUnprotected) {
    parts.push(
      alert("crit", [
        `The overlay mount failed for ${unprotected.map((entry) => entry.id).join(", ")}.`,
        "The tailscaled of that namespace writes the host file. Read the daemon log, and set dns.allow_unprotected to true to start the tailnet without the mount.",
      ]),
    );
  }

  const resolver = [
    valueRow("mode", model.mode, model.mode !== ""),
    valueRow("bind address", model.bindAddress, model.bindAddress !== ""),
  ];
  if (model.upstreams.length > 0) {
    resolver.push(listRow("upstreams", model.upstreams));
  }
  parts.push(
    `<section class="frame"><div class="frame-head"><h2 class="frame-title">Resolver</h2></div><dl class="kv-list">${resolver.join("")}</dl>` +
      (model.upstreams.length === 0
        ? '<p class="note">The forwarder reports no upstream. An upstream arrives when the daemon reads the host file, or when a tailnet reports a MagicDNS server.</p>'
        : "") +
      "</section>",
  );

  // FR-split-9. A split DNS domain that two tailnets claim is an error state, so the
  // console draws the alert. An empty set is no error, so absence is a note and no card
  // draws a denial.
  const splitAlerts = model.splitDNS
    .filter((entry) => entry.conflict !== "")
    .map((entry) =>
      alert("crit", [entry.conflict.replace(/^./, (letter) => letter.toUpperCase()) + "."]),
    )
    .join("");
  const splitRows = model.splitDNS
    .filter((entry) => entry.domains.length > 0)
    .map(
      (entry) =>
        '<div class="nsrow">' +
        `<span class="id mono">${esc(entry.tailnet)}</span>` +
        `<span class="domains mono">${entry.domains.map((domain) => esc(domain)).join("<br>")}</span>` +
        "</div>",
    )
    .join("");
  parts.push(
    '<section class="frame"><div class="frame-head"><h2 class="frame-title">Split DNS</h2></div>' +
      '<p class="note">The control server of a tailnet routes each split domain to the resolver of that tailnet.</p>' +
      (splitRows === "" && splitAlerts === ""
        ? '<p class="note">No tailnet reports a split DNS domain.</p>'
        : `<div class="nsrows">${splitRows}</div>` + splitAlerts) +
      "</section>",
  );

  const rows = model.namespaces
    .map(
      (entry) =>
        '<div class="nsrow">' +
        `<span class="id mono">${esc(entry.id)}</span>` +
        `<span class="st"><span class="dot ${entry.tone}"></span>${esc(entry.word)}</span>` +
        (entry.error !== "" ? `<span class="err mono">${esc(entry.error)}</span>` : "") +
        "</div>",
    )
    .join("");
  parts.push(
    '<section class="frame"><div class="frame-head"><h2 class="frame-title">Namespace protection</h2></div>' +
      '<p class="note">Each row states whether the private /etc overlay is mounted in that namespace. A namespace with the mount cannot write the host file.</p>' +
      (model.allowUnprotected
        ? '<p class="note">The configuration key dns.allow_unprotected is true, so a namespace starts without the mount and it holds no error state.</p>'
        : "") +
      (rows === ""
        ? '<p class="note">The daemon runs no namespace. A row arrives when the reconciler creates a namespace for a declared tailnet.</p>'
        : `<div class="nsrows">${rows}</div>`) +
      "</section>",
  );

  const at = timeParts(model.changedAt);
  const hostFile = [
    valueRow("path", model.hostPath, model.hostPath !== ""),
    valueRow("sha256", model.checksum, model.checksum !== ""),
    valueRow("last change", `${at.date} ${at.time}`.trim(), model.changedAt !== ""),
  ];
  parts.push(
    `<section class="frame"><div class="frame-head"><h2 class="frame-title">Host file</h2></div><dl class="kv-list">${hostFile.join("")}</dl>` +
      (model.changed
        ? ""
        : '<p class="note">The checksum matches the value that the daemon read at the start.</p>') +
      "</section>",
  );

  return parts.join("");
}

// ---------------------------------------------------------------------------
// The activity view
// ---------------------------------------------------------------------------

/**
 * policyCredentialNote returns the markup of one alert naming every tailnet whose policy
 * credential the control server rejected, and an empty string when no tailnet holds a
 * rejected credential or the poll reports no policy entry yet.
 *
 * entries is the field policy of the merged poll body, from GET /api/policy. Local
 * reachability and upstream policy are two independent systems (see
 * docs/specs/features/08-upstream-policy.md), so this note names a policy fact and it
 * writes no event into the log. See issue #287.
 *
 * A credential is optional, so an absent credential is not a fault and draws no alert.
 * The board of the overview and the policy view state it.
 */
function policyCredentialNote(entries) {
  const ids = (entries || [])
    .filter((entry) => entry.credential_state === "rejected")
    .map((entry) => entry.id);
  if (ids.length === 0) {
    return "";
  }
  const sentence = ids.length === 1
    ? `The control server rejected the policy credential of ${ids[0]}.`
    : `The control server rejected the policy credential of these tailnets: ${ids.join(", ")}.`;
  const body = `<p>${esc(sentence)} <a href="#/policy">The policy view</a> states the reason.</p>`;
  return `<div class="alert crit"><span class="dot crit"></span><div>${body}</div></div>`;
}

/**
 * ACTIVITY_ROW_LIMIT is the count of rows that the activity view draws.
 *
 * The daemon keeps the newest 1000 events in memory. The view drew every one of them. A
 * capture of the page reached 44681 pixels, which is 30 viewports. See issue #355.
 */
export const ACTIVITY_ROW_LIMIT = 100;

/** ROUTINE_EVENTS names the event types that every reconcile tick records. */
export const ROUTINE_EVENTS = new Set(["reconcile_start", "reconcile_apply", "reconcile_complete", "action_ok"]);

/**
 * activityModel returns the rows that the activity view draws, newest first.
 *
 * events is the field events of GET /api/events, oldest first. options.ticks shows the
 * reconcile ticks, and options.tailnet keeps the rows of one tailnet, or every row when it
 * is the empty string.
 *
 * A reconcile tick records four kinds of routine event, and an incident hides between
 * them. The model therefore folds each tick into one row: the time of its start, the
 * message of its end, and the actions it applied. A row of the kind "event" is any other
 * event, which the view always shows.
 */
export function activityModel(events, options = {}) {
  const ticks = options.ticks === true;
  const tailnet = options.tailnet || "";
  const rows = [];
  let open = null;

  for (const event of events || []) {
    if (!event) {
      continue;
    }
    const type = event.Type || "";
    if (!ROUTINE_EVENTS.has(type)) {
      rows.push({ kind: "event", event });
      continue;
    }
    if (type === "reconcile_start") {
      open = { kind: "tick", time: event.Time, message: "", actions: [], done: false };
      rows.push(open);
      continue;
    }
    if (open === null) {
      continue;
    }
    if (type === "action_ok") {
      open.actions.push({ tailnet: event.TailnetID || "", action: event.Message || "" });
    } else if (type === "reconcile_complete") {
      open.message = event.Message || "";
      open.done = true;
      open = null;
    }
  }

  const all = rows.reverse();
  const notable = all.filter((row) => row.kind === "event").length;
  const kept = all
    .filter((row) => ticks || row.kind === "event")
    .map((row) => {
      if (row.kind === "event" || tailnet === "") {
        return row;
      }
      return { ...row, actions: row.actions.filter((entry) => entry.tailnet === tailnet) };
    })
    .filter((row) => {
      if (tailnet === "") {
        return true;
      }
      return row.kind === "event" ? row.event.TailnetID === tailnet : row.actions.length > 0;
    });

  const tailnets = [...new Set((events || []).map((event) => event && event.TailnetID).filter(Boolean))].sort();
  return {
    rows: kept.slice(0, ACTIVITY_ROW_LIMIT),
    matched: kept.length,
    scanned: (events || []).length,
    notable,
    tailnets,
  };
}

/** tickSummary states the actions of one tick, grouped by tailnet. */
function tickSummary(actions) {
  const byTailnet = new Map();
  for (const entry of actions) {
    const key = entry.tailnet || "host";
    if (!byTailnet.has(key)) {
      byTailnet.set(key, []);
    }
    byTailnet.get(key).push(entry.action);
  }
  // The tailnets sort by name, so one tailnet holds one place in every row.
  return [...byTailnet.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([id, list]) => `${id}: ${list.join(", ")}`)
    .join(" · ");
}

/**
 * activityMarkup returns the event table of the activity view.
 *
 * model comes from activityModel. A time, a kind, and a tailnet identifier are machine
 * values, and a message is the sentence that the daemon wrote. policyEntries is the field
 * policy of the merged poll body, and it adds one alert above the table when the control
 * server rejected the credential of a tailnet. The table states one date for each day.
 */
export function activityMarkup(model, policyEntries = [], options = {}) {
  const note = policyCredentialNote(policyEntries);

  if (model.scanned === 0) {
    return (
      note +
      '<div class="frame empty"><p class="note">The daemon reports no event. An event arrives when the reconciler creates a namespace, connects a tailnet, or writes the access rules.</p></div>'
    );
  }

  if (model.rows.length === 0) {
    const scope = options.tailnet ? ` for ${esc(options.tailnet)}` : "";
    return (
      note +
      `<p class="note log-empty">No event other than a routine reconcile tick${scope} in the newest <span class="mono">${model.scanned}</span> events. Select Every event to show the ticks.</p>`
    );
  }

  const body = [];
  let day = null;
  for (const row of model.rows) {
    const at = timeParts(row.kind === "tick" ? row.time : row.event.Time);
    if (at.date !== day) {
      day = at.date;
      body.push(`<tr class="day"><th colspan="4" scope="colgroup" class="mono">${esc(at.date || "no date")}</th></tr>`);
    }
    if (row.kind === "tick") {
      const summary = tickSummary(row.actions);
      body.push(
        '<tr class="log-row tick">' +
          `<td class="mono t">${esc(at.time)}</td>` +
          '<td class="mono n"></td>' +
          '<td class="mono k">reconcile tick</td>' +
          `<td class="m">${esc(row.done ? row.message : "in progress")}` +
          (summary !== "" ? `<span class="mono d">${esc(summary)}</span>` : "") +
          "</td></tr>",
      );
      continue;
    }
    const event = row.event;
    body.push(
      '<tr class="log-row">' +
        `<td class="mono t"><time datetime="${esc(event.Time)}">${esc(at.time)}</time></td>` +
        `<td class="mono n">${esc(event.TailnetID || "")}</td>` +
        `<td class="mono k">${esc(event.Type || "")}</td>` +
        `<td class="m">${esc(event.Message || "")}</td>` +
        "</tr>",
    );
  }

  const cap = model.matched > model.rows.length
    ? `<p class="note log-cap">The view draws the newest <span class="mono">${model.rows.length}</span> rows of <span class="mono">${model.matched}</span>.</p>`
    : "";

  return (
    note +
    '<table class="log"><thead><tr><th scope="col">Time</th><th scope="col">Tailnet</th><th scope="col">Event</th><th scope="col">Message</th></tr></thead>' +
    `<tbody>${body.join("")}</tbody></table>` +
    cap
  );
}

// ---------------------------------------------------------------------------
// The settings view
// ---------------------------------------------------------------------------

/**
 * settingsModel returns the rows of the settings view.
 *
 * status is the body of GET /api/status, interval is the poll interval in milliseconds,
 * and host is the address that the browser reached. The model names five values and it
 * reads no other field of the status body, therefore no credential of a later route
 * reaches this view. See SA-1.
 *
 * A row that the daemon does not report holds reported false, and the view states that in
 * words rather than an invented value. FR-console-37.
 */
export function settingsModel(status, interval, host) {
  const body = status || {};
  const address = body.console_address || host || "";
  const seconds = Math.round((interval || 0) / 1000);

  const rows = [
    { label: "configuration file", value: body.config_path || "" },
    { label: "control socket", value: body.socket_path || "" },
    { label: "console address", value: address },
    { label: "poll interval", value: `${seconds}s` },
    { label: "version", value: body.server_version || "" },
  ].map((row) => ({ ...row, reported: row.value !== "" }));

  const paths = rows.filter((row) => row.label !== "poll interval");
  return { rows, ready: paths.some((row) => row.reported) };
}

/**
 * settingsMarkup returns the whole settings view as markup.
 * model comes from settingsModel. FR-console-37 and FR-console-38.
 */
export function settingsMarkup(model) {
  const parts = [];

  if (!model.ready) {
    parts.push(
      '<section class="frame empty">' +
        "<p class=\"note\">The daemon reports no path yet. This view shows the configuration path, the socket path, the console address, the poll interval, and the version when the poll succeeds.</p></section>",
    );
  }

  const rows = model.rows.map((row) => valueRow(row.label, row.value, row.reported)).join("");
  parts.push(`<section class="frame"><div class="frame-head"><h2 class="frame-title">Daemon</h2></div><dl class="kv-list">${rows}</dl></section>`);

  // FR-console-38. The section "The console has no authentication" of docs/specs/spec.md
  // records the accepted risk and it names the four controls that reduce it.
  parts.push(
    '<section class="frame"><div class="frame-head"><h2 class="frame-title">Console</h2></div>' +
      alert("warn", [
        "The console has no authentication. Any local account on this host reaches this address and drives the daemon, which runs as root.",
        "The daemon binds a loopback address only. Reach the console of another host through an SSH tunnel rather than through a wider bind address.",
      ]) +
      '<p class="note">The daemon records one event for every mutating request on the console listener. <a href="#/activity">The activity view</a> lists them.</p>' +
      "</section>",
  );

  return parts.join("");
}
