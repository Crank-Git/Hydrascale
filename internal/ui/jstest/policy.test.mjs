// The console has no build step and no package manager, so these tests run on the source
// file that the browser loads. The Go test TestTheConsoleJavaScriptTestsPass starts them.
//
// policy.js holds the model, the serializer, and the state of the policy view. Every
// function that these tests reach is pure, or it takes its transport as an argument,
// therefore this file asserts the drawn result exactly, with no browser and no network.
//
// A policy document is text that a control server holds, and the console has no
// authentication. An unescaped document, an unescaped identifier, and an unescaped message
// of the daemon are each script injection into this page. See SA-5 of docs/specs/spec.md.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { consoleRequestInit } from "../static/app.js";
import {
  CONFLICT_STATUS,
  EVERY_DEVICE_STATEMENT,
  POLICY_ROUTE,
  PUSH_STATEMENT,
  actionsMarkup,
  actionsModel,
  applyInputMemory,
  createInputMemory,
  createPolicyState,
  diffSummary,
  editorMarkup,
  editorModel,
  emptyStatement,
  matrixClickPlan,
  matrixMarkup,
  matrixModel,
  namedSetEntries,
  nodeAttrsEntryWithField,
  parseRulePorts,
  parseSSHCheckPeriod,
  policyDocumentRoute,
  policyListMarkup,
  policyRows,
  policySectionsEditRoute,
  policySectionsRoute,
  policyValidateRoute,
  readFailure,
  referencingRules,
  referencingSentence,
  removeGrantCapability,
  renameGrantCapability,
  resultMarkup,
  resultModel,
  ruleEntryWithPorts,
  ruleListMarkup,
  ruleRows,
  toggleMarkup,
  toggleModel,
  validateErrors,
  viewKey,
  visualMarkup,
} from "../static/policy.js";

// refusal returns the error that requestJSON rejects with: the message of the daemon word
// for word, and the status that the daemon returned. See internal/ui/static/app.js.
function refusal(status, message) {
  const err = new Error(message);
  err.status = status;
  return err;
}

// loaded returns a state that holds the list and one document, ready for an edit.
function loaded(request, overrides = {}) {
  const state = createPolicyState({ request });
  state.setList(listBody());
  state.setDocument("jbones", documentBody(overrides));
  return state;
}

// listBody is one answer of GET /api/policy, as internal/api/types.go declares it.
function listBody(overrides = {}) {
  return {
    tailnets: [
      { id: "jbones", kind: "tailscale", credential_present: true, write_available: true },
      { id: "homelab", kind: "headscale", credential_present: true, write_available: true },
      {
        id: "lab-hs",
        kind: "headscale",
        credential_present: false,
        write_available: false,
        reason: 'the tailnet "lab-hs" has no Headscale credential: set the credential in the file, or set HYDRASCALE_HS_API_KEY_LAB_HS',
      },
    ],
    ...overrides,
  };
}

// documentBody is one answer of GET /api/policy/{id}.
function documentBody(overrides = {}) {
  return {
    id: "jbones",
    kind: "tailscale",
    document: '{\n  "grants": [],\n}',
    etag: "e0b2816b418",
    write_available: true,
    ...overrides,
  };
}

// entryOf returns the editor model of one identifier out of a state.
function entryOf(state, id) {
  return editorModel(state, id);
}

// sectionsBody is one answer of POST /api/policy/{id}/sections, as internal/api/types.go
// declares it.
function sectionsBody(overrides = {}) {
  const body = {
    groups: { "group:admins": ["alice@example.com"] },
    hosts: {},
    tagOwners: {},
    ipsets: {},
    acls: [{ action: "accept", src: ["group:admins"], dst: ["*:*"] }],
    grants: [],
    ssh: [],
    autoApprovers: {},
    nodeAttrs: [],
    postures: {},
    tests: [],
    sshTests: [],
    opaque_keys: [],
    ...overrides,
  };
  // The server names every section key that the document holds, per FR-vadv-11. This
  // helper derives that list from the sections it answers, because a section that holds
  // an entry certainly holds its key. A test that needs an empty key present, or an
  // absent key, passes section_keys itself.
  if (!overrides.section_keys) {
    body.section_keys = SECTION_NAMES.filter((name) => sectionHoldsEntry(body[name]));
  }
  return body;
}

/** SECTION_NAMES lists every top-level key that FR-model-2 resolves into a section. */
const SECTION_NAMES = [
  "groups", "hosts", "tagOwners", "ipsets", "acls", "grants",
  "ssh", "autoApprovers", "nodeAttrs", "postures", "tests", "sshTests",
];

/** sectionHoldsEntry states whether one section of a sectionsBody answer holds an entry. */
function sectionHoldsEntry(value) {
  if (Array.isArray(value)) {
    return value.length > 0;
  }
  return Boolean(value) && Object.keys(value).length > 0;
}

// ---------------------------------------------------------------------------
// The tailnet list
// ---------------------------------------------------------------------------

test("the list states one row per tailnet with the identifier, the kind, and the state", () => {
  const rows = policyRows(listBody(), null);

  assert.equal(rows.length, 3);
  assert.deepEqual(rows[0], {
    id: "jbones",
    kind: "tailscale",
    tone: "ok",
    word: "read and write",
    reason: "",
    selected: false,
  });
  assert.equal(rows[2].tone, "crit");
  assert.equal(rows[2].word, "no credential");
});

test("a tailnet that holds a credential and takes no write reads as read only", () => {
  const body = listBody({
    tailnets: [{ id: "homelab", kind: "headscale", credential_present: true, write_available: false }],
  });

  assert.equal(policyRows(body, null)[0].word, "read only");
  assert.equal(policyRows(body, null)[0].tone, "warn");
});

test("the list marks the row that the operator selected", () => {
  const rows = policyRows(listBody(), "homelab");

  assert.equal(rows[1].selected, true);
  assert.equal(rows[0].selected, false);
  assert.match(policyListMarkup(rows), /data-id="homelab" aria-selected="true"/);
});

test("every row of the list reaches focus by keyboard", () => {
  const markup = policyListMarkup(policyRows(listBody(), null));

  assert.equal((markup.match(/tabindex="0"/g) || []).length, 3);
  assert.equal((markup.match(/role="option"/g) || []).length, 3);
});

test("the list draws the identifier and the kind in the mono typeface", () => {
  const markup = policyListMarkup(policyRows(listBody(), null));

  assert.match(markup, /<span class="pol-id mono">jbones<\/span>/);
  assert.match(markup, /<span class="pol-kind mono">tailscale<\/span>/);
});

test("the list states the credential state as a coloured dot and a lowercase word", () => {
  const markup = policyListMarkup(policyRows(listBody(), null));

  assert.match(markup, /<span class="dot ok"><\/span><span class="pol-word">read and write<\/span>/);
  assert.match(markup, /<span class="dot crit"><\/span><span class="pol-word">no credential<\/span>/);
});

test("the row of a tailnet with no credential names the credential that it needs", () => {
  const markup = policyListMarkup(policyRows(listBody(), null));

  assert.match(markup, /HYDRASCALE_HS_API_KEY_LAB_HS/);
});

test("the list escapes a hostile tailnet identifier", () => {
  const body = listBody({
    tailnets: [{ id: '<img src=x onerror="alert(1)">', kind: "tailscale", credential_present: true, write_available: true }],
  });

  const markup = policyListMarkup(policyRows(body, null));
  assert.ok(!markup.includes("<img"), markup);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("the list escapes a hostile reason", () => {
  const body = listBody({
    tailnets: [{ id: "lab", kind: "headscale", credential_present: false, write_available: false, reason: "</span><script>alert(1)</script>" }],
  });

  const markup = policyListMarkup(policyRows(body, null));
  assert.ok(!markup.includes("<script>"), markup);
});

// ---------------------------------------------------------------------------
// The editor
// ---------------------------------------------------------------------------

test("the editor draws the document with one line number per line", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  const model = entryOf(state, "jbones");
  assert.equal(model.state, "document");
  assert.equal(model.lines, 3);

  const markup = editorMarkup(model);
  assert.match(markup, /<div class="pol-gut" aria-hidden="true"><div>1<\/div><div>2<\/div><div>3<\/div><\/div>/);
});

test("the editor draws the document in the mono typeface", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  assert.match(editorMarkup(entryOf(state, "jbones")), /<textarea class="pol-doc mono"/);
});

test("a document that holds no line feed draws one line number", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody({ document: "{}" }));

  assert.equal(entryOf(state, "jbones").lines, 1);
});

test("a document of 400 kilobytes loads in the editor", () => {
  const line = '  { "src": ["autogroup:member"], "dst": ["tag:server"] },\n';
  const document = line.repeat(Math.ceil(400 * 1024 / line.length));
  assert.ok(document.length >= 400 * 1024);

  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody({ document }));

  const model = entryOf(state, "jbones");
  assert.equal(model.lines, document.split("\n").length);
  assert.ok(editorMarkup(model).includes("autogroup:member"));
});

test("the editor escapes a hostile document", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody({ document: '</textarea><script>alert(1)</script>' }));

  const markup = editorMarkup(entryOf(state, "jbones"));
  assert.ok(!markup.includes("</textarea><script>"), markup);
  assert.match(markup, /&lt;\/textarea&gt;&lt;script&gt;/);
});

test("the editor marks the document as edited when the text differs from the read", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  assert.equal(entryOf(state, "jbones").edited, false);

  state.setText("jbones", '{\n  "grants": [{}],\n}');
  const model = entryOf(state, "jbones");
  assert.equal(model.edited, true);
  assert.match(editorMarkup(model), /<span class="chip mono">edited<\/span>/);
});

test("the editor marks no edit when the text returns to the document of the read", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());
  state.setText("jbones", "changed");
  state.setText("jbones", documentBody().document);

  assert.equal(entryOf(state, "jbones").edited, false);
});

test("the editor states the etag of the read in the mono typeface", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  assert.match(editorMarkup(entryOf(state, "jbones")), /<span class="chip mono">etag e0b2816b418<\/span>/);
});

test("the view states that a policy change affects every device in the tailnet", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  assert.match(EVERY_DEVICE_STATEMENT, /every device in the tailnet/);
  assert.ok(editorMarkup(entryOf(state, "jbones")).includes(EVERY_DEVICE_STATEMENT));
});

// ---------------------------------------------------------------------------
// The states that the daemon reports
// ---------------------------------------------------------------------------

test("a tailnet that reports no write availability draws a read-only editor", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", kind: "headscale", etag: "", write_available: false }));

  const model = entryOf(state, "homelab");
  assert.equal(model.readOnly, true);
  assert.match(editorMarkup(model), /<textarea class="pol-doc mono" readonly/);
});

test("the file policy mode gives a read-only editor and the statement names the database mode", () => {
  const message = 'the Headscale control server runs the file policy mode, and a policy write needs policy.mode: "database"';
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setError("homelab", message);

  const model = entryOf(state, "homelab");
  assert.equal(model.state, "read-only");
  assert.equal(model.readOnly, true);
  assert.equal(readFailure(message).kind, "file-mode");

  const markup = editorMarkup(model);
  assert.match(markup, /policy\.mode: &quot;database&quot;/);
  assert.ok(!markup.includes("<textarea"), markup);
});

test("an unreachable control server names the address and the error", () => {
  const message = 'the policy read failed: Get "https://hs.example.net/api/v1/policy": dial tcp 10.0.0.9:443: connect: connection refused';
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setError("homelab", message);

  const model = entryOf(state, "homelab");
  assert.equal(model.state, "failed");
  assert.equal(readFailure(message).kind, "unreachable");

  const markup = editorMarkup(model);
  assert.match(markup, /hs\.example\.net/);
  assert.match(markup, /connection refused/);
  assert.match(markup, /The control server did not answer\./);
});

test("the editor region of a tailnet with no credential names the credential", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.select("lab-hs");

  const model = entryOf(state, "lab-hs");
  assert.equal(model.state, "no-credential");
  assert.match(editorMarkup(model), /HYDRASCALE_HS_API_KEY_LAB_HS/);
});

test("a read that the daemon refuses for a missing credential names the credential", () => {
  const message = 'the tailnet "lab-hs" has no Headscale credential: set the credential in the file';

  assert.equal(readFailure(message).kind, "no-credential");
});

test("the failure markup escapes the message of the daemon", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setError("homelab", "<script>alert(1)</script>");

  assert.ok(!editorMarkup(entryOf(state, "homelab")).includes("<script>"));
});

test("the view states what would fill it while no tailnet is declared", () => {
  assert.match(emptyStatement({ tailnets: [] }), /no tailnet/i);
  assert.equal(emptyStatement(listBody()), "");
});

test("the editor region states the next step while the operator selects no tailnet", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());

  const model = entryOf(state, null);
  assert.equal(model.state, "unselected");
  assert.match(editorMarkup(model), /Select a tailnet/);
});

// ---------------------------------------------------------------------------
// The state
// ---------------------------------------------------------------------------

test("the route of one document encodes the identifier", () => {
  assert.equal(POLICY_ROUTE, "/api/policy");
  assert.equal(policyDocumentRoute("lab hs/1"), "/api/policy/lab%20hs%2F1");
});

test("the state reads the list and then the document of the selection", async () => {
  const routes = [];
  const state = createPolicyState({
    request: async (route) => {
      routes.push(route);
      return route === POLICY_ROUTE ? listBody() : documentBody();
    },
  });

  await state.loadList();
  assert.equal(state.rows().length, 3);

  await state.open("jbones");
  assert.deepEqual(routes, [POLICY_ROUTE, "/api/policy/jbones"]);
  assert.equal(state.selected(), "jbones");
  assert.equal(entryOf(state, "jbones").state, "document");
});

test("the state reads the document of one selection once", async () => {
  let reads = 0;
  const state = createPolicyState({
    request: async (route) => {
      if (route !== POLICY_ROUTE) {
        reads += 1;
      }
      return route === POLICY_ROUTE ? listBody() : documentBody();
    },
  });

  await state.loadList();
  await state.open("jbones");
  await state.open("homelab");
  await state.open("jbones");

  assert.equal(reads, 2);
});

