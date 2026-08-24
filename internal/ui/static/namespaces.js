// The namespace view: the list, the panel, the add flow, and the removal dialog.
//
// Every function that builds a view is pure. It reads the payload of GET /api/status, the
// payload of GET /api/tailnet/{id}/detail, and the event list, and it returns a plain
// object. internal/ui/jstest asserts that output under Node, which is why the drawing code
// below holds no rule of its own.
//
// The view starts no timer. It reads the poll layer of app.js, and it asks for the detail
// of a tailnet once per poll interval.

import { registerView, requestJSON } from "./app.js";

/** The rule that config.IsValidID applies. internal/ui/namespaces_test.go compares it. */
export const IDENTIFIER_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/;

/** The sentence that the daemon returns for an identifier that it refuses. */
export const IDENTIFIER_RULE =
  "an id starts with a letter or a digit, holds the characters a-z A-Z 0-9 . _ - only, and holds 63 characters or fewer";

/** The age at which the console asks the daemon for the detail of a tailnet again. */
export const DETAIL_TTL_MS = 5000;

/** The marker that the view draws for a value that the daemon has not reported. */
const ABSENT = "—";

/**
 * validateIdentifier returns null for an identifier that the daemon accepts, and the
 * reason for one that it refuses. FR-console-32 requires this check before the request.
 */
export function validateIdentifier(id) {
  if (id === "") {
    return "an id is required";
  }
  if (!IDENTIFIER_PATTERN.test(id)) {
    return IDENTIFIER_RULE;
  }
  return null;
}

/** stateOf returns the reconciler state of one tailnet as a word and a dot tone. */
function stateOf(status, detail, id, removing) {
  if (removing) {
    return { word: "removing", tone: "" };
  }
  if (status.error_states && status.error_states[id]) {
    return { word: "error", tone: "crit" };
  }
  if (detail && detail.backend_state === "NeedsLogin") {
    return { word: "not authenticated", tone: "warn" };
  }
  if (status.paused_states && status.paused_states[id]) {
    return { word: "paused", tone: "" };
  }
  const actual = status.actual && status.actual[id];
  if (!actual || !actual.NsExists) {
    return { word: "no namespace", tone: "warn" };
  }
  if (!actual.DaemonHealthy) {
    return { word: "not healthy", tone: "crit" };
  }
  return { word: "healthy", tone: "ok" };
}

/**
 * reachabilityOf returns the measurement of one namespace as a word and a dot tone.
 *
 * The measurement answers a different question from the health of the process. Issue #172
 * found two tailnets that reported healthy while neither namespace reached anything, so
 * the view never merges the two words.
 */
function reachabilityOf(status, id) {
  const actual = status.actual && status.actual[id];
  const measured = actual && actual.measured_reachability;
  const state = measured && measured.state;
  if (state === "reachable") {
    return { word: "reachable", tone: "ok" };
  }
  if (state === "unreachable") {
    return { word: "unreachable", tone: "crit" };
  }
  return { word: "not probed", tone: "" };
}

/**
 * credentialOf returns the upstream policy credential problem of one tailnet as a dot
 * tone, a short word, and the reason of the daemon word for word. It returns null when the
 * credential is usable or the poll holds no policy entry for the tailnet yet.
 *
 * status.policy is the field that fetchConsoleState merges from GET /api/policy. Local
 * reachability and upstream policy are two independent systems (see
 * docs/specs/features/08-upstream-policy.md), so this state never replaces stateOf or
 * reachabilityOf; it is a second, additional line. See issue #287.
 *
 * The word and the reason are two fields, because the dot and word slot holds a short
 * lowercase word only. A reason in that slot is a sentence, and it overlapped the peer
 * count in the list row and it overflowed the panel. The word matches the word that the
 * policy view states for the same credential state. See issue #347.
 */
