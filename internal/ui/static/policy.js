// The policy view: the tailnet list, the document editor, and the states that the daemon
// reports.
//
// The upstream policy is the access policy that a control server holds. A local rule
// controls what this host forwards; an upstream policy controls what every device of the
// tailnet reaches. The view states that difference above the editor, because a warning
// comes before the step that it applies to. See FR-policy-28.
//
// The model holds the document that the daemon reported and the text of the operator. The
// difference between the two is the edit, which FR-policy-24 states.
//
// One entry holds one stage, and the stage is the whole state of the two actions. The
// stages are read, validating, validated, validate-failed, pushing, pushed, conflict, and
// push-failed. Push is enabled in the stage validated and in no other stage, therefore
// FR-policy-25 reads from one value rather than from two values that disagree. Every edit
// returns the entry to the stage read, which the behaviour rule of
// features/08-upstream-policy.md requires.
//
// The poll layer holds no policy route, because GET /api/policy/{id} reaches the control
// server and the control server rate-limits. The view therefore reads the list when the
// declared tailnets change, and it reads one document per selection. It starts no timer.
//
// Every function above the drawing section is pure, or it takes its transport as an
// argument, so internal/ui/jstest asserts it under Node with no browser and no network.

import { registerView, requestJSON } from "./app.js";

/** The route that lists every tailnet with its control server kind. */
export const POLICY_ROUTE = "/api/policy";

/** The one sentence that this view carries and no other view does. FR-policy-28. */
export const EVERY_DEVICE_STATEMENT =
  "A policy change affects every device in the tailnet, not only this host.";

/**
 * The two sentences that come before the push action.
 *
 * A push writes the document that every device in the tailnet reads, therefore the copy
 * names the exact effect and it states what survives. The console shows it beside the
 * action and not only in the heading of the view.
 */
export const PUSH_STATEMENT = {
  effect: "Push replaces the policy document of the control server, and it changes what every device in the tailnet reaches.",
  survives: "The local rule set of this host does not change, because the two systems are independent.",
};

/**
 * The status that the daemon returns when the control server reports a conflict.
 *
 * The Tailscale control server returns HTTP 412 when the If-Match value does not match its
 * ETag value, and internal/api/policy.go maps that to HTTP 409. The console reads this
 * status, because a message is text for a person and the status is the value that the
 * daemon owns. See FR-policy-18.
 */
export const CONFLICT_STATUS = 409;

/** The text of policy.ErrHeadscaleFileMode that names the mode a write needs. */
const FILE_MODE_MARKER = 'policy.mode: "database"';

