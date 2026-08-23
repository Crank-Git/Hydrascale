#!/usr/bin/env python3
"""Render the spec package into a multi-page browsable site.

    python3 docs/specs/render-spec.py

The markdown package is the spec; this script is the website generator. It reads
spec.md, every feature file its front-matter lists, and every supplement page in
an optional `pages:` front-matter list, and writes:

    spec.html               — the index: full spec.md content + site navigation
    html/page-<id>.html     — one page per `pages:` entry
    html/feature-<id>.html  — one page per feature file

Stale pages in html/ whose feature or page no longer exists are deleted on the
next render.

Properties the output keeps:

- Self-contained as a folder. Inline CSS; no external requests. Mermaid renders
  through assets/mermaid.min.js referenced relatively (the planner vendors it
  once); without it each diagram degrades to its readable source. The docs/specs
  folder travels as a unit — spec.html alone is no longer enough.
- Complete. Every section of every file appears in full on its page — never a
  summary that links out to the markdown. Relative links between the package's
  .md files rewrite to the generated pages; other relative links (mockups,
  assets, files outside the package) are re-based so they still resolve from
  html/. Each feature's mockups embed both ways: an <iframe> and an
  "Open in new tab" link (local iframes can silently render blank).
- Honest about freshness. On success the script stamps `html_generated` in
  spec.md's front-matter.
- Reviewable in place. Every generated page carries a lightweight review layer:
  hover any heading to attach a comment, comments persist in the browser's
  localStorage (keyed by the spec's slug, so two specs on one machine never
  share a store), and the floating Review panel exports the full comment set as
  review-comments.md (and imports one back). Save that export next to spec.md
  as `review-comments.md` and the next render embeds every comment under its
  section, so the reviewer and the reviser read the same file. The comment file
  format is:

      ## [<page-id>] <section-slug>
      status: open | resolved
      date: YYYY-MM-DD
      <comment body until the next ## block>

Standard library only — the copied script must run in any project. (Libraries
like python-markdown or MkDocs would do the conversion, but a pip dependency
would break the copy-anywhere contract, so the small converter stays.)
"""

import datetime
import html
import json
import os
import posixpath
import re
import sys

SPEC_DIR = os.path.dirname(os.path.abspath(__file__))
HTML_DIR = os.path.join(SPEC_DIR, "html")
MERMAID_ASSET = os.path.join(SPEC_DIR, "assets", "mermaid.min.js")
REVIEW_FILE = os.path.join(SPEC_DIR, "review-comments.md")
CALLOUT_HEADINGS = {"assumptions", "risks & open questions", "open questions"}


def slugify(text):
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def load_review_comments():
    """Parse review-comments.md into a list of comment dicts. Tolerant: a `##`
    heading that does not match the `## [page] section` shape becomes a
    page-level comment on the index rather than being dropped — losing a
    reviewer's comment is the one failure this loop must not have."""
    if not os.path.exists(REVIEW_FILE):
        return []
    comments, current = [], None
    for line in open(REVIEW_FILE, encoding="utf-8").read().splitlines():
        head = re.match(r"##\s*\[([^\]]+)\]\s*(\S*)\s*$", line)
        if head:
            if current:
                comments.append(current)
            current = {
                "page": head.group(1).strip(),
                "section": head.group(2).strip(),
                "status": "open",
                "date": "",
                "body": [],
            }
            continue
        if line.startswith("## "):
            if current:
                comments.append(current)
            current = {
                "page": "index",
                "section": "",
                "status": "open",
                "date": "",
                "body": [line[3:].strip()],
            }
            continue
        if current is None:
            continue
        meta = re.match(r"(status|date):\s*(.*)$", line.strip())
        if meta and not current["body"]:
            current[meta.group(1)] = meta.group(2).strip()
            continue
        current["body"].append(line)
    if current:
        comments.append(current)
    for c in comments:
        c["body"] = "\n".join(c["body"]).strip()
    return [c for c in comments if c["body"]]


# --- front-matter -----------------------------------------------------------


def split_front_matter(text):
    """Return (dict, body). Minimal parser: `key: value` lines and one level of
    `- key: value` list items under a `key:` line. No yaml dependency."""
    if not text.startswith("---"):
        return {}, text
    end = text.find("\n---", 3)
    if end == -1:
        return {}, text
    raw, body = text[3:end], text[end + 4 :]
    meta, current_list = {}, None
    for line in raw.splitlines():
        stripped = line.split("#", 1)[0].rstrip()
        if not stripped.strip():
            continue
        item = re.match(r"\s+-\s*(.*)$", stripped)
        if item and current_list is not None:
            entry = item.group(1)
            kv = re.match(r"(\w[\w-]*):\s*(.*)$", entry)
            meta[current_list].append({kv.group(1): kv.group(2)} if kv else entry)
            continue
        cont = re.match(r"\s+(\w[\w-]*):\s*(.*)$", stripped)
        if cont and current_list is not None and meta[current_list]:
            last = meta[current_list][-1]
            if isinstance(last, dict):
                last[cont.group(1)] = cont.group(2)
            continue
        kv = re.match(r"(\w[\w-]*):\s*(.*)$", stripped)
        if kv:
            key, value = kv.group(1), kv.group(2)
            if value in ("", "[]"):
                meta[key] = []
                # only a bare `key:` opens a block list; `[]` is a closed empty list
                current_list = key if value == "" else None
            else:
                meta[key] = value.strip("\"'")
                current_list = None
    return meta, body


# --- markdown ---------------------------------------------------------------