test("the state states the message of the daemon when the read fails", async () => {
  const state = createPolicyState({
    request: async (route) => {
      if (route === POLICY_ROUTE) {
        return listBody();
      }
      throw new Error("the policy read failed: 502");
    },
  });

  await state.loadList();
  await state.open("homelab");

  assert.equal(entryOf(state, "homelab").detail, "the policy read failed: 502");
});

test("the state keeps the text of the operator when the list arrives again", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());
  state.setText("jbones", "the text of the operator");

  state.setList(listBody());

  assert.equal(entryOf(state, "jbones").text, "the text of the operator");
  assert.equal(entryOf(state, "jbones").edited, true);
});

test("the state drops the selection when the list no longer holds the tailnet", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.select("lab-hs");
  assert.equal(state.selected(), "lab-hs");

  state.setList(listBody({ tailnets: [{ id: "jbones", kind: "tailscale", credential_present: true, write_available: true }] }));
  assert.equal(state.selected(), null);
});

// ---------------------------------------------------------------------------
// The actions: validate, push, re-read, and discard
// ---------------------------------------------------------------------------

// idsOf returns the identifier of every control of the actions row.
function idsOf(controls) {
  return controls.map((one) => one.id);
}

// controlOf returns one control of the actions row by its identifier.
function controlOf(controls, id) {
  return controls.find((one) => one.id === id);
}

test("the route of the validate action encodes the identifier", () => {
  assert.equal(policyValidateRoute("jbones"), "/api/policy/jbones/validate");
  assert.equal(policyValidateRoute("lab hs/1"), "/api/policy/lab%20hs%2F1/validate");
});

test("the view offers the validate action, the discard action, and the push action", () => {
  const state = loaded(async () => documentBody());

  assert.deepEqual(idsOf(actionsModel(entryOf(state, "jbones"))), ["validate", "discard", "push"]);
});

test("the accent marks the push action alone", () => {
  // CLAUDE.md gives the accent to one thing per view. This view now draws an affirmative
  // action, therefore the push takes the accent and the selected row keeps none.
  const state = loaded(async () => documentBody());

  const accented = actionsModel(entryOf(state, "jbones")).filter((one) => one.accent);
  assert.equal(accented.length, 1);
  assert.equal(accented[0].id, "push");
});

test("a disabled push draws no accent, because it is not the affirmative action yet", () => {
  // The stage read disables push. CLAUDE.md's accent rule marks the affirmative action,
  // and a disabled push is not the affirmative action.
  const state = loaded(async () => documentBody());

  assert.equal(controlOf(actionsModel(entryOf(state, "jbones")), "push").disabled, true);
  assert.match(actionsMarkup(actionsModel(entryOf(state, "jbones"))), /class="btn" data-act="push"[^>]*disabled/);
});

test("an enabled push keeps the accent", async () => {
  // The stage validated enables push. Push is the affirmative action again.
  const state = loaded(async () => ({ passed: true }));
  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
  assert.match(actionsMarkup(actionsModel(model)), /class="btn primary" data-act="push"/);
});

test("the push states that it changes what every device in the tailnet reaches", () => {
  // The destructive copy names the exact effect and it states what survives. It comes
  // before the action, which .claude/rules/ste.md requires of a warning.
  const state = loaded(async () => documentBody());

  assert.match(PUSH_STATEMENT.effect, /every device in the tailnet/);
  assert.match(PUSH_STATEMENT.survives, /local rule/);

  const markup = editorMarkup(entryOf(state, "jbones"));
  assert.ok(markup.includes(PUSH_STATEMENT.effect), markup);
  assert.ok(markup.includes(PUSH_STATEMENT.survives), markup);
  assert.ok(markup.indexOf(PUSH_STATEMENT.effect) < markup.indexOf('data-act="push"'), markup);
});

test("a read-only tailnet offers no validate action and no push action", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", write_available: false }));

  const controls = actionsModel(entryOf(state, "homelab"));
  assert.equal(controlOf(controls, "validate").disabled, true);
  assert.equal(controlOf(controls, "push").disabled, true);
});

test("the discard action returns the editor to the document that the console read", () => {
  const state = loaded(async () => documentBody());
  state.setText("jbones", "the text of the operator");
  assert.equal(entryOf(state, "jbones").edited, true);

  state.discard("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.text, documentBody().document);
  assert.equal(model.edited, false);
  assert.equal(model.stage, "read");
});

// ---------------------------------------------------------------------------
// The validate
// ---------------------------------------------------------------------------

test("push is disabled until the control server accepts the document", async () => {
  const state = loaded(async () => ({ passed: true }));

  assert.equal(controlOf(actionsModel(entryOf(state, "jbones")), "push").disabled, true);

  state.setText("jbones", '{\n  "grants": [{}],\n}');
  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validated");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
});

test("a validate result of the literal empty object states no message", async () => {
  // The control server answers a valid document with a literal "{}", which holds no
  // information for the operator to read.
  const state = loaded(async () => ({ passed: true, result: "{}" }));

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validated");
  assert.equal(resultModel(model).message, "");
});

test("a validate result that holds real content reaches the screen verbatim", async () => {
  const state = loaded(async () => ({ passed: true, result: "the document holds 3 warnings" }));

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(resultModel(model).message, "the document holds 3 warnings");
});

test("an edit after a successful validate disables push again", async () => {
  const state = loaded(async () => ({ passed: true }));
  await state.validate("jbones");
  assert.equal(controlOf(actionsModel(entryOf(state, "jbones")), "push").disabled, false);

  state.setText("jbones", '{\n  "grants": [{}],\n}');

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "read");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);
  assert.equal(resultModel(model), null);
});

test("a validate that returns after an edit leaves push disabled", async () => {
  // The operator edits while the request runs. The result covers text that the editor no
  // longer holds, therefore the state keeps push disabled.
  let release = () => {};
  const state = loaded(() => new Promise((resolve) => {
    release = () => resolve({ passed: true });
  }));

  const running = state.validate("jbones");
  state.setText("jbones", "the text of the operator");
  release();
  await running;

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "read");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);
});

test("the validate sends the text of the operator to the validate route", async () => {
  const sent = [];
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    return { passed: true };
  });
  state.setText("jbones", "the text of the operator");

  await state.validate("jbones");

  assert.deepEqual(sent, [{
    route: "/api/policy/jbones/validate",
    method: "POST",
    body: { document: "the text of the operator" },
  }]);
});

test("a successful validate states the result and enables push", async () => {
  const state = loaded(async () => ({ passed: true }));

  await state.validate("jbones");

  const result = resultModel(entryOf(state, "jbones"));
  assert.equal(result.tone, "ok");
  assert.equal(result.word, "validated");
  assert.match(result.sentence, /accepted the document/);
  assert.match(resultMarkup(result), /<span class="dot ok"><\/span>/);
});

test("a validate failure states each error with its line number", async () => {
  const body = '{"message":"line 12: unknown group \\"group:ops\\"\\nline 40: invalid port range"}';
  const state = loaded(async () => ({ passed: false, result: body }));

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validate-failed");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);

  const result = resultModel(model);
  assert.equal(result.errors.length, 2);
  assert.equal(result.errors[0].line, 12);
  assert.equal(result.errors[0].message, 'line 12: unknown group "group:ops"');
  assert.equal(result.errors[1].line, 40);

  const markup = resultMarkup(result);
  assert.match(markup, /<span class="pol-err-line mono">line 12<\/span>/);
  assert.match(markup, /unknown group &quot;group:ops&quot;/);
});

test("a failed test states that the control server takes no such document", async () => {
  const body = '{"message":"test(s) failed","data":[{"user":"test2@example.com","errors":["address \\"100.64.0.2:443\\": want: Drop, got: Accept"]}]}';
  const state = loaded(async () => ({ passed: false, tests_failed: true, result: body }));

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);

  const result = resultModel(model);
  assert.equal(result.word, "test failed");
  assert.match(result.sentence, /accepts no document whose test fails/);
});

test("a warning leaves Push enabled and names the answer as a warning", async () => {
  const body = '{"message":"warning(s) found","data":[{"user":"group:unknown@example.com","warnings":["group is not syncing from SCIM and will be ignored by rules in the policy file"]}]}';
  const state = loaded(async () => ({ passed: true, warning: true, result: body }));

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validated");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);

  const result = resultModel(model);
  assert.equal(result.word, "warning");
  assert.equal(result.tone, "warn");
  assert.match(result.sentence, /accepted the document/);
  assert.equal(result.message, body);
  assert.match(resultMarkup(result), /<span class="dot warn"><\/span>/);
});

test("a validate that passes after a warning clears the warning", async () => {
  let answer = { passed: true, warning: true, result: '{"message":"warning(s) found"}' };
  const state = loaded(async () => answer);

  await state.validate("jbones");
  answer = { passed: true, result: "" };
  await state.validate("jbones");

  const result = resultModel(entryOf(state, "jbones"));
  assert.equal(result.word, "validated");
  assert.equal(result.tone, "ok");
});

test("a document error states the rejection rather than a failed test", async () => {
  const body = '{"message":"line 3: unknown field \\"acl\\""}';
  const state = loaded(async () => ({ passed: false, result: body }));

  await state.validate("jbones");

  const result = resultModel(entryOf(state, "jbones"));
  assert.equal(result.word, "validate failed");
  assert.match(result.sentence, /rejected the document/);
});

test("a validate that passes after a failed test clears the failed test", async () => {
  let answer = { passed: false, tests_failed: true, result: '{"message":"test(s) failed"}' };
  const state = loaded(async () => answer);

  await state.validate("jbones");
  answer = { passed: true, result: "" };
  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.testsFailed, false);
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
});

test("a validate error that names a line and a character reads the line number", () => {
  const errors = validateErrors('{"message":"parse error: policy.hujson:7:3: unexpected token"}');

  assert.equal(errors.length, 1);
  assert.equal(errors[0].line, 7);
});

test("a validate error that names no line number states the message alone", () => {
  const errors = validateErrors("the control server rejected the document");

  assert.equal(errors.length, 1);
  assert.equal(errors[0].line, null);
  assert.equal(errors[0].message, "the control server rejected the document");
  assert.ok(!resultMarkup({ tone: "crit", word: "validate failed", sentence: "s", errors, message: "", reread: false }).includes("pol-err-line"));
});

test("the validate result of the control server reaches the screen verbatim", async () => {
  // The Headscale check route states its reason as text, therefore the result holds no
  // JSON and the console shows the whole text.
  const body = 'the policy check failed with HTTP 500: policy: unknown host "nas"';
  const state = loaded(async () => ({ passed: false, result: body }));

  await state.validate("jbones");

  const markup = resultMarkup(resultModel(entryOf(state, "jbones")));
  assert.match(markup, /the policy check failed with HTTP 500: policy: unknown host &quot;nas&quot;/);
});

test("the validate result markup escapes the message of the control server", async () => {
  const state = loaded(async () => ({ passed: false, result: "</span><script>alert(1)</script>" }));

  await state.validate("jbones");

  const markup = resultMarkup(resultModel(entryOf(state, "jbones")));
  assert.ok(!markup.includes("<script>"), markup);
  assert.match(markup, /&lt;script&gt;/);
});

test("a validate that the daemon refuses states the message of the daemon", async () => {
  const state = loaded(async () => {
    throw refusal(502, "the policy validate failed: the policy validate failed with HTTP 500");
  });

  await state.validate("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validate-failed");
  assert.equal(resultModel(model).errors[0].message, "the policy validate failed: the policy validate failed with HTTP 500");
});

// ---------------------------------------------------------------------------
// The push
// ---------------------------------------------------------------------------

test("the push sends the text of the operator and the etag of the read", async () => {
  const sent = [];
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    return route === policyValidateRoute("jbones") ? { passed: true } : documentBody({ etag: "e0b2816b419" });
  });
  state.setText("jbones", "the text of the operator");

  await state.validate("jbones");
  await state.push("jbones");

  assert.deepEqual(sent[1], {
    route: "/api/policy/jbones",
    method: "PUT",
    body: { document: "the text of the operator", etag: "e0b2816b418" },
  });
});

test("the push sends no etag for a tailnet whose control server holds none", async () => {
  const sent = [];
  const state = createPolicyState({
    request: async (route, method, body) => {
      sent.push({ route, method, body });
      return route.endsWith("/validate") ? { passed: true } : documentBody({ id: "homelab", etag: "" });
    },
  });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", kind: "headscale", etag: "" }));

  await state.validate("homelab");
  await state.push("homelab");

  assert.deepEqual(sent[1].body, { document: documentBody().document });
});

test("the console sends no push while the control server has not accepted the document", async () => {
  let requests = 0;
  const state = loaded(async () => {
    requests += 1;
    return documentBody();
  });

  await state.push("jbones");

  assert.equal(requests, 0);
  assert.equal(entryOf(state, "jbones").stage, "read");
});

