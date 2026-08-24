// The console shell: the navigation, the router, and the poll layer.
//
// The console is a static single-page application with no build step, no package
// manager, and no framework. The browser loads this file as a module.
//
// The poll layer takes its transport and its clock as arguments, so internal/ui/jstest
// drives it under Node with no browser and no network. The shell wiring runs only when a
// document exists, which is why an import under Node starts no timer.
//
// Issue #143, issue #144 and issue #145 add one module per view. A view module calls
// registerView and reads the snapshot that the poll layer gives it.

/** The route that the console polls. */
export const STATUS_ROUTE = "/api/status";

/** The route that holds the local rule set and the node list of the topology. */
export const ACCESS_ROUTE = "/api/access";

/** The route that holds the event log. */
export const EVENTS_ROUTE = "/api/events";

/** The route that holds the resolver settings and the DNS protection state. */
export const DNS_ROUTE = "/api/dns";

/** The route that lists every tailnet with its control server kind and credential state. */
export const POLICY_ROUTE = "/api/policy";

/** The poll interval that the console uses when the operator sets none. FR-console-15. */
export const DEFAULT_POLL_INTERVAL_MS = 5000;

/** The smallest interval that the settings view accepts. */
export const MINIMUM_POLL_INTERVAL_MS = 1000;

/** The time of continuous failure after which the console offers a manual retry. */
export const RETRY_AFTER_MS = 60000;

/** The key that holds the interval between one browser session and the next. */
export const POLL_INTERVAL_KEY = "hydrascale.poll-interval";

/** The header that every mutating console route requires. */
export const CONSOLE_HEADER = "X-Hydrascale-Console";

/**
 * VIEWS is the navigation, in order. FR-console-12 names five entries, and FR-policy-22
 * adds the policy view as the sixth.
 *
 * Each entry states the heading of the view, the sentence under it, the empty state, and
 * the sentence that the shell draws until the view itself lands. FR-console-17 states
 * that the console shows no invented data, so a view with nothing to show states what
 * would fill it.
 */
export const VIEWS = [
  {
    id: "overview",
    heading: "Overview",
    lead: "The state of every tailnet on this host.",
    empty: "No tailnet is configured. Add one, and this view shows the tailnet count, the peer count, the reconciler state, and the topology.",
    pending: "This view shows the tailnet count, the peer count, the reconciler state, the topology, and the five newest events.",
  },
  {
    id: "namespaces",
    heading: "Namespaces",
    lead: "One network namespace per tailnet, and the peers inside it.",
    empty: "No tailnet is configured. Add one, and this view lists it with its state, its peer count, and its address.",
    pending: "This view lists every tailnet with its state, its peer count, and its address, and it opens a panel on a selection. The view arrives with issue #144.",
  },
  {
    id: "access",
    heading: "Access",
    lead: "The local rules that this host enforces between the tailnets, the host, and the internet.",
    empty: "No tailnet is configured. Add one, and this view shows the paths that the host allows.",
    pending: "This view shows the paths that the host allows, and it edits them. The view arrives with the access editor of issue #147.",
  },
  {
    id: "policy",
    heading: "Policy",
    lead: "The access policy that each control server holds. A policy change affects every device in the tailnet, not only this host.",
    empty: "The daemon declares no tailnet. Add one, and this view lists it with its control server kind and its credential state.",
    pending: "This view lists every tailnet with its control server kind and its credential state, and it shows the policy document of a selection.",
  },
  {
    id: "activity",
    heading: "Activity",
    lead: "What the daemon did, newest first.",
    empty: "The daemon reports no event. An event arrives when the reconciler creates a namespace, connects a tailnet, or writes the access rules.",
    pending: "This view lists every event with its time, its kind, its tailnet, and its message.",
  },
  {
    id: "settings",
    heading: "Settings",
    lead: "The resolver, the host file, and the console that serves this page.",
    empty: "The daemon reports no configuration path yet. This view shows the configuration path, the socket path, and the console address when the poll succeeds.",
    pending: "This view shows the resolver, the protected state of every namespace, the host file, and the paths that the daemon reads.",
  },
];

