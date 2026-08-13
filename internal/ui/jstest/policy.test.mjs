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

import {
  CONFLICT_STATUS,
  EVERY_DEVICE_STATEMENT,
  POLICY_ROUTE,
  PUSH_STATEMENT,
  actionsMarkup,
  actionsModel,
  createPolicyState,
  editorMarkup,
  editorModel,
  emptyStatement,
  policyDocumentRoute,
  policyListMarkup,
  policyRows,
  policyValidateRoute,
  readFailure,
  resultMarkup,
  resultModel,
  validateErrors,
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
  assert.match(actionsMarkup(actionsModel(entryOf(state, "jbones"))), /class="btn primary" data-act="push"/);
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