function credentialOf(status, id) {
  const entries = (status && status.policy) || [];
  const entry = entries.find((tailnet) => tailnet.id === id);
  if (!entry || entry.credential_state === "usable") {
    return null;
  }
  const word = entry.credential_state === "rejected" ? "credential rejected" : "no credential";
  return { tone: "crit", word, reason: entry.reason || "" };
}

/** addressOf returns the first tailnet address of a namespace, or the absent marker. */
function addressOf(detail) {
  if (detail && detail.tailscale_ips && detail.tailscale_ips.length > 0) {
    return detail.tailscale_ips[0];
  }
  return ABSENT;
}

/** peerWord states a peer count as a lowercase phrase. */
function peerWord(count) {
  return count === 1 ? "1 peer" : `${count} peers`;
}

/**
 * buildRows returns one row per declared tailnet, in identifier order.
 *
 * status is the body of GET /api/status. details maps an identifier to the body of
 * GET /api/tailnet/{id}/detail. options.selected names the open tailnet and
 * options.removing lists the tailnets that a removal request already named.
 * buildRows returns an empty list before the first poll returns.
 */
export function buildRows(status, details, options = {}) {
  if (!status || !status.desired) {
    return [];
  }
  const removing = options.removing || [];
  return Object.keys(status.desired)
    .sort()
    .map((id) => {
      const detail = details[id] || null;
      const isRemoving = removing.includes(id);
      const count = detail ? detail.peer_count : null;
      return {
        id,
        state: stateOf(status, detail, id, isRemoving),
        reachability: reachabilityOf(status, id),
        credential: credentialOf(status, id),
        peerCount: count,
        peers: count === null ? "no peer count yet" : peerWord(count),
        address: addressOf(detail),
        namespace: (status.actual && status.actual[id] && status.actual[id].NsName) || `ns-${id}`,
        selected: options.selected === id,
        muted: isRemoving,
        actionsDisabled: isRemoving,
      };
    });
}

/** hostAccessWord states the host access of a tailnet. A null value follows the file. */
function hostAccessWord(desired) {
  if (!desired || desired.HostAccess === null || desired.HostAccess === undefined) {
    return "default";
  }
  return desired.HostAccess ? "on" : "off";
}

/**
 * buildPanel returns the contextual panel of one tailnet, and null when no tailnet is
 * selected. FR-console-27 states that the panel closes when the selection clears.
 *
 * events is the list of GET /api/events, and the panel keeps the events of this tailnet.
 */
export function buildPanel(status, details, events, id) {
  if (!id || !status || !status.desired || !status.desired[id]) {
    return null;
  }
  const detail = details[id] || null;
  const desired = status.desired[id];
  const actual = (status.actual && status.actual[id]) || null;
  const peers = (detail && detail.peers) || [];
  const credential = credentialOf(status, id);

  return {
    id,
    state: stateOf(status, detail, id, false),
    reachability: reachabilityOf(status, id),
    credential,
    fields: [
      { label: "namespace", value: (actual && actual.NsName) || `ns-${id}` },
      { label: "address", value: addressOf(detail) },
      { label: "magicdns", value: (detail && detail.magic_dns_name) || ABSENT },
      { label: "control server", value: desired.ControlURL || ABSENT },
      { label: "host access", value: hostAccessWord(desired) },
      { label: "exit node", value: desired.ExitNode || ABSENT },
    ],
    peerCount: peers.length,
    peerLabel: peerWord(peers.length),
    peers: peers.map((peer) => ({
      name: peer.host_name || peer.dns_name || ABSENT,
      address: (peer.tailscale_ips && peer.tailscale_ips[0]) || ABSENT,
      online: peer.online === true,
    })),
    // The region carries a fixed height, so 200 peers scroll inside the panel rather
    // than push the actions off the view.
    scrolls: true,
    events: (events || [])
      .filter((event) => event.TailnetID === id)
      .map((event) => ({ time: event.Time, kind: event.Type, message: event.Message })),
    loginURL: (detail && detail.login_url) || null,
    lastError: (status.last_errors && status.last_errors[id]) || null,
    detailError: (detail && detail.error) || null,
    actions: {
      connect: !(actual && actual.DaemonHealthy),
      disconnect: Boolean(actual && actual.DaemonHealthy),
      remove: true,
    },
  };
}

