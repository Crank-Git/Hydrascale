// The console has no build step and no package manager, so these tests run on the
// source file that the browser loads. The Go test TestTheConsoleJavaScriptTestsPass
// starts them, which is why they live outside internal/ui/static: go:embed places every
// file of that directory in the daemon binary and a test file belongs in neither.
//
// The poll layer takes its clock and its transport as arguments, so a test drives it
// without a browser and without a network.
import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_POLL_INTERVAL_MS,
  MINIMUM_POLL_INTERVAL_MS,
  RETRY_AFTER_MS,
  STATUS_ROUTE,
  VIEWS,
  consoleRequestInit,
  createPollLayer,
} from "../static/app.js";

// clock returns a fake timer set that a test steps by hand, so no test waits.
function clock() {
  let at = 0;
  let next = 1;
  const timers = new Map();
  return {
    now: () => at,
    setTimer(fn, ms) {
      const handle = next++;
      timers.set(handle, { fn, due: at + ms });
      return handle;
    },
    clearTimer(handle) {
      timers.delete(handle);
    },
    // advance moves the clock by ms and runs every timer that comes due.
    async advance(ms) {
      const target = at + ms;
      for (;;) {
        let due = null;
        let handle = null;
        for (const [key, timer] of timers) {
          if (timer.due <= target && (due === null || timer.due < due.due)) {
            due = timer;
            handle = key;
          }
        }
        if (due === null) {
          break;
        }
        at = due.due;
        timers.delete(handle);
        due.fn();
        await Promise.resolve();
        await Promise.resolve();
      }
      at = target;
    },
    pending: () => timers.size,
  };
}

// layerWith builds a poll layer over a queue of replies. A reply is either a status body
// or an Error, which the transport rejects with.
function layerWith(replies, options = {}) {
  const time = clock();
  const calls = [];
  const layer = createPollLayer({
    now: time.now,
    setTimer: time.setTimer,
    clearTimer: time.clearTimer,
    fetchStatus: () => {
      calls.push(time.now());
      const reply = replies.length > 1 ? replies.shift() : replies[0];
      if (reply instanceof Error) {
        return Promise.reject(reply);
      }
      return Promise.resolve(reply);
    },
    ...options,
  });
  return { layer, time, calls };
}

