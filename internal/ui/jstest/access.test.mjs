// The console has no build step and no package manager, so these tests run on the source
// file that the browser loads. The Go test TestTheConsoleJavaScriptTestsPass starts them.
//
// access.js holds the staged state model of the access view, the header model, the mode
// control, and the empty state. Every function here is pure, or it takes its transport as
// an argument, therefore this file asserts the result with no browser and no network.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { CONSOLE_HEADER, consoleRequestInit } from "../static/app.js";
import {
  MODE_ENFORCE,
  MODE_OBSERVE,
  OBSERVE_LOG_COMMAND,
  createAccessState,
  emptyStatement,
  headerModel,
  modeChange,
  observeStatement,
  sendModeChange,
} from "../static/access.js";

// accessBody is one answer of GET /api/access, as internal/api/types.go declares it.
function accessBody(overrides = {}) {
  return {
    mode: "enforce",
    rules: [
      { from: "jbones", to: "internet", ports: [] },
      { from: "jbones", to: "host", ports: ["tcp/22"] },
    ],
    nodes: [
      { id: "jbones", kind: "tailnet", peers: 6, veth: "10.99.0.2" },
      { id: "homelab", kind: "tailnet", peers: 3, veth: "10.99.0.6" },
      { id: "host", kind: "host" },
      { id: "internet", kind: "internet" },
    ],
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// The staged state model
// ---------------------------------------------------------------------------

test("the access view reads the mode and the rule set that GET /api/access returned", () => {
  // FR-editor-31.
  const state = createAccessState();
  state.setBase(accessBody());

  assert.equal(state.base().mode, MODE_ENFORCE);
  assert.equal(state.base().rules.length, 2);
  assert.equal(state.base().nodes.length, 4);
  assert.deepEqual(state.rules(), state.base().rules);
});

test("the staged count reads 0 when the console holds no staged edit", () => {
  // FR-editor-24.
  const state = createAccessState();
  state.setBase(accessBody());

  assert.equal(state.count(), 0);
  assert.deepEqual(state.difference(), { added: [], removed: [], changed: [] });
});

test("a staged edit sends no request to the daemon", async () => {
  // FR-editor-23. The console stages and the daemon applies, therefore the model holds
  // the edit and it opens no request. This test is the contract of the whole editor.
  const sent = [];
  const request = async (...args) => {
    sent.push(args);
    return {};
  };

  const state = createAccessState({ request });
  state.setBase(accessBody());
  state.setRules([
    ...state.rules(),
    { from: "jbones", to: "homelab", ports: [] },
  ]);

  assert.equal(state.count(), 1);
  assert.equal(sent.length, 0);
});

test("the staged count reads the number of added rules, removed rules, and changed rules", () => {
  // FR-editor-24.
  const state = createAccessState();
  state.setBase(accessBody());

  state.setRules([
    // The rule jbones to internet keeps its ports, therefore it counts for nothing.
    { from: "jbones", to: "internet", ports: [] },
    // The rule jbones to host takes a second port, therefore it counts as one change.
    { from: "jbones", to: "host", ports: ["tcp/22", "tcp/443"] },
    // One rule is new.
    { from: "homelab", to: "internet", ports: [] },
  ]);

  const difference = state.difference();
  assert.deepEqual(difference.added.map((rule) => rule.to), ["internet"]);
  assert.deepEqual(difference.changed.map((rule) => rule.to), ["host"]);
  assert.deepEqual(difference.removed, []);
  assert.equal(state.count(), 2);
});

test("the model counts a removed rule", () => {
  const state = createAccessState();
  state.setBase(accessBody());
  state.setRules([{ from: "jbones", to: "internet", ports: [] }]);

  assert.deepEqual(state.difference().removed.map((rule) => rule.to), ["host"]);
  assert.equal(state.count(), 1);
});

test("discard returns the staged rule set to the rule set of the daemon", () => {
  // FR-editor-27. Issue #152 draws the action; the model holds the operation.
  const state = createAccessState();
  state.setBase(accessBody());
  state.setRules([]);
  assert.equal(state.count(), 2);

  state.discard();
  assert.equal(state.count(), 0);
  assert.deepEqual(state.rules(), state.base().rules);
});

test("a new answer of the daemon replaces the staged rule set when no edit is staged", () => {
  // The poll runs every few seconds, so a view that keeps the first answer shows a rule
  // set that another console already replaced.
  const state = createAccessState();
  state.setBase(accessBody());
  state.setBase(accessBody({ mode: "observe", rules: [] }));

  assert.equal(state.base().mode, MODE_OBSERVE);
  assert.deepEqual(state.rules(), []);
  assert.equal(state.count(), 0);
});

test("a new answer of the daemon keeps every staged edit", () => {
  // The console applies no edit automatically, therefore a poll removes no edit of the
  // operator. Issue #152 owns the offer to rebase the edits.
  const state = createAccessState();
  state.setBase(accessBody());
  state.setRules([{ from: "homelab", to: "internet", ports: [] }]);

  state.setBase(accessBody({ rules: [{ from: "jbones", to: "internet", ports: [] }] }));

  assert.deepEqual(state.rules(), [{ from: "homelab", to: "internet", ports: [] }]);
  assert.equal(state.count(), 2);
});

// ---------------------------------------------------------------------------
// The header
// ---------------------------------------------------------------------------

test("the header states the mode as a coloured dot and a lowercase word", () => {
  // FR-editor-31 and FR-console-41.
  assert.deepEqual(headerModel(MODE_ENFORCE, 0).mode, { word: "enforce", tone: "ok" });
  assert.deepEqual(headerModel(MODE_OBSERVE, 0).mode, { word: "observe", tone: "warn" });
});

test("the header states the staged count", () => {
  // FR-editor-24.
  assert.equal(headerModel(MODE_ENFORCE, 0).staged, "0 staged");
  assert.equal(headerModel(MODE_ENFORCE, 3).staged, "3 staged");
});

test("the header marks one control with the accent colour", () => {
  // CLAUDE.md gives the accent to one thing per view, and this view gives it to the
  // affirmative action.
  for (const count of [0, 3]) {
    const accented = headerModel(MODE_ENFORCE, count).controls.filter((one) => one.accent);
    assert.equal(accented.length, 1);
    assert.equal(accented[0].id, "apply");
  }
});

test("every control of the header reaches focus by keyboard", () => {
  // A button reaches focus with no tabindex attribute, and no other element does.
  const controls = headerModel(MODE_ENFORCE, 1).controls;
  assert.ok(controls.length >= 3);
  for (const control of controls) {
    assert.equal(control.kind, "button");
    assert.ok(control.label.length > 0);
  }
  assert.deepEqual(controls.map((one) => one.id), ["mode", "discard", "apply"]);
});

test("the header offers apply and discard only while an edit is staged", () => {
  const resting = headerModel(MODE_ENFORCE, 0).controls;
  assert.equal(resting.find((one) => one.id === "apply").disabled, true);
  assert.equal(resting.find((one) => one.id === "discard").disabled, true);
  assert.equal(resting.find((one) => one.id === "mode").disabled, false);

  const staged = headerModel(MODE_ENFORCE, 1).controls;
  assert.equal(staged.find((one) => one.id === "apply").disabled, false);
  assert.equal(staged.find((one) => one.id === "discard").disabled, false);
});

// ---------------------------------------------------------------------------
// The mode
// ---------------------------------------------------------------------------

test("observe mode states that the daemon denies nothing and it names the log command", () => {
  // FR-editor-32.
  const statement = observeStatement(MODE_OBSERVE);
  assert.match(statement.sentence, /denies nothing/);
  assert.equal(statement.command, "journalctl -u hydrascale | grep hydrascale-would-deny");
  assert.equal(OBSERVE_LOG_COMMAND, statement.command);
});

test("enforce mode states no observe sentence", () => {
  assert.equal(observeStatement(MODE_ENFORCE), null);
});

test("the mode dialog states what the change does before the operator confirms it", () => {
  // FR-editor-33.
  const toObserve = modeChange(MODE_ENFORCE);
  assert.equal(toObserve.next, MODE_OBSERVE);
  assert.match(toObserve.heading, /observe/);
  assert.ok(toObserve.sentences.length > 0);
  assert.match(toObserve.sentences.join(" "), /drops no packet/);
  assert.match(toObserve.confirmLabel, /observe/);

  const toEnforce = modeChange(MODE_OBSERVE);
  assert.equal(toEnforce.next, MODE_ENFORCE);
  assert.match(toEnforce.heading, /enforce/);
  assert.match(toEnforce.sentences.join(" "), /drops the packet/);
  assert.match(toEnforce.confirmLabel, /enforce/);
});

test("the mode change sends the rule set of the daemon and not the staged rule set", async () => {
  // FR-editor-23. The mode change is the one mutating request of this view, and it must
  // not carry a rule that the operator has not applied.
  const sent = [];
  const request = async (route, method, body) => {
    sent.push({ route, method, body });
    return accessBody({ mode: "observe" });
  };

  const state = createAccessState({ request });
  state.setBase(accessBody());
  state.setRules([...state.rules(), { from: "homelab", to: "internet", ports: [] }]);

  await sendModeChange(state, MODE_OBSERVE);

  assert.equal(sent.length, 1);
  assert.equal(sent[0].route, "/api/access");
  assert.equal(sent[0].method, "PUT");
  assert.equal(sent[0].body.mode, "observe");
  assert.deepEqual(sent[0].body.rules, accessBody().rules);
  // The staged edit survives the mode change, because the daemon accepted no rule of it.
  assert.equal(state.count(), 1);
});

test("the mode change carries the console header", () => {
  // FR-console-8. Every mutating request of the console carries the header.
  const init = consoleRequestInit("PUT", { mode: "observe", rules: [] });
  assert.equal(init.headers[CONSOLE_HEADER], "1");
  assert.equal(CONSOLE_HEADER, "X-Hydrascale-Console");
});

// ---------------------------------------------------------------------------
// The empty state
// ---------------------------------------------------------------------------

test("the empty state states that nothing reaches anything and it names the matrix", () => {
  const statement = emptyStatement(accessBody({ rules: [] }));
  assert.match(statement.sentence, /nothing reaches anything/);
  assert.match(statement.sentence, /matrix/);
});

test("the empty state names the tailnet as the first step on a host that holds none", () => {
  // The console shows no invented data, so a host with no tailnet states what would fill
  // the view rather than an empty matrix.
  const statement = emptyStatement(accessBody({ rules: [], nodes: [] }));
  assert.match(statement.sentence, /No tailnet is configured/);
});

test("a rule set with one rule states no empty state", () => {
  assert.equal(emptyStatement(accessBody()), null);
});

// ---------------------------------------------------------------------------
// The drawing rules
// ---------------------------------------------------------------------------

test("no state of the access view holds the word denied", async () => {
  // Denial is the absence of a line and the absence of a row. The view writes the word
  // nowhere, therefore no later change can put it on the screen as a state.
  const source = await readFile(new URL("../static/access.js", import.meta.url), "utf8");
  assert.doesNotMatch(source, /denied/i);
});
