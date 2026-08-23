// The access view: the header, the staged state model, the mode control, the reachability
// matrix, the flow overview, the rule list, the staged list, and the apply.
//
// The flow overview reuses the renderer of the overview topology in topology.js. It reads
// the staged rule set, therefore an edit reaches the picture before the daemon holds it.
//
// The model holds three things: the rule set that the daemon reported, the staged rule
// set, and the difference between them. The staged count is the size of the difference.
// A staged edit changes the model alone. Only the apply sends a rule set, which
// FR-editor-23 states.
//
// The apply sends the dry run first and the write after it. On a failure it changes no
// staged edit and it repeats the message of the daemon word for word.
//
// The mode control is the one mutating request of this view. It sends the rule set of the
// daemon with the new mode, therefore a staged rule reaches no host until the operator
// applies it.
//
// Every function above the drawing section is pure, or it takes its transport as an
// argument, so internal/ui/jstest asserts it under Node with no browser and no network.

import { ACCESS_ROUTE, refreshConsole, registerView, requestJSON } from "./app.js";
import { buildTopology, textEquivalentMarkup, topologySVGMarkup } from "./topology.js";

/** The identifier of the element that holds the text equivalent of the flow overview.
 *  It differs from the identifier of the overview topology, because the console keeps the
 *  markup of a view that it does not show. */
export const FLOW_TEXT_ID = "access-flow-text";

/** The mode in which the daemon drops the packet that no rule allows. */
export const MODE_ENFORCE = "enforce";

/** The mode in which the daemon logs the packet that no rule allows and drops nothing. */
export const MODE_OBSERVE = "observe";

/** The command that shows what the daemon stops in enforce mode. FR-editor-32. */
export const OBSERVE_LOG_COMMAND = "journalctl -u hydrascale | grep hydrascale-would-deny";

/** ruleKey names one path. A path holds at most one rule, therefore the pair is the key. */
function ruleKey(rule) {
  // The separator is the space character, which no endpoint name holds: a tailnet
  // identifier matches the pattern of the daemon, and the two other names are host and
  // internet.
  return `${rule.from} ${rule.to}`;
}

/** normalizeRule returns one rule with a port list, because the daemon reports none for
 *  a rule that allows every port. */
function normalizeRule(rule) {
  return { from: rule.from, to: rule.to, ports: (rule.ports || []).slice() };
}

/** samePorts reports whether two rules hold the same ports in the same order. */
function samePorts(one, other) {
  const a = one.ports || [];
  const b = other.ports || [];
  return a.length === b.length && a.every((port, at) => port === b[at]);
}

/** sameRules reports whether two rule sets allow the same paths on the same ports.
 *  The daemon states no order for the rule set, therefore the comparison reads the key. */
function sameRules(one, other) {
  if (one.length !== other.length) {
    return false;
  }
  const byKey = new Map(other.map((rule) => [ruleKey(rule), rule]));
  return one.every((rule) => {
    const match = byKey.get(ruleKey(rule));
    return match !== undefined && samePorts(match, rule);
  });
}

/**
 * differenceBetween returns the added rules, the removed rules, and the changed rules.
 *
 * before is a rule set and after is a rule set. A rule of after that before does not hold
 * is added, a rule of before that after does not hold is removed, and a rule that both
 * hold on other ports is changed.
 */
function differenceBetween(before, after) {
  const beforeByKey = new Map(before.map((rule) => [ruleKey(rule), rule]));
  const afterByKey = new Map(after.map((rule) => [ruleKey(rule), rule]));

  const added = [];
  const changed = [];
  for (const [key, rule] of afterByKey) {
    const held = beforeByKey.get(key);
    if (!held) {
      added.push(rule);
    } else if (!samePorts(held, rule)) {
      changed.push(rule);
    }
  }
  const removed = before.filter((rule) => !afterByKey.has(ruleKey(rule)));
  return { added, removed, changed };
}

/**
 * createAccessState returns the staged state model of the access view.
 *
 * options.request sends one request. It takes the route, the method, and the body, and it
 * rejects with the message that the daemon stated. A test replaces it.
 *
 * The model opens no request of its own. The poll layer gives it the answer of
 * GET /api/access through setBase.
 */
