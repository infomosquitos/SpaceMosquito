/* SpaceMosquito browse UI (internal/browse, ADR-018). The server renders the
 * tree and content; this adds behaviour only. Search results are built with
 * DOM APIs (textContent), never innerHTML. */
(function () {
  "use strict";

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }
  function each(sel, root, fn) {
    Array.prototype.forEach.call((root || document).querySelectorAll(sel), fn);
  }

  // Page trees: title filter, expand/collapse all. Clearing the filter
  // restores the server-rendered open state (first level on the space page,
  // the path to the current page in the sidebar).
  each(".sm-tree-box", null, function (box) {
    var tree = box.querySelector("ul.sm-tree");
    if (!tree) return;
    var folders = Array.prototype.slice.call(tree.querySelectorAll("details"));
    var initial = folders.map(function (d) { return d.open; });
    // Each item keeps the titles of itself and its descendants (not counts).
    var items = Array.prototype.map.call(tree.querySelectorAll("li"), function (li) {
      var titles = Array.prototype.map.call(li.querySelectorAll("a"), function (a) { return a.textContent; });
      return { li: li, text: titles.join("\n").toLowerCase() };
    });
    function setAll(open) { folders.forEach(function (d) { d.open = open; }); }
    function reset() { folders.forEach(function (d, i) { d.open = initial[i]; }); }
    each(".sm-expand", box, function (b) {
      if (!folders.length) { b.classList.add("hidden"); return; }
      b.addEventListener("click", function () { setAll(true); });
    });
    each(".sm-collapse", box, function (b) {
      if (!folders.length) { b.classList.add("hidden"); return; }
      b.addEventListener("click", function () { setAll(false); });
    });
    var filter = box.querySelector(".sm-filter");
    if (filter) filter.addEventListener("input", function () {
      var q = filter.value.trim().toLowerCase();
      items.forEach(function (it) { it.li.classList.toggle("hidden", q !== "" && it.text.indexOf(q) < 0); });
      if (q !== "") setAll(true); else reset();
    });
  });

  // Sidebar: keep the current page in view; drawer on narrow windows.
  var nav = document.getElementById("sm-nav");
  var toggle = document.getElementById("sm-toggle");
  function revealCurrent() {
    var cur = nav && nav.querySelector(".sm-current");
    if (!cur || nav.clientHeight === 0) return;
    var top = cur.getBoundingClientRect().top - nav.getBoundingClientRect().top;
    nav.scrollTop += top - nav.clientHeight / 3;
  }
  if (nav) {
    revealCurrent();
    if (toggle) toggle.addEventListener("click", function () {
      document.body.classList.toggle("sm-nav-open");
      revealCurrent();
    });
    document.addEventListener("click", function (e) {
      if (!nav.contains(e.target) && e.target !== toggle) document.body.classList.remove("sm-nav-open");
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") document.body.classList.remove("sm-nav-open");
    });
  }

  // Excerpts wrap matches in U+E000 … U+E001 (search.MatchStart/MatchEnd).
  function appendMarked(target, s) {
    s.split("").forEach(function (part, i) {
      var end = part.indexOf("");
      if (i === 0 || end < 0) {
        target.appendChild(document.createTextNode(part.replace(//g, "")));
        return;
      }
      target.appendChild(el("mark", "", part.slice(0, end)));
      target.appendChild(document.createTextNode(part.slice(end + 1)));
    });
  }
  function search(q, space, limit) {
    return fetch("/api/search?highlight=1&limit=" + limit + "&q=" + encodeURIComponent(q) +
      (space ? "&space_key=" + encodeURIComponent(space) : ""))
      .then(function (r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (data) { return data.results || []; });
  }
  function linked(h) { return typeof h.browse_url === "string" && h.browse_url.indexOf("/browse/") === 0; }
  var unavailable = "Content search is unavailable. Is spacemosquito serve running?";
  var noHits = "No pages contain all of these words.";

  // Full results on the space and catalog pages; ?q= pre-fills the query
  // ("See all results" in a page's quick search).
  each(".sm-fullsearch", null, function (box) {
    var input = box.querySelector("input"), list = box.querySelector("ol"), status = box.querySelector(".sm-status");
    var space = box.getAttribute("data-space"), timer = null, seq = 0;
    function run() {
      var q = input.value.trim(), my = ++seq;
      list.textContent = "";
      status.textContent = "";
      if (q.length < 2) return;
      search(q, space, 20).then(function (hits) {
        if (my !== seq) return;
        if (!hits.length) status.textContent = noHits;
        hits.forEach(function (h) {
          var li = el("li"), title = el(linked(h) ? "a" : "span", "", h.title);
          if (linked(h)) title.href = h.browse_url;
          li.appendChild(title);
          if (!space) li.appendChild(el("span", "sm-meta", " · " + h.space_key));
          if (h.excerpt) {
            var p = el("p", "sm-excerpt");
            appendMarked(p, h.excerpt);
            li.appendChild(p);
          }
          list.appendChild(li);
        });
      }).catch(function () { if (my === seq) status.textContent = unavailable; });
    }
    input.addEventListener("input", function () {
      clearTimeout(timer);
      timer = setTimeout(run, 250);
    });
    var initial = new URLSearchParams(location.search).get("q");
    if (initial) {
      input.value = initial;
      run();
    }
  });

  // Quick search in a page's top bar: dropdown with keyboard navigation.
  each(".sm-quick", null, function (box) {
    var input = box.querySelector("input"), list = box.querySelector(".sm-results");
    var space = box.getAttribute("data-space"), home = box.getAttribute("data-all");
    var timer = null, seq = 0, active = -1;
    function links() { return list.querySelectorAll("a"); }
    function close() { list.hidden = true; active = -1; }
    function allResults() { return home + "?q=" + encodeURIComponent(input.value.trim()); }
    function note(text) {
      list.textContent = "";
      list.appendChild(el("li", "sm-note", text));
      list.hidden = false;
    }
    function highlight(i) {
      var as = links();
      if (!as.length) return;
      active = (i + as.length) % as.length;
      Array.prototype.forEach.call(as, function (a, j) { a.classList.toggle("sm-active", j === active); });
      as[active].scrollIntoView({ block: "nearest" });
    }
    function run() {
      var q = input.value.trim(), my = ++seq;
      if (q.length < 2) { close(); return; }
      search(q, space, 10).then(function (hits) {
        if (my !== seq) return;
        hits = hits.filter(linked);
        if (!hits.length) { note(noHits); return; }
        list.textContent = "";
        active = -1;
        hits.forEach(function (h) {
          var li = el("li"), a = el("a");
          a.href = h.browse_url;
          a.appendChild(el("span", "sm-title", h.title));
          if (h.excerpt) {
            var ex = el("span", "sm-excerpt");
            appendMarked(ex, h.excerpt);
            a.appendChild(ex);
          }
          li.appendChild(a);
          list.appendChild(li);
        });
        var li = el("li"), all = el("a", "sm-all", "See all results in the space →");
        all.href = allResults();
        li.appendChild(all);
        list.appendChild(li);
        list.hidden = false;
      }).catch(function () { if (my === seq) note(unavailable); });
    }
    input.addEventListener("input", function () {
      clearTimeout(timer);
      timer = setTimeout(run, 200);
    });
    input.addEventListener("focus", function () {
      if (list.children.length && input.value.trim().length >= 2) list.hidden = false;
    });
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        if (!list.hidden) highlight(active + (e.key === "ArrowDown" ? 1 : -1));
      } else if (e.key === "Enter") {
        e.preventDefault();
        var as = links();
        if (active >= 0 && as[active]) location.href = as[active].href;
        else if (input.value.trim().length >= 2) location.href = allResults();
      } else if (e.key === "Escape") {
        close();
        input.blur();
      }
    });
    document.addEventListener("click", function (e) { if (!box.contains(e.target)) close(); });
  });

  // File previews in the page (view-file → link titled "attachment") show
  // PDFs inline, as Confluence does; other files stay download links.
  each('main a[title="attachment"]', null, function (a) {
    var p = a.parentElement;
    if (!/\.pdf$/i.test(a.getAttribute("href") || "") || !p || p.tagName !== "P" || p.querySelectorAll("a").length !== 1) return;
    var frame = el("iframe", "sm-pdf");
    frame.src = a.getAttribute("href");
    frame.title = a.textContent;
    frame.loading = "lazy";
    p.parentNode.insertBefore(frame, p.nextSibling);
  });

  // "/" focuses content search from anywhere on the page.
  document.addEventListener("keydown", function (e) {
    var t = e.target, typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);
    if (e.key !== "/" || typing || e.metaKey || e.ctrlKey || e.altKey) return;
    var input = document.querySelector(".sm-quick input, .sm-fullsearch input");
    if (!input) return;
    e.preventDefault();
    input.focus();
    input.select();
  });
})();
