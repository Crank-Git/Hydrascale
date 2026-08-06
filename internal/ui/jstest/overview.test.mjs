// The activity list of the overview view. Node holds no layout engine, therefore these
// tests read the rules of app.css and the writer of overview.js rather than the pixels.
//
// The Go test TestTheConsoleJavaScriptTestsPass starts this file.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// ruleBody returns the declarations of one CSS rule of app.css.
async function ruleBody(selector) {
  const style = await readFile(new URL("../static/app.css", import.meta.url), "utf8");
  const pattern = new RegExp(`${selector.replace(/[.\s]/g, (c) => (c === "." ? "\\." : "\\s"))}\\{([^}]*)\\}`);
  const rule = style.match(pattern);
  assert.ok(rule, `app.css holds no rule ${selector}`);
  return rule[1];
}

test("the event type of an activity row never occupies the pixels of the message", async () => {
  // Issue #226. A fixed width draws a type wider than the box outside the box, and the
  // message beside it starts at the same place. A minimum width lets the column grow, and
  // the message takes the width that remains.
  const kind = await ruleBody(".ev-kind");
  assert.match(kind, /min-width:\s*\d+px/);
  assert.doesNotMatch(kind, /(^|;)\s*width:/);

  const message = await ruleBody(".ev p");
  assert.match(message, /min-width:\s*0/);
});

test("a row whose type is access.jump_displaced renders the whole type", async () => {
  // Issue #226. The type is the machine value that the operator scans, and a truncated
  // type reads as a different event, therefore the column truncates no type.
  const kind = await ruleBody(".ev-kind");
  assert.doesNotMatch(kind, /text-overflow/);
  assert.doesNotMatch(kind, /overflow:\s*hidden/);

  const source = await readFile(new URL("../static/overview.js", import.meta.url), "utf8");
  assert.match(source, /element\("span", "ev-kind mono", event\.Type\)/);
});

test("the row of the recent activity panel puts its message on a line of its own", async () => {
  // Issue #236. The panel is 320 pixels wide. A time of 58 pixels and a type of 148
  // pixels leave the message about 60 pixels, and the message then wraps to one word for
  // each line. The row therefore wraps: the time and the type take the first line, and
  // the message takes the whole width of the card below them.
  const source = await readFile(new URL("../static/overview.js", import.meta.url), "utf8");
  assert.match(source, /element\("div", "ev ev-stack"\)/);

  const stack = await ruleBody(".ev-stack");
  assert.match(stack, /flex-wrap:\s*wrap/);

  const message = await ruleBody(".ev-stack p");
  assert.match(message, /flex:\s*1 0 100%/);
});

test("the activity view keeps the one-line row, because it is wider than the panel", async () => {
  // Issue #236. The two-line row belongs to the 320 pixel panel of the overview. The
  // activity view holds the whole width of the page, therefore its row stays on one line
  // and the writer of the view adds no ev-stack class.
  const source = await readFile(new URL("../static/panels.js", import.meta.url), "utf8");
  assert.match(source, /'<div class="ev">'/);
  assert.doesNotMatch(source, /ev-stack/);

  const row = await ruleBody(".ev");
  assert.doesNotMatch(row, /flex-wrap/);
});