export function createAccessState(options = {}) {
  const request = options.request || ((route, method, body) => requestJSON(route, method, body));

  let base = { mode: MODE_ENFORCE, rules: [], nodes: [] };
  let staged = [];

  // stageBase is the rule set of the daemon that the operator staged the edits against. It
  // is null while the console holds no staged edit. The rebase needs it, because the
  // difference of the operator is the difference against that rule set and not against the
  // rule set that a later poll returned.
  let stageBase = null;

  // droppedRules holds every staged rule that names a node the daemon no longer reports.
  let droppedRules = [];

  function difference() {
    return differenceBetween(base.rules, staged);
  }

  function count() {
    const { added, removed, changed } = difference();
    return added.length + removed.length + changed.length;
  }

  return {
    /** base returns the mode, the rule set, and the node list that the daemon reported. */
    base() {
      return { mode: base.mode, rules: base.rules.map(normalizeRule), nodes: base.nodes.slice() };
    },

    /** rules returns the staged rule set, which the editor of the operator writes. */
    rules() {
      return staged.map(normalizeRule);
    },

    /**
     * setBase takes one answer of GET /api/access.
     *
     * setBase replaces the staged rule set when the operator staged no edit, so that the
     * view shows what the daemon holds now. It keeps every staged edit otherwise, because
     * the console applies no edit and removes no edit on its own.
     */
    setBase(body) {
      const wasClean = count() === 0;
      base = {
        mode: (body && body.mode) || MODE_ENFORCE,
        rules: ((body && body.rules) || []).map(normalizeRule),
        nodes: (body && body.nodes) || [],
      };
      if (wasClean) {
        staged = base.rules.map(normalizeRule);
        stageBase = null;
        droppedRules = [];
        return;
      }

      // A rule names a tailnet, the host, or the internet, and the daemon reports one node
      // for each. A staged rule that names a node of no answer reaches no host, therefore
      // the model drops it and dropped states which rule it dropped.
      const known = new Set(base.nodes.map((node) => node.id));
      if (known.size === 0) {
        return;
      }
      const holds = (rule) => known.has(rule.from) && known.has(rule.to);
      droppedRules = [...droppedRules, ...staged.filter((rule) => !holds(rule))];
      staged = staged.filter(holds);
    },

    /** setRules replaces the staged rule set. It sends nothing. FR-editor-23. */
    setRules(rules) {
      if (stageBase === null) {
        stageBase = { mode: base.mode, rules: base.rules.map(normalizeRule) };
      }
      staged = rules.map(normalizeRule);
      if (count() === 0) {
        // The staged rule set equals the rule set of the daemon, therefore the console
        // tracks the daemon again and a later poll replaces the staged rule set.
        stageBase = null;
      }
    },

    /** discard returns the staged rule set to the rule set of the daemon. FR-editor-27. */
    discard() {
      staged = base.rules.map(normalizeRule);
      stageBase = null;
      droppedRules = [];
    },

    /**
     * baseChanged reports whether the daemon holds another rule set than the rule set that
     * the operator staged the edits against.
     *
     * Another console that applies a change raises it. It is false while the console holds
     * no staged edit, because a clean console takes each answer of the daemon.
     */
    baseChanged() {
      return stageBase !== null && !sameRules(stageBase.rules, base.rules);
    },

    /**
     * rebase writes the staged edits onto the rule set that the daemon holds now.
     *
     * rebase keeps every rule that the other console added, and it repeats the added rule,
     * the changed rule, and the removed rule of the operator on that rule set. rebase
     * sends nothing.
     */
    rebase() {
      if (stageBase === null) {
        return;
      }
      const change = differenceBetween(stageBase.rules, staged);
      const next = new Map(base.rules.map((rule) => [ruleKey(rule), normalizeRule(rule)]));
      for (const rule of change.removed) {
        next.delete(ruleKey(rule));
      }
      for (const rule of [...change.added, ...change.changed]) {
        next.set(ruleKey(rule), normalizeRule(rule));
      }
      staged = [...next.values()];
      stageBase = { mode: base.mode, rules: base.rules.map(normalizeRule) };
      if (count() === 0) {
        stageBase = null;
      }
    },

    /** dropped returns every staged rule that named a node the daemon stopped reporting. */
    dropped() {
      return droppedRules.map(normalizeRule);
    },

    /** difference returns the added rules, the removed rules, and the changed rules. */
    difference,

    /** count returns the number of staged edits. FR-editor-24. */
    count,

    /** send runs one request through the transport that the model holds. */
    send(route, method, body) {
      return request(route, method, body);
    },
  };
}

/** The label of the apply action while the daemon applies the rule set. */
export const APPLYING_LABEL = "The daemon applies the rule set";

/**
 * headerModel returns the header of the view: the mode, the staged count, and the
 * controls.
 *
 * mode is the mode that the daemon reported and count is the number of staged edits.
 * applying is true while the daemon applies the rule set. Every control writes, therefore
 * every control is disabled while one request runs.
 * The accent belongs to one thing per view, and this view gives it to the apply action.
 */
export function headerModel(mode, count, applying = false) {
  const staged = count === 1 ? "1 staged" : `${count} staged`;
  return {
    mode: {
      word: mode === MODE_OBSERVE ? MODE_OBSERVE : MODE_ENFORCE,
      tone: mode === MODE_OBSERVE ? "warn" : "ok",
    },
    staged,
    controls: [
      { id: "mode", label: "Change the mode", kind: "button", accent: false, disabled: applying },
      { id: "discard", label: "Discard", kind: "button", accent: false, disabled: count === 0 || applying },
      {
        id: "apply",
        label: applying ? APPLYING_LABEL : "Apply",
        kind: "button",
        accent: true,
        disabled: count === 0 || applying,
      },
    ],
  };
}

/**
 * stagedListModel returns one row per staged edit. FR-editor-25.
 *
 * difference is the difference that the model returns. The list holds the added rules, the
 * changed rules, and the removed rules, in that order, and count is the number of rows.
 * The count and the header therefore read the one difference and they state one number.
 */
export function stagedListModel(difference) {
  const row = (kind, word, rule) => ({
    kind,
    word,
    from: rule.from,
    to: rule.to,
    ports: rule.ports.slice(),
    portsLabel: rule.ports.length === 0 ? ALL_PORTS : rule.ports.join(", "),
  });
  const rows = [
    ...difference.added.map((rule) => row("add", "allow", normalizeRule(rule))),
    ...difference.changed.map((rule) => row("change", "change the ports of", normalizeRule(rule))),
    ...difference.removed.map((rule) => row("remove", "remove", normalizeRule(rule))),
  ];
  return { count: rows.length, rows };
}

/**
 * applyFailureStatement returns the statement of an apply that the daemon refused.
 *
 * message is the message that the daemon stated. applyFailureStatement returns it
 * unchanged, because .claude/rules/ste.md states that a rewritten message is destroyed
 * evidence. FR-editor-30.
 */
export function applyFailureStatement(message) {
  return {
    lead: "The daemon refused the rule set. The host keeps the rule set that it held, and the console keeps every staged edit.",
    message,
  };
}

/**
 * rebaseOffer returns the offer that a changed rule set of the daemon raises, and null
 * when the daemon holds the rule set that the operator staged the edits against.
 *
 * changed is the value of baseChanged and count is the number of staged edits. The offer
 * takes no accent, because the accent marks the apply action alone.
 */
export function rebaseOffer(changed, count) {
  if (!changed) {
    return null;
  }
  const edits = count === 1 ? "1 staged edit" : `${count} staged edits`;
  return {
    sentences: [
      `Warning: another console changed the rule set of the daemon while this console held ${edits}.`,
      "Rebase writes the staged edits onto the rule set that the daemon holds now.",
      "Discard removes every staged edit and it returns the view to the daemon.",
    ],
    controls: [
      { id: "rebase", label: "Rebase the edits", kind: "button", accent: false },
      { id: "discard", label: "Discard the edits", kind: "button", accent: false },
    ],
  };
}

/**
 * droppedStatement returns the statement of every staged rule that the model dropped, and
 * null when the model dropped none.
 *
 * rules is the value of dropped. The daemon reports no node for the source or for the
 * destination of each rule, therefore the rule reaches no host.
 */
