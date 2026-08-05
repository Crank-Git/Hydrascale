// The settings view: the resolver, the host file, and the console.
//
// This file holds every line that needs a document. panels.js holds the model and the
// serializer, and internal/ui/jstest asserts those under Node.
//
// The navigation holds five entries, which FR-console-12 states, and
// mockups/05-dns-and-settings.html draws the DNS regions and the settings regions on one
// page. This view therefore holds the four regions that the mockup names: the resolver,
// the namespace protection, the host file, and the settings.
//
// The view reads the snapshot that the poll layer gives it. It opens no request of its
// own and it starts no timer, therefore the console keeps one data source. See
// FR-console-15.

import { STATUS_ROUTE, applyPollInterval, registerView } from "./app.js";
import { dnsMarkup, dnsModel, settingsMarkup, settingsModel } from "./panels.js";

/** The intervals that the poll interval control offers, in seconds. */
const INTERVALS = [1, 2, 5, 10, 30, 60];

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

/**
 * intervalCard builds the one control that this view owns.
 *
 * A control carries a handler, so this card is the one region that the serializer does
 * not write. The value it holds is the poll interval that the settings list states above.
 */
function intervalCard(snapshot) {
  const card = element("section", "card");
  card.append(element("h2", undefined, "Poll interval"));

  const note = element("p", "note", "The console reads ");
  note.append(element("code", "mono", `GET ${STATUS_ROUTE}`));
  note.append(
    document.createTextNode(
      " on this interval. A change applies at once and it holds for the next session.",
    ),
  );
  card.append(note);

  const row = element("div", "row");
  const select = element("select", "field");
  select.id = "poll-interval";
  select.setAttribute("aria-label", "Poll interval");
  for (const seconds of INTERVALS) {
    const option = document.createElement("option");
    option.value = String(seconds * 1000);
    option.textContent = `${seconds}s`;
    select.append(option);
  }
  select.value = String(snapshot.interval);
  select.addEventListener("change", () => applyPollInterval(Number(select.value)));
  row.append(select);
  card.append(row);
  return card;
}

/**
 * draw draws the settings view from one poll snapshot.
 *
 * snapshot.status holds the merged body of the poll: the fields of GET /api/status and
 * the field dns from GET /api/dns.
 */
function draw(section, snapshot) {
  section.replaceChildren();

  if (snapshot.loading) {
    const card = element("div", "card");
    card.append(element("span", "label", "Loading"));
    card.append(element("p", "note", "The first poll has not returned."));
    section.append(card);
    return;
  }

  const status = snapshot.status;
  const dns = dnsModel(status && status.dns);
  const settings = settingsModel(status, snapshot.interval, window.location.host);

  // The serializer escapes every value that the daemon reported, and a test asserts that.
  const written = element("div", "panels");
  written.innerHTML = dnsMarkup(dns) + settingsMarkup(settings);
  section.append(written);
  section.append(intervalCard(snapshot));
}

registerView("settings", draw);
