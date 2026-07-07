(function () {
  const t = window.I18N.t;
  const $ = (id) => document.getElementById(id);

  const el = {
    login: $("login"),
    app: $("app"),
    loginForm: $("login-form"),
    password: $("password"),
    loginError: $("login-error"),
    crumbs: $("crumbs"),
    rows: $("rows"),
    empty: $("empty"),
    fileInput: $("file-input"),
    drop: $("drop"),
    drophint: $("drophint"),
    progress: $("progress"),
    bar: $("bar"),
    upcard: $("upcard"),
    upName: $("up-name"),
    upPct: $("up-pct"),
    upFill: $("up-fill"),
    toast: $("toast"),
  };

  let cwd = "/";

  // ---- theme ----
  const prefersDark = () =>
    !window.matchMedia || window.matchMedia("(prefers-color-scheme: dark)").matches;

  function initTheme() {
    // Explicit user choice wins; otherwise follow the system preference.
    const saved = localStorage.getItem("theme");
    const theme = saved || (prefersDark() ? "dark" : "light");
    document.documentElement.setAttribute("data-theme", theme);
  }
  $("btn-theme").onclick = () => {
    const now = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", now);
    localStorage.setItem("theme", now);
  };
  // Track system changes while the user has made no explicit choice.
  if (window.matchMedia) {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (e) => {
      if (!localStorage.getItem("theme")) {
        document.documentElement.setAttribute("data-theme", e.matches ? "dark" : "light");
      }
    });
  }

  // ---- server info ----
  async function loadInfo() {
    let info;
    try {
      const res = await api("/api/info");
      if (!res.ok) return;
      info = await res.json();
    } catch (_) { return; }

    $("info-version").textContent = info.version || "—";
    const details = $("info-ftp-details");
    if (info.ftpEnabled) {
      $("info-ftp").textContent = t("enabled");
      details.classList.remove("hidden");
      const host = location.hostname || "localhost";
      const scheme = info.ftpTLS ? "ftps" : "ftp";
      $("info-ftp-url").textContent = scheme + "://" + host + ":" + info.ftpPort;
      $("info-host").textContent = host;
      $("info-port").textContent = info.ftpPort;
      $("info-tls").textContent = info.ftpTLS ? "FTPS (TLS)" : t("none");
    } else {
      $("info-ftp").textContent = t("disabled");
      details.classList.add("hidden");
    }
  }
  $("btn-info").onclick = () => { loadInfo(); $("info-overlay").classList.remove("hidden"); };
  $("info-close").onclick = () => $("info-overlay").classList.add("hidden");
  $("info-overlay").onclick = (e) => {
    if (e.target === $("info-overlay")) $("info-overlay").classList.add("hidden");
  };
  $("info-copy").onclick = async () => {
    const text = $("info-ftp-url").textContent;
    try {
      await navigator.clipboard.writeText(text);
      toast(t("copied"));
    } catch (_) {
      toast(t("error"), true);
    }
  };

  // Extensions we treat as viewable text, plus a size cap so we never pull a
  // huge file into the DOM. Files above the cap fall back to download only.
  const TEXT_EXT = new Set([
    "txt", "md", "markdown", "log", "json", "xml", "yaml", "yml", "csv", "tsv",
    "ini", "conf", "cfg", "toml", "env", "properties", "sh", "bash", "zsh",
    "js", "mjs", "cjs", "ts", "jsx", "tsx", "css", "scss", "less", "html",
    "htm", "svg", "go", "py", "rb", "rs", "c", "h", "cpp", "hpp", "cc", "java",
    "kt", "php", "pl", "lua", "sql", "r", "gitignore", "dockerfile", "makefile",
  ]);
  const TEXT_MAX = 2 * 1024 * 1024; // 2 MB

  function isTextFile(it) {
    if (it.type !== "file" || it.size > TEXT_MAX) return false;
    const name = it.name.toLowerCase();
    const dot = name.lastIndexOf(".");
    const ext = dot > 0 ? name.slice(dot + 1) : name; // dotfiles: match whole name
    return TEXT_EXT.has(ext);
  }

  // ---- text viewer ----
  let viewPath = null;
  async function openView(name) {
    const p = join(cwd, name);
    viewPath = p;
    $("view-name").textContent = name;
    $("view-body").textContent = t("loading");
    $("view-overlay").classList.remove("hidden");
    try {
      const res = await api("/api/download?path=" + encodeURIComponent(p));
      if (!res.ok) { $("view-body").textContent = t("error"); return; }
      $("view-body").textContent = await res.text();
    } catch (_) {
      $("view-body").textContent = t("error");
    }
  }
  function closeView() {
    $("view-overlay").classList.add("hidden");
    $("view-body").textContent = "";
    viewPath = null;
  }
  $("view-close").onclick = closeView;
  $("view-overlay").onclick = (e) => { if (e.target === $("view-overlay")) closeView(); };
  $("view-download").onclick = () => {
    if (viewPath) window.location = "/api/download?path=" + encodeURIComponent(viewPath);
  };
  // ---- notes ----
  // Notes are titled scraps of text kept apart from the file listing (server
  // stores them in a hidden folder): handy to shuttle text between machines or
  // to document what folders are for when several people share the drive.
  let editingId = null; // id of the note open in the editor, null = new note

  function showNotesView(which) {
    $("notes-list-view").classList.toggle("hidden", which !== "list");
    $("notes-edit-view").classList.toggle("hidden", which !== "edit");
  }
  function openNotes() {
    $("notes-overlay").classList.remove("hidden");
    showNotesView("list");
    loadNotes();
  }
  function closeNotes() { $("notes-overlay").classList.add("hidden"); }

  async function loadNotes() {
    let data;
    try {
      const res = await api("/api/notes");
      if (!res.ok) { toast(t("error"), true); return; }
      data = await res.json();
    } catch (_) { return; }
    renderNotes(data.notes || []);
  }
  function renderNotes(notes) {
    const ul = $("notes-items");
    ul.innerHTML = "";
    $("notes-empty").classList.toggle("hidden", notes.length > 0);
    for (const n of notes) {
      const li = document.createElement("li");
      li.className = "note-item";
      const title = document.createElement("span");
      title.className = "note-title";
      title.textContent = n.title || t("untitled");
      title.onclick = () => openEditor(n);
      const x = document.createElement("button");
      x.className = "note-del";
      x.textContent = "✕";
      x.title = t("delete");
      x.onclick = (e) => { e.stopPropagation(); delNote(n); };
      li.append(title, x);
      ul.appendChild(li);
    }
  }
  function openEditor(n) {
    editingId = n ? n.id : null;
    $("note-title").value = n ? (n.title || "") : "";
    $("note-body").value = n ? (n.body || "") : "";
    showNotesView("edit");
    setTimeout(() => (n ? $("note-body") : $("note-title")).focus(), 30);
  }
  async function delNote(n) {
    if (!confirm(t("confirmDelete", { name: n.title || t("untitled") }))) return;
    const res = await api("/api/notes?id=" + encodeURIComponent(n.id), { method: "DELETE" });
    if (res.ok) { toast(t("noteDeleted")); loadNotes(); }
    else { toast(t("error"), true); }
  }
  $("btn-note").onclick = openNotes;
  $("notes-close").onclick = closeNotes;
  $("notes-new").onclick = () => openEditor(null);
  $("note-back").onclick = () => { showNotesView("list"); loadNotes(); };
  $("notes-overlay").onclick = (e) => { if (e.target === $("notes-overlay")) closeNotes(); };
  $("note-save").onclick = async () => {
    const title = $("note-title").value.trim();
    if (!title) { toast(t("titleRequired"), true); return; }
    const payload = { title, body: $("note-body").value };
    if (editingId) payload.id = editingId;
    const res = await api("/api/notes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (res.ok) { toast(t("noteSaved")); showNotesView("list"); loadNotes(); }
    else { toast(res.status === 507 ? t("noSpace") : t("error"), true); }
  };

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    if (!$("notes-overlay").classList.contains("hidden")) {
      if (!$("notes-edit-view").classList.contains("hidden")) { showNotesView("list"); loadNotes(); }
      else closeNotes();
    } else if (!$("view-overlay").classList.contains("hidden")) {
      closeView();
    }
  });

  // ---- helpers ----
  function fmtSize(n) {
    if (n === 0) return "—";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? n : n.toFixed(1)) + " " + u[i];
  }
  function fmtDate(s) {
    const d = new Date(s);
    if (isNaN(d)) return "";
    return d.toLocaleDateString() + " " + d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  function join(dir, name) {
    return (dir === "/" ? "" : dir) + "/" + name;
  }
  function toast(msg, isErr) {
    el.toast.textContent = msg;
    el.toast.classList.toggle("err", !!isErr);
    el.toast.classList.remove("hidden");
    clearTimeout(toast._t);
    toast._t = setTimeout(() => el.toast.classList.add("hidden"), 2600);
  }

  async function api(path, opts) {
    const res = await fetch(path, Object.assign({ credentials: "same-origin" }, opts));
    if (res.status === 401) { showLogin(); throw new Error("unauthorized"); }
    return res;
  }

  // ---- auth ----
  function showLogin() {
    el.app.classList.add("hidden");
    el.login.classList.remove("hidden");
    el.password.value = "";
    setTimeout(() => el.password.focus(), 30);
  }
  function showApp() {
    el.login.classList.add("hidden");
    el.app.classList.remove("hidden");
    load(cwd);
  }
  el.loginForm.onsubmit = async (e) => {
    e.preventDefault();
    el.loginError.classList.add("hidden");
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ password: el.password.value }),
    });
    if (res.ok) { showApp(); }
    else { el.loginError.classList.remove("hidden"); }
  };
  $("btn-logout").onclick = async () => {
    await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
    showLogin();
  };

  // ---- listing ----
  function renderCrumbs() {
    el.crumbs.innerHTML = "";
    const parts = cwd.split("/").filter(Boolean);
    const root = document.createElement("a");
    root.textContent = "🧌 /";
    root.onclick = () => load("/");
    el.crumbs.appendChild(root);
    let acc = "";
    parts.forEach((p, i) => {
      acc += "/" + p;
      const sep = document.createElement("span");
      sep.className = "sep"; sep.textContent = "›";
      el.crumbs.appendChild(sep);
      const last = i === parts.length - 1;
      const a = document.createElement("a");
      a.textContent = p;
      if (last) { a.className = "here"; }
      else { const target = acc; a.onclick = () => load(target); }
      el.crumbs.appendChild(a);
    });
  }

  async function load(path) {
    cwd = path || "/";
    renderCrumbs();
    let data;
    try {
      const res = await api("/api/files?path=" + encodeURIComponent(cwd));
      if (!res.ok) { toast(t("error"), true); return; }
      data = await res.json();
    } catch (_) { return; }

    el.rows.innerHTML = "";
    const entries = data.entries || [];
    el.empty.classList.toggle("hidden", entries.length > 0);
    for (const it of entries) el.rows.appendChild(row(it));
  }

  function row(it) {
    const tr = document.createElement("tr");
    const p = join(cwd, it.name);

    const tdName = document.createElement("td");
    const wrap = document.createElement("div");
    wrap.className = "fname";
    const ico = document.createElement("span");
    ico.className = "fico";
    ico.textContent = it.type === "dir" ? "📁" : "📄";
    wrap.appendChild(ico);
    if (it.type === "dir") {
      const a = document.createElement("a");
      a.className = "dir"; a.textContent = it.name;
      a.onclick = () => load(p);
      wrap.appendChild(a);
    } else {
      const span = document.createElement("span");
      span.textContent = it.name;
      wrap.appendChild(span);
    }
    tdName.appendChild(wrap);

    const tdSize = document.createElement("td");
    tdSize.className = "col-size";
    tdSize.textContent = it.type === "dir" ? "" : fmtSize(it.size);

    const tdMod = document.createElement("td");
    tdMod.className = "col-mod";
    tdMod.textContent = fmtDate(it.modified);

    const tdAct = document.createElement("td");
    tdAct.className = "row-actions";
    if (isTextFile(it)) {
      tdAct.appendChild(mkBtn("👁", t("view"), () => openView(it.name)));
    }
    if (it.type === "file") {
      tdAct.appendChild(mkBtn("⬇", t("download"), () => {
        window.location = "/api/download?path=" + encodeURIComponent(p);
      }));
    }
    tdAct.appendChild(mkBtn("✎", t("rename"), () => rename(it.name)));
    tdAct.appendChild(mkBtn("🗑", t("delete"), () => del(it.name), "del"));

    tr.append(tdName, tdSize, tdMod, tdAct);
    return tr;
  }

  function mkBtn(label, title, onclick, cls) {
    const b = document.createElement("button");
    b.textContent = label; b.title = title; b.onclick = onclick;
    if (cls) b.className = cls;
    return b;
  }

  // ---- actions ----
  $("btn-newfolder").onclick = async () => {
    const name = prompt(t("newFolderName"));
    if (!name) return;
    const res = await api("/api/folder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: join(cwd, name) }),
    });
    if (res.ok) { toast(t("folderCreated")); load(cwd); }
    else { toast(t("error"), true); }
  };

  async function rename(name) {
    const to = prompt(t("renameTo"), name);
    if (!to || to === name) return;
    const res = await api("/api/rename", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ from: join(cwd, name), to: join(cwd, to) }),
    });
    if (res.ok) { toast(t("renamed")); load(cwd); }
    else { toast(t("error"), true); }
  }

  async function del(name) {
    if (!confirm(t("confirmDelete", { name }))) return;
    const res = await api("/api/files?path=" + encodeURIComponent(join(cwd, name)), { method: "DELETE" });
    if (res.ok) { toast(t("deleted")); load(cwd); }
    else { toast(t("error"), true); }
  }

  // ---- upload ----
  $("btn-upload").onclick = () => el.fileInput.click();
  el.fileInput.onchange = () => { uploadFiles(el.fileInput.files); el.fileInput.value = ""; };

  function uploadFiles(files) {
    if (!files || !files.length) return;
    const fd = new FormData();
    for (const f of files) fd.append("file", f, f.name);

    const label = files.length === 1 ? files[0].name : t("uploadingN", { n: files.length });
    showUpcard(label);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload?path=" + encodeURIComponent(cwd));
    xhr.withCredentials = true;
    el.progress.classList.remove("hidden");
    el.bar.style.width = "0";
    xhr.upload.onprogress = (e) => {
      if (!e.lengthComputable) return;
      const pct = Math.round((e.loaded / e.total) * 100);
      setUpcard(pct, e.loaded, e.total);
    };
    xhr.onload = () => {
      el.progress.classList.add("hidden");
      if (xhr.status === 401) { hideUpcard(); showLogin(); return; }
      if (xhr.status >= 200 && xhr.status < 300) {
        finishUpcard();
        toast(t("uploaded", { n: files.length }));
        load(cwd);
      } else {
        hideUpcard();
        toast(xhr.status === 507 ? t("noSpace") : t("error"), true);
      }
    };
    xhr.onerror = () => { hideUpcard(); el.progress.classList.add("hidden"); toast(t("error"), true); };
    xhr.send(fd);
  }

  function showUpcard(name) {
    el.upName.textContent = name;
    el.upPct.textContent = "0%";
    el.upFill.style.width = "0";
    el.upcard.classList.remove("hidden", "done");
  }
  function setUpcard(pct, loaded, total) {
    el.upFill.style.width = pct + "%";
    el.upPct.textContent = total ? pct + "% (" + fmtSize(loaded) + " / " + fmtSize(total) + ")" : pct + "%";
  }
  function finishUpcard() {
    el.upFill.style.width = "100%";
    el.upPct.textContent = "100%";
    el.upcard.classList.add("done");
    clearTimeout(showUpcard._t);
    showUpcard._t = setTimeout(hideUpcard, 1200);
  }
  function hideUpcard() { el.upcard.classList.add("hidden"); }

  // drag & drop
  let dragDepth = 0;
  el.drop.addEventListener("dragenter", (e) => {
    e.preventDefault(); dragDepth++;
    el.drop.classList.add("dragging"); el.drophint.classList.remove("hidden");
  });
  el.drop.addEventListener("dragover", (e) => e.preventDefault());
  el.drop.addEventListener("dragleave", () => {
    if (--dragDepth <= 0) { dragDepth = 0; el.drop.classList.remove("dragging"); el.drophint.classList.add("hidden"); }
  });
  el.drop.addEventListener("drop", (e) => {
    e.preventDefault(); dragDepth = 0;
    el.drop.classList.remove("dragging"); el.drophint.classList.add("hidden");
    if (e.dataTransfer.files.length) uploadFiles(e.dataTransfer.files);
  });

  // ---- boot ----
  initTheme();
  window.I18N.apply();
  (async () => {
    // Probe session; open mode returns 200, else 401 -> login.
    try {
      const res = await fetch("/api/files?path=/", { credentials: "same-origin" });
      if (res.ok) { showApp(); } else { showLogin(); }
    } catch (_) { showLogin(); }
  })();
})();