/**
 * buildRemovalDialog returns the dialog of FR-console-29 from the plan that
 * GET /api/tailnet/{id}/removal-plan returned.
 *
 * The daemon states the commands, so the console repeats no rule. The removal runs no
 * logout, therefore the dialog names the logout command as the step that the operator
 * runs to deauthorize the node, and it states that the node stays authorized.
 */
export function buildRemovalDialog(plan) {
  return {
    id: plan.id,
    heading: `Remove ${plan.id}`,
    lead: "The removal does the following on this host:",
    commands: plan.commands.slice(),
    ruleSentence: `The removal deletes ${plan.rule_count} iptables rules that name the veth device ${plan.host_veth}.`,
    logoutSentence: `The removal runs no logout. Run tailscale logout inside ${plan.namespace} first, if you want the node logged out.`,
    authorizationSentence:
      "The node stays authorized on the control server. Remove it there as well, if you want it gone from the tailnet.",
    confirmLabel: `Remove ${plan.id}`,
  };
}

/**
 * createAddFlow returns the add flow of FR-console-31.
 *
 * options.request sends one request. It takes the route, the method, and the body, and it
 * rejects with the message that the daemon stated.
 *
 * The flow validates the identifier before it sends anything, and it clears the auth key
 * as soon as it sends the request. FR-console-33 states that the console never displays
 * the key after the submit, and a key that the flow does not hold cannot reach a view.
 */
export function createAddFlow(options = {}) {
  const request = options.request || ((route, method, body) => requestJSON(route, method, body));

  let fields = { id: "", authKey: "", controlURL: "", exitNode: "", hostAccess: false };
  let errors = {};
  let sending = false;

  function clearKey() {
    fields = { ...fields, authKey: "" };
  }

  return {
    setField(name, value) {
      fields = { ...fields, [name]: value };
    },

    /** state returns what a view draws. It never holds the auth key. */
    state() {
      return { fields: { ...fields, authKey: "" }, errors: { ...errors }, sending };
    },

    reset() {
      fields = { id: "", authKey: "", controlURL: "", exitNode: "", hostAccess: false };
      errors = {};
      sending = false;
    },

    /**
     * submit validates the whole form and then sends POST /api/tailnet/add.
     * submit returns {ok: true} on success, and {ok: false, errors} otherwise. It sends
     * no request when the identifier fails the rule of the daemon.
     */
    async submit() {
      errors = {};
      const reason = validateIdentifier(fields.id);
      if (reason !== null) {
        errors = { id: reason };
        return { ok: false, errors: { ...errors } };
      }

      const body = { id: fields.id };
      if (fields.authKey !== "") {
        body.auth_key = fields.authKey;
      }
      if (fields.controlURL !== "") {
        body.control_url = fields.controlURL;
      }
      if (fields.exitNode !== "") {
        body.exit_node = fields.exitNode;
      }
      if (fields.hostAccess === true || fields.hostAccess === false) {
        body.host_access = fields.hostAccess;
      }

      sending = true;
      try {
        await request("/api/tailnet/add", "POST", body);
        return { ok: true, errors: {} };
      } catch (err) {
        errors = { form: err && err.message ? err.message : String(err) };
        return { ok: false, errors: { ...errors } };
      } finally {
        // The key leaves the console whether the daemon accepted it or refused it. A
        // refused add asks the operator for the key again, which costs one field and
        // keeps the key out of every later view.
        clearKey();
        sending = false;
      }
    },
  };
}

/**
 * planDetailRequests returns the identifiers whose detail the console asks for now.
 *
 * ids are the declared tailnets, cache maps an identifier to {at, value}, now is the
 * current time in milliseconds, and ttl is the age at which an entry expires. The console
 * starts no timer of its own, so this runs on each poll.
 */