def inline(text):
    text = html.escape(text, quote=False)
    text = re.sub(r"`([^`]+)`", r"<code>\1</code>", text)
    text = re.sub(r"\*\*([^*]+)\*\*", r"<strong>\1</strong>", text)
    text = re.sub(r"(?<!\*)\*([^*\s][^*]*)\*(?!\*)", r"<em>\1</em>", text)
    text = re.sub(
        r"\[([^\]]+)\]\(([^)\s]+)\)",
        lambda m: '<a href="%s">%s</a>' % (html.escape(m.group(2), quote=True), m.group(1)),
        text,
    )
    return text


def split_cells(row):
    """Split a table row on `|`, ignoring pipes inside backtick code spans —
    `pending | running` in a cell is one cell, not two."""
    cells, current, in_code = [], [], False
    for ch in row.strip().strip("|"):
        if ch == "`":
            in_code = not in_code
        if ch == "|" and not in_code:
            cells.append("".join(current).strip())
            current = []
        else:
            current.append(ch)
    cells.append("".join(current).strip())
    return cells


def md_to_html(body, demote=0, slug_prefix=""):
    """Convert the supported markdown subset. `demote` shifts heading levels so
    file content nests under the page's own hierarchy."""
    out, lines, i = [], body.splitlines(), 0
    list_stack = []  # open list tags, innermost last
    li_open = []  # parallel: is an <li> awaiting its </li> at that level

    def close_lists(to_depth=0):
        while len(list_stack) > to_depth:
            if li_open[-1]:
                out.append("</li>")
            out.append("</%s>" % list_stack.pop())
            li_open.pop()

    while i < len(lines):
        line = lines[i]
        fence = re.match(r"```(\w*)\s*$", line)
        if fence:
            close_lists()
            lang, block = fence.group(1), []
            i += 1
            while i < len(lines) and not lines[i].startswith("```"):
                block.append(lines[i])
                i += 1
            i += 1
            code = "\n".join(block)
            if lang == "mermaid":
                out.append(
                    '<div class="diagram"><pre class="mermaid">%s</pre></div>'
                    % html.escape(code, quote=False)
                )
            else:
                out.append(
                    '<pre class="code"><code>%s</code></pre>'
                    % html.escape(code, quote=False)
                )
            continue
        heading = re.match(r"(#{1,6})\s+(.*)$", line)
        if heading:
            close_lists()
            level = min(6, len(heading.group(1)) + demote)
            text = heading.group(2).strip()
            slug = slug_prefix + slugify(text)
            css = ' class="callout-h"' if text.lower() in CALLOUT_HEADINGS else ""
            out.append('<h%d id="%s"%s>%s</h%d>' % (level, slug, css, inline(text), level))
            i += 1
            continue
        if re.match(r"\s*([-*]|\d+\.)\s+", line):
            while i < len(lines):
                item = re.match(r"(\s*)([-*]|\d+\.)\s+(.*)$", lines[i])
                if not item:
                    break
                # 2- and 4-space indents both mean "one level deeper": raw depth is
                # clamped to one step below the current level, so indent width never
                # fabricates an empty intermediate list.
                depth = min(len(item.group(1).expandtabs(4)) // 2 + 1, len(list_stack) + 1)
                tag = "ol" if item.group(2).rstrip(".").isdigit() else "ul"
                if depth > len(list_stack):
                    # nest inside the still-open parent <li>
                    list_stack.append(tag)
                    li_open.append(False)
                    out.append("<%s>" % tag)
                else:
                    close_lists(depth)
                    if li_open[-1]:
                        out.append("</li>")
                        li_open[-1] = False
                out.append("<li>%s" % inline(item.group(3)))
                li_open[-1] = True
                i += 1
            close_lists()
            continue
        if line.startswith("|"):
            rows = []
            while i < len(lines) and lines[i].startswith("|"):
                rows.append(split_cells(lines[i]))
                i += 1
            rows = [r for r in rows if not all(re.fullmatch(r":?-+:?", c) for c in r)]
            if rows:
                out.append('<div class="table-wrap"><table>')
                out.append(
                    "<tr>%s</tr>" % "".join("<th>%s</th>" % inline(c) for c in rows[0])
                )
                for r in rows[1:]:
                    out.append(
                        "<tr>%s</tr>" % "".join("<td>%s</td>" % inline(c) for c in r)
                    )
                out.append("</table></div>")
            continue
        if re.fullmatch(r"(-{3,}|\*{3,})", line.strip()):
            close_lists()
            out.append("<hr>")
            i += 1
            continue
        if line.startswith(">"):
            quote = []
            while i < len(lines) and lines[i].startswith(">"):
                quote.append(lines[i].lstrip("> "))
                i += 1
            out.append("<blockquote>%s</blockquote>" % inline(" ".join(quote)))
            continue
        if not line.strip():
            close_lists()
            i += 1
            continue
        para = []
        while i < len(lines) and lines[i].strip() and not re.match(
            r"(#{1,6}\s|```|\||>|\s*([-*]|\d+\.)\s)", lines[i]
        ):
            para.append(lines[i].strip())
            i += 1
        out.append("<p>%s</p>" % inline(" ".join(para)))
    close_lists()
    return "\n".join(out)


# --- site -------------------------------------------------------------------

CSS = """
:root { --bg:#ffffff; --fg:#1a1a24; --muted:#5a5a68; --line:#e3e3ea; --accent:#3452c8;
        --card:#f6f6f9; --warn-bg:#fff7e8; --warn-line:#e8b34a; }
@media (prefers-color-scheme: dark) {
  :root { --bg:#14141b; --fg:#e8e8ee; --muted:#9a9aa8; --line:#2c2c38; --accent:#8ea6ff;
          --card:#1c1c26; --warn-bg:#2a2318; --warn-line:#a87c2a; } }
* { box-sizing:border-box; }
body { margin:0; background:var(--bg); color:var(--fg);
       font:16px/1.6 system-ui,-apple-system,sans-serif; }
.layout { display:flex; min-height:100vh; }
aside.site { width:17rem; flex:none; border-right:1px solid var(--line);
             padding:1.2rem 1rem 3rem; position:sticky; top:0; height:100vh;
             overflow-y:auto; background:var(--card); }
aside.site h2 { font-size:.8rem; text-transform:uppercase; letter-spacing:.06em;
                color:var(--muted); margin:1.2rem 0 .3rem; border:none; padding:0; }
aside.site ul { list-style:none; margin:0; padding:0; }
aside.site li { margin:.15rem 0; }
aside.site a { text-decoration:none; color:var(--fg); font-size:.92rem;
               display:block; padding:.15rem .5rem; border-radius:6px; }
aside.site a:hover { background:var(--bg); }
aside.site a.current { background:var(--accent); color:#fff; }
aside.site a.sub { padding-left:1.3rem; font-size:.84rem; color:var(--muted); }
aside.site .brand { font-weight:700; font-size:1.05rem; }
main { max-width:60rem; margin:0 auto; padding:2rem 1.5rem 6rem; min-width:0; flex:1; }
h1,h2,h3,h4 { line-height:1.25; }
h1 { font-size:2rem; margin:.2rem 0; }
h2 { border-bottom:1px solid var(--line); padding-bottom:.3rem; margin-top:2.5rem; }
a { color:var(--accent); }
code { background:var(--card); padding:.1em .35em; border-radius:4px; font-size:.9em; }
pre.code { background:var(--card); border:1px solid var(--line); border-radius:8px;
           padding:1rem; overflow-x:auto; }
pre.code code { background:none; padding:0; }
.table-wrap { overflow-x:auto; }
table { border-collapse:collapse; margin:1rem 0; min-width:50%; }
th,td { border:1px solid var(--line); padding:.4rem .7rem; text-align:left; }
th { background:var(--card); }
blockquote { border-left:3px solid var(--accent); margin:1rem 0; padding:.2rem 1rem;
             color:var(--muted); }
.meta { color:var(--muted); margin-bottom:2rem; }
.badge { display:inline-block; padding:.1rem .6rem; border-radius:999px;
         border:1px solid var(--accent); color:var(--accent); font-size:.85rem; }
.diagram { background:var(--card); border:1px solid var(--line); border-radius:8px;
           padding:1rem; margin:1rem 0; overflow-x:auto; }
.diagram pre.mermaid { margin:0; }
.diagram.raw::before { content:"diagram source (assets/mermaid.min.js not vendored)";
                       display:block; color:var(--muted); font-size:.8rem;
                       margin-bottom:.5rem; }
.callout-h, .callout-h ~ p, .callout-h ~ ul { background:var(--warn-bg); }
.callout-h { border-left:4px solid var(--warn-line); padding:.3rem .8rem; }
.mockup { margin:1.5rem 0; }
.mockup iframe { width:100%; height:32rem; border:1px solid var(--line);
                 border-radius:8px; background:#fff; }
nav.pager { display:flex; justify-content:space-between; margin-top:3rem;
            border-top:1px solid var(--line); padding-top:1rem; }
@media (max-width: 900px) {
  .layout { display:block; }
  aside.site { width:auto; height:auto; position:static; border-right:none;
               border-bottom:1px solid var(--line); }
}
h1[id]:hover .rv-add, h2[id]:hover .rv-add, h3[id]:hover .rv-add,
h4[id]:hover .rv-add { opacity:1; }
.rv-add { opacity:0; margin-left:.5rem; font-size:.75rem; cursor:pointer;
          border:1px solid var(--accent); color:var(--accent); background:none;
          border-radius:6px; padding:0 .45rem; vertical-align:middle;
          transition:opacity .15s; }
.rv-comment { border:1px solid var(--warn-line); border-left-width:4px;
              background:var(--warn-bg); border-radius:8px; padding:.6rem .9rem;
              margin:.8rem 0; font-size:.92rem; }
.rv-comment.rv-resolved { opacity:.6; border-left-color:var(--line); }
.rv-comment .rv-head { display:flex; gap:.6rem; align-items:center;
                       color:var(--muted); font-size:.8rem; margin-bottom:.3rem; }
.rv-comment .rv-head button { font-size:.75rem; cursor:pointer; background:none;
                              border:1px solid var(--line); border-radius:6px;
                              color:var(--muted); padding:0 .4rem; }
.rv-comment .rv-body { white-space:pre-wrap; }
.rv-editor textarea { width:100%; min-height:5rem; background:var(--bg);
                      color:var(--fg); border:1px solid var(--line);
                      border-radius:6px; padding:.5rem; font:inherit; }
.rv-editor .rv-actions { margin-top:.4rem; display:flex; gap:.5rem; }
.rv-editor button { cursor:pointer; border-radius:6px; padding:.2rem .8rem;
                    border:1px solid var(--accent); background:var(--accent);
                    color:#fff; }
.rv-editor button.rv-cancel { background:none; color:var(--accent); }
#rv-toggle { position:fixed; right:1rem; bottom:1rem; z-index:60;
             background:var(--accent); color:#fff; border:none; cursor:pointer;
             border-radius:999px; padding:.5rem 1rem; font-size:.85rem;
             box-shadow:0 2px 10px rgba(0,0,0,.2); }
#rv-drawer { position:fixed; top:0; right:0; bottom:0; width:21rem; z-index:55;
             background:var(--card); border-left:1px solid var(--line);
             display:flex; flex-direction:column; transform:translateX(100%);
             transition:transform .2s ease; box-shadow:-4px 0 16px rgba(0,0,0,.12); }
body.rv-open #rv-drawer { transform:none; }
body.rv-open .layout { padding-right:21rem; }
#rv-drawer header { padding:.8rem 1rem .5rem; border-bottom:1px solid var(--line); }
#rv-drawer header .rv-count { color:var(--muted); font-size:.8rem; }
#rv-drawer .rv-compose { padding:.8rem 1rem; border-bottom:1px solid var(--line); }
#rv-drawer .rv-compose label { display:block; font-size:.75rem;
                               color:var(--muted); margin-bottom:.25rem; }
#rv-drawer .rv-compose select { width:100%; margin-bottom:.4rem;
             background:var(--bg); color:var(--fg); border:1px solid var(--line);
             border-radius:6px; padding:.3rem; font:inherit; font-size:.85rem; }
#rv-drawer .rv-compose textarea { width:100%; min-height:5.5rem;
             background:var(--bg); color:var(--fg); border:1px solid var(--line);
             border-radius:6px; padding:.5rem; font:inherit; font-size:.9rem; }
#rv-drawer .rv-compose .rv-live { font-size:.72rem; color:var(--muted); }
#rv-drawer .rv-list { flex:1; overflow-y:auto; padding:.6rem 1rem; }
#rv-drawer .rv-item { border:1px solid var(--line); border-left:3px solid
             var(--warn-line); border-radius:8px; padding:.45rem .6rem;
             margin-bottom:.6rem; font-size:.82rem; cursor:pointer; }
#rv-drawer .rv-item.rv-resolved { opacity:.55; border-left-color:var(--line); }
#rv-drawer .rv-item .rv-where { color:var(--accent); font-size:.72rem; }
#rv-drawer .rv-item .rv-meta { color:var(--muted); font-size:.72rem;
                               display:flex; gap:.5rem; align-items:center; }
#rv-drawer .rv-item .rv-meta button { font-size:.7rem; cursor:pointer;
             background:none; border:1px solid var(--line); border-radius:5px;
             color:var(--muted); padding:0 .35rem; }
#rv-drawer .rv-item .rv-text { white-space:pre-wrap; margin:.15rem 0; }
#rv-drawer footer { padding:.6rem 1rem; border-top:1px solid var(--line);
                    display:flex; gap:.5rem; flex-wrap:wrap; }
#rv-drawer footer button, #rv-drawer .rv-compose button {
  cursor:pointer; border:1px solid var(--accent); background:none;
  color:var(--accent); border-radius:6px; padding:.2rem .7rem; font-size:.8rem; }
#rv-drawer .rv-compose button.rv-primary { background:var(--accent); color:#fff; }
#rv-drawer .rv-otherpages { color:var(--muted); font-size:.75rem;
                            margin:.4rem 0 0; }
@media (max-width: 1200px) { body.rv-open .layout { padding-right:0; } }
"""

REVIEW_JS = r"""
(function(){
  'use strict';
  var PAGE = document.body.getAttribute('data-page');
  // storage key carries the spec's slug: file:// pages can share one
  // localStorage pool, and two specs must never share a comment store
  var RVKEY = 'spec-review:' + (document.body.getAttribute('data-rvkey') || 'spec');
  var EMBED = [];
  var embedEl = document.getElementById('rv-embedded');
  if (embedEl) { try { EMBED = JSON.parse(embedEl.textContent); } catch(e){} }
  function loadLocal(){ try { return JSON.parse(localStorage.getItem(RVKEY)) || {comments:[], deleted:[]}; } catch(e){ return {comments:[], deleted:[]}; } }
  function saveLocal(s){ try { localStorage.setItem(RVKEY, JSON.stringify(s)); } catch(e){} }
  function cid(c){ return c.page + '|' + c.section + '|' + (c.date||'') + '|' + c.body.slice(0,24); }
  function allComments(){
    var s = loadLocal(), map = {};
    EMBED.forEach(function(c){ map[cid(c)] = c; });
    s.comments.forEach(function(c){ map[cid(c)] = c; });
    s.deleted.forEach(function(id){ delete map[id]; });
    return Object.keys(map).map(function(k){ return map[k]; });
  }
  function upsert(c){
    var s = loadLocal(), id = cid(c);
    s.comments = s.comments.filter(function(x){ return cid(x) !== id; });
    s.comments.push(c);
    s.deleted = s.deleted.filter(function(x){ return x !== id; });
    saveLocal(s); refresh();
  }
  function remove(c){
    var s = loadLocal(), id = cid(c);
    s.comments = s.comments.filter(function(x){ return cid(x) !== id; });
    if (EMBED.some(function(x){ return cid(x) === id; })) s.deleted.push(id);
    saveLocal(s); refresh();
  }
  function today(){ return new Date().toISOString().slice(0,10); }
  function exportMd(){
    var lines = ['# Review comments', '',
      '<!-- Exported from the spec review layer. Save this file as',
      '     docs/specs/review-comments.md and re-run render-spec.py to embed',
      '     every comment under its section for the next review round. -->', ''];
    allComments().sort(function(a,b){
      return (a.page+a.section).localeCompare(b.page+b.section);
    }).forEach(function(c){
      lines.push('## [' + c.page + '] ' + (c.section||''));
      lines.push('status: ' + (c.status||'open'));
      lines.push('date: ' + (c.date||today()));
      lines.push(c.body); lines.push('');
    });
    var blob = new Blob([lines.join('\n')], {type:'text/markdown'});
    var a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'review-comments.md';
    a.click(); URL.revokeObjectURL(a.href);
  }
  function importMd(file){
    var reader = new FileReader();
    reader.onload = function(){
      var text = String(reader.result);
      var blocks = text.split(/^## /m).slice(1);
      blocks.forEach(function(b){
        var head = b.match(/^\[([^\]]+)\]\s*(\S*)/);
        if (!head) return;
        var status = (b.match(/^status:\s*(\S+)/m)||[])[1] || 'open';
        var date = (b.match(/^date:\s*(\S+)/m)||[])[1] || today();
        var body = b.replace(/^\[[^\]]*\]\s*\S*\s*\n/, '')
                    .replace(/^status:.*\n?/m,'').replace(/^date:.*\n?/m,'').trim();
        if (body) upsert({page:head[1], section:head[2]||'', status:status, date:date, body:body});
      });
      refresh();
    };
    reader.readAsText(file);
  }
  function commentBox(c){
    var div = document.createElement('div');
    div.className = 'rv-comment' + (c.status === 'resolved' ? ' rv-resolved' : '');
    var head = document.createElement('div'); head.className = 'rv-head';
    head.textContent = '💬 ' + (c.date||'') + ' · ' + (c.status||'open');
    var tog = document.createElement('button');
    tog.textContent = c.status === 'resolved' ? 'reopen' : 'resolve';
    tog.onclick = function(){ remove(c); c = Object.assign({}, c, {status: c.status === 'resolved' ? 'open' : 'resolved'}); upsert(c); };
    var del = document.createElement('button'); del.textContent = 'delete';
    del.onclick = function(){ if (confirm('Delete this comment?')) remove(c); };
    head.appendChild(tog); head.appendChild(del);
    var body = document.createElement('div'); body.className = 'rv-body';
    body.textContent = c.body;
    div.appendChild(head); div.appendChild(body);
    return div;
  }
  function editor(section, after){
    var wrap = document.createElement('div');
    wrap.className = 'rv-comment rv-editor';
    var ta = document.createElement('textarea');
    ta.placeholder = 'Comment on this section…';
    var actions = document.createElement('div'); actions.className = 'rv-actions';
    var save = document.createElement('button'); save.textContent = 'Save';
    save.onclick = function(){
      if (ta.value.trim()) upsert({page:PAGE, section:section, status:'open', date:today(), body:ta.value.trim()});
      wrap.remove();
    };
    var cancel = document.createElement('button'); cancel.textContent = 'Cancel';
    cancel.className = 'rv-cancel'; cancel.onclick = function(){ wrap.remove(); };
    actions.appendChild(save); actions.appendChild(cancel);
    wrap.appendChild(ta); wrap.appendChild(actions);
    after.insertAdjacentElement('afterend', wrap);
    ta.focus();
  }
  // --- headings, inline buttons ---
  var mainH1 = document.querySelector('main h1');
  if (mainH1 && !mainH1.id) { mainH1.id = 'page-title'; }
  var headings = Array.prototype.slice.call(
    document.querySelectorAll('main h1[id], main h2[id], main h3[id], main h4[id]'));
  headings.forEach(function(h){
    var b = document.createElement('button');
    b.className = 'rv-add'; b.textContent = '💬 comment'; b.title = 'Comment on this section';
    b.onclick = function(){ editor(h.id, h); };
    h.appendChild(b);
  });
  function headingTitle(h){
    var t = h.childNodes[0] ? h.childNodes[0].textContent : h.textContent;
    return t.trim();
  }
  function titleFor(section){
    var el = section && document.getElementById(section);
    return el ? headingTitle(el) : (section || '(page)');
  }

  // --- drawer ---
  var drawer = document.createElement('div');
  drawer.id = 'rv-drawer';
  drawer.innerHTML =
    '<header><strong>Review</strong> <span class="rv-count"></span></header>' +
    '<div class="rv-compose">' +
    '<div class="rv-live">Commenting on the section in view:</div>' +
    '<label for="rv-sec"></label><select id="rv-sec"></select>' +
    '<textarea id="rv-text" placeholder="Type as you read… (Ctrl+Enter saves)"></textarea>' +
    '<div style="margin-top:.4rem; display:flex; gap:.5rem;">' +
    '<button class="rv-primary" id="rv-save">Save comment</button>' +
    '<button id="rv-clear">Clear</button></div></div>' +
    '<div class="rv-list"></div>' +
    '<footer><button id="rv-export">Export .md</button>' +
    '<button id="rv-import">Import</button>' +
    '<span class="rv-otherpages"></span></footer>';
  document.body.appendChild(drawer);
  var toggle = document.createElement('button');
  toggle.id = 'rv-toggle';
  toggle.onclick = function(){
    document.body.classList.toggle('rv-open');
    try { localStorage.setItem(RVKEY + ':open', document.body.classList.contains('rv-open') ? '1' : ''); } catch(e){}
  };
  document.body.appendChild(toggle);
  try { if (localStorage.getItem(RVKEY + ':open')) document.body.classList.add('rv-open'); } catch(e){}

  var sel = drawer.querySelector('#rv-sec');
  headings.forEach(function(h){
    var opt = document.createElement('option');
    opt.value = h.id; opt.textContent = headingTitle(h);
    sel.appendChild(opt);
  });
  var followScroll = true;
  sel.onchange = function(){ followScroll = false; };

  // scroll spy: pick the last heading above the upper third of the viewport
  function currentSection(){
    var line = window.innerHeight * 0.33, best = headings[0];
    for (var i = 0; i < headings.length; i++) {
      if (headings[i].getBoundingClientRect().top <= line) best = headings[i];
    }
    return best ? best.id : '';
  }
  window.addEventListener('scroll', function(){
    if (!followScroll) return;
    var s = currentSection();
    if (s && sel.value !== s) sel.value = s;
  }, {passive:true});
  // typing pins the target section so a stray scroll can't retarget the comment
  drawer.querySelector('#rv-text').addEventListener('input', function(){
    if (this.value.trim()) followScroll = false;
    else followScroll = true;
  });

  function saveFromDrawer(){
    var ta = drawer.querySelector('#rv-text');
    if (!ta.value.trim()) return;
    upsert({page:PAGE, section:sel.value, status:'open', date:today(), body:ta.value.trim()});
    ta.value = ''; followScroll = true;
  }
  drawer.querySelector('#rv-save').onclick = saveFromDrawer;
  drawer.querySelector('#rv-text').addEventListener('keydown', function(e){
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') saveFromDrawer();
  });
  drawer.querySelector('#rv-clear').onclick = function(){
    drawer.querySelector('#rv-text').value = ''; followScroll = true;
  };
  drawer.querySelector('#rv-export').onclick = exportMd;
  var fileIn = document.createElement('input');
  fileIn.type = 'file'; fileIn.accept = '.md,text/markdown'; fileIn.style.display = 'none';
  fileIn.onchange = function(){ if (fileIn.files[0]) importMd(fileIn.files[0]); };
  drawer.appendChild(fileIn);
  drawer.querySelector('#rv-import').onclick = function(){ fileIn.click(); };

  function drawerItem(c){
    var div = document.createElement('div');
    div.className = 'rv-item' + (c.status === 'resolved' ? ' rv-resolved' : '');
    var where = document.createElement('div'); where.className = 'rv-where';
    where.textContent = titleFor(c.section);
    var text = document.createElement('div'); text.className = 'rv-text';
    text.textContent = c.body;
    var meta = document.createElement('div'); meta.className = 'rv-meta';
    meta.textContent = (c.date||'') + ' · ' + (c.status||'open');
    var tog = document.createElement('button');
    tog.textContent = c.status === 'resolved' ? 'reopen' : 'resolve';
    tog.onclick = function(e){ e.stopPropagation(); remove(c);
      upsert(Object.assign({}, c, {status: c.status === 'resolved' ? 'open' : 'resolved'})); };
    var del = document.createElement('button'); del.textContent = 'delete';
    del.onclick = function(e){ e.stopPropagation();
      if (confirm('Delete this comment?')) remove(c); };
    meta.appendChild(tog); meta.appendChild(del);
    div.appendChild(where); div.appendChild(text); div.appendChild(meta);
    div.onclick = function(){
      var el = c.section && document.getElementById(c.section);
      if (el) el.scrollIntoView({behavior:'smooth', block:'start'});
    };
    return div;
  }

  function refresh(){
    document.querySelectorAll('.rv-comment:not(.rv-editor)').forEach(function(el){ el.remove(); });
    var all = allComments();
    var mine = all.filter(function(c){ return c.page === PAGE; });
    mine.forEach(function(c){
      var target = c.section && document.getElementById(c.section);
      var anchor = target || mainH1 || document.body.querySelector('main') || document.body;
      anchor.insertAdjacentElement('afterend', commentBox(c));
    });
    var open = all.filter(function(c){ return c.status !== 'resolved'; }).length;
    toggle.textContent = '💬 Review (' + open + ')';
    drawer.querySelector('.rv-count').textContent =
      open + ' open / ' + all.length + ' total';
    var list = drawer.querySelector('.rv-list');
    list.innerHTML = '';
    mine.slice().reverse().forEach(function(c){ list.appendChild(drawerItem(c)); });
    var others = {};
    all.forEach(function(c){ if (c.page !== PAGE) others[c.page] = (others[c.page]||0) + 1; });
    var otherKeys = Object.keys(others);
    drawer.querySelector('.rv-otherpages').textContent = otherKeys.length
      ? 'Other pages: ' + otherKeys.map(function(k){ return k + ' (' + others[k] + ')'; }).join(', ')
      : '';
  }
  refresh();
  var s0 = currentSection();
  if (s0) sel.value = s0;
})();
"""


def mockup_html(path):
    esc = html.escape(path, quote=True)
    return (
        '<div class="mockup"><iframe src="%s" loading="lazy"></iframe>'
        '<p><a href="%s" target="_blank">Open %s in new tab</a></p></div>'
        % (esc, esc, esc)
    )


def rewrite_links(body_html, link_map, source_dir, into_html_dir):
    """Rewrite relative hrefs in a converted markdown body so they resolve from
    the generated page's location. `source_dir` is the source file's directory
    relative to the spec root ("" for spec.md, "features" for a feature file);
    `into_html_dir` says whether the page is written into html/.

    A relative href is resolved file-relative first, then spec-root-relative
    (the old single-page convention — mockup links are written that way), each
    checked against the package's file map and the disk. Package .md targets
    rewrite to their generated pages; every other relative target is re-based
    so it still points at the same file.
    """

    def resolve(path):
        joined = posixpath.normpath(posixpath.join(source_dir, path))
        plain = posixpath.normpath(path)
        for cand in (joined, plain):
            if (
                cand in link_map
                or cand == "spec.md"
                or os.path.exists(os.path.join(SPEC_DIR, cand))
            ):
                return cand
        return plain

    def sub(match):
        href = match.group(1)
        if re.match(r"[a-zA-Z][a-zA-Z0-9+.-]*:", href) or href.startswith(("#", "/")):
            return match.group(0)
        path, _, frag = href.partition("#")
        if not path:
            return match.group(0)
        frag = ("#" + frag) if frag else ""
        root_rel = resolve(path)
        if root_rel == "spec.md":
            target = "../spec.html" if into_html_dir else "spec.html"
            return 'href="%s%s"' % (target, frag)
        target = link_map.get(root_rel)
        if target:
            prefix = "" if into_html_dir else "html/"
            return 'href="%s%s%s"' % (prefix, target, frag)
        prefix = "../" if into_html_dir else ""
        return 'href="%s%s%s"' % (prefix, root_rel, frag)

    return re.sub(r'href="([^"]+)"', sub, body_html)


def site_nav(meta, features, pages, current, index_sections=()):
    """Sidebar shared by every generated page. `current` is 'index',
    'page-<id>' or 'feature-<id>'. Link prefixes depend on where the viewing
    page lives; pages in html/ reach the index via ../spec.html. On the index
    the sidebar also lists spec.md's own ## sections as anchors."""
    at_index = current == "index"
    to_index = "spec.html" if at_index else "../spec.html"
    to_page = (lambda f: "html/%s" % f) if at_index else (lambda f: f)
    name = html.escape(meta.get("name", "Project spec"))
    out = ['<aside class="site"><a class="brand" href="%s">%s</a>' % (to_index, name)]
    out.append("<h2>Spec</h2><ul>")
    cls = ' class="current"' if at_index else ""
    out.append('<li><a%s href="%s">Overview &amp; index</a></li>' % (cls, to_index))
    if at_index:
        for slug, text in index_sections:
            out.append(
                '<li><a class="sub" href="#%s">%s</a></li>'
                % (html.escape(slug, quote=True), html.escape(text))
            )
    out.append("</ul>")
    if pages:
        out.append("<h2>Pages</h2><ul>")
        for pid, _rel, pmeta, _body in pages:
            fname = "page-%s.html" % pid
            cls = ' class="current"' if current == "page-%s" % pid else ""
            out.append(
                '<li><a%s href="%s">%s</a></li>'
                % (cls, to_page(fname), html.escape(pmeta.get("title", pid)))
            )
        out.append("</ul>")
    out.append("<h2>Features</h2><ul>")
    for fid, _rel, fmeta, _body in features:
        fname = "feature-%s.html" % fid
        cls = ' class="current"' if current == "feature-%s" % fid else ""
        out.append(
            '<li><a%s href="%s">%s</a></li>'
            % (cls, to_page(fname), html.escape(fmeta.get("feature", fid)))
        )
    out.append("</ul></aside>")
    return "\n".join(out)


def wrap_page(title, nav, content, mermaid_src, page_id, review_json, rvkey, meta_line=""):
    if mermaid_src:
        mermaid_block = (
            '<script src="%s"></script>\n<script>mermaid.initialize({startOnLoad:true,'
            "theme:matchMedia('(prefers-color-scheme: dark)').matches?'dark':'default'});"
            "</script>" % mermaid_src
        )
    else:
        mermaid_block = (
            "<script>document.querySelectorAll('.diagram')"
            ".forEach(function(d){d.classList.add('raw')});</script>"
        )
    review_block = (
        '<script type="application/json" id="rv-embedded">%s</script>\n'
        "<script>%s</script>"
        % (review_json.replace("</", "<\\/"), REVIEW_JS)
    )
    return (
        "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"
        '<meta name="viewport" content="width=device-width, initial-scale=1">\n'
        "<title>%s</title>\n<style>%s</style>\n</head>\n"
        '<body data-page="%s" data-rvkey="%s">\n'
        '<div class="layout">\n%s\n<main>\n%s%s\n</main>\n</div>\n%s\n%s\n</body>\n</html>\n'
        % (
            html.escape(title),
            CSS,
            html.escape(page_id, quote=True),
            html.escape(rvkey, quote=True),
            nav,
            meta_line,
            content,
            mermaid_block,
            review_block,
        )
    )


def pager(prev_item, next_item):
    parts = ['<nav class="pager">']
    if prev_item:
        parts.append(
            '<a href="%s">&larr; %s</a>' % (prev_item[0], html.escape(prev_item[1]))
        )
    else:
        parts.append("<span></span>")
    if next_item:
        parts.append(
            '<a href="%s">%s &rarr;</a>' % (next_item[0], html.escape(next_item[1]))
        )
    else:
        parts.append("<span></span>")
    parts.append("</nav>")
    return "\n".join(parts)


def load_entries(meta, key, id_key_fallback):
    entries = []
    for entry in meta.get(key, []):
        rel = entry.get("file") if isinstance(entry, dict) else None
        if not rel:
            continue
        path = os.path.join(SPEC_DIR, rel)
        if not os.path.exists(path):
            print("render-spec: missing %s file %s" % (key, rel), file=sys.stderr)
            continue
        with open(path, encoding="utf-8") as f:
            emeta, ebody = split_front_matter(f.read())
        eid = emeta.get("id") or (
            entry.get("id") if isinstance(entry, dict) else None
        ) or re.sub(
            r"[^a-z0-9-]+", "-", os.path.basename(rel).rsplit(".", 1)[0].lower()
        )
        if id_key_fallback not in emeta:
            emeta[id_key_fallback] = eid
        entries.append((eid, rel, emeta, ebody))
    return entries


def main():
    spec_path = os.path.join(SPEC_DIR, "spec.md")
    if not os.path.exists(spec_path):
        print("render-spec: no spec.md next to this script", file=sys.stderr)
        return 1
    with open(spec_path, encoding="utf-8") as f:
        meta, body = split_front_matter(f.read())

    features = load_entries(meta, "features", "feature")
    pages = load_entries(meta, "pages", "title")
    have_mermaid = os.path.exists(MERMAID_ASSET)
    os.makedirs(HTML_DIR, exist_ok=True)

    review_json = json.dumps(load_review_comments())
    rvkey = str(meta.get("slug") or meta.get("name") or "spec")

    link_map = {}
    for fid, rel, _m, _b in features:
        link_map[rel] = "feature-%s.html" % fid
    for pid, rel, _m, _b in pages:
        link_map[rel] = "page-%s.html" % pid

    name = meta.get("name", "Project spec")
    today = datetime.date.today().isoformat()

    index_sections = [
        (slugify(m.group(1).strip()), m.group(1).strip())
        for m in re.finditer(r"^##\s+(.+)$", body, re.M)
    ]

    # ordered site walk for prev/next: pages first, then features
    walk = [("page-%s.html" % pid, pmeta.get("title", pid)) for pid, _r, pmeta, _b in pages]
    walk += [
        ("feature-%s.html" % fid, "Feature: %s" % fmeta.get("feature", fid))
        for fid, _r, fmeta, _b in features
    ]

    # --- index (spec.html) ---
    nav = site_nav(meta, features, pages, "index", index_sections)
    meta_line = (
        '<p class="meta"><span class="badge">%s</span> &nbsp; created %s &nbsp; '
        "rendered %s</p>\n"
        % (
            html.escape(meta.get("status", "draft")),
            html.escape(str(meta.get("created", "?"))),
            today,
        )
    )
    content = "<h1>%s</h1>\n" % html.escape(name)
    content += rewrite_links(md_to_html(body), link_map, "", into_html_dir=False)
    if walk:
        content += pager(None, ("html/" + walk[0][0], walk[0][1]))
    with open(os.path.join(SPEC_DIR, "spec.html"), "w", encoding="utf-8") as f:
        f.write(
            wrap_page(
                name,
                nav,
                content,
                "assets/mermaid.min.js" if have_mermaid else None,
                "index",
                review_json,
                rvkey,
                meta_line,
            )
        )

    # --- supplement pages and feature pages ---
    written = set()

    def write_leaf(idx, fname, title, nav_leaf, page_html):
        if idx > 0:
            prev_item = (walk[idx - 1][0], walk[idx - 1][1])
        else:
            prev_item = ("../spec.html", "Overview & index")
        next_item = (walk[idx + 1][0], walk[idx + 1][1]) if idx + 1 < len(walk) else None
        page_html += pager(prev_item, next_item)
        with open(os.path.join(HTML_DIR, fname), "w", encoding="utf-8") as f:
            f.write(
                wrap_page(
                    "%s — %s" % (name, title),
                    nav_leaf,
                    page_html,
                    "../assets/mermaid.min.js" if have_mermaid else None,
                    fname.rsplit(".", 1)[0],
                    review_json,
                    rvkey,
                )
            )
        written.add(fname)

    idx = 0
    for pid, rel, pmeta, pbody in pages:
        title = pmeta.get("title", pid)
        nav_leaf = site_nav(meta, features, pages, "page-%s" % pid)
        page_html = "<h1>%s</h1>\n" % html.escape(title)
        page_html += rewrite_links(
            md_to_html(pbody, demote=0, slug_prefix=pid + "-"),
            link_map,
            posixpath.dirname(rel),
            into_html_dir=True,
        )
        write_leaf(idx, "page-%s.html" % pid, title, nav_leaf, page_html)
        idx += 1

    for fid, rel, fmeta, fbody in features:
        title = "Feature: %s" % fmeta.get("feature", fid)
        nav_leaf = site_nav(meta, features, pages, "feature-%s" % fid)
        page_html = "<h1>%s</h1>\n" % html.escape(title)
        epic = fmeta.get("epic")
        status = fmeta.get("status", "planned")
        page_html += '<p class="meta">%s<span class="badge">%s</span></p>\n' % (
            (html.escape(epic) + " &nbsp; ") if isinstance(epic, str) else "",
            html.escape(status if isinstance(status, str) else "planned"),
        )
        page_html += rewrite_links(
            md_to_html(fbody, demote=1, slug_prefix=fid + "-"),
            link_map,
            posixpath.dirname(rel),
            into_html_dir=True,
        )
        mockups = fmeta.get("mockups", [])
        if isinstance(mockups, str):
            mockups = [m.strip() for m in mockups.strip("[]").split(",") if m.strip()]
        if mockups:
            page_html += "<h2>Mockups</h2>"
            page_html += (
                "<p>Mockups convey intent — the rough shape a user will see. An "
                "implementer may diverge where framework conventions, components, "
                "accessibility, or a better idea serve the feature's requirements; "
                "the FRs and acceptance criteria are the contract, not the pixels.</p>"
            )
            for m in mockups:
                page_html += mockup_html("../" + (m if isinstance(m, str) else str(m)))
        write_leaf(idx, "feature-%s.html" % fid, title, nav_leaf, page_html)
        idx += 1

    # a retired or renamed entry must not leave its old page in the sidebarless dark
    for fname in sorted(os.listdir(HTML_DIR)):
        if re.fullmatch(r"(feature|page)-.+\.html", fname) and fname not in written:
            os.remove(os.path.join(HTML_DIR, fname))
            print("render-spec: removed stale page html/%s" % fname, file=sys.stderr)

    # --- stamp freshness ---
    source = open(spec_path, encoding="utf-8").read()
    if re.search(r"^html_generated:", source, re.M):
        stamped = re.sub(
            r"^html_generated:.*$",
            "html_generated: %s" % today,
            source,
            count=1,
            flags=re.M,
        )
    elif source.startswith("---") and "\n---" in source[3:]:
        # key was hand-removed from the front-matter: insert it rather than
        # silently skipping the stamp — freshness tracking is the field's point
        stamped = source.replace("\n---", "\nhtml_generated: %s\n---" % today, 1)
        print("render-spec: html_generated was missing from spec.md — inserted",
              file=sys.stderr)
    else:
        stamped = source
        print("render-spec: spec.md has no front-matter — html_generated not stamped",
              file=sys.stderr)
    with open(spec_path, "w", encoding="utf-8") as f:
        f.write(stamped)

    print(
        "render-spec: wrote spec.html + %d page%s + %d feature page%s in html/ (mermaid %s)"
        % (
            len(pages),
            "" if len(pages) == 1 else "s",
            len(features),
            "" if len(features) == 1 else "s",
            "via assets/mermaid.min.js" if have_mermaid
            else "not vendored — diagrams show as source",
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
