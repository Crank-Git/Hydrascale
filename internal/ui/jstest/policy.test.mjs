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
import test from "node:test";

import {
  EVERY_DEVICE_STATEMENT,
  POLICY_ROUTE,
  createPolicyState,
  editorMarkup,
  editorModel,
  emptyStatement,
  policyDocumentRoute,
  policyListMarkup,
  policyRows,
  readFailure,
} from "../static/policy.js";

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
  const message = 'the Headscale control server runs the file policy mode, and a policy write needs policy.mode: "db"';
  const state = createPolicyState({ request: async () => documentBody() });
  state.setList(listBody());
  state.setError("homelab", message);

  const model = entryOf(state, "homelab");
  assert.equal(model.state, "read-only");
  assert.equal(model.readOnly, true);
  assert.equal(readFailure(message).kind, "file-mode");

  const markup = editorMarkup(model);
  assert.match(markup, /policy\.mode: &quot;db&quot;/);
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