export function droppedStatement(rules) {
  if (!rules || rules.length === 0) {
    return null;
  }
  return {
    sentence: rules.length === 1
      ? "The daemon reports no node for one staged rule, therefore the console dropped it:"
      : "The daemon reports no node for these staged rules, therefore the console dropped them:",
    rules: rules.map((rule) => `${rule.from} to ${rule.to}`),
  };
}

/**
 * activePathWarning returns the warning of a staged edit that removes a path that carries
 * an active session, and null when no staged edit removes such a path. FR-editor-28.
 *
 * difference is the difference that the model returns, and activePaths is the field
 * active_paths of GET /api/access. The daemon reads the sessions of the host and it
 * reports the path of each one, therefore the warning reads no property of the console
 * request. A console request always arrives on the loopback address, and no local rule
 * governs that path.
 *
 * The warning carries no control. The operator applies the staged edits after it, which
 * FR-editor-29 states.
 */
export function activePathWarning(difference, activePaths) {
  const active = new Set((activePaths || []).map((path) => `${path.from} ${path.to}`));
  const cut = difference.removed.filter((rule) => active.has(ruleKey(rule)));
  if (cut.length === 0) {
    return null;
  }
  const lead = cut.length === 1
    ? "Warning: a staged edit removes a path that carries an active session."
    : `Warning: the staged edits remove ${cut.length} paths that carry an active session.`;
  return {
    sentences: [
      lead,
      "The daemon stops that session after the apply.",
      "The apply action stays available.",
    ],
    paths: cut.map((rule) => `${rule.from} to ${rule.to}`),
  };
}

/**
 * observeStatement returns the statement of observe mode, and null in enforce mode.
 *
 * FR-editor-32 states that the view names the log command that shows what the daemon
 * stops in enforce mode.
 */
export function observeStatement(mode) {
  if (mode !== MODE_OBSERVE) {
    return null;
  }
  return {
    sentence: "The daemon denies nothing in this mode. It logs the packet that no rule allows.",
    lead: "Read what the daemon stops in enforce mode with:",
    command: OBSERVE_LOG_COMMAND,
  };
}

/**
 * modeChange returns the dialog of the mode control. FR-editor-33.
 *
 * mode is the mode that the daemon holds now. The dialog states what the change does, and
 * the change to enforce carries the warning first, because a connection stops at once.
 */
export function modeChange(mode) {
  if (mode === MODE_OBSERVE) {
    return {
      next: MODE_ENFORCE,
      heading: "Change the mode to enforce",
      sentences: [
        "Warning: a connection that no rule allows stops at once.",
        "In enforce mode the daemon drops the packet that no rule allows.",
        "The rule set does not change, and the console sends no staged edit.",
      ],
      confirmLabel: "Change to enforce",
    };
  }
  return {
    next: MODE_OBSERVE,
    heading: "Change the mode to observe",
    sentences: [
      "In observe mode the daemon drops no packet.",
      "The daemon logs the packet that no rule allows, and it stops nothing.",
      "The rule set does not change, and the console sends no staged edit.",
    ],
    confirmLabel: "Change to observe",
  };
}

/**
 * sendModeChange sends the new mode with PUT /api/access.
 *
 * state is the model, and next is the mode to apply. sendModeChange sends the rule set of
 * the daemon rather than the staged rule set, therefore a staged edit reaches no host
 * before the operator applies it. See FR-editor-23.
 * sendModeChange rejects with the message that the daemon stated.
 */
export function sendModeChange(state, next) {
  return state.send(ACCESS_ROUTE, "PUT", { mode: next, rules: state.base().rules });
}

/** The route that computes the effect of a rule set and writes nothing. The daemon reads
 *  the query parameter in dryRunParam of internal/api/access.go. */
export const DRY_RUN_ROUTE = `${ACCESS_ROUTE}?dry_run=true`;

/** messageOf returns the message that a rejected request stated. */
function messageOf(err) {
  return err && err.message ? err.message : String(err);
}

/**
 * applyStagedRules sends the staged rule set to the daemon and returns what happened.
 *
 * state is the model. applyStagedRules sends the dry run first, which computes the effect
 * and writes nothing, and it sends the write after the dry run passes. Both requests carry
 * the whole rule set, so that two consoles cannot interleave partial writes. FR-editor-26.
 *
 * On a success applyStagedRules takes the answer as the new rule set of the daemon and it
 * clears the staged edits, which FR-editor-29 states. It returns {ok: true, body}.
 * On a failure it changes no staged edit and it returns {ok: false, error}, where error is
 * the message of the daemon word for word. FR-editor-30.
 */
export async function applyStagedRules(state) {
  const body = { mode: state.base().mode, rules: state.rules() };
  try {
    await state.send(DRY_RUN_ROUTE, "PUT", body);
  } catch (err) {
    return { ok: false, error: messageOf(err) };
  }

  let applied = null;
  try {
    applied = await state.send(ACCESS_ROUTE, "PUT", body);
  } catch (err) {
    return { ok: false, error: messageOf(err) };
  }

  state.setBase(applied);
  state.discard();
  return { ok: true, body: applied };
}

/**
 * emptyStatement returns the empty state of the view, and null for a rule set that holds
 * one rule.
 *
 * A host that declares no tailnet states the tailnet as the first step. A host that holds
 * no rule states that nothing reaches anything, and it names the matrix.
 */
export function emptyStatement(body) {
  const nodes = (body && body.nodes) || [];
  const rules = (body && body.rules) || [];
  if (nodes.filter((node) => node.kind === "tailnet").length === 0) {
    return {
      label: "Empty",
      sentence: "No tailnet is configured. Add a tailnet, and this view shows the paths that the host allows.",
    };
  }
  if (rules.length === 0) {
    return {
      label: "Empty",
      sentence: "No rule exists, therefore nothing reaches anything. Click a square in the matrix to allow a path.",
    };
  }
  return null;
}

/** The side of one square in pixels. FR-editor-15 states the corner radius; app.css holds
 *  it as the radius token of the brand. */
export const SQUARE_SIDE = 34;

/** The side of one square in pixels on a dense host. */
export const SQUARE_SIDE_DENSE = 28;

/** The number of tailnets from which the matrix draws the smaller square. */
const DENSE_TAILNET_COUNT = 12;