export function planDetailRequests(ids, cache, now, ttl) {
  return ids.filter((id) => {
    const entry = cache[id];
    return !entry || now - entry.at >= ttl;
  });
}

// ---------------------------------------------------------------------------
// The drawing. Everything below this line needs a document.
// ---------------------------------------------------------------------------

const details = {};
const events = { at: 0, value: [] };
const removing = new Set();
const pending = new Set();

let selected = null;
let dialog = null; // null, {kind: "add"}, or {kind: "remove", id, plan}
let toast = null;
let addFlow = null;
let section = null;
let snapshot = null;

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

/** stateSpan draws a state as a coloured dot and a lowercase word. FR-console-41. */
function stateSpan(state) {
  const wrap = el("span", "ns-state");
  wrap.append(el("span", state.tone ? `dot ${state.tone}` : "dot"));
  wrap.append(el("span", "ns-word", state.word));
  return wrap;
}

/** showToast states the result of one destructive action. */
function showToast(tone, message) {
  toast = { tone, message };
  render();
}

/** refresh asks the daemon for what the view needs, on the cadence of the poll layer. */
function refresh() {
  if (!snapshot || !snapshot.status || !snapshot.status.desired) {
    return;
  }
  const now = Date.now();
  const ids = Object.keys(snapshot.status.desired);

  for (const id of planDetailRequests(ids, details, now, DETAIL_TTL_MS)) {
    if (pending.has(id)) {
      continue;
    }
    pending.add(id);
    requestJSON(`/api/tailnet/${encodeURIComponent(id)}/detail`)
      .then((body) => {
        details[id] = { at: Date.now(), value: body };
      })
      .catch(() => {
        // The poll bar already states that the daemon does not answer. A failed detail
        // keeps the last value, and the row states no peer count until one arrives.
      })
      .finally(() => {
        pending.delete(id);
        render();
      });
  }

  // The removal completed when the poll no longer declares the tailnet.
  for (const id of Array.from(removing)) {
    if (!ids.includes(id)) {
      removing.delete(id);
      delete details[id];
    }
  }

  if (now - events.at >= DETAIL_TTL_MS && !pending.has("#events")) {
    pending.add("#events");
    requestJSON("/api/events")
      .then((body) => {
        events.at = Date.now();
        events.value = (body && body.events) || [];
      })
      .catch(() => {})
      .finally(() => {
        pending.delete("#events");
        render();
      });
  }
}

/** detailValues returns the detail bodies that the builders read. */
function detailValues() {
  const values = {};
  for (const [id, entry] of Object.entries(details)) {
    values[id] = entry.value;
  }
  return values;
}

/** send runs one mutating request and states the result. */
async function send(route, body, done) {
  try {
    await requestJSON(route, "POST", body);
    done(null);
  } catch (err) {
    done(err && err.message ? err.message : String(err));
  }
}

/** drawRow draws one row of the list. The row wraps when its line is too short. */
function drawRow(row) {
  const node = el("div", row.muted ? "ns-row ns-muted" : "ns-row");
  node.setAttribute("role", "option");
  node.setAttribute("aria-selected", row.selected ? "true" : "false");
  node.tabIndex = 0;

  node.append(el("span", "ns-id mono", row.id));
  node.append(stateSpan(row.state));
  node.append(stateSpan(row.reachability));
  if (row.credential) {
    node.append(stateSpan(row.credential));
  }
  node.append(el("span", "ns-peers mono", row.peers));

  const address = el("span", "ns-addr mono");
  address.textContent = `${row.address} · ${row.namespace}`;
  node.append(address);

  const open = () => {
    if (row.actionsDisabled) {
      return;
    }
    selected = row.selected ? null : row.id;
    render();
  };
  node.addEventListener("click", open);
  node.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      open();
    }
  });
  return node;
}