test("a successful push takes the document that the control server returned", async () => {
  const written = '{\n  "grants": [{"src": ["*"]}],\n}';
  const state = loaded(async (route) =>
    route.endsWith("/validate") ? { passed: true } : documentBody({ document: written, etag: "e0b2816b419" }));
  state.setText("jbones", written);

  await state.validate("jbones");
  await state.push("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "pushed");
  assert.equal(model.text, written);
  assert.equal(model.edited, false);
  assert.equal(model.etag, "e0b2816b419");
  assert.match(resultModel(model).sentence, /accepted the document/);
});

test("the editor takes no edit while the push runs", async () => {
  let release = () => {};
  const state = loaded((route) =>
    route.endsWith("/validate")
      ? Promise.resolve({ passed: true })
      : new Promise((resolve) => {
        release = () => resolve(documentBody());
      }));

  await state.validate("jbones");
  const running = state.push("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "pushing");
  assert.equal(model.readOnly, true);
  assert.match(editorMarkup(model), /<textarea class="pol-doc mono" readonly/);

  release();
  await running;
});

// ---------------------------------------------------------------------------
// The conflict
// ---------------------------------------------------------------------------

test("a push conflict states that the policy changed and it offers the re-read action", async () => {
  const message = "the policy changed since the read: read the policy again and apply the change to the new document";
  const state = loaded(async (route) => {
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    throw refusal(CONFLICT_STATUS, message);
  });
  state.setText("jbones", "the text of the operator");

  await state.validate("jbones");
  await state.push("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(CONFLICT_STATUS, 409);
  assert.equal(model.stage, "conflict");

  const result = resultModel(model);
  assert.equal(result.reread, true);
  assert.match(result.sentence, /changed the policy document/);
  assert.equal(result.message, message);
  assert.match(resultMarkup(result), /data-act="reread"/);
  assert.deepEqual(idsOf(actionsModel(model)), ["validate", "discard", "push"]);
});

test("the console reads the status and never the message to detect a conflict", async () => {
  // The daemon returns HTTP 409 for a conflict. A message that holds the same words with
  // another status is not a conflict, therefore the view offers no re-read.
  const state = loaded(async (route) => {
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    throw refusal(502, "the policy write failed: the policy changed since the read");
  });

  await state.validate("jbones");
  await state.push("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "push-failed");
  assert.equal(resultModel(model).reread, false);
  assert.equal(resultModel(model).message, "the policy write failed: the policy changed since the read");
});

test("the conflict markup escapes the message of the control server", async () => {
  const state = loaded(async (route) => {
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    throw refusal(CONFLICT_STATUS, "</p><script>alert(1)</script>");
  });

  await state.validate("jbones");
  await state.push("jbones");

  const markup = resultMarkup(resultModel(entryOf(state, "jbones")));
  assert.ok(!markup.includes("<script>"), markup);
  assert.match(markup, /&lt;script&gt;/);
});

test("the re-read keeps the edited text of the operator", async () => {
  // held is the document that another person wrote while the operator edited.
  const held = '{\n  "grants": [{"src": ["tag:ops"]}],\n}';
  const routes = [];
  const state = loaded(async (route, method) => {
    routes.push(route);
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    if (method === "PUT") {
      throw refusal(CONFLICT_STATUS, "the policy changed since the read");
    }
    return documentBody({ document: held, etag: "e0b2816b420" });
  });
  state.setText("jbones", "the text of the operator");

  await state.validate("jbones");
  await state.push("jbones");
  assert.equal(entryOf(state, "jbones").stage, "conflict");

  await state.reread("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.text, "the text of the operator");
  assert.equal(model.edited, true);
  assert.equal(model.etag, "e0b2816b420");
  assert.equal(model.stage, "read");
  assert.equal(state.entry("jbones").base, held);
  assert.equal(routes[2], "/api/policy/jbones");
});

test("a re-read that fails keeps the conflict and it keeps the text of the operator", async () => {
  const state = loaded(async (route, method) => {
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    if (method === "PUT") {
      throw refusal(CONFLICT_STATUS, "the policy changed since the read");
    }
    throw refusal(502, "the policy read failed: dial tcp 10.0.0.9:443: connect: connection refused");
  });
  state.setText("jbones", "the text of the operator");

  await state.validate("jbones");
  await state.push("jbones");
  assert.equal(entryOf(state, "jbones").stage, "conflict");

  await state.reread("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "conflict");
  assert.equal(model.text, "the text of the operator");
  assert.match(resultModel(model).message, /connection refused/);
});

// ---------------------------------------------------------------------------
// The rate limit
// ---------------------------------------------------------------------------

test("the console sends one request when the control server rate-limits the validate", async () => {
  // An automatic retry against a rate limit makes the limit worse, and the console applies
  // no action that the operator did not ask for.
  let requests = 0;
  const state = loaded(async () => {
    requests += 1;
    throw refusal(502, "the policy validate failed: the policy validate failed with HTTP 429: rate limit exceeded");
  });

  await state.validate("jbones");

  assert.equal(requests, 1);
  assert.equal(entryOf(state, "jbones").stage, "validate-failed");
  assert.match(resultModel(entryOf(state, "jbones")).errors[0].message, /rate limit exceeded/);
});

test("the console sends one request when the control server rate-limits the push", async () => {
  let writes = 0;
  const state = loaded(async (route) => {
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    writes += 1;
    throw refusal(502, "the policy write failed: the policy write failed with HTTP 429: rate limit exceeded");
  });

  await state.validate("jbones");
  await state.push("jbones");

  assert.equal(writes, 1);
  assert.equal(entryOf(state, "jbones").stage, "push-failed");
  assert.match(resultModel(entryOf(state, "jbones")).message, /rate limit exceeded/);
});

test("the stylesheet hides every element that carries the attribute hidden", async () => {
  // Issue #278. The editor writes `chip.hidden = !state.edited(id)`, therefore the state
  // of the element is correct. A class selector holds a higher specificity than the rule
  // [hidden]{display:none} of the user agent, so `.chip{display:inline-flex}` won the
  // cascade and the word `edited` painted on a document that holds no edit. A guard for
  // one class repeats the defect for the next class, therefore the rule covers every
  // element.
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\[hidden\]\{display:none *!important\}/);
});

test("the editor gives the gutter and the text one scrolling box", async () => {
  // Issue #277. A flex container aligns its items with `stretch`, therefore both children
  // took the height of the container: the text area then held its own scroll under
  // overflow-y:hidden and the line numbers stood still while the text moved. The container
  // scrolls both only while each child keeps its whole height.
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");

  assert.match(style, /\.pol-code\{[^}]*align-items:flex-start/);
  assert.match(style, /\.pol-code\{[^}]*overflow:auto/);
  // The text area must not scroll on its own, because its `rows` attribute holds the whole
  // line count and the container carries the scroll.
  assert.match(style, /\.pol-doc\{[^}]*overflow-y:hidden/);
});

test("the editor sizes the text area to the line count of the document", async () => {
  // The `rows` attribute is what gives the text area its whole height inside the scrolling
  // box. A document of three lines therefore draws three rows.
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  const model = entryOf(state, "jbones");
  assert.equal(model.lines, 3);
  assert.match(editorMarkup(model), /<textarea class="pol-doc mono"[^>]*rows="3"/);
});

test("a tailnet whose credential the control server refuses reads as rejected", () => {
  // Issue #276. The row states the presence of a credential, therefore a credential that
  // exists and works for no request read `read and write` and the operator learned of the
  // mistake through a failed policy read alone.
  const body = listBody({
    tailnets: [{
      id: "jbones",
      kind: "tailscale",
      credential_present: true,
      write_available: false,
      credential_state: "rejected",
      reason: "the value is a device authentication key, which enrols a device and reaches no API: generate an OAuth client in the admin console of the control server, whose secret starts with tskey-client",
    }],
  });

  const row = policyRows(body, null)[0];
  assert.equal(row.word, "credential rejected");
  assert.equal(row.tone, "crit");
  assert.match(row.reason, /device authentication key/);
});

test("the rejected state names the expected prefix to the operator", () => {
  const body = listBody({
    tailnets: [{
      id: "jbones",
      kind: "tailscale",
      credential_present: true,
      write_available: false,
      credential_state: "rejected",
      reason: "the value is no Tailscale OAuth client secret: such a secret starts with tskey-client",
    }],
  });

  assert.match(policyListMarkup(policyRows(body, null)), /tskey-client/);
});

test("a usable credential still reads as read and write", () => {
  const body = listBody({
    tailnets: [{ id: "jbones", kind: "tailscale", credential_present: true, write_available: true, credential_state: "usable" }],
  });

  const row = policyRows(body, null)[0];
  assert.equal(row.word, "read and write");
  assert.equal(row.tone, "ok");
});

// ---------------------------------------------------------------------------
// The Visual/Text toggle (FR-vacl-1 to FR-vacl-3)
// ---------------------------------------------------------------------------

test("the route of the sections action encodes the identifier", () => {
  assert.equal(policySectionsRoute("jbones"), "/api/policy/jbones/sections");
  assert.equal(policySectionsRoute("lab hs/1"), "/api/policy/lab%20hs%2F1/sections");
});

test("a document that holds no toggle attempt shows the Text view and no disabled Visual control", () => {
  const state = loaded(async () => documentBody());
  const toggle = toggleModel(state, "jbones");

  assert.equal(toggle.view, "text");
  assert.equal(toggle.visualDisabled, false);

  const markup = toggleMarkup(toggle);
  assert.match(markup, /<button type="button" role="tab" aria-selected="false" data-view="visual">Visual<\/button>/);
  assert.match(markup, /<button type="button" role="tab" aria-selected="true" data-view="text">Text<\/button>/);
});

test("a successful load of the sections switches the entry to the visual view", async () => {
  const state = loaded(async () => sectionsBody());

  await state.loadSections("jbones");

  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.view, "visual");
  assert.equal(toggle.visualDisabled, false);
  assert.deepEqual(toggle.sections, sectionsBody());
});

test("the sections request sends the staged text, per FR-vacl-2", async () => {
  const sent = [];
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    return sectionsBody();
  });
  state.setText("jbones", "the text of the operator");

  await state.loadSections("jbones");

  assert.deepEqual(sent, [{
    route: "/api/policy/jbones/sections",
    method: "POST",
    body: { document: "the text of the operator" },
  }]);
});

test("a parse failure keeps the Visual control present but disabled, and states the error inline", async () => {
  const state = loaded(async () => {
    throw refusal(400, "policy: parsing document: hujson: line 3, column 1: parsing value: unexpected EOF");
  });

  await state.loadSections("jbones");

  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.view, "text");
  assert.equal(toggle.visualDisabled, true);
  assert.match(toggle.error, /line 3, column 1/);

  const markup = toggleMarkup(toggle);
  assert.match(markup, /<button type="button" role="tab" aria-selected="false" data-view="visual" disabled>Visual<\/button>/);
  assert.match(markup, /line 3, column 1/);
});

test("an edit after a parse failure clears the error and re-enables the Visual control", async () => {
  const state = loaded(async () => {
    throw refusal(400, "line 3, column 1: unexpected EOF");
  });
  await state.loadSections("jbones");
  assert.equal(toggleModel(state, "jbones").visualDisabled, true);

  state.setText("jbones", '{\n  "acls": [],\n}');

  assert.equal(toggleModel(state, "jbones").visualDisabled, false);
  assert.equal(toggleModel(state, "jbones").error, "");
});

test("selecting Text switches the view and sends no request", () => {
  const sent = [];
  const state = loaded(async (route) => {
    sent.push(route);
    return sectionsBody();
  });

  state.setView("jbones", "visual");
  assert.equal(toggleModel(state, "jbones").view, "visual");

  state.setView("jbones", "text");
  assert.equal(toggleModel(state, "jbones").view, "text");
  assert.deepEqual(sent, []);
});

test("a text edit after a visual edit toggles to Visual again and matches the new text", async () => {
  // FR-vacl-2. loadSections always sends the current staged text, so a toggle to
  // Visual after an edit re-parses that text and never a stale answer.
  const requests = [];
  const state = loaded(async (route, method, body) => {
    requests.push(body.document);
    return sectionsBody({ acls: body.document.includes("tag:laptop") ? [{ action: "accept", src: ["tag:laptop"], dst: ["*:*"] }] : [] });
  });

  await state.loadSections("jbones");
  assert.equal(toggleModel(state, "jbones").sections.acls.length, 0);

  state.setView("jbones", "text");
  state.setText("jbones", '{\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["*:*"]}],\n}');
  await state.loadSections("jbones");

  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.view, "visual");
  assert.equal(toggle.sections.acls.length, 1);
  assert.equal(toggle.sections.acls[0].src[0], "tag:laptop");
});

test("a sections request that returns after an edit applies neither the sections nor the error", async () => {
  let release = () => {};
  const state = loaded(() => new Promise((resolve) => {
    release = () => resolve(sectionsBody());
  }));

  const running = state.loadSections("jbones");
  state.setText("jbones", "the text of the operator");
  release();
  await running;

  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.view, "text");
  assert.equal(toggle.sections, null);
});

test("loadSections sends one request, and no second request while the first runs", async () => {
  let calls = 0;
  let release = () => {};
  const state = loaded(() => new Promise((resolve) => {
    calls += 1;
    release = () => resolve(sectionsBody());
  }));

  const first = state.loadSections("jbones");
  const second = state.loadSections("jbones");
  release();
  await Promise.all([first, second]);

  assert.equal(calls, 1);
});

test("the visual region draws the count of each section", () => {
  const markup = visualMarkup(sectionsBody({
    groups: { "group:admins": [], "group:eng": [] },
    hosts: { server: "100.64.0.1" },
    acls: [{ action: "accept", src: ["a"], dst: ["b"] }],
    grants: [{ src: ["c"], dst: ["d"] }],
  }));

  assert.match(markup, /<span class="name">Groups<\/span><span class="mono">2<\/span>/);
  assert.match(markup, /<span class="name">Hosts<\/span><span class="mono">1<\/span>/);
  assert.match(markup, /<span class="name">Rules<\/span><span class="mono">2<\/span>/);
});

test("the visual region names an opaque key once, per FR-vacl-18", () => {
  const markup = visualMarkup(sectionsBody({ opaque_keys: ["randomizeClientPort"] }));

  assert.match(markup, /randomizeClientPort/);
});

test("the visual region never draws ssh, autoApprovers, nodeAttrs, postures, tests, or sshTests, per FR-vacl-17", () => {
  const markup = visualMarkup(
    sectionsBody({
      ssh: [{ action: "accept", src: ["autogroup:member"], dst: ["autogroup:self"], users: ["autogroup:nonroot"] }],
      autoApprovers: { routes: { "10.0.0.0/8": ["tag:router"] } },
      nodeAttrs: [{ target: ["tag:server"], attr: ["funnel"] }],
      postures: { "posture:latest": ["node:os in ['linux']"] },
      tests: [{ src: "group:eng", accept: ["tag:server:22"] }],
      sshTests: [{ src: "group:eng", accept: ["tag:server:22"] }],
    }),
  );

  assert.doesNotMatch(markup, /autogroup:nonroot/);
  assert.doesNotMatch(markup, /10\.0\.0\.0\/8/);
  assert.doesNotMatch(markup, /funnel/);
  assert.doesNotMatch(markup, /posture:latest/);
  assert.doesNotMatch(markup, /group:eng/);
});