/** square returns one square of the matrix. allowed holds the key of every allowed path. */
function square(from, to, allowed) {
  // The daemon rejects a rule where from equals to, therefore the diagonal takes no rule
  // and it accepts no click. See FR-access-11 and FR-editor-10.
  const inert = from === to;
  const on = !inert && allowed.has(`${from} ${to}`);
  let label = `${from} to ${to}, no rule`;
  if (inert) {
    label = `${from} to ${to}, not applicable`;
  } else if (on) {
    label = `${from} to ${to}, allowed`;
  }
  return { from, to, allowed: on, inert, kind: "button", disabled: inert, label };
}

/**
 * matrixModel returns the grid of squares of the reachability matrix.
 *
 * base is the answer of the daemon, which carries the node list. staged is the staged rule
 * set, therefore the matrix shows the edit of the operator before the daemon holds it.
 * A source is a tailnet or the host, and a destination is a tailnet, the host, or the
 * internet, which features/05-reachability-model.md states. FR-editor-7.
 *
 * matrixModel carries no port, because the ports live in the rule list. FR-editor-14.
 */
export function matrixModel(base, staged) {
  const nodes = (base && base.nodes) || [];
  const tailnets = nodes.filter((node) => node.kind === "tailnet").map((node) => node.id);
  const holdsHost = nodes.some((node) => node.kind === "host");
  const holdsInternet = nodes.some((node) => node.kind === "internet");

  const sources = holdsHost ? [...tailnets, "host"] : [...tailnets];
  const destinations = [...tailnets];
  if (holdsHost) {
    destinations.push("host");
  }
  if (holdsInternet) {
    destinations.push("internet");
  }

  const allowed = new Set((staged || []).map(ruleKey));
  return {
    side: tailnets.length >= DENSE_TAILNET_COUNT ? SQUARE_SIDE_DENSE : SQUARE_SIDE,
    sources,
    destinations,
    rows: sources.map((source) => ({
      source,
      squares: destinations.map((destination) => square(source, destination, allowed)),
    })),
  };
}

/**
 * toggleSquare returns the staged rule set that one click on a square produces.
 *
 * rules is the staged rule set, and the pair from and to names the path. A click on an
 * empty square adds one rule that allows every port, and a click on a filled square
 * removes every rule of that path. See FR-editor-11 and FR-editor-12.
 * toggleSquare returns the rule set unchanged for the diagonal. FR-editor-10.
 */
export function toggleSquare(rules, from, to) {
  const staged = rules.map(normalizeRule);
  if (from === to) {
    return staged;
  }
  const key = `${from} ${to}`;
  const kept = staged.filter((rule) => ruleKey(rule) !== key);
  if (kept.length < staged.length) {
    return kept;
  }
  return [...staged, { from, to, ports: [] }];
}

/**
 * hoverMarks returns the two labels that the square under the pointer marks.
 *
 * The mark names the row label and the column label, and the view draws no third element.
 * FR-editor-13.
 */
export function hoverMarks(from, to) {
  return { row: from, column: to };
}

/** The words that a rule with no port shows. FR-editor-17. */
export const ALL_PORTS = "all ports";

/** portPattern matches one port entry: a protocol, one port, and an optional second port.
 *  internal/access/rules.go holds the same pattern, and the daemon rejects with the same
 *  three messages, therefore the console repeats one rule rather than a second grammar. */
const portPattern = /^(tcp|udp)\/([0-9]{1,5})(?:-([0-9]{1,5}))?$/;

/** portFailure returns the message of a bad port entry, and null for a good entry.
 *  entry is one item of the port list of a rule, in the form tcp/22 or udp/1-1024.
 *  The three messages repeat internal/access/rules.go word for word. FR-access-10. */
function portFailure(entry) {
  const match = portPattern.exec(entry);
  if (match === null) {
    return `invalid port ${JSON.stringify(entry)}: the form is tcp/<n>, udp/<n>, tcp/<n>-<m>, or udp/<n>-<m>, for example tcp/22`;
  }
  const range = `invalid port ${JSON.stringify(entry)}: a port number is between 1 and 65535`;
  const low = Number(match[2]);
  if (low < 1 || low > 65535) {
    return range;
  }
  if (match[3] !== undefined) {
    const high = Number(match[3]);
    if (high < 1 || high > 65535) {
      return range;
    }
    if (high < low) {
      return `invalid port ${JSON.stringify(entry)}: the second number is lower than the first`;
    }
  }
  return null;
}

/**
 * parsePorts returns the port list that one port field holds.
 *
 * text is the text of the field. The comma separates the entries, and parsePorts removes
 * the space around each entry. An empty field returns an empty port list, which allows
 * every port and every protocol. FR-access-9.
 * parsePorts returns {ports: null, error: "<message>"} for a bad entry, therefore one bad
 * entry rejects the whole field and the console corrects no value. FR-editor-22.
 */
export function parsePorts(text) {
  const entries = String(text)
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry !== "");
  for (const entry of entries) {
    const failure = portFailure(entry);
    if (failure !== null) {
      return { ports: null, error: failure };
    }
  }
  return { ports: entries, error: null };
}

/**
 * setRulePorts returns the staged rule set in which one path holds the given ports.
 *
 * rules is the staged rule set, the pair from and to names the path, and ports is the port
 * list that parsePorts returned. setRulePorts sends nothing. FR-editor-19.
 */
export function setRulePorts(rules, from, to, ports) {
  const key = `${from} ${to}`;
  return rules.map(normalizeRule).map((rule) => {
    if (ruleKey(rule) !== key) {
      return rule;
    }
    return { from: rule.from, to: rule.to, ports: ports.slice() };
  });
}

/**
 * deleteRule returns the staged rule set with no rule for one path.
 *
 * rules is the staged rule set, and the pair from and to names the path. deleteRule sends
 * nothing, therefore the deletion counts as one staged edit. FR-editor-20.
 */
export function deleteRule(rules, from, to) {
  const key = `${from} ${to}`;
  return rules.map(normalizeRule).filter((rule) => ruleKey(rule) !== key);
}

/**
 * ruleListModel returns one row per rule of the staged rule set.
 *
 * base is the answer of the daemon and staged is the staged rule set. A row exists for an
 * allowed path alone, therefore the list holds no row for a path that no rule allows.
 * FR-editor-21.
 * A row that the daemon does not hold, and a row whose ports differ from the ports of the
 * daemon, both carry staged: true. FR-editor-18.
 */