/** drawPanel draws the contextual panel. FR-console-26. */
function drawPanel(panel) {
  const aside = el("aside", "card ns-panel");
  aside.setAttribute("aria-label", `The tailnet ${panel.id}`);

  const head = el("div", "ns-panel-head");
  head.append(el("h2", "mono", panel.id));
  const close = el("button", "btn ns-close", "Close");
  close.type = "button";
  close.addEventListener("click", () => {
    selected = null;
    render();
  });
  head.append(close);
  aside.append(head);

  const states = el("div", "ns-panel-states");
  states.append(stateSpan(panel.state));
  states.append(stateSpan(panel.reachability));
  if (panel.credential) {
    states.append(stateSpan(panel.credential));
  }
  aside.append(states);

  const list = el("dl", "ns-kv");
  for (const field of panel.fields) {
    const pair = el("div", "ns-pair");
    pair.append(el("dt", undefined, field.label));
    pair.append(el("dd", "mono", field.value));
    list.append(pair);
  }
  aside.append(list);

  // The reason is a sentence of the daemon, so it takes a note that wraps. The definition
  // list holds one short value per row and it overflowed the panel with this text before.
  // See issue #347.
  if (panel.credential && panel.credential.reason) {
    const card = el("div", "ns-notice");
    card.append(el("span", "label", "Credential"));
    card.append(el("p", "note ns-reason", panel.credential.reason));
    aside.append(card);
  }
  if (panel.loginURL) {
    const card = el("div", "ns-notice");
    card.append(el("span", "label", "Not authenticated"));
    card.append(el("p", "note", "The node is not logged in. Open the login address, or set an auth key and reconcile."));
    card.append(el("code", "ns-cmds mono", panel.loginURL));
    aside.append(card);
  }
  if (panel.lastError) {
    const card = el("div", "ns-notice");
    card.append(el("span", "label", "Last error"));
    card.append(el("code", "ns-cmds mono", panel.lastError));
    aside.append(card);
  }
  if (panel.detailError) {
    const card = el("div", "ns-notice");
    card.append(el("span", "label", "No live status"));
    card.append(el("code", "ns-cmds mono", panel.detailError));
    aside.append(card);
  }

  aside.append(el("span", "label ns-section", `Peers · ${panel.peerLabel}`));
  if (panel.peers.length === 0) {
    aside.append(el("p", "note", "The daemon reports no peer. A peer arrives when the node reaches the control server."));
  } else {
    const peers = el("div", "ns-peers-list");
    for (const peer of panel.peers) {
      const line = el("div", "ns-peer");
      line.append(el("span", "mono", peer.name));
      line.append(el("span", "mono ns-peer-addr", peer.address));
      peers.append(line);
    }
    aside.append(peers);
  }

  aside.append(el("span", "label ns-section", "Recent events"));
  if (panel.events.length === 0) {
    aside.append(el("p", "note", "The daemon reports no event for this tailnet."));
  } else {
    const log = el("div", "ns-events");
    for (const event of panel.events.slice(-5).reverse()) {
      const line = el("div", "ns-event");
      line.append(el("span", "mono ns-event-kind", event.kind));
      line.append(el("span", "ns-event-text", event.message));
      log.append(line);
    }
    aside.append(log);
  }

  const acts = el("div", "ns-acts");
  if (panel.actions.connect) {
    const connect = el("button", "btn ns-sm", "Connect");
    connect.type = "button";
    connect.addEventListener("click", () => {
      send("/api/tailnet/connect", { id: panel.id }, (error) => {
        if (error) {
          showToast("crit", error);
        } else {
          render();
        }
      });
    });
    acts.append(connect);
  }
  if (panel.actions.disconnect) {
    const disconnect = el("button", "btn ns-sm", "Disconnect");
    disconnect.type = "button";
    disconnect.addEventListener("click", () => {
      send("/api/tailnet/disconnect", { id: panel.id }, (error) => {
        if (error) {
          showToast("crit", error);
        } else {
          render();
        }
      });
    });
    acts.append(disconnect);
  }
  const remove = el("button", "btn ns-sm ns-danger", "Remove");
  remove.type = "button";
  remove.addEventListener("click", () => openRemoval(panel.id));
  acts.append(remove);
  aside.append(acts);

  return aside;
}