test("an unsupported section shows the reason in place of its editor, per FR-vacl-19", () => {
  const markup = visualMarkup(sectionsBody({ ipsets: { "ipset:eng": ["10.0.0.0/24"] } }), "ipsets", null, null, "headscale");

  assert.match(markup, /does not support/);
  assert.doesNotMatch(markup, /data-add-key/);
  assert.doesNotMatch(markup, /ipset:eng/);
});

test("a Tailscale tailnet still edits IP sets normally, per FR-vacl-19", () => {
  const markup = visualMarkup(sectionsBody({ ipsets: { "ipset:eng": ["10.0.0.0/24"] } }), "ipsets", null, null, "tailscale");

  assert.match(markup, /ipset:eng/);
  assert.match(markup, /data-add-key/);
});

test("the visual region draws nothing before the first load", () => {
  assert.equal(visualMarkup(null), `<div class="pol-visual"></div>`);
});

test("the editor draws the visual region when the toggle selects Visual", async () => {
  const state = loaded(async () => sectionsBody());
  await state.loadSections("jbones");

  const markup = editorMarkup(entryOf(state, "jbones"), toggleModel(state, "jbones"));
  assert.ok(markup.includes(`<div class="pol-visual">`), markup);
  assert.ok(!markup.includes("<textarea"), markup);
});

test("the editor draws the text region when the toggle selects Text", () => {
  const state = loaded(async () => documentBody());

  const markup = editorMarkup(entryOf(state, "jbones"), toggleModel(state, "jbones"));
  assert.ok(markup.includes("<textarea"), markup);
  assert.ok(!markup.includes(`<div class="pol-visual">`), markup);
});

test("the editor draws no toggle before a tailnet is selected", () => {
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());

  const markup = editorMarkup(entryOf(state, null));
  assert.ok(!markup.includes('role="tablist"'), markup);
});

// ---------------------------------------------------------------------------
// The section nav: Groups, Hosts, Tag owners, IP sets (#315, FR-vacl-4, FR-vacl-5)
// ---------------------------------------------------------------------------

test("namedSetEntries returns one pair per key, in sorted order", () => {
  const sections = sectionsBody({ groups: { "group:eng": ["b"], "group:admins": ["a"] } });
  assert.deepEqual(namedSetEntries(sections, "groups"), [
    ["group:admins", ["a"]],
    ["group:eng", ["b"]],
  ]);
});

test("namedSetEntries returns an empty list for a section the answer holds none of", () => {
  assert.deepEqual(namedSetEntries(sectionsBody({ groups: {} }), "groups"), []);
});

test("the section nav marks the selected section, and Rules is selected by default", () => {
  const defaultMarkup = visualMarkup(sectionsBody());
  assert.match(defaultMarkup, /data-nav="rules" aria-selected="true"/);
  assert.match(defaultMarkup, /data-nav="groups" aria-selected="false"/);

  const groupsMarkup = visualMarkup(sectionsBody(), "groups");
  assert.match(groupsMarkup, /data-nav="groups" aria-selected="true"/);
  assert.match(groupsMarkup, /data-nav="hosts" aria-selected="false"/);
  assert.match(groupsMarkup, /data-nav="rules" aria-selected="false"/);
});

test("the section nav draws the matrix, not a named-set entry list, before the operator selects a named-set section", () => {
  const markup = visualMarkup(sectionsBody());
  assert.ok(markup.includes("ac-matrix"));
  assert.ok(!markup.includes("setlist"));
});

test("the Rules row reopens the matrix after a named-set section was open", () => {
  const markup = visualMarkup(sectionsBody(), "rules");
  assert.ok(markup.includes("ac-matrix"));
  assert.ok(!markup.includes("setlist"));
});

test("Rules draws the matrix and the rule list together, per the mockup", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
  });

  const markup = visualMarkup(sections);
  assert.ok(markup.includes("ac-matrix"));
  assert.ok(markup.includes("ac-rulelist"));
  assert.ok(markup.indexOf("ac-matrix") < markup.indexOf("ac-rulelist"), "the matrix comes before the rule list");
  assert.match(markup, /tag:laptop/);
});

test("switching to a named-set section replaces the matrix and the rule list, not just the matrix", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
  });

  const markup = visualMarkup(sections, "groups");
  assert.ok(!markup.includes("ac-matrix"));
  assert.ok(!markup.includes("ac-rulelist"));
  assert.ok(markup.includes("setlist"));
});

test("the Groups section lists every group and its members", () => {
  const markup = visualMarkup(
    sectionsBody({ groups: { "group:admins": ["alice@example.com", "bob@example.com"] } }),
    "groups",
  );
  assert.match(markup, /value="group:admins"/);
  assert.match(markup, /alice@example\.com/);
  assert.match(markup, /bob@example\.com/);
});

test("the Hosts section shows the address of each alias as a field, not a member list", () => {
  const markup = visualMarkup(sectionsBody({ hosts: { server: "100.64.0.1" } }), "hosts");
  assert.match(markup, /value="100\.64\.0\.1"/);
  assert.ok(!markup.includes("setmembers"));
});

test("a section with no entry states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ groups: {} }), "groups");
  assert.match(markup, /This section holds no entry/);
});

test("the section nav escapes a hostile group name and member", () => {
  const markup = visualMarkup(sectionsBody({ groups: { '"><script>': ['"><script>'] } }), "groups");
  assert.ok(!markup.includes("<script>"));
});

// ---------------------------------------------------------------------------
// The section nav: SSH access, Auto-approvers, Node attributes, Postures, Tests
// (#321, FR-vadv-1, FR-vadv-2)
// ---------------------------------------------------------------------------

test("the section nav lists the five Epic 13 sections with their entry counts", () => {
  const markup = visualMarkup(
    sectionsBody({
      ssh: [{ action: "accept", src: ["a"], dst: ["b"], users: ["root"] }],
      autoApprovers: { routes: { "10.0.0.0/8": ["tag:router"] }, exitNode: ["tag:exit"] },
      nodeAttrs: [{ target: ["tag:server"], attr: ["funnel"] }],
      postures: { "posture:latest": ["node:os in ['linux']"] },
      tests: [{ src: "group:eng", accept: ["tag:server:22"] }],
      sshTests: [{ src: "group:eng", accept: ["tag:server:22"] }],
    }),
  );

  assert.match(markup, /<span class="name">SSH access<\/span><span class="mono">1<\/span>/);
  assert.match(markup, /<span class="name">Auto-approvers<\/span><span class="mono">2<\/span>/);
  assert.match(markup, /<span class="name">Node attributes<\/span><span class="mono">1<\/span>/);
  assert.match(markup, /<span class="name">Postures<\/span><span class="mono">1<\/span>/);
  assert.match(markup, /<span class="name">Tests<\/span><span class="mono">2<\/span>/);
});

test("the SSH access section shows one row per ssh entry with its source, destination, users, and action", () => {
  const sections = sectionsBody({
    ssh: [{ action: "accept", src: ["group:eng"], dst: ["tag:server"], users: ["autogroup:nonroot"] }],
  });

  const markup = visualMarkup(sections, "ssh");

  assert.match(markup, /group:eng/);
  assert.match(markup, /tag:server/);
  assert.match(markup, /autogroup:nonroot/);
  assert.match(markup, /value="accept"/);
});

test("a check action shows the check period, per FR-vadv-4", () => {
  const sections = sectionsBody({
    ssh: [{ action: "check", src: ["tag:laptop"], dst: ["tag:server"], users: ["root"], checkPeriod: "20h" }],
  });

  const markup = visualMarkup(sections, "ssh");

  assert.match(markup, /ssh-checkperiod/);
  assert.match(markup, /value="20h"/);
});

test("an accept action shows no check period field", () => {
  const sections = sectionsBody({
    ssh: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server"], users: ["root"] }],
  });

  const markup = visualMarkup(sections, "ssh");

  assert.ok(!markup.includes("ssh-checkperiod"));
});

test("an SSH access section with no entry states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ ssh: [] }), "ssh");
  assert.match(markup, /This section holds no entry/);
});

test("the SSH access section escapes a hostile source", () => {
  const markup = visualMarkup(
    sectionsBody({ ssh: [{ action: "accept", src: ['"><script>'], dst: ["b"], users: ["root"] }] }),
    "ssh",
  );
  assert.ok(!markup.includes("<script>"));
});

// ---------------------------------------------------------------------------
// Auto-approvers (#322, FR-vadv-6, FR-vadv-7)
// ---------------------------------------------------------------------------

test("the Auto-approvers section shows one row per route CIDR and one row for the exit node", () => {
  const sections = sectionsBody({
    autoApprovers: { routes: { "10.0.0.0/8": ["tag:router"] }, exitNode: ["group:eng"] },
  });

  const markup = visualMarkup(sections, "autoApprovers");

  assert.match(markup, /10\.0\.0\.0\/8/);
  assert.match(markup, /tag:router/);
  assert.match(markup, /exit node/);
  assert.match(markup, /group:eng/);
});

test("an Auto-approvers section with no route still draws the exit node row", () => {
  const markup = visualMarkup(sectionsBody({ autoApprovers: {} }), "autoApprovers");
  assert.match(markup, /exit node/);
});

test("an Auto-approvers section with no route and no exit node approver states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ autoApprovers: {} }), "autoApprovers");
  assert.match(markup, /This section holds no entry/);
});

test("an Auto-approvers section that holds one exit node approver states no empty state", () => {
  const markup = visualMarkup(
    sectionsBody({ autoApprovers: { exitNode: ["group:eng"] } }),
    "autoApprovers",
  );
  assert.ok(!markup.includes("This section holds no entry"));
});

test("an Auto-approvers section that holds one route states no empty state", () => {
  const markup = visualMarkup(
    sectionsBody({ autoApprovers: { routes: { "10.0.0.0/8": [] } } }),
    "autoApprovers",
  );
  assert.ok(!markup.includes("This section holds no entry"));
});

test("the Auto-approvers section escapes a hostile CIDR and a hostile approver", () => {
  const markup = visualMarkup(
    sectionsBody({ autoApprovers: { routes: { '"><script>': ['"><script>'] } } }),
    "autoApprovers",
  );
  assert.ok(!markup.includes("<script>"));
});

test("addAutoApproverRoute adds a route through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ autoApprovers: { routes: { "10.0.0.0/8": [] } } });
  });

  await state.addAutoApproverRoute("jbones", "10.0.0.0/8", []);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "autoApprovers.routes",
      op: "add",
      key: "10.0.0.0/8",
      entry: [],
    },
  });
  assert.equal(calls[1].route, policySectionsRoute("jbones"));
  assert.deepEqual(toggleModel(state, "jbones").sections.autoApprovers.routes, { "10.0.0.0/8": [] });
});

test("replaceAutoApproverRoute stages an approver added to a route's list", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ autoApprovers: { routes: { "10.0.0.0/8": ["tag:router"] } } });
  });

  await state.replaceAutoApproverRoute("jbones", "10.0.0.0/8", ["tag:router"]);

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "autoApprovers.routes",
    op: "replace",
    key: "10.0.0.0/8",
    entry: ["tag:router"],
  });
});

test("removeAutoApproverRoute removes a route through sections/edit", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ autoApprovers: {} });
  });

  await state.removeAutoApproverRoute("jbones", "10.0.0.0/8");

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "autoApprovers.routes",
    op: "remove",
    key: "10.0.0.0/8",
  });
});

test("setAutoApproverExitNode replaces the whole exit node approver list, and carries no key", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ autoApprovers: { exitNode: ["group:eng"] } });
  });

  await state.setAutoApproverExitNode("jbones", ["group:eng"]);

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "autoApprovers.exitNode",
    op: "replace",
    entry: ["group:eng"],
  });
});

test("parseSSHCheckPeriod accepts a duration in the control server's documented form", () => {
  assert.deepEqual(parseSSHCheckPeriod("20h"), { value: "20h", error: "" });
  assert.deepEqual(parseSSHCheckPeriod("1h30m"), { value: "1h30m", error: "" });
  assert.deepEqual(parseSSHCheckPeriod(""), { value: "", error: "" });
});

test("parseSSHCheckPeriod rejects a bad form, naming the expected form", () => {
  const result = parseSSHCheckPeriod("banana");
  assert.equal(result.value, "");
  assert.match(result.error, /duration/);
  assert.match(result.error, /20h/);
});

test("stageListAdd adds an ssh entry through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const newEntry = { action: "accept", src: ["group:eng"], dst: ["tag:server"], users: ["root"] };
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ ssh: [newEntry] });
  });

  await state.stageListAdd("jbones", "ssh", newEntry);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "ssh",
      op: "add",
      entry: newEntry,
    },
  });
  assert.equal(calls[1].route, policySectionsRoute("jbones"));
  assert.deepEqual(toggleModel(state, "jbones").sections.ssh, [newEntry]);
});

// ---------------------------------------------------------------------------
// Node attributes (#323, FR-vadv-8, FR-vadv-9)
// ---------------------------------------------------------------------------

test("the node attributes section shows one row per nodeAttrs entry with its targets and attributes", () => {
  const sections = sectionsBody({
    nodeAttrs: [{ target: ["tag:server", "tag:db"], attr: ["funnel"] }],
  });

  const markup = visualMarkup(sections, "nodeAttrs");

  assert.match(markup, /tag:server/);
  assert.match(markup, /tag:db/);
  assert.match(markup, /funnel/);
});

test("a target of asterisk shows the literal character, per FR-vadv-8's edge case", () => {
  const sections = sectionsBody({
    nodeAttrs: [{ target: ["*"], attr: ["funnel"] }],
  });

  const markup = visualMarkup(sections, "nodeAttrs");

  assert.match(markup, /value="\*"/);
});

test("a node attributes section with no entry states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ nodeAttrs: [] }), "nodeAttrs");
  assert.match(markup, /This section holds no entry/);
});

test("the node attributes section escapes a hostile target", () => {
  const markup = visualMarkup(
    sectionsBody({ nodeAttrs: [{ target: ['"><script>'], attr: ["funnel"] }] }),
    "nodeAttrs",
  );
  assert.ok(!markup.includes("<script>"));
});