export function ruleListModel(base, staged) {
  const held = new Map(((base && base.rules) || []).map((rule) => [ruleKey(rule), rule]));
  return {
    rows: (staged || []).map(normalizeRule).map((rule) => {
      const before = held.get(ruleKey(rule));
      const empty = rule.ports.length === 0;
      return {
        from: rule.from,
        to: rule.to,
        // Every allowed path of this product is a dotted line, and the row is one.
        connector: "dotted",
        chips: rule.ports.slice(),
        allPorts: empty,
        portsLabel: empty ? ALL_PORTS : rule.ports.join(", "),
        text: rule.ports.join(", "),
        staged: !before || !samePorts(before, rule),
        controls: [
          { id: "ports", kind: "input", label: `The ports of ${rule.from} to ${rule.to}` },
          { id: "delete", kind: "button", label: `Delete the rule ${rule.from} to ${rule.to}` },
        ],
      };
    }),
  };
}

// ---------------------------------------------------------------------------
// The flow overview
// ---------------------------------------------------------------------------

/**
 * flowModel returns the drawn model of the flow overview.
 *
 * status is the body of GET /api/status, which carries the reachability of each tailnet.
 * nodes is the node list of GET /api/access and rules is the staged rule set. The rule set
 * holds allow rules alone, therefore a pair of nodes with no rule between them gets no
 * curve at all: the absence of a line is the whole statement. See FR-editor-4.
 *
 * flowModel places the tailnets on the left and the destinations on the right, which
 * FR-editor-1 states. It is pure.
 */
export function flowModel(status, nodes, rules) {
  return buildTopology(status, { nodes: nodes || [], rules: rules || [] });
}

/**
 * flowMarkup returns the picture as SVG source.
 *
 * selected holds the identifier of the source that the operator chose, or null. The paths
 * that start at that source take the accent and every other path goes quiet, which
 * FR-editor-5 states. One source draws at a time.
 *
 * The serializer escapes every value that the daemon reported, so a hostile identifier
 * reaches the page as text.
 */
export function flowMarkup(model, selected) {
  return topologySVGMarkup(model, selected, { bySource: true });
}

/** flowTextMarkup returns the text that a screen reader reads in place of the curves. */
export function flowTextMarkup(model) {
  return textEquivalentMarkup(model);
}

/**
 * flowSelection returns the source that one click leaves selected.
 *
 * current is the source that the picture holds now and id is the node that the operator
 * chose. A second click on the same source returns the picture to the resting state, and a
 * node that the model does not hold selects nothing.
 */
export function flowSelection(model, current, id) {
  if (!model.nodes.some((node) => node.id === id)) {
    return null;
  }
  return current === id ? null : id;
}

/**
 * flowCaption states which source the picture draws.
 *
 * label carries the sans typeface and id carries the mono typeface, because the identifier
 * belongs to the machine. sentence states what the operator reads under the picture.
 */
export function flowCaption(model, selected) {
  if (selected === null || selected === undefined) {
    return {
      label: "every source",
      id: "",
      sentence: "Select a node to draw the paths that start at it. A path that no rule allows has no line.",
    };
  }
  const owned = model.paths.filter((path) => path.from === selected).length;
  return {
    label: "source",
    id: selected,
    sentence: owned === 0 ? "No path starts at this node." : "Every other path is muted.",
  };
}

// ---------------------------------------------------------------------------
// The drawing. Everything below this line needs a document.
// ---------------------------------------------------------------------------

const state = createAccessState();

let dialog = null; // null, or {sending, error}
let redraw = () => {};

/** applying is true while the daemon applies the staged rule set. */
let applying = false;

/** applyError holds the message that the daemon stated on a refused apply, or null. */
let applyError = null;

/** source holds the source that the picture draws, and null when the operator chose none. */
let source = null;

/** takeFocus is true after a click or a key selected a source. The poll redraws the view
 *  every few seconds, and a redraw that always moves the focus takes it from the operator. */
let takeFocus = false;

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

/** chip builds one chip. A machine value takes the mono typeface. */
function chip(text, dotTone) {
  const node = el("span", "chip");
  if (dotTone) {
    node.append(el("span", `dot ${dotTone}`));
  }
  node.append(el("span", "mono", text));
  return node;
}

/** control builds one button of the header from its model. */
function control(model, onClick) {
  const button = el("button", model.accent ? "btn primary" : "btn", model.label);
  button.type = "button";
  button.disabled = model.disabled;
  button.addEventListener("click", onClick);
  return button;
}

/** The handler of each header control, by identifier. */
const headerActions = {
  mode: openModeDialog,
  discard: discardEdits,
  apply: runApply,
};

/** drawHeader draws the mode, the staged count, and the three controls. */
function drawHeader(base) {
  const header = headerModel(base.mode, state.count(), applying);
  const row = el("div", "ac-head");

  const states = el("div", "ac-states");
  states.append(chip(header.mode.word, header.mode.tone));
  states.append(chip(header.staged));
  row.append(states);

  const acts = el("div", "ac-acts");
  for (const model of header.controls) {
    acts.append(control(model, headerActions[model.id]));
  }
  row.append(acts);
  return row;
}

/** discardEdits returns the view to the rule set of the daemon. FR-editor-27. */
function discardEdits() {
  state.discard();
  applyError = null;
  redraw();
}

/**
 * runApply sends the staged rule set and it draws the result.
 *
 * The operator starts it. The view starts no timer, therefore the console applies no edit
 * on its own.
 */
function runApply() {
  applying = true;
  applyError = null;
  redraw();

  applyStagedRules(state).then((result) => {
    applying = false;
    applyError = result.ok ? null : result.error;
    redraw();
    if (result.ok) {
      // The daemon holds the new rule set, therefore the console asks the poll layer for a
      // new tick rather than showing the state of the last tick. FR-editor-29.
      refreshConsole();
    }
  });
}

/** rebaseEdits writes the staged edits onto the rule set that the daemon holds now. */
function rebaseEdits() {
  state.rebase();
  redraw();
}

/**
 * drawRebaseOffer draws the offer that a changed rule set of the daemon raises.
 *
 * The offer comes before the staged list and before the operator selects Apply, because
 * .claude/rules/ste.md states that a warning comes before the step it applies to.
 */