/** esc states a value that the daemon reported as text that an XML parser accepts. */
function esc(value) {
  return String(value === null || value === undefined ? "" : value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/** policyDocumentRoute returns the route of one policy document. */
export function policyDocumentRoute(id) {
  return `${POLICY_ROUTE}/${encodeURIComponent(id)}`;
}

/** policyValidateRoute returns the route that checks one document and writes nothing. */
export function policyValidateRoute(id) {
  return `${policyDocumentRoute(id)}/validate`;
}

/** policySectionsRoute returns the route that parses one document into its sections. */
export function policySectionsRoute(id) {
  return `${policyDocumentRoute(id)}/sections`;
}

/** policySectionsEditRoute returns the route that applies one edit to a document. */
export function policySectionsEditRoute(id) {
  return `${policySectionsRoute(id)}/edit`;
}

/** messageOf returns the message that a rejected request stated, word for word. */
function messageOf(err) {
  return err && err.message ? err.message : String(err);
}

/**
 * lineNumberOf returns the line number that one error names, and null for an error that
 * names none.
 *
 * The control servers state a line in two shapes: "line 12" and "policy.hujson:7:3". The
 * function reads both, and it changes no character of the message. See FR-policy-26.
 */
function lineNumberOf(message) {
  const named = /\bline (\d+)/i.exec(message);
  if (named) {
    return Number(named[1]);
  }
  const positional = /:(\d+):\d+/.exec(message);
  if (positional) {
    return Number(positional[1]);
  }
  return null;
}

/**
 * validateErrors returns one entry per error of a validate result.
 *
 * result is the field result of POST /api/policy/{id}/validate, which holds the answer of
 * the control server word for word. A Tailscale answer is JSON that carries the field
 * message, and a Headscale answer is the message of the daemon. validateErrors takes the
 * message of the JSON answer when the answer holds one, and the whole text otherwise. It
 * returns one entry per line, each with its line number and its message word for word.
 */
export function validateErrors(result) {
  const text = String(result === null || result === undefined ? "" : result).trim();
  if (text === "") {
    return [];
  }
  return statedMessage(text)
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .map((message) => ({ line: lineNumberOf(message), message }));
}

/** statedMessage returns the field message of a JSON answer, or the whole text. */
function statedMessage(text) {
  try {
    const parsed = JSON.parse(text);
    if (parsed && typeof parsed.message === "string" && parsed.message !== "") {
      return parsed.message;
    }
  } catch {
    return text;
  }
  return text;
}

/**
 * policyRows returns one row per tailnet of GET /api/policy.
 *
 * body is the answer of the route and selectedId is the identifier that the operator
 * selected. A row states the credential state as a tone and a lowercase word, and reason
 * holds the message of the daemon word for word.
 */
export function policyRows(body, selectedId) {
  const tailnets = (body && body.tailnets) || [];
  return tailnets.map((tailnet) => {
    let tone = "ok";
    let word = "read and write";
    if (!tailnet.credential_present) {
      tone = "crit";
      word = "no credential";
    } else if (tailnet.credential_state === "rejected") {
      // The tailnet holds a credential that the control server takes for no request. The
      // state read `read and write` before, because the row states the presence of a
      // credential and a present credential that works for nothing looks the same.
      // See issue #276.
      tone = "crit";
      word = "credential rejected";
    } else if (!tailnet.write_available) {
      tone = "warn";
      word = "read only";
    }
    return {
      id: tailnet.id,
      kind: tailnet.kind,
      tone,
      word,
      reason: tailnet.reason || "",
      selected: tailnet.id === selectedId,
    };
  });
}

/** policyListMarkup returns the list of tailnets. Every row reaches focus by keyboard. */
export function policyListMarkup(rows) {
  const lines = rows.map((row) => {
    const reason = row.reason ? `<p class="pol-reason mono">${esc(row.reason)}</p>` : "";
    return (
      `<div class="pol-row" role="option" data-id="${esc(row.id)}" aria-selected="${row.selected ? "true" : "false"}" tabindex="0">` +
      `<div class="pol-line">` +
      `<span class="pol-id mono">${esc(row.id)}</span>` +
      `<span class="pol-state"><span class="dot ${esc(row.tone)}"></span><span class="pol-word">${esc(row.word)}</span></span>` +
      `</div>` +
      `<span class="pol-kind mono">${esc(row.kind)}</span>` +
      reason +
      `</div>`
    );
  });
  return `<div class="pol-list" role="listbox" aria-label="Tailnets">${lines.join("")}</div>`;
}

/**
 * emptyStatement returns what would fill the view, and an empty string when the daemon
 * reports a tailnet. FR-console-17.
 */
export function emptyStatement(body) {
  if (policyRows(body, null).length > 0) {
    return "";
  }
  return "The daemon declares no tailnet. Add one, and this view lists it with its control server kind and its credential state.";
}

/**
 * readFailure classifies the message that a failed read returned.
 *
 * message is the message of the daemon word for word. readFailure returns the kind of the
 * failure and the sentence that comes before the message. It rewrites no message, because
 * the message names the address, the status, and the reason.
 */
export function readFailure(message) {
  const text = String(message === null || message === undefined ? "" : message);
  if (text.includes(FILE_MODE_MARKER)) {
    return {
      kind: "file-mode",
      sentence: "This tailnet is read only. The control server runs the file policy mode.",
    };
  }
  if (text.includes("has no") && text.includes("credential")) {
    return {
      kind: "no-credential",
      sentence: "This tailnet needs a credential before the console reads its policy document.",
    };
  }
  return { kind: "unreachable", sentence: "The control server did not answer." };
}

/**
 * editorModel returns what the editor region draws for one identifier.
 *
 * state is the model that createPolicyState returned and id is the selected identifier, or
 * null. The state field names one of unselected, no-credential, loading, document,
 * read-only, and failed. Each one reads a value that the daemon reported.
 * The stage field names the stage of the two actions, and the result field holds the
 * answer of the last action word for word.
 */
export function editorModel(state, id) {
  if (!id) {
    return { id: "", state: "unselected", stage: "read", result: "", lines: 0, text: "", edited: false, readOnly: true, etag: "", detail: "", sentence: "" };
  }

  const row = state.rows().find((entry) => entry.id === id);
  const held = state.entry(id);
  const model = {
    id,
    kind: row ? row.kind : "",
    state: "loading",
    stage: held.stage,
    result: held.result,
    lines: 0,
    text: held.text,
    edited: held.text !== held.base,
    readOnly: true,
    etag: held.etag,
    detail: held.error,
    sentence: "",
  };

  if (row && row.word === "no credential") {
    model.state = "no-credential";
    model.detail = row.reason;
    model.sentence = "This tailnet needs a credential before the console reads its policy document.";
    return model;
  }
  if (held.error) {
    const failure = readFailure(held.error);
    model.state = failure.kind === "file-mode" ? "read-only" : "failed";
    model.sentence = failure.sentence;
    return model;
  }
  if (!held.loaded) {
    model.sentence = "The console reads the policy document from the control server.";
    return model;
  }

  model.state = "document";
  // The editor takes no edit while the push runs, because the control server holds the
  // text of that request and an edit would then cover a document that no push carried.
  model.readOnly = !held.writeAvailable || held.stage === "pushing";
  model.lines = held.text.split("\n").length;
  if (!held.writeAvailable) {
    model.sentence = "This tailnet is read only. The daemon reports no write availability for its control server.";
  }
  return model;
}

/**
 * toggleModel returns the state of the Visual/Text control of the header, per
 * FR-vacl-1.
 *
 * id is the identifier that the operator selected. The Visual control stays present
 * and it disables only while the last attempt to parse the staged text failed, per
 * FR-vacl-2: sectionsError then names the parse error, and view stays "text" because
 * the visual editor cannot draw a document it cannot parse.
 */
export function toggleModel(state, id) {
  const entry = state.entry(id);
  return {
    view: entry.view,
    visualDisabled: entry.sectionsError !== "",
    error: entry.sectionsError,
    sections: entry.sections,
  };
}

/**
 * toggleMarkup returns the Visual/Text control, and the parse error beside it.
 *
 * toggle is the value that toggleModel returned. The parse error reaches the screen
 * word for word, because .claude/rules/ste.md treats an error message as evidence, and
 * it names the line and the column that internal/policy.Parse returns per FR-model-4.
 */
export function toggleMarkup(toggle) {
  const visualSelected = toggle.view === "visual";
  const error = toggle.error ? `<p class="note pol-parse-error mono">${esc(toggle.error)}</p>` : "";
  return (
    `<span class="seg" role="tablist" aria-label="Editor view">` +
    `<button type="button" role="tab" aria-selected="${visualSelected}" data-view="visual"${toggle.visualDisabled ? " disabled" : ""}>Visual</button>` +
    `<button type="button" role="tab" aria-selected="${!visualSelected}" data-view="text">Text</button>` +
    `</span>` +
    error
  );
}

/**
 * visualMarkup returns the visual editor region, drawn from the sections that the
 * daemon parsed out of the staged text.
 *
 * features/12-visual-acl-editor.md's sibling issues #315 to #317 build the matrix, the
 * rule list, and the named-set editors. This issue draws only the count of each
 * section, which proves that a toggle to Visual re-parses and re-renders the current
 * text, per FR-vacl-2 and FR-vacl-3. The opaque keys reach the screen once, per
 * FR-vacl-18, so the operator knows to use Text for them.
 */
export function visualMarkup(sections) {
  if (!sections) {
    return `<div class="pol-visual"></div>`;
  }
  const count = (value) => (Array.isArray(value) ? value.length : Object.keys(value || {}).length);
  const rows = [
    ["Groups", count(sections.groups)],
    ["Hosts", count(sections.hosts)],
    ["Tag owners", count(sections.tagOwners)],
    ["IP sets", count(sections.ipsets)],
    ["Rules", count(sections.acls) + count(sections.grants)],
  ];
  const items = rows
    .map(([label, n]) => `<div class="setrow"><span class="name">${esc(label)}</span><span class="mono">${n}</span></div>`)
    .join("");
  const opaque = sections.opaque_keys && sections.opaque_keys.length
    ? `<p class="note">Use Text to read or change ${esc(sections.opaque_keys.join(", "))}.</p>`
    : "";
  const matrix = matrixMarkup(matrixModel(sections));
  return `<div class="pol-visual">${items}${opaque}${matrix}</div>`;
}

/**
 * destHost returns the destination of one acls entry with its port removed.
 *
 * dst is one entry of an acls entry's dst field, in the form tag:server:* or
 * tag:server:443. The port comes after the last colon. A grants entry carries no port
 * in its dst field, because the Tailscale grants syntax holds the port in the ip field
 * instead, so destHost applies to an acls entry alone.
 */
function destHost(dst) {
  const at = dst.lastIndexOf(":");
  return at === -1 ? dst : dst.slice(0, at);
}

/** aclPairs returns one {src, dst} pair for every source and destination that one acls
 *  entry names, with the port removed from each destination. */
function aclPairs(rule) {
  const srcs = (rule && rule.src) || [];
  const dsts = ((rule && rule.dst) || []).map(destHost);
  const pairs = [];
  for (const src of srcs) {
    for (const dst of dsts) {
      pairs.push({ src, dst });
    }
  }
  return pairs;
}

/** grantPairs returns one {src, dst} pair for every source and destination that one
 *  grants entry names. A grants entry's dst field carries no port. */
function grantPairs(entry) {
  const srcs = (entry && entry.src) || [];
  const dsts = (entry && entry.dst) || [];
  const pairs = [];
  for (const src of srcs) {
    for (const dst of dsts) {
      pairs.push({ src, dst });
    }
  }
  return pairs;
}

/** reachabilityPairs returns one {src, dst} pair for every path that an acls entry or a
 *  grants entry of sections allows. */
function reachabilityPairs(sections) {
  const acls = (sections && sections.acls) || [];
  const grants = (sections && sections.grants) || [];
  return [...acls.flatMap(aclPairs), ...grants.flatMap(grantPairs)];
}

/**
 * matrixSquare returns one square of the reachability matrix.
 *
 * allowed holds the key "source destination" of every allowed path. The diagonal is
 * inert, in the manner of FR-editor-10 of features/07-console-access-editor.md, because
 * a path from one node to itself names nothing to allow or to deny.
 */
function matrixSquare(from, to, allowed) {
  const inert = from === to;
  const on = !inert && allowed.has(`${from} ${to}`);
  let label = `${from} to ${to}, no rule`;
  if (inert) {
    label = `${from} to ${to}, not applicable`;
  } else if (on) {
    label = `${from} to ${to}, allowed`;
  }
  return { from, to, allowed: on, inert, label };
}

/**
 * matrixModel returns the grid of squares of the reachability matrix, per FR-vacl-7.
 *
 * sections is the answer of POST .../sections. The matrix places every tag, group, and
 * autogroup that an acls entry or a grants entry references on both axes, sorted so the
 * grid draws the same order on every call. matrixModel carries no port, because the
 * ports live in the rule list that features/12-visual-acl-editor.md's issue #317 builds.
 */
export function matrixModel(sections) {
  const pairs = reachabilityPairs(sections);
  const nodeSet = new Set();
  for (const pair of pairs) {
    nodeSet.add(pair.src);
    nodeSet.add(pair.dst);
  }
  const nodes = [...nodeSet].sort();
  const allowed = new Set(pairs.map((pair) => `${pair.src} ${pair.dst}`));
  return {
    nodes,
    rows: nodes.map((source) => ({
      source,
      squares: nodes.map((destination) => matrixSquare(source, destination, allowed)),
    })),
  };
}

/**
 * matrixClickPlan returns the edit that one click on a matrix square stages, and null
 * for the diagonal, per FR-vacl-7.
 *
 * An empty square plans one acls entry that allows every port from the row's source to
 * the column's destination, per FR-vacl-8. A filled square plans the removal of every
 * acls entry and every grants entry that names that path, per FR-vacl-9. Removing an
 * entry at index i shifts every later index of its own section down by one, so the plan
 * orders each section's removals from the highest index to the lowest.
 */
export function matrixClickPlan(sections, from, to) {
  if (from === to) {
    return null;
  }
  const acls = (sections && sections.acls) || [];
  const grants = (sections && sections.grants) || [];
  const matches = (entries, pairsOf) =>
    entries.reduce((indices, entry, index) => {
      if (pairsOf(entry).some((pair) => pair.src === from && pair.dst === to)) {
        indices.push(index);
      }
      return indices;
    }, []);
  const removals = [
    ...matches(acls, aclPairs)
      .sort((a, b) => b - a)
      .map((index) => ({ section: "acls", index })),
    ...matches(grants, grantPairs)
      .sort((a, b) => b - a)
      .map((index) => ({ section: "grants", index })),
  ];
  if (removals.length > 0) {
    return { op: "remove", removals };
  }
  return {
    op: "add",
    section: "acls",
    entry: { action: "accept", src: [from], dst: [`${to}:*`] },
  };
}

/**
 * matrixMarkup returns the reachability matrix region.
 *
 * model is the value that matrixModel returned. The square is a button, so it reaches
 * focus by keyboard, and the diagonal carries the disabled attribute per FR-vacl-7. The
 * console binds the click, the hover, the focus, and the blur handlers after it sets
 * this markup, because a handler that a string carries never runs.
 */
export function matrixMarkup(model) {
  const columns = model.nodes.map((node) => `<th class="col" scope="col">${esc(node)}</th>`).join("");
  const rows = model.rows
    .map((row) => {
      const cells = row.squares
        .map((square) => {
          const className = square.inert ? "ac-square inert" : square.allowed ? "ac-square on" : "ac-square";
          const disabled = square.inert ? " disabled" : "";
          const data = square.inert ? "" : ` data-from="${esc(square.from)}" data-to="${esc(square.to)}"`;
          return `<td><button type="button" class="${className}" aria-label="${esc(square.label)}"${disabled}${data}></button></td>`;
        })
        .join("");
      return `<tr><th class="row" scope="row">${esc(row.source)}</th>${cells}</tr>`;
    })
    .join("");
  return (
    `<div class="card ac-matrix">` +
    `<span class="label">Reachability</span>` +
    `<p class="note">A filled square means that an acls entry or a grants entry allows the path. The ports live in the rule list.</p>` +
    `<table class="ac-mtx"><thead><tr><th></th>${columns}</tr></thead><tbody>${rows}</tbody></table>` +
    `</div>`
  );
}

/** The label of the validate action while the control server checks the document. */
export const VALIDATING_LABEL = "The control server checks the document";

/** The label of the push action while the control server takes the document. */
export const PUSHING_LABEL = "The control server takes the document";

/**
 * actionsModel returns the controls of the editor region, in the order that they draw.
 *
 * model is the value that editorModel returned. The push is enabled in the stage validated
 * and in no other stage, which FR-policy-25 states. The accent marks the push alone,
 * because the push is the affirmative action of this view.
 * A tailnet that takes no write gets every control disabled, and a request that runs
 * disables every control until the answer arrives.
 */
export function actionsModel(model) {
  if (model.state !== "document") {
    return [];
  }
  const busy = model.stage === "validating" || model.stage === "pushing";
  const writable = !model.readOnly && !busy;
  return [
    {
      id: "validate",
      label: model.stage === "validating" ? VALIDATING_LABEL : "Validate",
      accent: false,
      disabled: !writable,
    },
    { id: "discard", label: "Discard", accent: false, disabled: !model.edited || busy },
    {
      id: "push",
      label: model.stage === "pushing" ? PUSHING_LABEL : "Push",
      accent: true,
      disabled: model.stage !== "validated",
    },
  ];
}

/**
 * resultModel returns what the result region states, and null while no action returned.
 *
 * model is the value that editorModel returned. Every message of the control server and of
 * the daemon reaches the field message or the field errors word for word, because
 * .claude/rules/ste.md states that a rewritten message is destroyed evidence.
 * The field reread is true for the conflict alone, which offers the re-read action.
 */
export function resultModel(model) {
  const result = (tone, word, sentence, extra = {}) => ({
    tone,
    word,
    sentence,
    errors: [],
    message: "",
    reread: false,
    ...extra,
  });

  switch (model.stage) {
    case "validated": {
      const detail = model.result && model.result !== "{}" ? { message: model.result } : {};
      return result("ok", "validated", "The control server accepted the document.", detail);
    }
    case "validate-failed":
      return result("crit", "validate failed", "The control server rejected the document.", {
        errors: validateErrors(model.result),
      });
    case "pushed":
      return result("ok", "pushed", "The control server accepted the document, and the daemon recorded the event policy.pushed.", {
        message: model.result,
      });
    case "conflict":
      return result("warn", "conflict", "Another person changed the policy document of the control server after this console read it. The control server kept its document, and this editor keeps your text.", {
        message: model.result,
        reread: true,
      });
    case "push-failed":
      return result("crit", "push failed", "The control server did not take the document.", { message: model.result });
    default:
      return null;
  }
}

/** gutterMarkup returns one element per line of the document. */
function gutterMarkup(lines) {
  let markup = "";
  for (let line = 1; line <= lines; line += 1) {
    markup += `<div>${line}</div>`;
  }
  return `<div class="pol-gut" aria-hidden="true">${markup}</div>`;
}

/** noteMarkup returns one sentence of the editor region. */
function noteMarkup(sentence) {
  return sentence ? `<p class="note">${esc(sentence)}</p>` : "";
}

/** detailMarkup returns the message of the daemon word for word, as a machine value. */
function detailMarkup(detail) {
  return detail ? `<p class="pol-detail mono">${esc(detail)}</p>` : "";
}

/** buttonMarkup returns one control of the editor region. */
function buttonMarkup(control) {
  const className = control.accent && !control.disabled ? "btn primary" : "btn";
  return (
    `<button type="button" class="${className}" data-act="${esc(control.id)}"${control.disabled ? " disabled" : ""}>` +
    `${esc(control.label)}</button>`
  );
}

/**
 * actionsMarkup returns the controls of the editor region.
 *
 * controls is the list that actionsModel returned. The two sentences of PUSH_STATEMENT
 * come before the controls, because .claude/rules/ste.md states that a warning comes
 * before the step that it applies to.
 */
export function actionsMarkup(controls) {
  if (controls.length === 0) {
    return `<div class="pol-acts"></div>`;
  }
  return (
    `<div class="pol-acts">` +
    `<div class="pol-push-note">` +
    `<p class="note">${esc(PUSH_STATEMENT.effect)}</p>` +
    `<p class="note">${esc(PUSH_STATEMENT.survives)}</p>` +
    `</div>` +
    `<div class="pol-btns">${controls.map(buttonMarkup).join("")}</div>` +
    `</div>`
  );
}

/**
 * resultMarkup returns the result region of the last action.
 *
 * result is the value that resultModel returned, or null. The state reads as a coloured
 * dot and a lowercase word. Every message of the control server draws in the mono
 * typeface, because the control server owns that text.
 */
export function resultMarkup(result) {
  if (!result) {
    return `<div class="pol-result"></div>`;
  }
  const errors = result.errors
    .map((one) => {
      const line = one.line === null ? "" : `<span class="pol-err-line mono">line ${one.line}</span>`;
      return `<div class="pol-err">${line}<span class="pol-err-msg mono">${esc(one.message)}</span></div>`;
    })
    .join("");
  const message = result.message ? `<p class="pol-detail mono">${esc(result.message)}</p>` : "";
  const reread = result.reread
    ? `<div class="pol-btns">${buttonMarkup({ id: "reread", label: "Read the document again", accent: false, disabled: false })}</div>`
    : "";
  return (
    `<div class="pol-result">` +
    `<div class="pol-res-head"><span class="dot ${esc(result.tone)}"></span><span class="pol-word">${esc(result.word)}</span></div>` +
    `<p class="note">${esc(result.sentence)}</p>` +
    message +
    (errors ? `<div class="pol-errs">${errors}</div>` : "") +
    reread +
    `</div>`
  );
}

/**
 * editorMarkup returns the editor region of one model.
 *
 * The statement of FR-policy-28 comes first in every state, therefore the operator reads
 * it before the document. The serializer escapes every value that the daemon reported: a
 * policy document is text of a control server and the console has no authentication.
 *
 * toggle is the value that toggleModel returned, and it is null before a tailnet holds a
 * document. It draws the Visual/Text control of FR-vacl-1, and it selects the visual
 * editor region or the text editor region below it, per FR-vacl-2 and FR-vacl-3.
 */
export function editorMarkup(model, toggle = null) {
  const warning = `<p class="note pol-warning">${esc(EVERY_DEVICE_STATEMENT)}</p>`;

  if (model.state !== "document") {
    const label = model.id ? `<span class="pol-name mono">${esc(model.id)}</span>` : "";
    const sentence = model.state === "unselected"
      ? "Select a tailnet to read the policy document that its control server holds."
      : model.sentence;
    return (
      `<div class="card pol-region">${warning}${label}` +
      noteMarkup(sentence) +
      detailMarkup(model.detail) +
      `</div>`
    );
  }

  const chip = `<span class="chip mono"${model.edited ? "" : " hidden"}>edited</span>`;
  const etag = model.etag ? `<span class="chip mono">etag ${esc(model.etag)}</span>` : "";
  const readOnly = model.readOnly ? " readonly" : "";
  const showVisual = toggle && toggle.view === "visual";
  const body = showVisual
    ? visualMarkup(toggle.sections)
    : `<div class="pol-ed">` +
      `<div class="pol-bar"><span class="pol-name mono">${esc(model.id)} &middot; policy.hujson</span>${chip}</div>` +
      `<div class="pol-code">${gutterMarkup(model.lines)}` +
      // The rows attribute holds the line count, therefore the text area needs no
      // scrolling box of its own and the line numbers stay beside their lines.
      `<textarea class="pol-doc mono"${readOnly} rows="${model.lines}" spellcheck="false" wrap="off" aria-label="The policy document of ${esc(model.id)}">${esc(model.text)}</textarea>` +
      `</div></div>`;
  return (
    `<div class="pol-region">${warning}` +
    noteMarkup(model.sentence) +
    (toggle ? toggleMarkup(toggle) : "") +
    body +
    (etag ? `<div class="pol-meta">${etag}</div>` : "") +
    actionsMarkup(actionsModel(model)) +
    resultMarkup(resultModel(model)) +
    `</div>`
  );
}

/**
 * createPolicyState returns the model of the policy view.
 *
 * options.request sends one request. It takes the route, the method, and the body, and it
 * rejects with the message that the daemon stated and the status in the field status. A
 * test replaces it.
 *
 * The model holds the list of GET /api/policy, and one entry per tailnet that the operator
 * opened. An entry holds the document of the read, the text of the operator, the message
 * of a failed read, and the stage of the two actions.
 * The validate action and the push action each send one request. Neither one sends a
 * second request after a failure, because an automatic retry against a rate limit of the
 * control server makes that limit worse.
 */
export function createPolicyState(options = {}) {
  const request = options.request || ((route, method, body) => requestJSON(route, method, body));

  let list = null;
  let selectedId = null;
  const entries = new Map();

  function entryOf(id) {
    let entry = entries.get(id);
    if (!entry) {
      entry = {
        loaded: false, base: "", text: "", etag: "", writeAvailable: false, error: "", stage: "read", result: "",
        // view holds "text" or "visual", per FR-vacl-1. sections holds the last answer
        // of POST .../sections, and sectionsError names the parse error of the last
        // attempt to open Visual, per FR-vacl-2. sectionsPending is true while that
        // request runs.
        view: "text", sections: null, sectionsError: "", sectionsPending: false,
      };
      entries.set(id, entry);
    }
    return entry;
  }

  /**
   * rest returns the entry to the stage read, which disables the push. It clears the
   * parse error of the last attempt to open Visual, because that error covers text the
   * entry no longer holds.
   */
  function rest(entry) {
    entry.stage = "read";
    entry.result = "";
    entry.sectionsError = "";
  }

  return {
    /** rows returns one row per tailnet of the list, with the selection marked. */
    rows() {
      return policyRows(list, selectedId);
    },

    /** body returns the answer of GET /api/policy, or null before the first read. */
    body() {
      return list;
    },

    /** selected returns the identifier that the operator selected, or null. */
    selected() {
      return selectedId;
    },

    /** entry returns the document, the text, and the failure of one identifier. */
    entry(id) {
      return entryOf(id);
    },

    /**
     * setList takes one answer of GET /api/policy.
     *
     * setList keeps the text of the operator, because the console removes no edit of its
     * own. It drops the selection when the answer no longer holds the tailnet.
     */
    setList(body) {
      list = body;
      const known = new Set(policyRows(list, null).map((row) => row.id));
      if (selectedId !== null && !known.has(selectedId)) {
        selectedId = null;
      }
    },

    /** setDocument takes one answer of GET /api/policy/{id}. */
    setDocument(id, body) {
      const entry = entryOf(id);
      entry.loaded = true;
      entry.error = "";
      entry.base = (body && body.document) || "";
      entry.text = entry.base;
      entry.etag = (body && body.etag) || "";
      entry.writeAvailable = Boolean(body && body.write_available);
      rest(entry);
    },

    /** setError states the message that a failed read returned, word for word. */
    setError(id, message) {
      const entry = entryOf(id);
      entry.error = String(message === null || message === undefined ? "" : message);
    },

    /**
     * setText replaces the text of the operator. It sends nothing.
     *
     * An edit returns the entry to the stage read, therefore an edit after a validate that
     * passed disables the push again. The behaviour rule of
     * features/08-upstream-policy.md requires that.
     */
    setText(id, text) {
      const entry = entryOf(id);
      if (entry.text === text) {
        return;
      }
      entry.text = text;
      rest(entry);
    },

    /** discard returns the text to the document that the console read. */
    discard(id) {
      const entry = entryOf(id);
      entry.text = entry.base;
      rest(entry);
    },

    /** edited reports whether the text differs from the document of the read. */
    edited(id) {
      const entry = entryOf(id);
      return entry.text !== entry.base;
    },

    /** select marks one identifier, and null marks none. It opens no request. */
    select(id) {
      selectedId = id;
    },

    /** setView switches between the Text editor and the Visual editor. It sends no request. */
    setView(id, view) {
      entryOf(id).view = view;
    },

    /**
     * loadSections sends the staged text to POST .../sections, per FR-vacl-2 and
     * FR-vacl-3. On success, it switches the entry to the visual view and it holds the
     * parsed sections. On a parse failure, it keeps the entry on the text view and it
     * states the parse error inline, because the visual editor cannot draw a document
     * it cannot parse. loadSections sends one request, and it sends no second request
     * while the previous one runs.
     * The operator edits the text while the request runs, therefore loadSections reads
     * the text again when the answer arrives. A text that changed applies neither the
     * sections nor the error of the stale answer.
     */
    async loadSections(id) {
      const entry = entryOf(id);
      if (entry.sectionsPending) {
        return;
      }
      const sent = entry.text;
      entry.sectionsPending = true;
      try {
        const answer = await request(policySectionsRoute(id), "POST", { document: sent });
        if (entry.text !== sent) {
          return;
        }
        entry.sections = answer;
        entry.sectionsError = "";
        entry.view = "visual";
      } catch (err) {
        if (entry.text !== sent) {
          return;
        }
        entry.sectionsError = messageOf(err);
      } finally {
        entry.sectionsPending = false;
      }
    },

    /**
     * stageMatrixClick applies the edit that one click on a matrix square stages, per
     * FR-vacl-8 and FR-vacl-9.
     *
     * id is the tailnet identifier, and from and to name the row's source and the
     * column's destination. stageMatrixClick sends one edit request for an add, or one
     * request per removal for a remove, each one against the document text that the
     * previous request returned, because /sections/edit is stateless and every request
     * after the first must see the earlier request's result. It sends no request for
     * the diagonal. After the last edit, it reads the sections again, per the Interfaces
     * section of features/12-visual-acl-editor.md.
     */
    async stageMatrixClick(id, from, to) {
      const entry = entryOf(id);
      if (!entry.sections) {
        return;
      }
      const plan = matrixClickPlan(entry.sections, from, to);
      if (!plan) {
        return;
      }
      if (plan.op === "add") {
        const answer = await request(policySectionsEditRoute(id), "POST", {
          document: entry.text,
          section: plan.section,
          op: "add",
          entry: plan.entry,
        });
        entry.text = answer.document;
      } else {
        for (const removal of plan.removals) {
          const answer = await request(policySectionsEditRoute(id), "POST", {
            document: entry.text,
            section: removal.section,
            op: "remove",
            index: removal.index,
          });
          entry.text = answer.document;
        }
      }
      rest(entry);
      await this.loadSections(id);
    },

    /** loadList reads GET /api/policy. */
    async loadList() {
      this.setList(await request(POLICY_ROUTE));
    },

    /**
     * open selects one tailnet and reads its document.
     *
     * open reads the document once per selection, because a read reaches the control
     * server. It reads no document for a tailnet that holds no credential, and it states
     * the message of the daemon when the read fails.
     */
    async open(id) {
      selectedId = id;
      const row = this.rows().find((entry) => entry.id === id);
      if (row && row.word === "no credential") {
        return;
      }
      const entry = entryOf(id);
      if (entry.loaded || entry.error) {
        return;
      }
      try {
        this.setDocument(id, await request(policyDocumentRoute(id)));
      } catch (err) {
        this.setError(id, messageOf(err));
      }
    },

    /**
     * validate sends the text of the operator to POST /api/policy/{id}/validate.
     *
     * validate writes nothing on the control server. It sends one request, and it sends no
     * second request after a failure.
     * The operator edits while the request runs, therefore validate reads the text again
     * when the answer arrives. A text that changed leaves the entry in the stage read, and
     * the push stays disabled.
     */
    async validate(id) {
      const entry = entryOf(id);
      if (!entry.loaded) {
        return;
      }
      const sent = entry.text;
      entry.stage = "validating";
      entry.result = "";
      try {
        const answer = await request(policyValidateRoute(id), "POST", { document: sent });
        if (entry.text !== sent) {
          return;
        }
        entry.stage = answer && answer.passed ? "validated" : "validate-failed";
        entry.result = (answer && answer.result) || "";
      } catch (err) {
        if (entry.text !== sent) {
          return;
        }
        entry.stage = "validate-failed";
        entry.result = messageOf(err);
      }
    },

    /**
     * push sends the text of the operator to PUT /api/policy/{id}.
     *
     * push sends no request while the stage is not validated, which FR-policy-25 states.
     * It sends the ETag value of the read, and it sends none for a control server that
     * holds none.
     * A refusal with the status CONFLICT_STATUS moves the entry to the stage conflict. push
     * reads that status and never the message of the daemon.
     * push sends one request, and it sends no second request after a failure.
     */
    async push(id) {
      const entry = entryOf(id);
      if (entry.stage !== "validated") {
        return;
      }
      const body = { document: entry.text };
      if (entry.etag) {
        body.etag = entry.etag;
      }
      entry.stage = "pushing";
      entry.result = "";
      try {
        const answer = await request(policyDocumentRoute(id), "PUT", body);
        entry.base = answer && typeof answer.document === "string" ? answer.document : body.document;
        entry.text = entry.base;
        entry.etag = (answer && answer.etag) || "";
        entry.stage = "pushed";
        entry.sectionsError = "";
      } catch (err) {
        entry.stage = err && err.status === CONFLICT_STATUS ? "conflict" : "push-failed";
        entry.result = messageOf(err);
      }
    },

    /**
     * reread reads GET /api/policy/{id} again and it keeps the text of the operator.
     *
     * The operator compares their text against the document that the control server holds
     * now, therefore reread replaces the document of the read and it keeps the text. The
     * console removes no work of the operator to resolve a conflict.
     * A read that fails keeps the stage conflict, and it states the message of the failed
     * read. reread sends one request and no second request.
     */
    async reread(id) {
      const entry = entryOf(id);
      const kept = entry.text;
      try {
        const answer = await request(policyDocumentRoute(id));
        this.setDocument(id, answer);
        entry.text = kept;
      } catch (err) {
        entry.result = messageOf(err);
      }
    },
  };
}

// ---------------------------------------------------------------------------
// The drawing. Everything below this line needs a document.
// ---------------------------------------------------------------------------

const state = createPolicyState({});

let redraw = () => {};

// listKey is the set of tailnets that the last list read covers. The view reads the list
// again when the daemon declares another set, and on no other tick.
let listKey = null;
let listPending = false;
let listError = "";

// caret holds the position of the operator inside the editor. A poll draws the region
// again, therefore the view returns the focus and the position that the poll took.
let caret = null;

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

/** declaredKey states which tailnets the daemon declares, in one value. */
function declaredKey(snapshot) {
  const desired = snapshot.status && snapshot.status.desired;
  return Object.keys(desired || {}).sort().join(" ");
}

/** readList reads GET /api/policy when the declared tailnets change. */
function readList(snapshot) {
  const key = declaredKey(snapshot);
  if (listPending || key === listKey) {
    return;
  }
  listPending = true;
  state
    .loadList()
    .then(() => {
      listKey = key;
      listError = "";
    })
    .catch((err) => {
      listError = err && err.message ? err.message : String(err);
    })
    .finally(() => {
      listPending = false;
      redraw();
    });
}

/** drawGutter writes one line number per line of the text. */
function drawGutter(gutter, text) {
  gutter.replaceChildren();
  const lines = text.split("\n").length;
  for (let line = 1; line <= lines; line += 1) {
    gutter.append(element("div", undefined, String(line)));
  }
}

/** bindRows opens the tailnet that the operator selects, by pointer and by keyboard. */
function bindRows(holder) {
  for (const row of holder.querySelectorAll("[data-id]")) {
    const id = row.getAttribute("data-id");
    const open = () => {
      caret = null;
      state.open(id).then(redraw);
      redraw();
    };
    row.addEventListener("click", open);
    row.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        open();
      }
    });
  }
}