test("stageListAdd adds a nodeAttrs entry through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const newEntry = { target: ["tag:server"], attr: ["funnel"] };
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ nodeAttrs: [newEntry] });
  });

  await state.stageListAdd("jbones", "nodeAttrs", newEntry);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "nodeAttrs",
      op: "add",
      entry: newEntry,
    },
  });
  assert.equal(calls[1].route, policySectionsRoute("jbones"));
  assert.deepEqual(toggleModel(state, "jbones").sections.nodeAttrs, [newEntry]);
});

test("nodeAttrsEntryWithField replaces one field of a nodeAttrs entry and keeps the other", () => {
  const entry = { target: ["tag:server"], attr: ["funnel"] };
  assert.deepEqual(nodeAttrsEntryWithField(entry, "attr", ["funnel", "webserver"]), {
    target: ["tag:server"],
    attr: ["funnel", "webserver"],
  });
  assert.deepEqual(nodeAttrsEntryWithField(entry, "target", ["*"]), {
    target: ["*"],
    attr: ["funnel"],
  });
});

// ---------------------------------------------------------------------------
// Postures (#324, FR-vadv-10, FR-vadv-11)
// ---------------------------------------------------------------------------

test("the Postures section shows one row per entry with its name and its expression", () => {
  const sections = sectionsBody({
    postures: { "posture:latest": ["node:os == 'macos'", "node:tsVersion >= '1.40'"] },
  });

  const markup = visualMarkup(sections, "postures");

  assert.match(markup, /posture:latest/);
  assert.match(markup, /node:os == &#39;macos&#39; &amp;&amp; node:tsVersion &gt;= &#39;1\.40&#39;/);
});

test("a Postures section with no entry states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ postures: {} }), "postures");
  assert.match(markup, /This section holds no entry/);
});

test("the Postures section escapes a hostile name and expression", () => {
  const markup = visualMarkup(
    sectionsBody({ postures: { '"><script>': ['"><script>'] } }),
    "postures",
  );
  assert.ok(!markup.includes("<script>"));
});

test("addSetEntry adds a posture through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ postures: { "posture:latest": ["node:os == 'linux'"] } });
  });

  await state.addSetEntry("jbones", "postures", "posture:latest", ["node:os == 'linux'"]);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "postures",
      op: "add",
      key: "posture:latest",
      entry: ["node:os == 'linux'"],
    },
  });
  assert.equal(calls[1].route, policySectionsRoute("jbones"));
  assert.deepEqual(toggleModel(state, "jbones").sections.postures, {
    "posture:latest": ["node:os == 'linux'"],
  });
});

test("a Headscale tailnet's Postures section shows every entry read-only and states the reason, per FR-vadv-11", () => {
  const markup = visualMarkup(
    sectionsBody({ postures: { "posture:latest": ["node:os == 'linux'"] } }),
    "postures",
    null,
    null,
    "headscale",
  );

  assert.match(markup, /does not support/);
  assert.match(markup, /posture:latest/);
  assert.match(markup, /node:os == &#39;linux&#39;/);
  assert.ok(!markup.includes("data-add-key"));
  assert.ok(!markup.includes("rename-posture"));
});

test("a Tailscale tailnet still edits Postures normally", () => {
  const markup = visualMarkup(
    sectionsBody({ postures: { "posture:latest": ["node:os == 'linux'"] } }),
    "postures",
    null,
    null,
    "tailscale",
  );

  assert.match(markup, /posture:latest/);
  assert.match(markup, /data-add-key/);
});

test("push disables while a Headscale document holds a postures key, per FR-vadv-11", async () => {
  const state = createPolicyState({
    request: async (route) => {
      if (route === policyValidateRoute("homelab")) {
        return { passed: true };
      }
      return sectionsBody({ postures: { "posture:latest": ["node:os == 'linux'"] } });
    },
  });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", kind: "headscale" }));

  await state.validate("homelab");
  await state.loadSections("homelab");

  const model = entryOf(state, "homelab");
  assert.equal(model.stage, "validated");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);
});

test("push disables while a Headscale document holds an empty postures key, per FR-vadv-11", async () => {
  const state = createPolicyState({
    request: async (route) => {
      if (route === policyValidateRoute("homelab")) {
        return { passed: true };
      }
      return sectionsBody({ postures: {}, section_keys: ["postures"] });
    },
  });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", kind: "headscale" }));

  await state.validate("homelab");
  await state.loadSections("homelab");

  const model = entryOf(state, "homelab");
  assert.equal(model.stage, "validated");
  assert.equal(controlOf(actionsModel(model), "push").disabled, true);
});

test("the section nav counts an empty postures key as no entry, per FR-vadv-2", () => {
  const markup = visualMarkup(sectionsBody({ postures: {}, section_keys: ["postures"] }));

  assert.match(markup, /<span class="name">Postures<\/span><span class="mono">0<\/span>/);
});

test("push re-enables when a Headscale document holds no postures key", async () => {
  const state = createPolicyState({
    request: async (route) => {
      if (route === policyValidateRoute("homelab")) {
        return { passed: true };
      }
      return sectionsBody({ postures: {}, section_keys: ["groups", "acls"] });
    },
  });
  state.setList(listBody());
  state.setDocument("homelab", documentBody({ id: "homelab", kind: "headscale" }));

  await state.validate("homelab");
  await state.loadSections("homelab");

  const model = entryOf(state, "homelab");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
});

test("push stays enabled when a Tailscale document holds a postures key", async () => {
  const state = createPolicyState({
    request: async (route) => {
      if (route === policyValidateRoute("jbones")) {
        return { passed: true };
      }
      return sectionsBody({ postures: { "posture:latest": ["node:os == 'linux'"] } });
    },
  });
  state.setList(listBody());
  state.setDocument("jbones", documentBody());

  await state.validate("jbones");
  await state.loadSections("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
});

// ---------------------------------------------------------------------------
// Tests (#325, FR-vadv-12 to FR-vadv-14)
// ---------------------------------------------------------------------------

test("the Tests section shows one row per tests and sshTests entry with its source and expected result", () => {
  const sections = sectionsBody({
    tests: [{ src: "group:eng", accept: ["tag:server:22"] }],
    sshTests: [{ src: "tag:laptop", dst: ["tag:server"], deny: ["root"] }],
  });

  const markup = visualMarkup(sections, "tests");

  assert.match(markup, /group:eng/);
  assert.match(markup, /accept tag:server:22/);
  assert.match(markup, /tag:laptop/);
  assert.match(markup, /deny root/);
});

test("an sshTests entry's expected result joins its accept, check, and deny user lists", () => {
  const sections = sectionsBody({
    sshTests: [{ src: "tag:laptop", dst: ["tag:server"], accept: ["dave"], check: ["admin"], deny: ["root"] }],
  });

  const markup = visualMarkup(sections, "tests");

  assert.match(markup, /accept dave; check admin; deny root/);
});

test("a Tests section with no entry states that it holds none", () => {
  const markup = visualMarkup(sectionsBody({ tests: [], sshTests: [] }), "tests");
  assert.match(markup, /This section holds no entry/);
});

test("the Tests section escapes a hostile source", () => {
  const markup = visualMarkup(sectionsBody({ tests: [{ src: '"><script>', accept: ["a:1"] }] }), "tests");
  assert.ok(!markup.includes("<script>"));
});

test("the Tests section offers Run, Add, and Remove controls", () => {
  const markup = visualMarkup(
    sectionsBody({ tests: [{ src: "group:eng", accept: ["tag:server:22"] }] }),
    "tests",
  );
  assert.match(markup, /data-act="run-tests"/);
  assert.match(markup, /data-act="add-test"/);
  assert.match(markup, /data-act="test-delete"/);
});

test("the Tests section marks a row pass or fail from the validate answer, per FR-vadv-13", () => {
  const sections = sectionsBody({
    tests: [
      { src: "group:eng", accept: ["tag:server:22"] },
      { src: "tag:server", accept: ["autogroup:internet:443"] },
    ],
  });
  const answer = {
    passed: false,
    result: JSON.stringify({
      message: "test(s) failed",
      data: [{ user: "tag:server", errors: ['address "1.2.3.4:443": want: Drop, got: Accept'] }],
    }),
  };

  const markup = visualMarkup(sections, "tests", null, null, "", { pending: false, answer });

  assert.match(markup, /group:eng[\s\S]*dot ok[\s\S]*pass/);
  assert.match(markup, /tag:server[\s\S]*want: Drop, got: Accept/);
});

test("the Tests section shows the validate error in place of the rows when the answer carries no assertion result", () => {
  const sections = sectionsBody({ tests: [{ src: "group:eng", accept: ["tag:server:22"] }] });
  const answer = { passed: false, result: "the Headscale control server rejected the document: invalid huJSON" };

  const markup = visualMarkup(sections, "tests", null, null, "", { pending: false, answer });

  assert.match(markup, /invalid huJSON/);
  assert.ok(!markup.includes("group:eng"));
});

test("runTests sends the staged document to POST /api/policy/{id}/validate and holds the answer, per FR-vadv-13", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policyValidateRoute("jbones")) {
      return { passed: false, result: '{"message":"test(s) failed","data":[]}' };
    }
    return sectionsBody();
  });

  await state.runTests("jbones");

  assert.deepEqual(calls[0], {
    route: policyValidateRoute("jbones"),
    method: "POST",
    body: { document: '{\n  "grants": [],\n}' },
  });
  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.testsPending, false);
  assert.deepEqual(toggle.testsAnswer, { passed: false, result: '{"message":"test(s) failed","data":[]}' });
});

test("a run of the tests changes no field that Push reads, per FR-vadv-14", async () => {
  let calls = 0;
  const state = loaded(async (route) => {
    if (route === policyValidateRoute("jbones")) {
      calls += 1;
      if (calls === 1) {
        return { passed: true };
      }
      return {
        passed: false,
        result: '{"message":"test(s) failed","data":[{"user":"tag:server","errors":["boom"]}]}',
      };
    }
    return sectionsBody({ tests: [{ src: "tag:server", accept: ["autogroup:internet:443"] }] });
  });

  await state.validate("jbones");
  assert.equal(entryOf(state, "jbones").stage, "validated");

  await state.runTests("jbones");

  const model = entryOf(state, "jbones");
  assert.equal(model.stage, "validated");
  assert.equal(controlOf(actionsModel(model), "push").disabled, false);
  assert.equal(toggleModel(state, "jbones").testsAnswer.passed, false);
});

test("stageListAdd adds a tests entry through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const newEntry = { src: "group:eng", accept: ["tag:server:22"] };
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ tests: [newEntry] });
  });

  await state.stageListAdd("jbones", "tests", newEntry);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "tests",
      op: "add",
      entry: newEntry,
    },
  });
  assert.deepEqual(toggleModel(state, "jbones").sections.tests, [newEntry]);
});

test("referencingRules finds a group or a tag named in an acls or a grants src or dst", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["group:admins"], dst: ["tag:server"] }],
    grants: [{ src: ["group:eng"], dst: ["tag:db"] }],
  });
  assert.deepEqual(referencingRules(sections, "group:admins"), [
    { section: "acls", src: "group:admins", dst: "tag:server" },
  ]);
  assert.deepEqual(referencingRules(sections, "tag:db"), [
    { section: "grants", src: "group:eng", dst: "tag:db" },
  ]);
  assert.deepEqual(referencingRules(sections, "group:none"), []);
});

test("referencingSentence names each referencing rule and states that removal keeps it", () => {
  const sentence = referencingSentence([{ section: "acls", src: "group:admins", dst: "tag:server" }]);
  assert.match(sentence, /One rule references this entry/);
  assert.match(sentence, /acls: group:admins to tag:server/);
  assert.match(sentence, /does not remove the rule/);
});

test("referencingSentence is empty for no referencing rule", () => {
  assert.equal(referencingSentence([]), "");
});

test("addSetEntry adds a key to Groups through sections/edit, then re-reads sections", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody({ groups: { "group:eng": ["carol@example.com"] } });
  });

  await state.addSetEntry("jbones", "groups", "group:eng", ["carol@example.com"]);

  assert.deepEqual(calls[0], {
    route: policySectionsEditRoute("jbones"),
    method: "POST",
    body: {
      document: '{\n  "grants": [],\n}',
      section: "groups",
      op: "add",
      key: "group:eng",
      entry: ["carol@example.com"],
    },
  });
  assert.equal(calls[1].route, policySectionsRoute("jbones"));
  assert.equal(entryOf(state, "jbones").text, "the edited text");
  assert.deepEqual(toggleModel(state, "jbones").sections.groups, { "group:eng": ["carol@example.com"] });
});

test("renameSetEntry changes the key of a Hosts alias and keeps its address", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody();
  });

  await state.renameSetEntry("jbones", "hosts", "server", "primary-server");

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "hosts",
    op: "rename",
    key: "server",
    new_key: "primary-server",
  });
});

test("removeSetEntry removes an IP set entry at once, because FR-vacl-6 checks a group and a tag alone", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody();
  });

  await state.removeSetEntry("jbones", "ipsets", "internal");

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "ipsets",
    op: "remove",
    key: "internal",
  });
});

test("replaceSetValue replaces the member list of a Tag owners entry", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody();
  });

  await state.replaceSetValue("jbones", "tagOwners", "tag:prod", ["group:sre"]);

  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "tagOwners",
    op: "replace",
    key: "tag:prod",
    entry: ["group:sre"],
  });
});

// The tests below read the entry field from the wire format rather than from the body
// object. A test that asserts the body object alone passes while the field holds array
// text as a JSON string. Issue #348 is that defect. internal/api/policy.go reads Entry as
// json.RawMessage, so the bytes decide whether the edit succeeds.

// wireEntry returns the entry field as the daemon reads it. consoleRequestInit is the one
// serializer of every console request, therefore a body that passes through it here
// carries the exact bytes the browser sends. See internal/ui/static/app.js.
function wireEntry(body) {
  return JSON.parse(consoleRequestInit("POST", body).body).entry;
}

// editBody returns the request body of the sections/edit request that run sends.
async function editBody(run) {
  let sent = null;
  const state = loaded(async (route, method, body) => {
    if (route === policySectionsEditRoute("jbones")) {
      sent = body;
      return { document: "the edited text" };
    }
    return sectionsBody();
  });
  await run(state);
  return sent;
}

