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
    toast: $("toast"),
  };

  let cwd = "/";

  // ---- theme ----
  function initTheme() {
    const saved = localStorage.getItem("theme") || "dark";
    document.documentElement.setAttribute("data-theme", saved);
  }
  $("btn-theme").onclick = () => {
    const now = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", now);
    localStorage.setItem("theme", now);
  };

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
  el.fileInput.onchange = () => { uploadFiles(el.fileInput.files); el.fileInput.value = ""; };

  function uploadFiles(files) {
    if (!files || !files.length) return;
    const fd = new FormData();
    for (const f of files) fd.append("file", f, f.name);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload?path=" + encodeURIComponent(cwd));
    xhr.withCredentials = true;
    el.progress.classList.remove("hidden");
    el.bar.style.width = "0";
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) el.bar.style.width = (e.loaded / e.total * 100) + "%";
    };
    xhr.onload = () => {
      el.progress.classList.add("hidden");
      if (xhr.status === 401) { showLogin(); return; }
      if (xhr.status >= 200 && xhr.status < 300) { toast(t("uploaded", { n: files.length })); load(cwd); }
      else if (xhr.status === 507) { toast(t("noSpace"), true); }
      else { toast(t("error"), true); }
    };
    xhr.onerror = () => { el.progress.classList.add("hidden"); toast(t("error"), true); };
    xhr.send(fd);
  }

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
