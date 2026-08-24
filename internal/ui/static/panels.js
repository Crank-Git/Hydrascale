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
      '<section class="card empty"><span class="label">Empty</span>' +
        "<p>The daemon reports no DNS state yet. This view shows the resolver mode, the bind address, the upstream servers, and the protected state of every namespace.</p></section>",
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
    `<section class="card"><h2>Resolver</h2><dl class="kv-list">${resolver.join("")}</dl>` +
      (model.upstreams.length === 0
        ? '<p class="note">The forwarder reports no upstream. An upstream arrives when the daemon reads the host file, or when a tailnet reports a MagicDNS server.</p>'
        : "") +
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
    '<section class="card"><h2>Namespace protection</h2>' +
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
    `<section class="card"><h2>Host file</h2><dl class="kv-list">${hostFile.join("")}</dl>` +
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
 * activityRows returns every event of the log, newest first.
 *
 * events is the field events of GET /api/events. The daemon appends one entry per event,
 * so the newest entry is the last one. FR-console-36.
 */
export function activityRows(events) {
  return (events || [])
    .slice()
    .reverse()
    .map((event) => {
      const at = timeParts(event.Time);
      return {
        date: at.date,
        time: at.time,
        kind: event.Type || "",
        tailnet: event.TailnetID || "",
        message: event.Message || "",
      };
    });
}

/**
 * policyCredentialNote returns the markup of one note line naming every tailnet that
 * holds no usable policy credential, and an empty string when every declared tailnet
 * holds one or the poll reports no policy entry yet.
 *
 * entries is the field policy of the merged poll body, from GET /api/policy. Local
 * reachability and upstream policy are two independent systems (see
 * docs/specs/features/08-upstream-policy.md), so this note names a policy fact and it
 * writes no event into the log. See issue #287.
 */
function policyCredentialNote(entries) {
  const ids = (entries || [])
    .filter((entry) => entry.credential_state !== "usable")
    .map((entry) => entry.id);
  if (ids.length === 0) {
    return "";
  }
  const sentence = ids.length === 1
    ? `The tailnet ${ids[0]} needs a policy credential. Open the Policy tab for the reason.`
    : `These tailnets need a policy credential: ${ids.join(", ")}. Open the Policy tab for the reason.`;
  return alert("crit", [sentence]);
}

/**
 * ACTIVITY_ROW_LIMIT is the count of event rows that the activity view draws.
 *
 * The daemon keeps the newest 1000 events in memory, and the view drew every one of them.
 * A capture of the page reached 44500 pixels, which is 30 viewports. See issue #355.
 *
 * The view draws a count of the events that it hides, and it holds no control that draws
 * the rest. The poll rewrites the whole section on every tick, therefore a control that
 * opens the rest loses its state within one poll interval.
 */
export const ACTIVITY_ROW_LIMIT = 100;

/**
 * activityMarkup returns the whole activity view as markup.
 * rows comes from activityRows. A time, a kind, and a tailnet identifier are machine
 * values, and a message is the sentence that the daemon wrote. policyEntries is the field
 * policy of the merged poll body, and it adds one note line above the event list when a
 * declared tailnet holds no usable credential.
 *
 * The view draws the newest ACTIVITY_ROW_LIMIT rows, and it states the count of the log
 * when the log holds more. See issue #355.
 */
export function activityMarkup(rows, policyEntries = []) {
  const note = policyCredentialNote(policyEntries);

  if (rows.length === 0) {
    return (
      note +
      '<section class="card empty"><span class="label">Empty</span>' +
      "<p>The daemon reports no event. An event arrives when the reconciler creates a namespace, connects a tailnet, or writes the access rules.</p></section>"
    );
  }

  const drawn = rows.slice(0, ACTIVITY_ROW_LIMIT);
  const cap = drawn.length < rows.length
    ? `<p class="note">The view draws the newest ${drawn.length} events of ${rows.length}.</p>`
    : "";

  const list = drawn
    .map(
      (row) =>
        '<div class="ev">' +
        `<time class="mono" datetime="${esc(row.date)}">${esc(row.time)}</time>` +
        `<span class="ev-date mono">${esc(row.date)}</span>` +
        `<span class="ev-kind mono">${esc(row.kind)}</span>` +
        (row.tailnet !== "" ? `<span class="ev-net mono">${esc(row.tailnet)}</span>` : '<span class="ev-net"></span>') +
        `<p>${esc(row.message)}</p>` +
        "</div>",
    )
    .join("");

  return (
    note +
    '<section class="card"><span class="label">Events</span>' +
    cap +
    `<div class="events">${list}</div>` +
    '<p class="note">The daemon holds the newest events in memory. It records one event for every mutating request on the console listener.</p>' +
    "</section>"
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
      '<section class="card empty"><span class="label">Empty</span>' +
        "<p>The daemon reports no path yet. This view shows the configuration path, the socket path, the console address, the poll interval, and the version when the poll succeeds.</p></section>",
    );
  }

  const rows = model.rows.map((row) => valueRow(row.label, row.value, row.reported)).join("");
  parts.push(`<section class="card"><h2>Daemon</h2><dl class="kv-list">${rows}</dl></section>`);

  // FR-console-38. The section "The console has no authentication" of docs/specs/spec.md
  // records the accepted risk and it names the four controls that reduce it.
  parts.push(
    '<section class="card"><h2>Console</h2>' +
      alert("warn", [
        "The console has no authentication. Any local account on this host reaches this address and drives the daemon, which runs as root.",
        "The daemon binds a loopback address only. Reach the console of another host through an SSH tunnel rather than through a wider bind address.",
      ]) +
      '<p class="note">The daemon records one event for every mutating request on the console listener. <a href="#/activity">The activity view</a> lists them.</p>' +
      "</section>",
  );

  return parts.join("");
}