test("the poll layer requests the status route every five seconds", async () => {
  assert.equal(STATUS_ROUTE, "/api/status");
  assert.equal(DEFAULT_POLL_INTERVAL_MS, 5000);

  const { layer, time, calls } = layerWith([{ server_version: "1.0.0" }]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(calls.length, 1, "the layer polls once at start");

  await time.advance(5000);
  assert.equal(calls.length, 2);
  await time.advance(15000);
  assert.equal(calls.length, 5);
  layer.stop();
});

test("the poll layer holds no state before the first reply", async () => {
  const { layer } = layerWith([{ server_version: "1.0.0" }]);
  const before = layer.snapshot();
  assert.equal(before.status, null);
  assert.equal(before.loading, true);
  assert.equal(before.stale, false);
  assert.equal(before.lastSuccessAt, null);
  layer.stop();
});

test("a failed poll keeps the last state and marks it stale", async () => {
  const good = { server_version: "1.0.0", desired: { jbones: {} } };
  const { layer, time } = layerWith([good, new Error("connection refused")]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();

  const fresh = layer.snapshot();
  assert.deepEqual(fresh.status, good);
  assert.equal(fresh.stale, false);
  assert.equal(fresh.lastSuccessAt, 0);

  await time.advance(5000);
  const stale = layer.snapshot();
  assert.deepEqual(stale.status, good, "the layer keeps the last state");
  assert.equal(stale.stale, true);
  assert.equal(stale.lastSuccessAt, 0, "the layer holds the time of the last success");
  assert.match(stale.error, /connection refused/);
  layer.stop();
});

test("a poll that fails for sixty seconds offers a manual retry", async () => {
  const { layer, time } = layerWith([
    { server_version: "1.0.0" },
    new Error("connection refused"),
  ]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();

  await time.advance(30000);
  assert.equal(layer.snapshot().retryOffered, false, "thirty seconds is not enough");

  await time.advance(35000);
  const failing = layer.snapshot();
  assert.equal(failing.retryOffered, true);
  assert.equal(failing.stale, true);
  layer.stop();
});

test("a poll that succeeds again clears the stale marker", async () => {
  const replies = [
    { server_version: "1.0.0" },
    new Error("connection refused"),
    { server_version: "1.0.0", desired: {} },
  ];
  const { layer, time } = layerWith(replies);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  await time.advance(5000);
  assert.equal(layer.snapshot().stale, true);

  await time.advance(5000);
  const recovered = layer.snapshot();
  assert.equal(recovered.stale, false);
  assert.equal(recovered.retryOffered, false);
  assert.equal(recovered.error, null);
  assert.equal(recovered.lastSuccessAt, 10000);
  layer.stop();
});

test("a daemon that restarts with another version raises the reload notice", async () => {
  const replies = [{ server_version: "1.0.0" }, { server_version: "1.0.1" }];
  const { layer, time } = layerWith(replies);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(layer.snapshot().restarted, false);

  await time.advance(5000);
  assert.equal(
    layer.snapshot().restarted,
    true,
    "the embedded console came from the old binary, so the operator reloads",
  );
  layer.stop();
});

test("a manual retry polls at once and does not keep the old timer", async () => {
  const { layer, time, calls } = layerWith([{ server_version: "1.0.0" }]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(calls.length, 1);

  await time.advance(1000);
  layer.refresh();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(calls.length, 2, "the retry polls at once");

  await time.advance(5000);
  assert.equal(calls.length, 3, "one timer runs, not two");
  layer.stop();
});

test("the poll interval is a setting and it refuses a value below one second", async () => {
  const { layer, time, calls } = layerWith([{ server_version: "1.0.0" }]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();

  layer.setInterval(20000);
  assert.equal(layer.getInterval(), 20000);
  await time.advance(5000);
  assert.equal(calls.length, 1, "the old cadence stops");
  await time.advance(15000);
  assert.equal(calls.length, 2);

  assert.equal(MINIMUM_POLL_INTERVAL_MS, 1000);
  assert.throws(() => layer.setInterval(10), /interval/);
  assert.throws(() => layer.setInterval("5s"), /interval/);
  assert.equal(layer.getInterval(), 20000, "a refused value changes nothing");
  layer.stop();
});

test("a subscriber reads every state change and unsubscribes", async () => {
  const { layer, time } = layerWith([{ server_version: "1.0.0" }]);
  const seen = [];
  const unsubscribe = layer.subscribe((snapshot) => seen.push(snapshot));
  assert.equal(seen.length, 1, "a subscriber reads the current state at once");

  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(seen.length, 2);

  unsubscribe();
  await time.advance(5000);
  assert.equal(seen.length, 2, "an unsubscribed reader gets nothing");
  layer.stop();
});

test("a stopped layer schedules no further poll", async () => {
  const { layer, time, calls } = layerWith([{ server_version: "1.0.0" }]);
  layer.start();
  await Promise.resolve();
  await Promise.resolve();
  layer.stop();
  await time.advance(60000);
  assert.equal(calls.length, 1);
  assert.equal(time.pending(), 0);
});

test("a mutating request carries the console header and a read does not", () => {
  const read = consoleRequestInit("GET");
  assert.equal(read.headers["X-Hydrascale-Console"], undefined);

  for (const method of ["POST", "PUT", "PATCH", "DELETE"]) {
    const write = consoleRequestInit(method, { id: "jbones" });
    assert.equal(write.method, method);
    assert.equal(write.headers["X-Hydrascale-Console"], "1");
    assert.equal(write.headers["Content-Type"], "application/json");
    assert.equal(write.body, '{"id":"jbones"}');
  }
});

test("the shell names five views and each one holds a heading and an empty state", () => {
  assert.deepEqual(
    VIEWS.map((view) => view.id),
    ["overview", "namespaces", "access", "activity", "settings"],
  );
  for (const view of VIEWS) {
    assert.equal(typeof view.heading, "string");
    assert.ok(view.heading.length > 0, `${view.id} holds no heading`);
    assert.ok(
      view.empty.endsWith("."),
      `${view.id} states no empty state as a sentence`,
    );
  }
});
