(function(){
  "use strict";
  var list = document.getElementById("list");
  var posts = [].slice.call(document.querySelectorAll(".post"));
  if (!list || !posts.length) return;
  var order = posts.map(function(p){ return p.getAttribute("data-slug"); });
  var bySlug = {};
  posts.forEach(function(p, i){ bySlug[p.getAttribute("data-slug")] = { el: p, i: i, title: p.getAttribute("data-title") }; });
  var baseTitle = "博客 · spool";

  /* ---------- enhance prose ---------- */
  function guessLang(txt){
    var t = (txt || "").trim();
    if (/^\{|^\[/.test(t)) return "json";
    if (/\$env:|setx |choco |winget /.test(t)) return "powershell";
    if (/^[a-z_]+ *= *"|^\[[a-z_.]+\]/m.test(t)) return "toml";
    if (/^\s*(curl|npm|brew|export|cd |cat |mkdir|echo|claude|codex|node )/m.test(t)) return "bash";
    return "text";
  }
  document.querySelectorAll(".body pre").forEach(function(pre){
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
  document.querySelectorAll(".body table").forEach(function(t){
    if (t.parentNode && t.parentNode.classList.contains("table-wrap")) return;
    var w = document.createElement("div");
    w.className = "table-wrap";
    t.parentNode.insertBefore(w, t);
    w.appendChild(t);
  });

  /* ---------- copy ---------- */
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

  /* ---------- article nav ---------- */
  posts.forEach(function(p, i){
    var prev = order[i - 1], next = order[i + 1];
    var ap = p.querySelector(".pg.prev"), an = p.querySelector(".pg.next");
    if (prev) { ap.classList.remove("hide"); ap.href = "#" + prev; ap.querySelector(".tt").textContent = bySlug[prev].title; }
    if (next) { an.classList.remove("hide"); an.href = "#" + next; an.querySelector(".tt").textContent = bySlug[next].title; }
  });

  /* ---------- routing ---------- */
  function route(){
    var slug = decodeURIComponent((location.hash || "").replace(/^#/, ""));
    var rec = bySlug[slug];
    list.classList.toggle("show", !rec);
    posts.forEach(function(p){ p.classList.toggle("show", !!rec && p === rec.el); });
    document.title = rec ? (rec.title + " · 博客 · spool") : baseTitle;
    window.scrollTo(0, 0);
  }
  window.addEventListener("hashchange", route);
  route();
})();
