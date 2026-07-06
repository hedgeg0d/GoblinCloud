// Minimal i18n scaffold. English only for now; add a language by dropping a new
// key set into TRANSLATIONS and it becomes selectable. Detection order:
// localStorage("lang") -> navigator.language -> "en".
(function () {
  const TRANSLATIONS = {
    en: {
      login: "Log in",
      password: "Password",
      badPassword: "Wrong password.",
      newFolder: "New folder",
      upload: "Upload",
      name: "Name",
      size: "Size",
      modified: "Modified",
      emptyFolder: "This folder is empty.",
      dropHere: "Drop files to upload",
      download: "Download",
      rename: "Rename",
      delete: "Delete",
      confirmDelete: "Delete “{name}”?",
      newFolderName: "New folder name",
      renameTo: "Rename to",
      uploading: "Uploading…",
      uploadingN: "{n} files",
      uploaded: "Uploaded {n} file(s)",
      deleted: "Deleted",
      renamed: "Renamed",
      folderCreated: "Folder created",
      error: "Something went wrong",
      noSpace: "No storage space left",
    },
    // Add more languages here, e.g. `ru: { ... }`.
  };

  const AVAILABLE = Object.keys(TRANSLATIONS);

  function pick() {
    const saved = localStorage.getItem("lang");
    if (saved && AVAILABLE.includes(saved)) return saved;
    const nav = (navigator.language || "en").slice(0, 2);
    return AVAILABLE.includes(nav) ? nav : "en";
  }

  let lang = pick();

  function t(key, vars) {
    let s = (TRANSLATIONS[lang] && TRANSLATIONS[lang][key]) || TRANSLATIONS.en[key] || key;
    if (vars) for (const k in vars) s = s.replace("{" + k + "}", vars[k]);
    return s;
  }

  function setLang(l) {
    if (AVAILABLE.includes(l)) {
      lang = l;
      localStorage.setItem("lang", l);
      apply();
    }
  }

  // Replace text/placeholder for any element carrying data-i18n / data-i18n-ph.
  function apply() {
    document.documentElement.lang = lang;
    document.querySelectorAll("[data-i18n]").forEach((el) => {
      el.textContent = t(el.getAttribute("data-i18n"));
    });
    document.querySelectorAll("[data-i18n-ph]").forEach((el) => {
      el.setAttribute("placeholder", t(el.getAttribute("data-i18n-ph")));
    });
  }

  window.I18N = { t, setLang, apply, available: AVAILABLE, current: () => lang };
})();