/**
 * consoleRequestInit builds the request options of one console request.
 *
 * A mutating route requires the console header, and a read route does not. See
 * FR-console-8. Every path is relative, so the browser reaches the console origin and no
 * other host.
 *
 * @param {string} method the HTTP method, in upper case.
 * @param {object} [body] the JSON body of a mutating request.
 */
export function consoleRequestInit(method, body) {
  const init = { method, headers: {} };
  const mutating = ["POST", "PUT", "PATCH", "DELETE"].includes(method);
  if (mutating) {
    init.headers[CONSOLE_HEADER] = "1";
  }
  if (body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  return init;
}

/**
 * requestJSON sends one request to the console origin and reads the JSON reply.
 * It rejects with the message that the daemon returned, because every route states the
 * reason in the error field.
 * The rejected error carries the status in the field status. A caller that classifies a
 * failure reads that status, because a message is text for a person and the status is the
 * value that the daemon owns.
 */
export async function requestJSON(route, method = "GET", body) {
  const reply = await fetch(route, consoleRequestInit(method, body));
  const text = await reply.text();
  let parsed = null;
  if (text !== "") {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }
  if (!reply.ok) {
    const stated = parsed && parsed.error ? parsed.error : `${reply.status} ${reply.statusText}`;
    const failure = new Error(stated);
    failure.status = reply.status;
    throw failure;
  }
  return parsed;
}

/**
 * fetchConsoleState reads the five routes of one poll and returns one body.
 *
 * The status route holds neither the peer count, nor the local rule set, nor the event
 * log, nor the DNS state, nor the policy credential state, and the console shows all
 * five. The console therefore reads the five routes together on one tick rather than on
 * five timers, so the whole console still holds one data source and one cadence. See
 * FR-console-15.
 *
 * The policy route is GET /api/policy, the list route. It reports the credential state of
 * every declared tailnet and it never reaches a control server, unlike
 * GET /api/policy/{id}, so it never fails the whole poll on a missing credential.
 *
 * A failure of any one route fails the poll, which marks the state stale. The five routes
 * come from the one daemon on the one listener, so a partial answer states nothing that a
 * whole answer does not.
 *
 * @param {function} [request] the transport, which a test replaces.
 */
export async function fetchConsoleState(request = requestJSON) {
  const [status, access, events, dns, policy] = await Promise.all([
    request(STATUS_ROUTE),
    request(ACCESS_ROUTE),
    request(EVENTS_ROUTE),
    request(DNS_ROUTE),
    request(POLICY_ROUTE),
  ]);
  return {
    ...status,
    access_model: access,
    events: (events && events.events) || [],
    dns,
    policy: (policy && policy.tailnets) || [],
  };
}

/**
 * createPollLayer builds the one data source of the console.
 *
 * The layer polls STATUS_ROUTE on an interval. On a success it replaces the state. On a
 * failure it keeps the last state, marks it stale, and holds the time of the last
 * success, which FR-console-16 requires. It keeps polling after a failure, so a daemon
 * that restarts needs no action from the operator; after RETRY_AFTER_MS of continuous
 * failure the snapshot offers a manual retry as well.
 *
 * A daemon that restarts with another version answers with another server_version. The
 * embedded console came from the old binary, so the snapshot states that the operator
 * reloads the page.
 */
export function createPollLayer(options = {}) {
  const {
    fetchStatus = fetchConsoleState,
    now = () => Date.now(),
    setTimer = (fn, ms) => setTimeout(fn, ms),
    clearTimer = (handle) => clearTimeout(handle),
    interval = DEFAULT_POLL_INTERVAL_MS,
  } = options;

  let period = interval;
  let running = false;
  let timer = null;
  let version = null;
  const readers = [];

  const state = {
    status: null,
    loading: true,
    stale: false,
    error: null,
    lastSuccessAt: null,
    failedSince: null,
    retryOffered: false,
    restarted: false,
  };

  function snapshot() {
    return { ...state, interval: period };
  }

  function notify() {
    const current = snapshot();
    for (const reader of readers.slice()) {
      reader(current);
    }
  }

  function cancel() {
    if (timer !== null) {
      clearTimer(timer);
      timer = null;
    }
  }

  function schedule() {
    if (!running) {
      return;
    }
    cancel();
    timer = setTimer(poll, period);
  }

  function onSuccess(body) {
    if (version !== null && body && body.server_version && body.server_version !== version) {
      state.restarted = true;
    }
    if (body && body.server_version) {
      version = body.server_version;
    }
    state.status = body;
    state.loading = false;
    state.stale = false;
    state.error = null;
    state.lastSuccessAt = now();
    state.failedSince = null;
    state.retryOffered = false;
  }

  function onFailure(err) {
    state.loading = false;
    state.error = err && err.message ? err.message : String(err);
    if (state.status !== null) {
      state.stale = true;
    }
    if (state.failedSince === null) {
      state.failedSince = now();
    }
    state.retryOffered = now() - state.failedSince >= RETRY_AFTER_MS;
  }

  async function poll() {
    cancel();
    try {
      onSuccess(await fetchStatus());
    } catch (err) {
      onFailure(err);
    }
    notify();
    schedule();
  }

  return {
    snapshot,

    /** subscribe adds a reader and calls it at once with the current state. */
    subscribe(reader) {
      readers.push(reader);
      reader(snapshot());
      return () => {
        const at = readers.indexOf(reader);
        if (at >= 0) {
          readers.splice(at, 1);
        }
      };
    },

    /** start polls at once and then on every interval. */
    start() {
      if (running) {
        return;
      }
      running = true;
      poll();
    },

    /** stop cancels the timer. The layer keeps its state. */
    stop() {
      running = false;
      cancel();
    },

    /** refresh polls at once and moves the next poll a whole interval away. */
    refresh() {
      poll();
    },

    getInterval() {
      return period;
    },

    /** setInterval refuses a value that is not a whole number of milliseconds, and a
     *  value below MINIMUM_POLL_INTERVAL_MS. */
    setInterval(ms) {
      if (!Number.isInteger(ms) || ms < MINIMUM_POLL_INTERVAL_MS) {
        throw new Error(`the poll interval must be a whole number of milliseconds, ${MINIMUM_POLL_INTERVAL_MS} or more`);
      }
      period = ms;
      schedule();
      notify();
    },
  };
}

// ---------------------------------------------------------------------------
// The shell. Everything below this line needs a document.
// ---------------------------------------------------------------------------

const mounts = new Map();

/**
 * registerView names the function that draws one view.
 *
 * The function reads the section element and the current snapshot, and it runs again on
 * every poll and on every navigation. A view that no module registers shows its empty
 * state and the sentence that VIEWS holds.
 */
export function registerView(id, draw) {
  mounts.set(id, draw);
  if (typeof document !== "undefined") {
    drawCurrent();
  }
}

let poller = null;
let current = VIEWS[0].id;
let latest = null;

/** viewFor returns the view entry of an identifier, or the first entry. */
function viewFor(id) {
  return VIEWS.find((view) => view.id === id) || VIEWS[0];
}

/** readSavedInterval reads the interval that a previous session stored. */
function readSavedInterval() {
  try {
    const saved = Number(window.localStorage.getItem(POLL_INTERVAL_KEY));
    if (Number.isInteger(saved) && saved >= MINIMUM_POLL_INTERVAL_MS) {
      return saved;
    }
  } catch {
    // A browser that refuses storage is not a failure. The console uses the default.
  }
  return DEFAULT_POLL_INTERVAL_MS;
}

function saveInterval(ms) {
  try {
    window.localStorage.setItem(POLL_INTERVAL_KEY, String(ms));
  } catch {
    // The interval holds for this session only. That is the whole consequence.
  }
}

/** clockTime states a time of day as the operator reads it. */
function clockTime(at) {
  const time = new Date(at);
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(time.getHours())}:${pad(time.getMinutes())}:${pad(time.getSeconds())}`;
}

/** tailnetCount reads the count of declared tailnets, and 0 before the first poll. */
function tailnetCount(status) {
  if (!status || !status.desired) {
    return 0;
  }
  return Object.keys(status.desired).length;
}

/** element builds one element with a class and a text. */
function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

/** drawPollBar states the state of the poll layer. It hides while every poll succeeds. */
function drawPollBar(snapshot) {
  const bar = document.getElementById("poll-bar");
  const dot = document.getElementById("poll-dot");
  const text = document.getElementById("poll-text");
  const retry = document.getElementById("poll-retry");

  if (snapshot.restarted) {
    bar.hidden = false;
    dot.className = "dot warn";
    text.textContent = "The daemon restarted with another version. Reload the page to load the console of the running binary.";
    retry.hidden = false;
    retry.textContent = "Reload";
    return;
  }

  if (!snapshot.error) {
    bar.hidden = true;
    retry.hidden = true;
    return;
  }

  bar.hidden = false;
  retry.textContent = "Retry now";
  retry.hidden = !snapshot.retryOffered;

  // A time, an address, and a message that the daemon wrote are machine values, so each
  // one renders in the mono typeface inside the sentence.
  text.replaceChildren();
  if (snapshot.stale) {
    dot.className = "dot warn";
    text.append(document.createTextNode("The daemon does not answer. This is the state of "));
    text.append(element("span", "mono", clockTime(snapshot.lastSuccessAt)));
    text.append(document.createTextNode(" and it is stale. "));
  } else {
    dot.className = "dot crit";
    text.append(document.createTextNode("The daemon does not answer on "));
    text.append(element("span", "mono", window.location.host));
    text.append(document.createTextNode(". Start the daemon, or read its log. "));
  }
  text.append(element("span", "mono", snapshot.error));
}

/** drawVersion writes the daemon version. It writes no value that no poll returned. */
function drawVersion(snapshot) {
  const node = document.getElementById("daemon-version");
  const version = snapshot.status && snapshot.status.server_version;
  node.textContent = version ? version : "no version yet";
}

/** drawPlaceholder draws the frame of a view that a later issue fills. */
function drawPlaceholder(section, view, snapshot) {
  section.replaceChildren();

  if (snapshot.loading) {
    const card = element("div", "card");
    card.append(element("span", "label", "Loading"));
    card.append(element("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  if (tailnetCount(snapshot.status) === 0) {
    const card = element("div", "card empty");
    card.append(element("span", "label", "Empty"));
    card.append(element("p", undefined, view.empty));
    section.append(card);
    return;
  }

  const card = element("div", "card");
  card.append(element("span", "label", view.heading));
  card.append(element("p", "note", view.pending));
  section.append(card);
}

/**
 * applyPollInterval changes the cadence of the one poll layer and stores the value.
 *
 * The settings view owns the control, and the poll layer belongs to the shell, so the
 * shell states the one function that changes it. It refuses a value that the poll layer
 * refuses. See FR-console-15.
 */
export function applyPollInterval(ms) {
  poller.setInterval(ms);
  saveInterval(ms);
}

/**
 * refreshConsole polls the daemon at once.
 *
 * A view that changed the state of the daemon calls it, so that the console shows the new
 * state rather than the state of the last tick. The poll layer is the one data source, so
 * a view reads no route a second time.
 * refreshConsole does nothing before the shell starts, which is every import under Node.
 */
export function refreshConsole() {
  if (poller !== null) {
    poller.refresh();
  }
}

/** drawCurrent draws the view that the address names. */
function drawCurrent() {
  if (latest === null) {
    return;
  }
  const view = viewFor(current);

  document.getElementById("view-heading").textContent = view.heading;
  document.getElementById("view-lead").textContent = view.lead;

  for (const entry of VIEWS) {
    const section = document.getElementById(`view-${entry.id}`);
    const link = document.querySelector(`.nav-link[data-view="${entry.id}"]`);
    const selected = entry.id === view.id;
    section.hidden = !selected;
    if (selected) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
    if (!selected) {
      continue;
    }
    const draw = mounts.get(entry.id);
    if (draw) {
      draw(section, latest);
    } else {
      drawPlaceholder(section, entry, latest);
    }
  }
}

/** route reads the identifier out of the address, and it defaults to the first view. */
function route() {
  const id = window.location.hash.replace(/^#\/?/, "");
  current = viewFor(id).id;
  drawCurrent();
}

function start() {
  poller = createPollLayer({ interval: readSavedInterval() });

  document.getElementById("poll-retry").addEventListener("click", () => {
    if (poller.snapshot().restarted) {
      window.location.reload();
      return;
    }
    poller.refresh();
  });

  poller.subscribe((snapshot) => {
    latest = snapshot;
    drawVersion(snapshot);
    drawPollBar(snapshot);
    drawCurrent();
  });

  window.addEventListener("hashchange", route);
  route();
  poller.start();
}

if (typeof document !== "undefined") {
  start();
}