function drawRebaseOffer(offer) {
  const card = el("div", "card ac-notice");
  const alert = el("div", "alert");
  alert.append(el("span", "dot warn"));
  const text = el("div");
  for (const sentence of offer.sentences) {
    text.append(el("p", undefined, sentence));
  }
  alert.append(text);
  card.append(alert);

  const acts = el("div", "ns-acts");
  const actions = { rebase: rebaseEdits, discard: discardEdits };
  for (const model of offer.controls) {
    acts.append(control(model, actions[model.id]));
  }
  card.append(acts);
  return card;
}

/**
 * drawActivePathWarning states every path that a staged edit removes and that carries an
 * active session.
 *
 * The card holds no control, therefore the operator reads it and then selects Apply or
 * Discard in the header.
 */
function drawActivePathWarning(warning) {
  const card = el("div", "card ac-notice");
  const alert = el("div", "alert");
  alert.append(el("span", "dot crit"));
  const text = el("div");
  for (const sentence of warning.sentences) {
    text.append(el("p", undefined, sentence));
  }
  alert.append(text);
  card.append(alert);

  const list = el("div", "ns-cmds mono");
  for (const path of warning.paths) {
    list.append(el("span", undefined, path));
  }
  card.append(list);
  return card;
}

/** drawDropped states every staged rule that the console dropped. */
function drawDropped(statement) {
  const card = el("div", "card ac-notice");
  const alert = el("div", "alert");
  alert.append(el("span", "dot warn"));
  const text = el("div");
  text.append(el("p", undefined, statement.sentence));
  alert.append(text);
  card.append(alert);

  const list = el("div", "ns-cmds mono");
  for (const rule of statement.rules) {
    list.append(el("span", undefined, rule));
  }
  card.append(list);
  return card;
}

/**
 * drawApplyFailure states the message of the daemon.
 *
 * The message reaches the screen through textContent, therefore a message that holds
 * markup characters reads as text and never as markup.
 */
function drawApplyFailure(statement) {
  const card = el("div", "card ac-notice");
  const alert = el("div", "alert");
  alert.append(el("span", "dot crit"));
  const text = el("div");
  text.append(el("p", undefined, statement.lead));
  alert.append(text);
  card.append(alert);
  card.append(el("code", "ns-cmds mono", statement.message));
  return card;
}

/** drawStagedList draws one row per staged edit. FR-editor-25. */
function drawStagedList(model) {
  const card = el("div", "card ac-staged");
  card.append(el("span", "label", "Staged edits"));
  card.append(el("p", "note", "The console holds these edits. The daemon holds none of them until the operator selects Apply."));

  const list = el("div", "ac-stagedlist");
  for (const row of model.rows) {
    const line = el("div", "ac-stagedrow");
    line.append(el("span", "ac-stagedverb", row.word));
    line.append(el("span", "ac-end mono", row.from));
    line.append(el("span", "ac-conn"));
    line.append(el("span", "ac-end mono", row.to));
    line.append(el("span", "ac-noports mono", row.portsLabel));
    list.append(line);
  }
  card.append(list);
  return card;
}

/** drawObserve draws the statement of observe mode and the log command. */
function drawObserve(statement) {
  const card = el("div", "card ac-observe");
  const alert = el("div", "alert");
  alert.append(el("span", "dot warn"));
  const text = el("div");
  text.append(el("p", undefined, statement.sentence));
  text.append(el("p", undefined, statement.lead));
  alert.append(text);
  card.append(alert);
  card.append(el("code", "ns-cmds mono", statement.command));
  return card;
}

/** drawEmpty draws the empty state of the view. */
function drawEmpty(statement) {
  const card = el("div", "card empty");
  card.append(el("span", "label", statement.label));
  card.append(el("p", undefined, statement.sentence));
  return card;
}

/**
 * drawFlow draws the flow overview and its text equivalent.
 *
 * The serializer returns SVG source, so the function writes it once and then binds the
 * handlers on the groups that it wrote. The picture reads the staged rule set, therefore a
 * staged edit draws its curve before the operator applies it.
 */
function drawFlow(status, nodes) {
  const model = flowModel(status, nodes, state.rules());
  if (source !== null && !model.nodes.some((node) => node.id === source)) {
    // The daemon removed the tailnet that the operator had selected.
    source = null;
  }
  const caption = flowCaption(model, source);

  const card = el("section", "card ac-flow");
  card.setAttribute("aria-labelledby", "ac-flow-heading");

  const head = el("div", "card-head");
  const heading = el("span", "label", "Allowed paths");
  heading.id = "ac-flow-heading";
  head.append(heading);
  const mark = el("div", "ac-flow-cap");
  mark.append(el("span", "label", caption.label));
  if (caption.id) {
    mark.append(el("span", "mono", caption.id));
  }
  head.append(mark);
  card.append(head);

  // The serializer escapes every value that the daemon reported, and a test asserts that.
  const figure = el("div", "flow-wrap");
  figure.innerHTML = flowMarkup(model, source);
  const picture = figure.querySelector("svg.flow");
  if (picture) {
    picture.setAttribute("aria-label", "The paths that the staged rule set allows.");
    picture.setAttribute("aria-describedby", FLOW_TEXT_ID);
  }
  card.append(figure);

  const text = el("div", "sr");
  text.id = FLOW_TEXT_ID;
  text.innerHTML = flowTextMarkup(model);
  card.append(text);

  card.append(el("p", "note", caption.sentence));

  for (const group of figure.querySelectorAll("g.node")) {
    const id = group.dataset.node;
    const choose = () => {
      source = flowSelection(model, source, id);
      takeFocus = true;
      redraw();
    };
    group.addEventListener("click", choose);
    group.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        choose();
      }
    });
  }

  if (takeFocus && source !== null) {
    const chosen = figure.querySelector(`g.node[data-node="${CSS.escape(source)}"]`);
    if (chosen) {
      chosen.focus();
    }
  }
  takeFocus = false;
  return card;
}

/** stageSquare stages the edit of one square and it draws the view again. */
function stageSquare(from, to) {
  state.setRules(toggleSquare(state.rules(), from, to));
  redraw();
}