/**
 * runAction draws the busy state, and it draws the answer when the request returns.
 *
 * work is the promise of one action of the state. The operator starts every action, and
 * the view starts no timer, therefore the console sends no request of its own and it
 * repeats no request that failed.
 */
function runAction(work) {
  caret = null;
  redraw();
  work.then(redraw);
}

/** bindActions wires each control of the editor region to its action. */
function bindActions(holder, id) {
  const actions = {
    validate: () => runAction(state.validate(id)),
    push: () => runAction(state.push(id)),
    reread: () => runAction(state.reread(id)),
    discard: () => {
      state.discard(id);
      caret = null;
      redraw();
    },
  };
  for (const button of holder.querySelectorAll("[data-act]")) {
    const action = actions[button.getAttribute("data-act")];
    button.addEventListener("click", action);
  }
}

/**
 * bindToggle wires the Visual and the Text control to the state, per FR-vacl-1.
 *
 * Selecting Text switches the view and it sends no request, because the two editors
 * share one staged string. Selecting Visual sends the staged text to
 * POST .../sections, per FR-vacl-2.
 */
function bindToggle(holder, id) {
  for (const button of holder.querySelectorAll("[data-view]")) {
    const view = button.getAttribute("data-view");
    button.addEventListener("click", () => {
      if (view === "text") {
        state.setView(id, "text");
        caret = null;
        redraw();
        return;
      }
      runAction(state.loadSections(id));
    });
  }
}