test("addSetEntry sends a member list that the daemon reads as a JSON array", async () => {
  const body = await editBody((state) => state.addSetEntry("jbones", "groups", "group:eng", ["carol@example.com"]));

  assert.deepEqual(wireEntry(body), ["carol@example.com"]);
});

test("replaceSetValue sends a member list that the daemon reads as a JSON array", async () => {
  const body = await editBody((state) => state.replaceSetValue("jbones", "tagOwners", "tag:prod", ["group:sre"]));

  assert.deepEqual(wireEntry(body), ["group:sre"]);
});

test("replaceSetValue sends a host address that the daemon reads as a JSON string", async () => {
  const body = await editBody((state) => state.replaceSetValue("jbones", "hosts", "server", "100.64.0.1"));

  assert.equal(wireEntry(body), "100.64.0.1");
});

test("addAutoApproverRoute sends an approver list that the daemon reads as a JSON array", async () => {
  const body = await editBody((state) => state.addAutoApproverRoute("jbones", "10.0.0.0/8", []));

  assert.deepEqual(wireEntry(body), []);
});

test("replaceAutoApproverRoute sends an approver list that the daemon reads as a JSON array", async () => {
  const body = await editBody((state) => state.replaceAutoApproverRoute("jbones", "10.0.0.0/8", ["tag:router"]));

  assert.deepEqual(wireEntry(body), ["tag:router"]);
});

test("setAutoApproverExitNode sends an approver list that the daemon reads as a JSON array", async () => {
  const body = await editBody((state) => state.setAutoApproverExitNode("jbones", ["group:eng"]));

  assert.deepEqual(wireEntry(body), ["group:eng"]);
});

test("a refused section edit states the message and sends no second request", async () => {
  const calls = [];
  const state = loaded(async (route) => {
    calls.push(route);
    throw refusal(400, 'section "groups" already holds key "group:admins"');
  });

  await state.addSetEntry("jbones", "groups", "group:admins", ["a"]);

  assert.equal(calls.length, 1);
  assert.equal(toggleModel(state, "jbones").error, 'section "groups" already holds key "group:admins"');
});

test("selectNav opens and closes a named-set section, and it sends no request", async () => {
  const state = loaded(async () => sectionsBody());

  state.selectNav("jbones", "groups");
  assert.equal(toggleModel(state, "jbones").nav, "groups");

  state.selectNav("jbones", "groups");
  assert.equal(toggleModel(state, "jbones").nav, "");
});

test("removing a tag that a rule references states which rule references it, and removes neither on its own", async () => {
  const calls = [];
  const state = loaded(async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === policySectionsEditRoute("jbones")) {
      return { document: "the edited text" };
    }
    return sectionsBody();
  });
  state.entry("jbones").sections = sectionsBody({
    tagOwners: { "tag:prod": ["group:sre"] },
    acls: [{ action: "accept", src: ["group:sre"], dst: ["tag:prod"] }],
  });

  await state.removeSetEntry("jbones", "tagOwners", "tag:prod");

  assert.equal(calls.length, 0, "removeSetEntry sent a request before the operator confirmed it");
  const toggle = toggleModel(state, "jbones");
  assert.ok(toggle.pendingRemoval);
  assert.equal(toggle.pendingRemoval.key, "tag:prod");
  assert.equal(toggle.pendingRemoval.rules.length, 1);
  assert.equal(toggle.pendingRemoval.rules[0].section, "acls");

  const markup = visualMarkup(toggle.sections, "tagOwners", toggle.pendingRemoval);
  assert.match(markup, /One rule references this entry/);
  assert.match(markup, /Remove anyway/);

  await state.confirmRemoval("jbones");

  assert.equal(calls.length, 2);
  assert.deepEqual(calls[0].body, {
    document: '{\n  "grants": [],\n}',
    section: "tagOwners",
    op: "remove",
    key: "tag:prod",
  });
  assert.equal(toggleModel(state, "jbones").pendingRemoval, null);
});

test("cancelRemoval discards a paused removal and sends no request", () => {
  const state = loaded(async () => sectionsBody());
  state.entry("jbones").sections = sectionsBody({
    tagOwners: { "tag:prod": ["group:sre"] },
    acls: [{ action: "accept", src: ["group:sre"], dst: ["tag:prod"] }],
  });

  state.removeSetEntry("jbones", "tagOwners", "tag:prod");
  assert.ok(toggleModel(state, "jbones").pendingRemoval);

  state.cancelRemoval("jbones");
  assert.equal(toggleModel(state, "jbones").pendingRemoval, null);
});

// ---------------------------------------------------------------------------
// The reachability matrix (FR-vacl-7 to FR-vacl-9)
// ---------------------------------------------------------------------------

// squareAt returns the square of a matrix model at one row and one column.
function squareAt(model, from, to) {
  const row = model.rows.find((entry) => entry.source === from);
  return row.squares.find((square) => square.to === to);
}

test("the matrix places every tag, group, and autogroup that a rule references on both axes", () => {
  // groups is empty so that this test measures the referenced identities alone. The
  // matrix also draws every named identity, which issue #351's own tests cover.
  const sections = sectionsBody({
    groups: {},
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
    grants: [{ src: ["group:eng"], dst: ["tag:server"], ip: ["tcp:22"] }],
  });

  const model = matrixModel(sections);

  assert.deepEqual(model.nodes, ["group:eng", "tag:laptop", "tag:server"]);
  assert.equal(model.rows.length, 3);
  for (const row of model.rows) {
    assert.equal(row.squares.length, 3);
  }
});

test("a filled square means at least one acls or grants entry allows the path", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
    grants: [{ src: ["group:eng"], dst: ["tag:server"], ip: ["tcp:22"] }],
  });

  const model = matrixModel(sections);

  assert.equal(squareAt(model, "tag:laptop", "tag:server").allowed, true);
  assert.equal(squareAt(model, "group:eng", "tag:server").allowed, true);
});

test("an empty square means no acls or grants entry allows the path", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
    grants: [],
  });

  const model = matrixModel(sections);

  assert.equal(squareAt(model, "tag:server", "tag:laptop").allowed, false);
});

test("the matrix drops the port that an acls destination carries, and draws no separate node for it", () => {
  const sections = sectionsBody({
    groups: {},
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:443"] }],
    grants: [],
  });

  const model = matrixModel(sections);

  assert.deepEqual(model.nodes, ["tag:laptop", "tag:server"]);
  assert.equal(squareAt(model, "tag:laptop", "tag:server").allowed, true);
});

test("the diagonal square is inert, per FR-vacl-7", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
    grants: [],
  });

  const model = matrixModel(sections);

  const diagonal = squareAt(model, "tag:laptop", "tag:laptop");
  assert.equal(diagonal.inert, true);
  assert.equal(diagonal.allowed, false);
});

test("hovering a square marks the row label and the column label alone", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
    grants: [],
  });

  const markup = matrixMarkup(matrixModel(sections));

  assert.match(markup, /data-from="tag:laptop" data-to="tag:server"/);
  assert.ok(!markup.includes("denied"), markup);
});

test("the matrix markup escapes a hostile node name", () => {
  const sections = sectionsBody({
    acls: [{ action: "accept", src: ['<img src=x onerror="alert(1)">'], dst: ["tag:server:*"] }],
    grants: [],
  });

  const markup = matrixMarkup(matrixModel(sections));

  assert.ok(!markup.includes("<img"), markup);
  assert.match(markup, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("the matrix scrolls sideways inside its own card", async () => {
  // Issue #387. The square holds a fixed size, therefore a document that names many
  // identities makes the table wider than the card, and the table overflowed into the next
  // card. The wrapper carries the scroll, so the matrix stays inside its own card. The
  // mockup docs/specs/mockups/06-visual-acl-editor.html holds the same treatment in
  // `.mtxwrap`.
  const sections = sectionsBody({ groups: {}, acls: [], grants: [] });

  const markup = matrixMarkup(matrixModel(sections));

  assert.match(markup, /<div class="ac-mtx-wrap"><table class="ac-mtx"/);

  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ac-mtx-wrap\{[^}]*overflow-x:auto/);
});

test("the matrix draws a row and a column for a group that no rule references", () => {
  // Issue #351. The operator decided on 2026-08-24 that the matrix draws every named
  // identity. A group therefore reaches the axes before a rule names it.
  const sections = sectionsBody({
    groups: { "group:admins": ["alice@example.com"], "group:new": ["bob@example.com"] },
    acls: [{ action: "accept", src: ["group:admins"], dst: ["*:*"] }],
    grants: [],
  });

  const model = matrixModel(sections);

  assert.ok(model.nodes.includes("group:new"), model.nodes.join(" "));
  assert.equal(squareAt(model, "group:new", "group:admins").allowed, false);
});

test("the matrix draws a row and a column for a host alias and for a tag owner that no rule references", () => {
  // Issue #351.
  const sections = sectionsBody({
    groups: {},
    hosts: { "build-box": "100.64.0.9" },
    tagOwners: { "tag:new": ["group:admins"] },
    acls: [],
    grants: [],
  });

  const model = matrixModel(sections);

  assert.deepEqual(model.nodes, ["build-box", "tag:new"]);
  assert.equal(squareAt(model, "build-box", "tag:new").allowed, false);
});

test("a click on the square of an identity that no rule references stages a new acls entry", () => {
  // Issue #351. The row alone grants no path. FR-vacl-8 must accept the click that the
  // new square draws.
  const sections = sectionsBody({
    groups: {},
    hosts: { "build-box": "100.64.0.9" },
    tagOwners: { "tag:new": ["group:admins"] },
    acls: [],
    grants: [],
  });

  assert.deepEqual(matrixClickPlan(sections, "build-box", "tag:new"), {
    op: "add",
    section: "acls",
    entry: { action: "accept", src: ["build-box"], dst: ["tag:new:*"] },
  });
});

test("the matrix draws no row for an IP set", () => {
  // Issue #351. The operator's decision of 2026-08-24 names a tag, a group, and a host
  // alone. A rule names an IP set as `ipset:<name>`, and this repository writes the key
  // in both forms. The key alone therefore does not give the axis label. Headscale
  // supports no ipsets section, per FR-vacl-19.
  const sections = sectionsBody({
    groups: {},
    ipsets: { "ipset:corp": ["10.0.0.0/24"] },
    acls: [],
    grants: [],
  });

  const model = matrixModel(sections);

  assert.deepEqual(model.nodes, []);
});

test("the matrix keeps the wildcard square when it also draws an identity that no rule references", () => {
  // Issue #351 with issue #349. A larger node set must not change the wildcard square.
  const sections = sectionsBody({
    groups: { "group:new": ["bob@example.com"] },
    acls: [],
    grants: [{ src: ["*"], dst: ["*"], ip: ["*"] }],
  });

  const model = matrixModel(sections);

  assert.deepEqual(model.nodes, ["*", "group:new"]);
  assert.equal(squareAt(model, "*", "*").allowed, true);
  assert.equal(squareAt(model, "*", "*").inert, false);
  assert.equal(squareAt(model, "group:new", "group:new").inert, true);
});

test("the route of the sections edit action encodes the identifier", () => {
  assert.equal(policySectionsEditRoute("jbones"), "/api/policy/jbones/sections/edit");
  assert.equal(policySectionsEditRoute("lab hs/1"), "/api/policy/lab%20hs%2F1/sections/edit");
});

test("the diagonal accepts no click", () => {
  // FR-vacl-7, in the manner of FR-editor-10 of features/07-console-access-editor.md.
  const sections = sectionsBody({ acls: [], grants: [] });

  assert.equal(matrixClickPlan(sections, "tag:laptop", "tag:laptop"), null);
});

test("the wildcard square is not inert, because * names every identity and not one identity", () => {
  // Issue #349. A document whose one rule allows every path holds * on both axes, so the
  // diagonal rule of FR-vacl-7 hides the one real square of the matrix.
  const sections = sectionsBody({ acls: [], grants: [{ src: ["*"], dst: ["*"], ip: ["*"] }] });

  const square = squareAt(matrixModel(sections), "*", "*");

  assert.equal(square.inert, false);
  assert.equal(square.allowed, true);
  assert.equal(square.label, "* to *, allowed");
});

test("the wildcard square carries the click data that the console binds", () => {
  // groups is empty so that the wildcard square is the one square of the matrix. A named
  // identity would add an inert diagonal square, which the disabled assertion below reads.
  const sections = sectionsBody({ groups: {}, acls: [], grants: [{ src: ["*"], dst: ["*"], ip: ["*"] }] });

  const markup = matrixMarkup(matrixModel(sections));

  assert.match(markup, /data-from="\*" data-to="\*"/);
  assert.ok(!markup.includes("disabled"), markup);
});

test("a click on the wildcard square plans the removal of the rule that allows every path", () => {
  const sections = sectionsBody({ acls: [], grants: [{ src: ["*"], dst: ["*"], ip: ["*"] }] });

  const plan = matrixClickPlan(sections, "*", "*");

  assert.deepEqual(plan, { op: "remove", removals: [{ section: "grants", index: 0 }] });
});

test("a click on an empty square plans an acls entry that allows every port, per FR-vacl-8", () => {
  const sections = sectionsBody({ acls: [], grants: [] });

  const plan = matrixClickPlan(sections, "tag:laptop", "tag:server");

  assert.deepEqual(plan, {
    op: "add",
    section: "acls",
    entry: { action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] },
  });
});

test("a click on a filled square plans the removal of every acls and grants entry for that path, per FR-vacl-9", () => {
  // The correction on issue #316 (posted after the batch cross-check): removing index i
  // shifts every later index down by one, so the plan removes each section's matching
  // entries highest index first.
  const sections = sectionsBody({
    acls: [
      { action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] },
      { action: "accept", src: ["tag:laptop"], dst: ["tag:other:*"] },
      { action: "accept", src: ["tag:laptop"], dst: ["tag:server:443"] },
    ],
    grants: [
      { src: ["tag:other"], dst: ["tag:server"] },
      { src: ["tag:laptop"], dst: ["tag:server"] },
    ],
  });

  const plan = matrixClickPlan(sections, "tag:laptop", "tag:server");

  assert.deepEqual(plan, {
    op: "remove",
    removals: [
      { section: "acls", index: 2 },
      { section: "acls", index: 0 },
      { section: "grants", index: 1 },
    ],
  });
});

