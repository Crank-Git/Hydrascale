// The activity view: what the daemon did, newest first.
//
// This file holds every line that needs a document. panels.js holds the model and the
// serializer, and internal/ui/jstest asserts those under Node.
//
// The view reads the snapshot that the poll layer gives it. It opens no request of its
// own and it starts no timer, therefore the console keeps one data source. See
// FR-console-15.
//
// The view holds no accent, because it holds no affirmative action and no selection. See
// FR-console-40.

import { registerView } from "./app.js";
import { activityMarkup, activityRows } from "./panels.js";

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
    const card = document.createElement("div");
    card.className = "card";
    const label = document.createElement("span");
    label.className = "label";
    label.textContent = "Loading";
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = "The first poll has not returned.";
    card.append(label, note);
    section.append(card);
    return;
  }

  const status = snapshot.status;
  const events = (status && status.events) || [];
  // The serializer escapes every value that the daemon reported, and a test asserts that.
  section.innerHTML = activityMarkup(activityRows(events), (status && status.policy) || []);
}

registerView("activity", draw);