/**
 * bindMatrix wires the reachability matrix to the state, per FR-vacl-7 to FR-vacl-9.
 *
 * The square is a button, so the pointer and the keyboard both reach it. Hovering or
 * focusing a square marks its row label and its column label alone, per FR-vacl-7, and
 * a click stages the add or the remove that matrixClickPlan names. The diagonal carries
 * no data-from attribute, so bindMatrix wires it to no handler.
 */
function bindMatrix(holder, id) {
  const table = holder.querySelector(".ac-mtx");
  if (!table) {
    return;
  }
  const rowLabels = new Map([...table.querySelectorAll("th.row")].map((th) => [th.textContent, th]));
  const colLabels = new Map([...table.querySelectorAll("th.col")].map((th) => [th.textContent, th]));
  for (const button of table.querySelectorAll("button[data-from]")) {
    const from = button.getAttribute("data-from");
    const to = button.getAttribute("data-to");
    const mark = (on) => {
      const rowLabel = rowLabels.get(from);
      const colLabel = colLabels.get(to);
      if (rowLabel) {
        rowLabel.classList.toggle("hot", on);
      }
      if (colLabel) {
        colLabel.classList.toggle("hot", on);
      }
    };
    button.addEventListener("mouseenter", () => mark(true));
    button.addEventListener("mouseleave", () => mark(false));
    button.addEventListener("focus", () => mark(true));
    button.addEventListener("blur", () => mark(false));
    button.addEventListener("click", () => runAction(state.stageMatrixClick(id, from, to)));
  }
}

