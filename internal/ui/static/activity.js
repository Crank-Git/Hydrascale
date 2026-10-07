// The activity view: what the daemon did, newest first.
//
// This file holds every line that needs a document. panels.js holds the model and the
// serializer, and internal/ui/jstest asserts those under Node.
//
// The view reads the snapshot that the poll layer gives it. It opens no request of its
// own and it starts no timer, therefore the console keeps one data source. See
// FR-console-15.
//
// The view holds no accent. Its two filters are selections, and the current choice of a
// filter is a reversed cell, as the current entry of the navigation is. See FR-console-40.

import { registerView } from "./app.js";
import { activityMarkup, activityModel } from "./panels.js";

/**
 * filter holds the two choices of the operator. The poll draws the whole view again on
 * every tick, so the choices live here and not in the elements.
 */
const filter = { ticks: false, tailnet: "" };

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

/** segment builds one segmented control. Each choice is a button with aria-pressed. */
function segment(label, choices, current, choose) {
  const group = element("div", "seg");
  group.setAttribute("role", "group");
  group.setAttribute("aria-label", label);
  for (const choice of choices) {
    const button = element("button", undefined, choice.label);
    button.type = "button";
    button.setAttribute("aria-pressed", choice.value === current ? "true" : "false");
    button.addEventListener("click", () => choose(choice.value));
    group.append(button);
  }
  return group;
}

/**
 * draw draws the activity view from one poll snapshot.
 *
 * snapshot.status holds the merged body of the poll: its field events holds the body of
 * GET /api/events, and its field policy holds the body of GET /api/policy. The shell
 * draws the stale marker in the poll banner, so this view draws the last known list and
 * adds no second marker.
 */
function draw(section, snapshot) {
  section.replaceChildren();

  if (snapshot.loading) {
    const frame = element("div", "frame");
    frame.append(element("p", "note", "The first poll has not returned."));
    section.append(frame);
    return;
  }

  const status = snapshot.status;
  const events = (status && status.events) || [];
  const model = activityModel(events, filter);
  if (filter.tailnet !== "" && !model.tailnets.includes(filter.tailnet)) {
    filter.tailnet = "";
  }
  const redraw = () => draw(section, snapshot);

  const frame = element("section", "frame log-frame");
  frame.setAttribute("aria-labelledby", "events-heading");
  const head = element("div", "frame-head");
  const heading = element("h2", "frame-title", "Events");
  heading.id = "events-heading";
  head.append(heading);

  const tools = element("div", "frame-tools");
  tools.append(
    segment(
      "Rows",
      [
        { label: "Notable", value: false },
        { label: "Every event", value: true },
      ],
      filter.ticks,
      (value) => {
        filter.ticks = value;
        redraw();
      },
    ),
  );
  if (model.tailnets.length > 0) {
    tools.append(
      segment(
        "Tailnet",
        [{ label: "Every tailnet", value: "" }, ...model.tailnets.map((id) => ({ label: id, value: id }))],
        filter.tailnet,
        (value) => {
          filter.tailnet = value;
          redraw();
        },
      ),
    );
  }
  head.append(tools);

  const meta = element("span", "frame-meta mono", `${model.notable} notable · ${model.scanned} in memory`);
  head.append(meta);
  frame.append(head);

  // The serializer escapes every value that the daemon reported, and a test asserts that.
  const body = element("div", "log-wrap");
  body.innerHTML = activityMarkup(model, (status && status.policy) || [], filter);
  frame.append(body);

  frame.append(
    element(
      "p",
      "note",
      "The daemon holds the newest events in memory. It records one event for every mutating request on the console listener.",
    ),
  );
  section.append(frame);
}

registerView("activity", draw);
