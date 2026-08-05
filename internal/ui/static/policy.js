// The policy view: the tailnet list, the document editor, and the states that the daemon
// reports.
//
// The upstream policy is the access policy that a control server holds. A local rule
// controls what this host forwards; an upstream policy controls what every device of the
// tailnet reaches. The view states that difference above the editor, because a warning
// comes before the step that it applies to. See FR-policy-28.
//
// This view draws. The validate action and the push action arrive with issue #159.
//
// The model holds the document that the daemon reported and the text of the operator. The
// difference between the two is the edit, which FR-policy-24 states. The state changes
// alone; the view sends no document.
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

/** The text of policy.ErrHeadscaleFileMode that names the mode a write needs. */
const FILE_MODE_MARKER = 'policy.mode: "db"';

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
 */
export function editorModel(state, id) {
  if (!id) {
    return { id: "", state: "unselected", lines: 0, text: "", edited: false, readOnly: true, etag: "", detail: "", sentence: "" };
  }

  const row = state.rows().find((entry) => entry.id === id);
  const held = state.entry(id);
  const model = {
    id,
    kind: row ? row.kind : "",
    state: "loading",
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
  model.readOnly = !held.writeAvailable;
  model.lines = held.text.split("\n").length;
  if (model.readOnly) {
    model.sentence = "This tailnet is read only. The daemon reports no write availability for its control server.";
  }
  return model;
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

/**
 * editorMarkup returns the editor region of one model.
 *
 * The statement of FR-policy-28 comes first in every state, therefore the operator reads
 * it before the document. The serializer escapes every value that the daemon reported: a
 * policy document is text of a control server and the console has no authentication.
 */
export function editorMarkup(model) {
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
  return (
    `<div class="pol-region">${warning}` +
    noteMarkup(model.sentence) +
    `<div class="pol-ed">` +
    `<div class="pol-bar"><span class="pol-name mono">${esc(model.id)} &middot; policy.hujson</span>${chip}</div>` +
    `<div class="pol-code">${gutterMarkup(model.lines)}` +
    // The rows attribute holds the line count, therefore the text area needs no scrolling
    // box of its own and the line numbers stay beside their lines.
    `<textarea class="pol-doc mono"${readOnly} rows="${model.lines}" spellcheck="false" wrap="off" aria-label="The policy document of ${esc(model.id)}">${esc(model.text)}</textarea>` +
    `</div></div>` +
    (etag ? `<div class="pol-meta">${etag}</div>` : "") +
    `</div>`
  );
}

/**
 * createPolicyState returns the model of the policy view.
 *
 * options.request sends one request. It takes the route and it rejects with the message
 * that the daemon stated. A test replaces it.
 *
 * The model holds the list of GET /api/policy, and one entry per tailnet that the operator
 * opened. An entry holds the document of the read, the text of the operator, and the
 * message of a failed read. The model sends no document.
 */
export function createPolicyState(options = {}) {
  const request = options.request || ((route) => requestJSON(route));

  let list = null;
  let selectedId = null;
  const entries = new Map();

  function entryOf(id) {
    let entry = entries.get(id);
    if (!entry) {
      entry = { loaded: false, base: "", text: "", etag: "", writeAvailable: false, error: "" };
      entries.set(id, entry);
    }
    return entry;
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
    },

    /** setError states the message that a failed read returned, word for word. */
    setError(id, message) {
      const entry = entryOf(id);
      entry.error = String(message === null || message === undefined ? "" : message);
    },

    /** setText replaces the text of the operator. It sends nothing. */
    setText(id, text) {
      entryOf(id).text = text;
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
        this.setError(id, err && err.message ? err.message : String(err));
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
  editor.innerHTML = editorMarkup(editorModel(state, state.selected()));
  if (state.selected()) {
    bindEditor(editor, state.selected());
  }
  grid.append(editor);

  section.append(grid);
}

registerView("policy", draw);
