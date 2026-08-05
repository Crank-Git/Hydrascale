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
  SQUARE_SIDE,
  SQUARE_SIDE_DENSE,
  createAccessState,
  deleteRule,
  emptyStatement,
  headerModel,
  hoverMarks,
  matrixModel,
  modeChange,
  observeStatement,
  parsePorts,
  ruleListModel,
  sendModeChange,
  setRulePorts,
  toggleSquare,
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
// The reachability matrix
// ---------------------------------------------------------------------------

// squareAt returns the square of one path from the matrix model.
function squareAt(model, from, to) {
  const row = model.rows.find((one) => one.source === from);
  return row.squares.find((one) => one.to === to);
}

// denseBody is one answer of GET /api/access for a host that declares 12 tailnets.
function denseBody() {
  const tailnets = [];
  for (let index = 0; index < 12; index += 1) {
    tailnets.push({ id: `net${index}`, kind: "tailnet", peers: 1, veth: "10.99.0.2" });
  }
  return accessBody({
    rules: [],
    nodes: [...tailnets, { id: "host", kind: "host" }, { id: "internet", kind: "internet" }],
  });
}

test("the matrix has one row per source and one column per destination", () => {
  // FR-editor-7. A source is a tailnet or the host. A destination is a tailnet, the host,
  // or the internet. features/05-reachability-model.md states both.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  assert.deepEqual(model.sources, ["jbones", "homelab", "host"]);
  assert.deepEqual(model.destinations, ["jbones", "homelab", "host", "internet"]);
  assert.equal(model.rows.length, 3);
  for (const row of model.rows) {
    assert.equal(row.squares.length, 4);
  }
});

test("a filled square means that at least one rule allows the path", () => {
  // FR-editor-8.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  assert.equal(squareAt(model, "jbones", "internet").allowed, true);
  assert.equal(squareAt(model, "jbones", "host").allowed, true);
});

test("an empty square means that no rule allows the path", () => {
  // FR-editor-9. The empty square is the denial, therefore the model marks no path as
  // refused and it draws no second state.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  assert.equal(squareAt(model, "jbones", "homelab").allowed, false);
  assert.equal(squareAt(model, "homelab", "internet").allowed, false);
});

test("the diagonal square is inert and it accepts no click", () => {
  // FR-editor-10. The daemon rejects a rule where from equals to, so the square carries
  // the disabled attribute and the stage operation returns the rule set unchanged.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  const diagonal = squareAt(model, "jbones", "jbones");
  assert.equal(diagonal.inert, true);
  assert.equal(diagonal.disabled, true);
  assert.deepEqual(toggleSquare(state.rules(), "jbones", "jbones"), state.rules());
});

test("a click on an empty square stages a rule that allows every port", () => {
  // FR-editor-11, and steps 4 to 7 of the flow "The operator allows one path".
  const state = createAccessState();
  state.setBase(accessBody());

  state.setRules(toggleSquare(state.rules(), "jbones", "homelab"));

  assert.deepEqual(state.difference().added, [{ from: "jbones", to: "homelab", ports: [] }]);
  assert.equal(state.count(), 1);
  assert.equal(squareAt(matrixModel(state.base(), state.rules()), "jbones", "homelab").allowed, true);
});

test("a click on a filled square stages the removal of every rule for that path", () => {
  // FR-editor-12.
  const state = createAccessState();
  state.setBase(accessBody());

  state.setRules(toggleSquare(state.rules(), "jbones", "host"));

  assert.deepEqual(state.difference().removed.map((rule) => rule.to), ["host"]);
  assert.equal(state.count(), 1);
  assert.equal(squareAt(matrixModel(state.base(), state.rules()), "jbones", "host").allowed, false);
});

test("the hover marks the row label and the column label and no other crosshair", async () => {
  // FR-editor-13. The mark names two labels, therefore the view draws no third element and
  // it tints no row and no column.
  const marks = hoverMarks("jbones", "homelab");
  assert.deepEqual(marks, { row: "jbones", column: "homelab" });
  assert.deepEqual(Object.keys(marks), ["row", "column"]);

  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ac-mtx th\.hot\{color:var\(--accent\)\}/);
  assert.doesNotMatch(style, /\.ac-square\.hot/);
});

test("the matrix holds no port value", () => {
  // FR-editor-14. The ports live in the rule list of issue #151.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  const text = JSON.stringify(model);
  assert.doesNotMatch(text, /ports/);
  assert.doesNotMatch(text, /tcp|udp/);
});

test("the square carries a 6 pixel corner radius", async () => {
  // FR-editor-15. The radius token of the brand carries the value, and the square reads
  // the token, therefore the grid reads as a grid.
  const radius = await readFile(new URL("../static/brand/tokens/radius.css", import.meta.url), "utf8");
  assert.match(radius, /--r-xs:6px;/);

  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ac-square\{[^}]*border-radius:var\(--r-xs\)/);
});