/** openRemoval reads the plan of the daemon and then opens the dialog. */
function openRemoval(id) {
  dialog = { kind: "remove", id, plan: null, error: null };
  render();
  requestJSON(`/api/tailnet/${encodeURIComponent(id)}/removal-plan`)
    .then((plan) => {
      dialog = { kind: "remove", id, plan, error: null };
    })
    .catch((err) => {
      dialog = { kind: "remove", id, plan: null, error: err.message };
    })
    .finally(render);
}

/** drawRemoval draws the dialog that names every command of the removal. */
function drawRemoval() {
  const box = el("div", "ns-dialog");
  box.setAttribute("role", "dialog");
  box.setAttribute("aria-modal", "true");
  box.setAttribute("aria-label", `Remove ${dialog.id}`);

  if (dialog.error) {
    box.append(el("h3", undefined, `Remove ${dialog.id}`));
    box.append(el("p", "note", "The daemon states no removal plan, so the console shows no command."));
    box.append(el("code", "ns-cmds mono", dialog.error));
  } else if (!dialog.plan) {
    box.append(el("h3", undefined, `Remove ${dialog.id}`));
    box.append(el("p", "note", "The console reads the plan of the daemon."));
  } else {
    const view = buildRemovalDialog(dialog.plan);
    box.append(el("h3", undefined, view.heading));
    box.append(el("p", "note", view.lead));

    const cmds = el("div", "ns-cmds mono");
    for (const command of view.commands) {
      cmds.append(el("span", undefined, command));
    }
    box.append(cmds);

    box.append(el("p", "note", view.ruleSentence));
    box.append(el("p", "note", view.logoutSentence));
    box.append(el("p", "note", view.authorizationSentence));

    const acts = el("div", "ns-acts ns-dialog-acts");
    const cancel = el("button", "btn", "Cancel");
    cancel.type = "button";
    cancel.addEventListener("click", closeDialog);
    acts.append(cancel);

    const confirm = el("button", "btn ns-danger", view.confirmLabel);
    confirm.type = "button";
    confirm.addEventListener("click", () => {
      const id = dialog.id;
      removing.add(id);
      closeDialog();
      selected = null;
      send("/api/tailnet/remove", { id }, (error) => {
        if (error) {
          removing.delete(id);
          showToast("crit", error);
          return;
        }
        showToast("ok", `The daemon removed ${id}. The rows update on the next poll.`);
      });
    });
    acts.append(confirm);
    box.append(acts);
  }

  if (dialog.error || !dialog.plan) {
    const acts = el("div", "ns-acts ns-dialog-acts");
    const cancel = el("button", "btn", "Cancel");
    cancel.type = "button";
    cancel.addEventListener("click", closeDialog);
    acts.append(cancel);
    box.append(acts);
  }
  return box;
}