/**
 * syncActions draws the controls and the result of the editor region again.
 *
 * An edit changes the stage, therefore it changes which controls the operator reaches.
 * syncActions replaces the two regions beside the text area, so the caret of the operator
 * stays where it is. The two serializers escape every value that the control server and
 * the daemon reported, and a test asserts each one.
 */
function syncActions(holder, id) {
  const model = editorModel(state, id);
  const acts = holder.querySelector(".pol-acts");
  if (acts) {
    acts.outerHTML = actionsMarkup(actionsModel(model));
  }
  const result = holder.querySelector(".pol-result");
  if (result) {
    result.outerHTML = resultMarkup(resultModel(model));
  }
  bindActions(holder, id);
}

/**
 * bindEditor keeps the text of the operator and the position of the caret.
 *
 * The console edits no document for the operator, therefore the handler stores the text
 * word for word. It writes the line numbers and the edited state in place, so a keystroke
 * moves no caret.
 */
function bindEditor(holder, id) {
  const field = holder.querySelector("textarea");
  if (!field) {
    return;
  }
  const gutter = holder.querySelector(".pol-gut");
  const chip = holder.querySelector(".chip");

  field.addEventListener("input", () => {
    state.setText(id, field.value);
    caret = { start: field.selectionStart, end: field.selectionEnd };
    field.rows = field.value.split("\n").length;
    if (gutter) {
      drawGutter(gutter, field.value);
    }
    if (chip) {
      chip.hidden = !state.edited(id);
    }
    syncActions(holder, id);
  });
  field.addEventListener("blur", () => {
    if (field.isConnected) {
      caret = null;
    }
  });

  if (caret) {
    field.focus();
    field.setSelectionRange(caret.start, caret.end);
  }
}

