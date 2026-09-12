(function(){
  "use strict";
  var chapters = [].slice.call(document.querySelectorAll(".chapter"));
  if (!chapters.length) return;
  var order = chapters.map(function(c){ return c.getAttribute("data-id"); });
  var byId = {};
  chapters.forEach(function(c, i){ byId[c.getAttribute("data-id")] = { el: c, i: i, title: c.getAttribute("data-title") }; });
  var side = document.getElementById("side");
  var scrim = document.getElementById("scrim");
  var tbtn = document.getElementById("tbtn");
  var tnow = document.getElementById("tnow");
  var crumbNow = document.getElementById("crumb-now");
  var pgPrev = document.getElementById("pg-prev");
  var pgNext = document.getElementById("pg-next");
  var anchorOf = {};   /* anchor id -> chapter id */
  var current = null;

  /* ---------- 1. enhance content ---------- */
  var LBL = { note: "说明", tip: "提示", warn: "注意" };

  function guessLang(txt){
    var t = (txt || "").trim();
    if (/^\{|^\[/.test(t)) return "json";
    if (/^\s*(POST|GET)\s/.test(t)) return "http";
    if (/\$env:|setx |choco |winget |Set-ExecutionPolicy|curl\.exe/.test(t)) return "powershell";
    if (/^\s*(import |from )/m.test(t)) return "python";
    if (/^[a-z_]+ *= *"|^\[[a-z_.]+\]/m.test(t)) return "toml";
    if (/^\s*(curl|npm|brew|export|cd |cat |mkdir|echo|claude|codex|node |code |cursor )/m.test(t)) return "bash";
    return "text";
  }

  function enhance(root){
    root.querySelectorAll(".callout").forEach(function(c){
      var kind = c.classList.contains("tip") ? "tip" : (c.classList.contains("warn") ? "warn" : "note");
      var p = c.querySelector("p");
      if (!p) return;
      var f = p.firstElementChild;
      if (!(f && f.tagName === "STRONG" && p.firstChild === f)) {
        var s = document.createElement("strong");
        s.textContent = LBL[kind] + "：";
        p.insertBefore(s, p.firstChild);
      }
    });

    root.querySelectorAll("table").forEach(function(t){
      if (t.parentNode && t.parentNode.classList.contains("table-wrap")) return;
      var w = document.createElement("div");
      w.className = "table-wrap";
      t.parentNode.insertBefore(w, t);
      w.appendChild(t);
    });

    root.querySelectorAll("pre").forEach(function(pre){
      if (pre.parentNode && pre.parentNode.classList.contains("code-block")) return;
      var code = pre.querySelector("code");
      var lang = "";
      if (code) {
        var m = (code.className || "").match(/lang-([\w-]+)/);
        if (m) lang = m[1];
      }
      if (!lang) lang = guessLang(code ? code.textContent : pre.textContent);
      var wrap = document.createElement("div");
      wrap.className = "code-block";
      wrap.innerHTML = '<div class="cb-head"><span class="dot d1"></span><span class="dot d2"></span><span class="dot d3"></span>' +
        '<span class="cb-lang"></span><button class="cb-copy" type="button" aria-label="复制代码">' +
        '<svg class="ic"><use href="#i-copy"></use></svg><span>复制</span></button></div>';
      wrap.querySelector(".cb-lang").textContent = lang;
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
    });
  }

  chapters.forEach(function(c){
    var id = c.getAttribute("data-id");
    var body = c.querySelector(".body");
    enhance(body);
    var n = 0;
    body.querySelectorAll("h2, h3").forEach(function(h){
      n++;
      if (!h.id) h.id = id + "-s" + n;
      anchorOf[h.id] = id;
    });
  });

  /* ---------- 2. copy ---------- */
  function feedback(btn){
    btn.classList.add("done");
    btn.querySelector("span").textContent = "已复制";
    btn.querySelector("use").setAttribute("href", "#i-check");
    setTimeout(function(){
      btn.classList.remove("done");
      btn.querySelector("span").textContent = "复制";
      btn.querySelector("use").setAttribute("href", "#i-copy");
    }, 1600);
  }
  function legacyCopy(text, ok){
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.cssText = "position:fixed;top:-1000px;left:0;opacity:0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); ok(); } catch (e) { /* 复制失败则静默 */ }
    document.body.removeChild(ta);
  }
  document.addEventListener("click", function(e){
    var btn = e.target.closest ? e.target.closest(".cb-copy") : null;
    if (!btn) return;
    var block = btn.closest(".code-block");
    var code = block && block.querySelector("code, pre");
    if (!code) return;
    var text = code.textContent;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function(){ feedback(btn); }, function(){ legacyCopy(text, function(){ feedback(btn); }); });
    } else {
      legacyCopy(text, function(){ feedback(btn); });
    }
  });

  /* ---------- 3. sidebar sub-anchors ---------- */
  function buildSub(id){
    side.querySelectorAll(".sub").forEach(function(s){ s.remove(); });
    var li = side.querySelector('li[data-for="' + id + '"]');
    if (!li) return;
    var body = byId[id].el.querySelector(".body");
    var hs = [].slice.call(body.querySelectorAll("h2"));
    if (!hs.length) hs = [].slice.call(body.querySelectorAll("h3"));
    if (!hs.length) return;
    var ul = document.createElement("ul");
    ul.className = "sub";
    hs.forEach(function(h){
      var a = document.createElement("a");
      a.href = "#" + h.id;
      a.textContent = h.textContent;
      a.setAttribute("data-target", h.id);
      var item = document.createElement("li");
      item.appendChild(a);
      ul.appendChild(item);
    });
    li.appendChild(ul);
  }

  /* ---------- 4. chapter switching ---------- */
  function setPager(i){
    var p = order[i - 1], n = order[i + 1];
    if (p) {
      pgPrev.classList.remove("hide");
      pgPrev.href = "#" + p;
      pgPrev.querySelector(".tt").textContent = byId[p].title;
    } else { pgPrev.classList.add("hide"); }
    if (n) {
      pgNext.classList.remove("hide");
      pgNext.href = "#" + n;
      pgNext.querySelector(".tt").textContent = byId[n].title;
    } else { pgNext.classList.add("hide"); }
  }

  function show(id, anchor){
    var rec = byId[id];
    if (!rec) return;
    if (current !== id) {
      chapters.forEach(function(c){ c.classList.toggle("show", c === rec.el); });
      side.querySelectorAll("li[data-for]").forEach(function(li){
        li.classList.toggle("active", li.getAttribute("data-for") === id);
      });
      buildSub(id);
      crumbNow.textContent = rec.title;
      tnow.textContent = rec.title;
      document.title = rec.title + " · 使用教程 · spool";
      setPager(rec.i);
      current = id;
    }
    if (anchor) {
      var t = document.getElementById(anchor);
      if (t) {
        var y = t.getBoundingClientRect().top + window.pageYOffset - 96;
        window.scrollTo({ top: y < 0 ? 0 : y, behavior: "auto" });
        markActive(anchor);
        return;
      }
    }
    window.scrollTo(0, 0);
    markActive(null);
  }

  var alias = { claude: "claude-code-setup", "claude-code": "claude-code-setup", cc: "claude-code-setup",
                codex: "codex-install", gpt: "codex-install", ide: "claude-code-ide", desktop: "chatbox",
                image: "gpt-image-2", images: "gpt-image-2", token: "token-create", key: "token-create",
                keys: "token-create" };

  function route(){
    var h = decodeURIComponent((location.hash || "").replace(/^#/, ""));
    if (byId[h]) { show(h, null); return; }
    if (anchorOf[h]) { show(anchorOf[h], h); return; }
    var a = alias[h.toLowerCase()];
    if (a && byId[a]) { location.replace("#" + a); return; }
    if (location.hash) { location.replace("#" + order[0]); return; }
    show(order[0], null);
  }

  window.addEventListener("hashchange", function(){ route(); closeDrawer(); });

  /* ---------- 5. scroll spy ---------- */
  var subLinks = null;
  function markActive(id){
    side.querySelectorAll(".sub a").forEach(function(a){
      a.classList.toggle("on", a.getAttribute("data-target") === id);
    });
  }
  var ticking = false;
  window.addEventListener("scroll", function(){
    if (ticking) return;
    ticking = true;
    window.requestAnimationFrame(function(){
      ticking = false;
      if (!current) return;
      subLinks = side.querySelectorAll(".sub a");
      if (!subLinks.length) return;
      var top = 130, best = null;
      for (var i = 0; i < subLinks.length; i++) {
        var el = document.getElementById(subLinks[i].getAttribute("data-target"));
        if (!el) continue;
        if (el.getBoundingClientRect().top <= top) best = subLinks[i].getAttribute("data-target");
      }
      if (!best) best = subLinks[0].getAttribute("data-target");
      if (window.innerHeight + window.scrollY >= document.body.scrollHeight - 4) {
        best = subLinks[subLinks.length - 1].getAttribute("data-target");
      }
      markActive(best);
    });
  }, { passive: true });

  /* ---------- 6. mobile drawer ---------- */
  var navwrap = document.querySelector(".navwrap");
  function syncDrawerTop(){
    if (!navwrap) return;
    var b = navwrap.getBoundingClientRect().bottom;
    document.documentElement.style.setProperty("--drawer-top", (b > 0 ? Math.round(b) : 0) + "px");
  }
  function openDrawer(){
    syncDrawerTop();
    side.classList.add("open");
    scrim.classList.add("open");
    tbtn.setAttribute("aria-expanded", "true");
  }
  function closeDrawer(){
    side.classList.remove("open");
    scrim.classList.remove("open");
    tbtn.setAttribute("aria-expanded", "false");
  }
  tbtn.addEventListener("click", function(){
    if (side.classList.contains("open")) closeDrawer(); else openDrawer();
  });
  scrim.addEventListener("click", closeDrawer);
  window.addEventListener("scroll", function(){ if (side.classList.contains("open")) syncDrawerTop(); }, { passive: true });
  window.addEventListener("resize", function(){ if (side.classList.contains("open")) syncDrawerTop(); });
  side.addEventListener("click", function(e){
    if (e.target.closest && e.target.closest("a")) closeDrawer();
  });
  document.addEventListener("keydown", function(e){
    if (e.key === "Escape") closeDrawer();
  });

  route();
})();