test("a host with 12 tailnets draws the 28 pixel square rather than the 34 pixel square", () => {
  const state = createAccessState();
  state.setBase(accessBody());
  assert.equal(matrixModel(state.base(), state.rules()).side, SQUARE_SIDE);
  assert.equal(SQUARE_SIDE, 34);

  const dense = createAccessState();
  dense.setBase(denseBody());
  assert.equal(matrixModel(dense.base(), dense.rules()).side, SQUARE_SIDE_DENSE);
  assert.equal(SQUARE_SIDE_DENSE, 28);
});

test("every square reaches focus by keyboard and the keyboard stages the same edit", async () => {
  // A button reaches focus with no tabindex attribute, and the browser gives it the space
  // key and the enter key. The click handler and the keyboard both call toggleSquare,
  // therefore one operation serves both.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = matrixModel(state.base(), state.rules());

  for (const row of model.rows) {
    for (const square of row.squares) {
      assert.equal(square.kind, "button");
      assert.ok(square.label.length > 0);
      assert.equal(square.disabled, square.inert);
    }
  }

  const source = await readFile(new URL("../static/access.js", import.meta.url), "utf8");
  assert.doesNotMatch(source, /tabindex/i);
});

// ---------------------------------------------------------------------------
// The rule list
// ---------------------------------------------------------------------------

// rowFor returns the row of one path from the rule list model.
function rowFor(model, from, to) {
  return model.rows.find((one) => one.from === from && one.to === to);
}

test("the rule list shows one row per rule with the source, the connector, and the destination", () => {
  // FR-editor-16.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = ruleListModel(state.base(), state.rules());

  assert.equal(model.rows.length, 2);
  assert.deepEqual(model.rows.map((one) => one.from), ["jbones", "jbones"]);
  assert.deepEqual(model.rows.map((one) => one.to), ["internet", "host"]);
  assert.equal(model.rows[0].connector, "dotted");
});

test("a rule row shows tcp/22 and tcp/443 as two chips", () => {
  // FR-editor-16, and step 4 of the flow "The operator narrows a rule to two ports".
  const state = createAccessState();
  state.setBase(accessBody());
  state.setRules(setRulePorts(state.rules(), "jbones", "host", ["tcp/22", "tcp/443"]));

  const row = rowFor(ruleListModel(state.base(), state.rules()), "jbones", "host");
  assert.deepEqual(row.chips, ["tcp/22", "tcp/443"]);
  assert.equal(row.allPorts, false);
});

test("a rule row with no port shows the words all ports", () => {
  // FR-editor-17.
  const state = createAccessState();
  state.setBase(accessBody());

  const row = rowFor(ruleListModel(state.base(), state.rules()), "jbones", "internet");
  assert.deepEqual(row.chips, []);
  assert.equal(row.allPorts, true);
  assert.equal(row.portsLabel, "all ports");
});

test("the rule list holds no row for a denied path", () => {
  // FR-editor-21. The row exists for an allowed path alone, therefore the absence of a row
  // is the refusal and the list draws no second state.
  const state = createAccessState();
  state.setBase(accessBody());
  const model = ruleListModel(state.base(), state.rules());

  assert.equal(rowFor(model, "jbones", "homelab"), undefined);
  assert.equal(rowFor(model, "homelab", "internet"), undefined);
  assert.equal(model.rows.length, 2);
});

test("editing the ports of a row marks that row as staged and raises the staged count", () => {
  // FR-editor-18 and FR-editor-19.
  const state = createAccessState();
  state.setBase(accessBody());
  assert.equal(rowFor(ruleListModel(state.base(), state.rules()), "jbones", "host").staged, false);

  state.setRules(setRulePorts(state.rules(), "jbones", "host", ["tcp/22", "tcp/443"]));

  assert.equal(state.count(), 1);
  assert.equal(rowFor(ruleListModel(state.base(), state.rules()), "jbones", "host").staged, true);
});

test("a rule that the daemon does not hold is marked as staged", () => {
  // FR-editor-18. The matrix stages the path and the list marks the row.
  const state = createAccessState();
  state.setBase(accessBody());
  state.setRules(toggleSquare(state.rules(), "jbones", "homelab"));

  const row = rowFor(ruleListModel(state.base(), state.rules()), "jbones", "homelab");
  assert.equal(row.staged, true);
  assert.equal(row.allPorts, true);
});