/**
 * drawMatrix draws the grid of squares.
 *
 * The square is a button, therefore it reaches focus with the tab key and the browser
 * gives it the space key and the enter key. The pointer and the keyboard both mark the row
 * label and the column label, and the mark changes the class of the two labels alone.
 */
function drawMatrix(model) {
  const card = el("div", "card ac-matrix");
  card.append(el("span", "label", "Reachability"));
  card.append(el("p", "note", "A filled square means that a rule allows the path. The ports live in the rule list."));

  const table = el("table", "ac-mtx");
  table.style.setProperty("--ac-square", `${model.side}px`);

  const headRow = el("tr");
  headRow.append(el("th"));
  const columnLabels = model.destinations.map((destination) => {
    const cell = el("th", "col", destination);
    cell.scope = "col";
    headRow.append(cell);
    return cell;
  });
  const head = el("thead");
  head.append(headRow);
  table.append(head);

  const body = el("tbody");
  for (const row of model.rows) {
    const line = el("tr");
    const rowLabel = el("th", undefined, row.source);
    rowLabel.scope = "row";
    line.append(rowLabel);

    row.squares.forEach((one, column) => {
      const holder = el("td");
      const button = el("button", one.allowed ? "ac-square on" : "ac-square");
      button.type = "button";
      button.setAttribute("aria-label", one.label);
      if (one.inert) {
        button.className = "ac-square inert";
        button.disabled = true;
      } else {
        const marks = hoverMarks(row.source, one.to);
        const mark = (on) => {
          rowLabel.classList.toggle("hot", on && marks.row === row.source);
          columnLabels[column].classList.toggle("hot", on && marks.column === one.to);
        };
        button.addEventListener("mouseenter", () => mark(true));
        button.addEventListener("mouseleave", () => mark(false));
        button.addEventListener("focus", () => mark(true));
        button.addEventListener("blur", () => mark(false));
        button.addEventListener("click", () => stageSquare(one.from, one.to));
      }
      holder.append(button);
      line.append(holder);
    });
    body.append(line);
  }
  table.append(body);

  card.append(table);
  return card;
}

/**
 * The draft of one port field.
 *
 * The poll draws the view again every few seconds. The draft holds the text that the
 * operator entered and the message of a refused entry, therefore a poll interrupts no
 * edit. The draft is null while no operator edits a port field.
 */
let portDraft = null; // null, or {key, text, error, focused}

/** stagePorts stages the ports of one path and it draws the view again. */
function stagePorts(from, to, ports) {
  portDraft = null;
  state.setRules(setRulePorts(state.rules(), from, to, ports));
  redraw();
}

/** stageDelete stages the deletion of one rule and it draws the view again. */
function stageDelete(from, to) {
  portDraft = null;
  state.setRules(deleteRule(state.rules(), from, to));
  redraw();
}

/**
 * drawRuleRow draws one row of the rule list: the source, the dotted connector, the
 * destination, the ports, the port field, and the delete control.
 *
 * The port field states the message of the daemon for a bad entry and it stages nothing,
 * therefore the console corrects no value. FR-editor-22.
 */
function drawRuleRow(row) {
  const key = `${row.from} ${row.to}`;
  const draft = portDraft && portDraft.key === key ? portDraft : null;

  const line = el("div", row.staged ? "ac-rule staged" : "ac-rule");
  line.append(el("span", "ac-end mono", row.from));
  line.append(el("span", "ac-conn"));
  line.append(el("span", "ac-end mono", row.to));

  if (row.allPorts) {
    line.append(el("span", "ac-noports mono", row.portsLabel));
  } else {
    const ports = el("div", "ac-ports");
    for (const port of row.chips) {
      ports.append(chip(port));
    }
    line.append(ports);
  }

  if (row.staged) {
    line.append(chip("staged"));
  }

  const field = el("input", "field ac-portfield");
  field.type = "text";
  field.value = draft ? draft.text : row.text;
  field.placeholder = ALL_PORTS;
  field.setAttribute("aria-label", row.controls[0].label);
  const message = el("span", "ns-error", draft ? draft.error || "" : "");
  if (draft && draft.error) {
    field.setAttribute("aria-invalid", "true");
  }

  field.addEventListener("focus", () => {
    portDraft = { key, text: field.value, error: draft ? draft.error : null, focused: true };
  });
  field.addEventListener("input", () => {
    portDraft = { key, text: field.value, error: null, focused: true };
    message.textContent = "";
    field.removeAttribute("aria-invalid");
  });
  field.addEventListener("blur", () => {
    // A poll removes this element and it draws a new one. The removed element reports the
    // loss of the focus, so the draft keeps the focus of the new element.
    if (field.isConnected && portDraft && portDraft.key === key) {
      portDraft.focused = false;
    }
  });
  field.addEventListener("change", () => {
    const result = parsePorts(field.value);
    if (result.error) {
      // The view draws nothing again here, so that the field keeps the focus and the text
      // of the operator.
      portDraft = { key, text: field.value, error: result.error, focused: true };
      message.textContent = result.error;
      field.setAttribute("aria-invalid", "true");
      return;
    }
    stagePorts(row.from, row.to, result.ports);
  });

  const remove = el("button", "btn ac-del", "Delete");
  remove.type = "button";
  remove.setAttribute("aria-label", row.controls[1].label);
  remove.addEventListener("click", () => stageDelete(row.from, row.to));

  line.append(field);
  line.append(remove);

  const holder = el("div", "ac-ruleline");
  holder.append(line);
  holder.append(message);
  return { holder, field, restore: Boolean(draft && draft.focused) };
}

/**
 * drawRules draws the rule list. FR-editor-16 to FR-editor-22.
 *
 * The list draws one row per rule of the staged rule set. A path that no rule allows takes
 * no row, therefore the absence of the row is the refusal.
 */
function drawRules(model) {
  const card = el("div", "card ac-rules");
  card.append(el("span", "label", "Rules"));
  card.append(el("p", "note", "One row per rule. The port field takes a list that the comma separates, in the form tcp/22, udp/1-1024."));

  const list = el("div", "ac-rulelist");
  const restore = [];
  for (const row of model.rows) {
    const drawn = drawRuleRow(row);
    list.append(drawn.holder);
    if (drawn.restore) {
      restore.push(drawn.field);
    }
  }
  card.append(list);
  return { card, restore };
}