/** drawAdd draws the add flow. FR-console-31. */
function drawAdd() {
  const state = addFlow.state();
  const box = el("div", "ns-dialog");
  box.setAttribute("role", "dialog");
  box.setAttribute("aria-modal", "true");
  box.setAttribute("aria-label", "Add a tailnet");
  box.append(el("h3", undefined, "Add a tailnet"));
  box.append(el("p", "note", "The daemon writes the tailnet into the configuration file and the reconciler creates the namespace."));

  const form = el("form", "ns-form");

  const fields = [
    { name: "id", label: "Identifier", type: "text", hint: IDENTIFIER_RULE },
    { name: "authKey", label: "Auth key", type: "password", hint: "The console sends the key once and it never shows it again." },
    { name: "controlURL", label: "Control server URL", type: "text", hint: "Leave it empty for the default control server." },
    { name: "exitNode", label: "Exit node", type: "text", hint: "The address of the peer that carries the traffic." },
  ];
  for (const field of fields) {
    const row = el("label", "ns-field");
    row.append(el("span", "label", field.label));
    const input = el("input", "field");
    input.type = field.type;
    input.name = field.name;
    input.value = state.fields[field.name];
    input.addEventListener("input", () => addFlow.setField(field.name, input.value));
    row.append(input);
    row.append(el("span", "note", field.hint));
    if (state.errors[field.name]) {
      row.append(el("span", "ns-error", state.errors[field.name]));
    }
    form.append(row);
  }

  const access = el("label", "ns-field ns-check");
  const check = el("input");
  check.type = "checkbox";
  check.name = "hostAccess";
  check.checked = state.fields.hostAccess === true;
  check.addEventListener("change", () => addFlow.setField("hostAccess", check.checked));
  access.append(check);
  access.append(el("span", undefined, "Host access: the host reaches the peers of this tailnet."));
  form.append(access);

  if (state.errors.form) {
    form.append(el("span", "ns-error", state.errors.form));
  }

  const acts = el("div", "ns-acts ns-dialog-acts");
  const cancel = el("button", "btn", "Cancel");
  cancel.type = "button";
  cancel.addEventListener("click", closeDialog);
  acts.append(cancel);

  const submit = el("button", "btn primary", "Add tailnet");
  submit.type = "submit";
  acts.append(submit);
  form.append(acts);

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    addFlow.submit().then((result) => {
      if (result.ok) {
        closeDialog();
        showToast("ok", "The daemon accepted the tailnet. The rows update on the next poll.");
        return;
      }
      render();
    });
  });

  box.append(form);
  return box;
}

function closeDialog() {
  dialog = null;
  addFlow = null;
  render();
}

/** render draws the whole view from the last snapshot and the cached bodies. */
function render() {
  if (!section || !snapshot) {
    return;
  }
  section.replaceChildren();

  if (snapshot.loading) {
    const card = el("div", "card");
    card.append(el("span", "label", "Loading"));
    card.append(el("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  const values = detailValues();
  const rows = buildRows(snapshot.status, values, { selected, removing: Array.from(removing) });

  const head = el("div", "ns-head");
  const add = el("button", "btn primary", "Add tailnet");
  add.type = "button";
  add.addEventListener("click", () => {
    addFlow = createAddFlow({});
    dialog = { kind: "add" };
    render();
  });
  head.append(add);
  section.append(head);

  if (rows.length === 0) {
    const card = el("div", "card empty");
    card.append(el("span", "label", "Empty"));
    card.append(el("p", undefined, "No tailnet is configured. Add one, and this view lists it with its state, its peer count, and its address."));
    section.append(card);
  } else {
    const grid = el("div", "ns-grid");
    const list = el("div", "ns-list");
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-label", "The tailnets of this host");
    for (const row of rows) {
      list.append(drawRow(row));
    }
    grid.append(list);

    const panel = buildPanel(snapshot.status, values, events.value, selected);
    if (panel) {
      grid.append(drawPanel(panel));
    }
    section.append(grid);
  }

  if (toast) {
    const note = el("div", "ns-toast");
    note.setAttribute("role", "status");
    note.append(el("span", toast.tone === "ok" ? "dot ok" : "dot crit"));
    note.append(el("p", "bar-text", toast.message));
    const dismiss = el("button", "btn", "Dismiss");
    dismiss.type = "button";
    dismiss.addEventListener("click", () => {
      toast = null;
      render();
    });
    note.append(dismiss);
    section.append(note);
  }

  if (dialog) {
    const scrim = el("div", "ns-scrim");
    scrim.append(dialog.kind === "add" ? drawAdd() : drawRemoval());
    section.append(scrim);
  }
}

/** draw is the entry point that the shell calls on every poll and every navigation. */
function draw(node, current) {
  section = node;
  snapshot = current;
  refresh();
  render();
}

registerView("namespaces", draw);