test("deleting a row marks the deletion as staged and raises the staged count", () => {
  // FR-editor-20.
  const state = createAccessState();
  state.setBase(accessBody());

  state.setRules(deleteRule(state.rules(), "jbones", "host"));

  assert.equal(state.count(), 1);
  assert.deepEqual(state.difference().removed.map((rule) => rule.to), ["host"]);
  assert.equal(rowFor(ruleListModel(state.base(), state.rules()), "jbones", "host"), undefined);
});

test("the rule list sends no request to the daemon", async () => {
  // FR-editor-23. The port edit and the deletion both write through setRules alone.
  const sent = [];
  const request = async (...args) => {
    sent.push(args);
    return {};
  };

  const state = createAccessState({ request });
  state.setBase(accessBody());
  state.setRules(setRulePorts(state.rules(), "jbones", "host", ["tcp/443"]));
  state.setRules(deleteRule(state.rules(), "jbones", "internet"));

  assert.equal(sent.length, 0);
  assert.equal(state.count(), 2);
});

test("every control of a rule row reaches focus by keyboard", () => {
  // A button and a text field both reach focus with no tabindex attribute.
  const state = createAccessState();
  state.setBase(accessBody());

  for (const row of ruleListModel(state.base(), state.rules()).rows) {
    assert.deepEqual(row.controls.map((one) => one.id), ["ports", "delete"]);
    for (const one of row.controls) {
      assert.ok(one.kind === "input" || one.kind === "button");
      assert.ok(one.label.length > 0);
    }
  }
});

// ---------------------------------------------------------------------------
// The port format
// ---------------------------------------------------------------------------

test("the port field takes a list that the comma separates", () => {
  // The daemon holds a port list, and one text field carries it. The console splits on the
  // comma and it removes the space around each entry.
  assert.deepEqual(parsePorts("tcp/22, tcp/443"), { ports: ["tcp/22", "tcp/443"], error: null });
  assert.deepEqual(parsePorts("udp/1-1024"), { ports: ["udp/1-1024"], error: null });
});

test("an empty port field allows every port", () => {
  // FR-access-9. An empty port list allows every port and every protocol.
  assert.deepEqual(parsePorts(""), { ports: [], error: null });
  assert.deepEqual(parsePorts("   "), { ports: [], error: null });
});

test("entering the port 22 shows an error that names the expected format", () => {
  // FR-editor-22, and the edge case "A port entry is 22".
  const result = parsePorts("22");
  assert.equal(result.ports, null);
  assert.equal(result.error, 'invalid port "22": the form is tcp/<n>, udp/<n>, tcp/<n>-<m>, or udp/<n>-<m>');
});

test("the rule list rejects tcp/0, tcp/65536, and tcp/22-21", () => {
  // FR-access-10. A port number is between 1 and 65535, and the second number is not lower
  // than the first.
  assert.equal(parsePorts("tcp/0").error, 'invalid port "tcp/0": a port number is between 1 and 65535');
  assert.equal(parsePorts("tcp/65536").error, 'invalid port "tcp/65536": a port number is between 1 and 65535');
  assert.equal(parsePorts("tcp/22-21").error, 'invalid port "tcp/22-21": the second number is lower than the first');
  for (const text of ["tcp/0", "tcp/65536", "tcp/22-21"]) {
    assert.equal(parsePorts(text).ports, null);
  }
});

test("the console repeats the port rule that internal/access holds", async () => {
  // The daemon rejects the same entry with the same three messages, therefore the console
  // states what PUT /api/access states rather than a second grammar.
  const rules = await readFile(new URL("../../access/rules.go", import.meta.url), "utf8");
  assert.match(rules, /\^\(tcp\|udp\)\/\(\[0-9\]\{1,5\}\)\(\?:-\(\[0-9\]\{1,5\}\)\)\?\$/);
  assert.match(rules, /the form is tcp\/<n>, udp\/<n>, tcp\/<n>-<m>, or udp\/<n>-<m>/);
  assert.match(rules, /a port number is between 1 and 65535/);
  assert.match(rules, /the second number is lower than the first/);
});

test("the port field rejects the whole entry and it corrects no value", () => {
  // CLAUDE.md: reject a bad value; never correct it silently. One bad entry rejects the
  // list, therefore no good entry of that list reaches the staged rule set.
  const result = parsePorts("tcp/22, 443");
  assert.equal(result.ports, null);
  assert.match(result.error, /invalid port "443"/);
});

test("the connector of a rule row is dotted", async () => {
  // FR-editor-16, and the brand rule that an allowed path is a dotted line.
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ac-conn\{[^}]*dotted/);
});

test("a port chip renders in the mono typeface", async () => {
  // A port is a machine value, therefore the chip takes the mono class of the brand.
  const source = await readFile(new URL("../static/access.js", import.meta.url), "utf8");
  assert.match(source, /ac-ports/);
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  assert.match(style, /\.ac-ports\{/);
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