test("clicking an empty matrix square stages an acls entry", async () => {
  const sent = [];
  const editedDocument = '{\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      return { document: editedDocument };
    }
    return body.document === editedDocument
      ? sectionsBody({ acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }] })
      : sectionsBody({ acls: [] });
  });
  state.setText("jbones", '{\n  "acls": [],\n}');
  await state.loadSections("jbones");
  assert.equal(toggleModel(state, "jbones").sections.acls.length, 0);

  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");

  assert.deepEqual(sent[1], {
    route: "/api/policy/jbones/sections/edit",
    method: "POST",
    body: {
      document: '{\n  "acls": [],\n}',
      section: "acls",
      op: "add",
      entry: { action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] },
    },
  });
  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.sections.acls.length, 1);
  assert.equal(state.edited("jbones"), true);
});

test("clicking a filled matrix square removes every matching entry, highest index first", async () => {
  const sent = [];
  const documents = [
    '{\n  "acls": [{"a":1},{"a":2},{"a":3}],\n}',
    '{\n  "acls": [{"a":1},{"a":2}],\n}',
    '{\n  "acls": [{"a":2}],\n}',
  ];
  let editCall = 0;
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      editCall += 1;
      return { document: documents[editCall] };
    }
    return sectionsBody({
      acls: [
        { action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] },
        { action: "accept", src: ["tag:laptop"], dst: ["tag:other:*"] },
        { action: "accept", src: ["tag:laptop"], dst: ["tag:server:443"] },
      ],
    });
  });
  state.setText("jbones", documents[0]);
  await state.loadSections("jbones");

  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");

  const edits = sent.filter((one) => one.route.endsWith("/sections/edit"));
  assert.equal(edits.length, 2);
  assert.equal(edits[0].body.index, 2);
  assert.equal(edits[0].body.document, documents[0]);
  assert.equal(edits[1].body.index, 0);
  assert.equal(edits[1].body.document, documents[1]);
});

test("clicking the wildcard square stages the removal of the rule that allows every path", async () => {
  // Issue #349. The document of a tailnet that allows everything holds this one rule, so
  // the wildcard square is the operator's only way to remove it.
  const sent = [];
  const readDocument = '{\n  "grants": [{"src":["*"],"dst":["*"],"ip":["*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      return { document: '{\n  "grants": [],\n}' };
    }
    return sectionsBody({ acls: [], grants: [{ src: ["*"], dst: ["*"], ip: ["*"] }] });
  });
  state.setText("jbones", readDocument);
  await state.loadSections("jbones");

  await state.stageMatrixClick("jbones", "*", "*");

  const edits = sent.filter((one) => one.route.endsWith("/sections/edit"));
  assert.equal(edits.length, 1);
  assert.equal(edits[0].body.op, "remove");
  assert.equal(edits[0].body.section, "grants");
  assert.equal(edits[0].body.index, 0);
  assert.equal(edits[0].body.document, readDocument);
});

// ---------------------------------------------------------------------------
// The rule list. FR-vacl-10 to FR-vacl-12.
// ---------------------------------------------------------------------------

test("a rule staged by the matrix appears in the rule list, marked staged", async () => {
  const added = '{\n  "grants": [],\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    if (route.endsWith("/sections/edit")) {
      return { document: added };
    }
    return body.document === added
      ? sectionsBody({ acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }], grants: [] })
      : sectionsBody({ acls: [], grants: [] });
  });

  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");

  const toggle = toggleModel(state, "jbones");
  const rows = ruleRows(toggle.sections, toggle.baseSections);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].from, "tag:laptop");
  assert.equal(rows[0].to, "tag:server");
  assert.equal(rows[0].staged, true);
  assert.match(ruleListMarkup(rows), /<span class="chip mono">staged<\/span>/);
});

test("a rule that matches the document the console read shows no staged chip", () => {
  const base = sectionsBody({ acls: [{ action: "accept", src: ["a"], dst: ["b:*"] }], grants: [] });
  const rows = ruleRows(base, base);

  assert.equal(rows[0].staged, false);
  assert.ok(!ruleListMarkup(rows).includes("staged"));
});

test("a grants entry's capability name shows in its row, per FR-vacl-11", () => {
  const entry = { src: ["tag:laptop"], dst: ["tag:server"], app: { "tailscale.com/cap/ssh": [{}] } };
  const rows = ruleRows(sectionsBody({ acls: [], grants: [entry] }));

  assert.equal(rows.length, 1);
  assert.equal(rows[0].section, "grants");
  assert.equal(rows[0].chip, "grants · app: tailscale.com/cap/ssh");
  assert.match(ruleListMarkup(rows), /tailscale\.com\/cap\/ssh/);
});

test("editing a grants entry removes a capability and keeps every other field", () => {
  const entry = { src: ["a"], dst: ["b"], ip: ["tcp:22"], app: { "cap-a": [{ x: 1 }], "cap-b": [] } };

  const removed = removeGrantCapability(entry, "cap-a");

  assert.deepEqual(removed, { src: ["a"], dst: ["b"], ip: ["tcp:22"], app: { "cap-b": [] } });
});

test("removing the last capability of a grants entry drops the app field entirely", () => {
  const entry = { src: ["a"], dst: ["b"], app: { "cap-a": [{}] } };

  const removed = removeGrantCapability(entry, "cap-a");

  assert.equal("app" in removed, false);
});

test("editing a grants entry renames a capability and keeps its parameters", () => {
  const entry = { src: ["a"], dst: ["b"], app: { "cap-a": [{ x: 1 }] } };

  const renamed = renameGrantCapability(entry, "cap-a", "cap-a-renamed");

  assert.deepEqual(renamed.app, { "cap-a-renamed": [{ x: 1 }] });
});

test("a grants entry with no capability, only ports, shows identically to an acls entry", () => {
  const rows = ruleRows(sectionsBody({ acls: [], grants: [{ src: ["a"], dst: ["b"], ip: ["tcp:22"] }] }));

  assert.equal(rows[0].chip, "grants");
  assert.deepEqual(rows[0].ports, ["tcp/22"]);
});

test("a bad port format is rejected with the concrete example message, per FR-vacl-12", () => {
  const result = parseRulePorts("tcp/22, garbage");

  assert.equal(result.ports, null);
  assert.equal(
    result.error,
    'invalid port "garbage": the form is tcp/<n>, udp/<n>, tcp/<n>-<m>, or udp/<n>-<m>, for example tcp/22',
  );
});

test("a good port list parses into its entries, matching internal/access/rules.go's format", () => {
  const result = parseRulePorts(" tcp/22, udp/1-1024 ");

  assert.deepEqual(result, { ports: ["tcp/22", "udp/1-1024"], error: null });
});

test("two rules with the same source and destination and different ports both show as separate rows", () => {
  const sections = sectionsBody({
    acls: [
      { action: "accept", src: ["tag:laptop"], dst: ["tag:server:22"] },
      { action: "accept", src: ["tag:laptop"], dst: ["tag:server:443"] },
    ],
    grants: [],
  });

  const rows = ruleRows(sections);

  assert.equal(rows.length, 2);
  assert.equal(rows[0].to, "tag:server");
  assert.equal(rows[1].to, "tag:server");
  assert.deepEqual(rows[0].ports, ["22"]);
  assert.deepEqual(rows[1].ports, ["443"]);
});

test("an acls entry with no port reads all ports", () => {
  const rows = ruleRows(sectionsBody({ acls: [{ action: "accept", src: ["a"], dst: ["b:*"] }], grants: [] }));

  assert.equal(rows[0].allPorts, true);
  assert.match(ruleListMarkup(rows), /<span class="ac-noports mono">all ports<\/span>/);
});

test("a grants entry whose ip is the wildcard reads all ports", () => {
  const rows = ruleRows(sectionsBody({ acls: [], grants: [{ src: ["a"], dst: ["b"], ip: ["*"] }] }));

  assert.equal(rows[0].allPorts, true);
  assert.deepEqual(rows[0].ports, []);
  assert.match(ruleListMarkup(rows), /<span class="ac-noports mono">all ports<\/span>/);
});

test("the port field of a wildcard grants entry holds no value, so it shows the words all ports", () => {
  const rows = ruleRows(sectionsBody({ acls: [], grants: [{ src: ["a"], dst: ["b"], ip: ["*"] }] }));

  const markup = ruleListMarkup(rows);

  assert.equal(rows[0].portsText, "");
  assert.match(markup, /class="field ac-portfield mono" data-act="rule-ports" value="" placeholder="all ports"/);
});

test("ruleEntryWithPorts replaces a grants entry's ip field, converting the slash to a colon", () => {
  const row = { section: "grants", entry: { src: ["a"], dst: ["b"] } };

  const next = ruleEntryWithPorts(row, ["tcp/22", "udp/1-1024"]);

  assert.deepEqual(next, { src: ["a"], dst: ["b"], ip: ["tcp:22", "udp:1-1024"] });
});

test("ruleEntryWithPorts keeps the wildcard ip of a grants entry the operator clears back to all ports", () => {
  const row = { section: "grants", entry: { src: ["a"], dst: ["b"], ip: ["*"] } };

  const next = ruleEntryWithPorts(row, []);

  assert.deepEqual(next, { src: ["a"], dst: ["b"], ip: ["*"] });
});

test("ruleEntryWithPorts replaces the wildcard ip of a grants entry with the ports the operator types", () => {
  const row = { section: "grants", entry: { src: ["a"], dst: ["b"], ip: ["*"] } };

  const next = ruleEntryWithPorts(row, ["tcp/22"]);

  assert.deepEqual(next, { src: ["a"], dst: ["b"], ip: ["tcp:22"] });
});

test("ruleEntryWithPorts replaces an acls entry's dst ports and its proto", () => {
  const row = { section: "acls", entry: { action: "accept", src: ["a"], dst: ["b:*"] } };

  const next = ruleEntryWithPorts(row, ["tcp/22", "tcp/443"]);

  assert.deepEqual(next, { action: "accept", src: ["a"], dst: ["b:22", "b:443"], proto: "tcp" });
});

test("ruleEntryWithPorts clears the ports and the proto of an acls entry back to all ports", () => {
  const row = { section: "acls", entry: { action: "accept", src: ["a"], dst: ["b:22"], proto: "tcp" } };

  const next = ruleEntryWithPorts(row, []);

  assert.deepEqual(next, { action: "accept", src: ["a"], dst: ["b:*"] });
});

test("deleting a row of the rule list sends the remove op with the row's section and index", async () => {
  const sent = [];
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      return { document: '{\n  "acls": [],\n}' };
    }
    return sectionsBody({ acls: [] });
  });
  state.setText("jbones", '{\n  "acls": [{"action":"accept","src":["a"],"dst":["b:*"]}],\n}');

  await state.stageRuleRemove("jbones", "acls", 0);

  assert.deepEqual(sent[0], {
    route: "/api/policy/jbones/sections/edit",
    method: "POST",
    body: { document: '{\n  "acls": [{"action":"accept","src":["a"],"dst":["b:*"]}],\n}', section: "acls", op: "remove", index: 0 },
  });
});

test("replacing a row of the rule list sends the replace op with the row's section, index, and entry", async () => {
  const sent = [];
  const nextEntry = { action: "accept", src: ["a"], dst: ["b:22"], proto: "tcp" };
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      return { document: '{\n  "acls": [{"action":"accept","src":["a"],"dst":["b:22"],"proto":"tcp"}],\n}' };
    }
    return sectionsBody({ acls: [nextEntry] });
  });
  state.setText("jbones", '{\n  "acls": [{"action":"accept","src":["a"],"dst":["b:*"]}],\n}');

  await state.stageRuleReplace("jbones", "acls", 0, nextEntry);

  assert.deepEqual(sent[0].body, {
    document: '{\n  "acls": [{"action":"accept","src":["a"],"dst":["b:*"]}],\n}',
    section: "acls",
    op: "replace",
    index: 0,
    entry: nextEntry,
  });
});

test("the rule list states no acls entry and no grants entry when the document holds none", () => {
  assert.match(ruleListMarkup([]), /No acls entry and no grants entry exist/);
});

test("the rule list escapes a hostile source", () => {
  const rows = ruleRows(sectionsBody({ acls: [{ action: "accept", src: ['<script>alert(1)</script>'], dst: ["b"] }], grants: [] }));

  const markup = ruleListMarkup(rows);
  assert.ok(!markup.includes("<script>"), markup);
});

// ---------------------------------------------------------------------------
// The staged summary and Push. FR-vacl-13 to FR-vacl-16, issue #318.
// ---------------------------------------------------------------------------

test("the staged summary names each section-level change", () => {
  const base = sectionsBody({
    groups: { "group:eng": ["a@example.com"] },
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }],
  });
  const staged = sectionsBody({
    groups: { "group:eng": ["a@example.com"], "group:ops": ["b@example.com"] },
    acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:443"] }],
  });

  const summary = diffSummary(staged, base);

  assert.deepEqual(summary, ["1 group added", "1 rule narrowed to one port"]);
});

test("the staged summary is empty before the console holds the sections of the read document", () => {
  assert.deepEqual(diffSummary(sectionsBody(), null), []);
});

test("the staged summary is empty while the staged text matches the read document", () => {
  const base = sectionsBody();

  assert.deepEqual(diffSummary(base, base), []);
});

test("the staged summary names an added member and a removed member of a named set", () => {
  const base = sectionsBody({ tagOwners: { "tag:server": ["group:eng"] } });
  const staged = sectionsBody({ tagOwners: { "tag:server": ["group:ops"] } });

  const summary = diffSummary(staged, base);

  assert.deepEqual(summary, [
    "1 member added to tag owner tag:server",
    "1 member removed from tag owner tag:server",
  ]);
});