/** openModeDialog opens the dialog that states what the mode change does. */
function openModeDialog() {
  dialog = { sending: false, error: null };
  redraw();
}

/** closeModeDialog closes the dialog and keeps the mode that the daemon holds. */
function closeModeDialog() {
  dialog = null;
  redraw();
}

/** confirmModeChange sends the new mode and closes the dialog on a success. */
function confirmModeChange(next) {
  dialog = { sending: true, error: null };
  redraw();
  sendModeChange(state, next)
    .then((body) => {
      state.setBase(body);
      dialog = null;
      redraw();
    })
    .catch((err) => {
      dialog = { sending: false, error: err && err.message ? err.message : String(err) };
      redraw();
    });
}

/** drawModeDialog draws the dialog of the mode control. FR-editor-33. */
function drawModeDialog(mode) {
  const change = modeChange(mode);
  const box = el("div", "ac-dialog");
  box.setAttribute("role", "dialog");
  box.setAttribute("aria-modal", "true");
  box.setAttribute("aria-label", change.heading);
  box.append(el("h3", undefined, change.heading));
  for (const sentence of change.sentences) {
    box.append(el("p", "note", sentence));
  }
  if (dialog.error) {
    box.append(el("p", "ns-error", "The daemon refused the change."));
    box.append(el("code", "ns-cmds mono", dialog.error));
  }

  const acts = el("div", "ns-acts ns-dialog-acts");
  const cancel = el("button", "btn", "Cancel");
  cancel.type = "button";
  cancel.addEventListener("click", closeModeDialog);
  acts.append(cancel);

  const confirm = el("button", "btn", dialog.sending ? "The daemon applies the change" : change.confirmLabel);
  confirm.type = "button";
  confirm.disabled = dialog.sending;
  confirm.addEventListener("click", () => confirmModeChange(change.next));
  acts.append(confirm);

  box.append(acts);
  return box;
}

/**
 * draw draws the access view from one poll snapshot.
 *
 * snapshot.status.access_model holds the answer of GET /api/access, which the poll layer
 * reads on every tick. The view opens no request of its own and it starts no timer.
 */
/**
 * accessSectionOrder returns the order that draw appends the sections of the view, as an
 * array of section names. shown states which optional section holds content for this draw.
 * The staged list renders last, after the matrix and the rule list, so a section that a
 * value turns on or off never moves the matrix or the flow overview. Issue #289.
 */
export function accessSectionOrder(shown) {
  const order = ["header"];
  if (shown.warning) {
    order.push("warning");
  }
  if (shown.offer) {
    order.push("offer");
  }
  if (shown.dropped) {
    order.push("dropped");
  }
  if (shown.applyError) {
    order.push("applyFailure");
  }
  if (shown.observe) {
    order.push("observe");
  }
  order.push(shown.empty ? "empty" : "flow");
  if (shown.matrixRows) {
    order.push("matrix");
  }
  if (shown.ruleRows) {
    order.push("rules");
  }
  if (shown.stagedCount > 0) {
    order.push("staged");
  }
  return order;
}

function draw(section, snapshot) {
  redraw = () => draw(section, snapshot);
  section.replaceChildren();

  if (snapshot.loading) {
    const card = el("div", "card");
    card.append(el("span", "label", "Loading"));
    card.append(el("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  const body = snapshot.status && snapshot.status.access_model;
  if (body) {
    state.setBase(body);
  }
  const base = state.base();

  // The warning, the offer, the dropped statement, and the failure all come before the
  // staged list, and the operator reads each one before the next apply. The warning comes
  // first, because it names the one edit that stops a session of the operator.
  const warning = activePathWarning(state.difference(), body && body.active_paths);
  const offer = rebaseOffer(state.baseChanged(), state.count());
  const dropped = droppedStatement(state.dropped());
  const stagedList = stagedListModel(state.difference());
  const observe = observeStatement(base.mode);

  // The empty state reads the staged rule set, because the view draws that rule set. A
  // view that reads the rule set of the daemon states that no rule exists while the
  // picture already holds a staged curve.
  const empty = emptyStatement({ nodes: base.nodes, rules: state.rules() });

  // A host that declares no tailnet and no host node produces no row, so the empty
  // statement stands alone and the view draws no grid with no square.
  const matrix = matrixModel(base, state.rules());

  // A rule set with no rule draws no list, because the empty statement above already names
  // the matrix as the first step.
  const list = ruleListModel(base, state.rules());

  let restore = [];
  const elements = {
    header: drawHeader(base),
    warning: warning ? drawActivePathWarning(warning) : null,
    offer: offer ? drawRebaseOffer(offer) : null,
    dropped: dropped ? drawDropped(dropped) : null,
    applyFailure: applyError !== null ? drawApplyFailure(applyFailureStatement(applyError)) : null,
    observe: observe ? drawObserve(observe) : null,
    empty: empty ? drawEmpty(empty) : null,
    flow: empty ? null : drawFlow(snapshot.status, base.nodes),
    matrix: matrix.rows.length > 0 ? drawMatrix(matrix) : null,
    rules: null,
    staged: stagedList.count > 0 ? drawStagedList(stagedList) : null,
  };
  if (list.rows.length > 0) {
    const rules = drawRules(list);
    elements.rules = rules.card;
    restore = rules.restore;
  }

  // The staged list renders last, after the matrix and the rule list, so staging the first
  // edit grows content below the matrix rather than shifting it. accessSectionOrder holds
  // that order. Issue #289.
  for (const key of accessSectionOrder({
    warning: Boolean(warning),
    offer: Boolean(offer),
    dropped: Boolean(dropped),
    applyError: applyError !== null,
    observe: Boolean(observe),
    empty: Boolean(empty),
    matrixRows: matrix.rows.length > 0,
    ruleRows: list.rows.length > 0,
    stagedCount: stagedList.count,
  })) {
    section.append(elements[key]);
  }

  // The field is in the document now, therefore the view returns the focus that the poll
  // took from the operator.
  for (const field of restore) {
    field.focus();
  }

  if (dialog) {
    const scrim = el("div", "ac-scrim");
    scrim.append(drawModeDialog(base.mode));
    section.append(scrim);
  }
}

registerView("access", draw);