/**
 * draw draws the policy view from one poll snapshot.
 *
 * The snapshot states which tailnets the daemon declares. The document of one tailnet
 * comes from GET /api/policy/{id}, which reaches the control server, therefore the view
 * reads it on a selection alone.
 */
function draw(section, snapshot) {
  redraw = () => draw(section, snapshot);
  section.replaceChildren();

  if (snapshot.loading) {
    const card = element("div", "card");
    card.append(element("span", "label", "Loading"));
    card.append(element("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  readList(snapshot);

  if (listError) {
    const card = element("div", "card");
    card.append(element("span", "label", "Policy"));
    card.append(element("p", "note", "The daemon did not answer the policy list."));
    card.append(element("p", "pol-detail mono", listError));
    section.append(card);
    return;
  }

  if (state.body() === null) {
    const card = element("div", "card");
    card.append(element("span", "label", "Loading"));
    card.append(element("p", "note", "The console reads the policy list."));
    section.append(card);
    return;
  }

  const empty = emptyStatement(state.body());
  if (empty) {
    const card = element("div", "card empty");
    card.append(element("span", "label", "Empty"));
    card.append(element("p", undefined, empty));
    section.append(card);
    return;
  }

  const grid = element("div", "pol-grid");

  // The serializer escapes every value that the daemon reported, and a test asserts that.
  const list = element("div", "pol-side");
  list.innerHTML = policyListMarkup(state.rows());
  bindRows(list);
  grid.append(list);

  const editor = element("div", "pol-main");
  const selected = state.selected();
  const model = editorModel(state, selected);
  editor.innerHTML = editorMarkup(model, selected ? toggleModel(state, selected) : null);
  if (selected) {
    bindToggle(editor, selected);
    bindMatrix(editor, selected);
    if (model.state === "document") {
      bindEditor(editor, selected);
      bindActions(editor, selected);
    }
  }
  grid.append(editor);

  section.append(grid);
}

registerView("policy", draw);