test("the staged summary names an added rule and a removed rule", () => {
  const base = sectionsBody({ acls: [{ action: "accept", src: ["a"], dst: ["b:*"] }] });
  const staged = sectionsBody({ acls: [{ action: "accept", src: ["c"], dst: ["d:*"] }] });

  const summary = diffSummary(staged, base);

  assert.deepEqual(summary, ["1 rule added", "1 rule removed"]);
});

test("the staged count and the staged summary render above the section nav", async () => {
  const added = '{\n  "grants": [],\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    if (route.endsWith("/sections/edit")) {
      return { document: added };
    }
    return body.document === added
      ? sectionsBody({ acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }], grants: [] })
      : sectionsBody({ acls: [], grants: [] });
  });

  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");

  const toggle = toggleModel(state, "jbones");
  const markup = visualMarkup(toggle.sections, toggle.nav, toggle.pendingRemoval, toggle.baseSections);
  const diffbarAt = markup.indexOf("diffbar");
  const setrowAt = markup.indexOf("setrow");
  assert.ok(diffbarAt !== -1, markup);
  assert.ok(diffbarAt < setrowAt, markup);
  assert.match(markup, /<span class="n mono">1<\/span>/);
  assert.match(markup, /1 rule added/);
});

test("discard returns the visual editor to the document that the console read, and the staged count reads 0", async () => {
  const added = '{\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    if (route.endsWith("/sections/edit")) {
      return { document: added };
    }
    return body.document === added
      ? sectionsBody({ acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }] })
      : sectionsBody({ acls: [] });
  });

  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");
  assert.equal(state.edited("jbones"), true);

  state.discard("jbones");

  assert.equal(state.edited("jbones"), false);
  const toggle = toggleModel(state, "jbones");
  assert.deepEqual(diffSummary(toggle.sections, toggle.baseSections), []);
  assert.ok(!visualMarkup(toggle.sections, toggle.nav, toggle.pendingRemoval, toggle.baseSections).includes("diffbar"));
});

test("push sends the whole staged document through the existing route, after a visual edit", async () => {
  const sent = [];
  const added = '{\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}';
  const state = loaded(async (route, method, body) => {
    sent.push({ route, method, body });
    if (route.endsWith("/sections/edit")) {
      return { document: added };
    }
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    if (route.endsWith("/sections")) {
      return sectionsBody({ acls: [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }] });
    }
    return documentBody({ document: added, etag: "e0b2816b419" });
  });

  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");
  await state.validate("jbones");
  await state.push("jbones");

  const puts = sent.filter((one) => one.method === "PUT");
  assert.equal(puts.length, 1);
  assert.equal(puts[0].route, "/api/policy/jbones");
  assert.deepEqual(puts[0].body, { document: added, etag: "e0b2816b418" });
});

// ---------------------------------------------------------------------------
// The input memory of the Visual editor
// ---------------------------------------------------------------------------

// fakeField is one field of the Visual editor, with the members that applyInputMemory
// reads. The console has no build step and these tests hold no browser, therefore a test
// builds the field rather than a document. type writes what an operator types.
function fakeField(attributes) {
  const handlers = new Map();
  const start = (attributes.value || "").length;
  const node = {
    value: attributes.value || "",
    selectionStart: start,
    selectionEnd: start,
    focused: false,
    getAttribute: (name) => (name in attributes ? attributes[name] : null),
    getAttributeNames: () => Object.keys(attributes),
    addEventListener: (name, handler) => {
      handlers.set(name, [...(handlers.get(name) || []), handler]);
    },
    fire: (name) => {
      for (const handler of handlers.get(name) || []) {
        handler();
      }
    },
    focus: () => {
      node.focused = true;
      node.fire("focus");
    },
    setSelectionRange: (from, to) => {
      node.selectionStart = from;
      node.selectionEnd = to;
    },
    type: (text) => {
      node.value = text;
      node.selectionStart = text.length;
      node.selectionEnd = text.length;
      node.fire("input");
    },
  };
  return node;
}

// fieldsOf reads every input and every select out of one markup string, in the order the
// markup states them.
function fieldsOf(markup) {
  const fields = [];
  for (const tag of markup.match(/<(?:input|select)\b[^>]*>/g) || []) {
    const attributes = {};
    for (const [, name, , quoted] of tag.matchAll(/([a-zA-Z0-9-]+)(="([^"]*)")?/g)) {
      if (name === "input" || name === "select") {
        continue;
      }
      attributes[name] = quoted === undefined ? "" : quoted;
    }
    fields.push(fakeField(attributes));
  }
  return fields;
}

// fakeEditor is one draw of the Visual editor. applyInputMemory asks it for the fields
// alone, therefore querySelectorAll answers every field and reads no selector.
function fakeEditor(markup) {
  const fields = fieldsOf(markup);
  return { fields, querySelectorAll: () => fields };
}

// addField returns the field that carries one add marker, for example data-add-key.
function addField(editor, marker) {
  return editor.fields.find((field) => field.getAttribute(marker) !== null);
}

// flush returns after every pending answer reached its caller.
function flush() {
  return new Promise((resolve) => setImmediate(resolve));
}

test("a draw returns the text that the operator typed into a Visual editor field", () => {
  const memory = createInputMemory();
  const first = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(first, memory);
  addField(first, "data-add-key").type("group:eng");
  addField(first, "data-add-value").type("alice@example.com");

  const second = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(second, memory);

  assert.equal(addField(second, "data-add-key").value, "group:eng");
  assert.equal(addField(second, "data-add-value").value, "alice@example.com");
});

test("a draw returns the focus and the caret of the operator", () => {
  const memory = createInputMemory();
  const first = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(first, memory);
  const typed = addField(first, "data-add-key");
  typed.type("group:eng");
  typed.setSelectionRange(6, 6);
  typed.fire("keyup");

  const second = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(second, memory);

  const returned = addField(second, "data-add-key");
  assert.equal(returned.focused, true);
  assert.equal(returned.selectionStart, 6);
  assert.equal(returned.selectionEnd, 6);
  assert.equal(addField(second, "data-add-value").focused, false);
});

test("a draw returns the text of every section that the Visual editor draws", () => {
  const sections = sectionsBody({
    ssh: [{ action: "accept", src: ["group:admins"], dst: ["tag:server"], users: ["root"] }],
    nodeAttrs: [{ target: ["tag:server"], attr: ["funnel"] }],
    postures: { "posture:latest": ["node:os IN ['linux']"] },
    autoApprovers: { routes: { "10.0.0.0/24": ["tag:router"] } },
  });
  const markers = {
    groups: "data-add-key",
    ssh: "data-add-src",
    nodeAttrs: "data-add-target",
    postures: "data-add-key",
    autoApprovers: "data-add-cidr",
  };

  for (const [nav, marker] of Object.entries(markers)) {
    const memory = createInputMemory();
    const first = fakeEditor(visualMarkup(sections, nav));
    applyInputMemory(first, memory);
    assert.ok(addField(first, marker), `${nav} draws ${marker}`);
    addField(first, marker).type("typed");

    const second = fakeEditor(visualMarkup(sections, nav));
    applyInputMemory(second, memory);

    assert.equal(addField(second, marker).value, "typed", `${nav} keeps the text`);
  }
});

test("a draw tells two fields that carry one marker apart, row by row", () => {
  const sections = sectionsBody({
    ssh: [
      { action: "accept", src: ["group:admins"], dst: ["tag:server"], users: ["root"] },
      { action: "accept", src: ["group:eng"], dst: ["tag:lab"], users: ["ubuntu"] },
    ],
  });
  const memory = createInputMemory();
  const first = fakeEditor(visualMarkup(sections, "ssh"));
  applyInputMemory(first, memory);
  const rows = first.fields.filter((field) => field.getAttribute("data-act") === "ssh-src");
  assert.equal(rows.length, 2);
  rows[1].type("group:ops");

  const second = fakeEditor(visualMarkup(sections, "ssh"));
  applyInputMemory(second, memory);

  const drawn = second.fields.filter((field) => field.getAttribute("data-act") === "ssh-src");
  assert.equal(drawn[0].value, "group:admins");
  assert.equal(drawn[1].value, "group:ops");
});

test("an action that changes the state drops the text that the memory holds", () => {
  const memory = createInputMemory();
  const first = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(first, memory);
  addField(first, "data-add-key").type("group:eng");

  memory.forget();

  const second = fakeEditor(visualMarkup(sectionsBody(), "groups"));
  applyInputMemory(second, memory);

  assert.equal(addField(second, "data-add-key").value, "");
  assert.equal(addField(second, "data-add-key").focused, false);
});

test("the view key is equal for two draws of one state, and it differs after an edit", () => {
  const state = loaded(async () => sectionsBody());
  const key = () => viewKey(policyListMarkup(state.rows()), editorMarkup(editorModel(state, "jbones"), null));

  const first = key();
  assert.equal(key(), first);

  state.setText("jbones", "{}");
  assert.notEqual(key(), first);
});

test("a sections read that starts while another runs reads the text of the entry again", async () => {
  const sent = [];
  const waiting = [];
  const state = loaded((route, method, body) => {
    sent.push(body.document);
    return new Promise((resolve) => waiting.push(() => resolve(sectionsBody({ hosts: { server: "100.64.0.1" } }))));
  });
  const base = state.entry("jbones").text;

  const running = state.loadSections("jbones");
  await flush();
  state.setText("jbones", "{\n}");
  await state.loadSections("jbones");
  waiting.shift()();
  await flush();

  assert.deepEqual(sent, [base, "{\n}"]);

  waiting.shift()();
  await running;

  assert.deepEqual(state.entry("jbones").sections.hosts, { server: "100.64.0.1" });
});

// pushed returns a state that staged one rule and pushed it. Each answer of
// POST .../sections describes the document that the request holds. Every matrix click
// adds one rule, up to three.
async function pushed() {
  const rule = (src) => ({ action: "accept", src: [src], dst: ["tag:server:*"] });
  const documentOf = (rules) => `{\n  "acls": ${JSON.stringify(rules)},\n}`;
  const read = documentOf([]);
  const one = documentOf([rule("tag:laptop")]);
  const two = documentOf([rule("tag:laptop"), rule("tag:phone")]);
  const three = documentOf([rule("tag:laptop"), rule("tag:phone"), rule("tag:router")]);
  const sections = {
    [read]: sectionsBody({ acls: [] }),
    [one]: sectionsBody({ acls: [rule("tag:laptop")] }),
    [two]: sectionsBody({ acls: [rule("tag:laptop"), rule("tag:phone")] }),
    [three]: sectionsBody({ acls: [rule("tag:laptop"), rule("tag:phone"), rule("tag:router")] }),
  };
  const next = { [read]: one, [one]: two, [two]: three };
  const state = loaded(async (route, method, body) => {
    if (route.endsWith("/sections/edit")) {
      return { document: next[body.document] };
    }
    if (route.endsWith("/sections")) {
      return sections[body.document];
    }
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    return documentBody({ document: body.document, etag: "e0b2816b419" });
  }, { document: read });
  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");
  await state.validate("jbones");
  await state.push("jbones");
  return state;
}

test("a rule staged after a push shows the staged chip, per FR-vacl-10", async () => {
  const state = await pushed();

  await state.stageMatrixClick("jbones", "tag:phone", "tag:server");

  const toggle = toggleModel(state, "jbones");
  const rows = ruleRows(toggle.sections, toggle.baseSections);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows.map((row) => row.staged), [false, true]);
  assert.match(ruleListMarkup(rows), /<span class="chip mono">staged<\/span>/);
});

test("the staged summary names a rule staged after a push, per FR-vacl-14", async () => {
  const state = await pushed();

  await state.stageMatrixClick("jbones", "tag:phone", "tag:server");

  const toggle = toggleModel(state, "jbones");
  assert.deepEqual(diffSummary(toggle.sections, toggle.baseSections), ["1 rule added"]);
  assert.match(visualMarkup(toggle.sections, toggle.nav, toggle.pendingRemoval, toggle.baseSections), /diffbar/);
});

test("the staged summary counts every rule staged after the same push", async () => {
  const state = await pushed();

  await state.stageMatrixClick("jbones", "tag:phone", "tag:server");
  await state.stageMatrixClick("jbones", "tag:router", "tag:server");

  const toggle = toggleModel(state, "jbones");
  const rows = ruleRows(toggle.sections, toggle.baseSections);
  assert.deepEqual(rows.map((row) => row.staged), [false, true, true]);
  assert.deepEqual(diffSummary(toggle.sections, toggle.baseSections), ["2 rules added"]);
});

test("the staged summary is empty directly after a push", async () => {
  const state = await pushed();

  const toggle = toggleModel(state, "jbones");
  assert.deepEqual(diffSummary(toggle.sections, toggle.baseSections), []);
  assert.deepEqual(ruleRows(toggle.sections, toggle.baseSections).map((row) => row.staged), [false]);
});

test("a push that the control server rewrote stages no rule of its own", async () => {
  const rewritten = '{\n  // the control server wrote this comment\n  "acls": [],\n}';
  const state = loaded(async (route, method, body) => {
    if (route.endsWith("/sections/edit")) {
      return { document: '{\n  "acls": [{"action":"accept","src":["tag:laptop"],"dst":["tag:server:*"]}],\n}' };
    }
    if (route.endsWith("/sections")) {
      return sectionsBody({ acls: body.document === rewritten ? [] : [{ action: "accept", src: ["tag:laptop"], dst: ["tag:server:*"] }] });
    }
    if (route.endsWith("/validate")) {
      return { passed: true };
    }
    return documentBody({ document: rewritten, etag: "e0b2816b419" });
  }, { document: '{\n  "acls": [],\n}' });

  await state.loadSections("jbones");
  await state.stageMatrixClick("jbones", "tag:laptop", "tag:server");
  await state.validate("jbones");
  await state.push("jbones");

  // The document that the answer holds is not the document that the sections describe.
  // The baseline is therefore empty, and the rule list marks no row staged.
  const toggle = toggleModel(state, "jbones");
  assert.equal(toggle.baseSections, null);
  assert.deepEqual(diffSummary(toggle.sections, toggle.baseSections), []);
  assert.deepEqual(ruleRows(toggle.sections, toggle.baseSections).map((row) => row.staged), [false]);
});
