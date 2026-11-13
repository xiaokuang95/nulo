/*
 * nulo desktop — interactive prototype.
 *
 * Everything here is design-fixture behaviour: sessions, approvals, diffs and
 * context meters are driven locally so the shell can be reviewed without the
 * Go host. The real desktop host is expected to consume sdk.Session events.
 */
(function () {
  "use strict";

  const DATA = window.NULO_DATA;
  const clone = (value) =>
    typeof structuredClone === "function"
      ? structuredClone(value)
      : JSON.parse(JSON.stringify(value));

  const projects = clone(DATA.projects);
  const sessions = clone(DATA.sessions);

  for (const session of Object.values(sessions)) {
    if (!Array.isArray(session.reviewComments)) session.reviewComments = [];
    if (session.changes && Array.isArray(session.changes.files)) {
      for (const file of session.changes.files) {
        if (typeof file.reviewed !== "boolean") file.reviewed = Boolean(session.changes.reviewed);
        if (!file.hunkDecisions) file.hunkDecisions = {};
        file.staged = Boolean(file.staged);
      }
      session.changes.reviewed = session.changes.files.every((file) => file.reviewed);
    }
    session.checkpoints = Array.isArray(session.checkpoints) ? session.checkpoints : null;
    session.lastCommit = session.lastCommit || null;
  }

  const STORAGE_KEY = "nulo.desktop.prototype.v1";
  const codeSnippets = new Map();
  let codeSnippetCounter = 0;

  const PERMISSIONS = {
    manual: {
      label: "Manual",
      desc: "Ask before file edits and commands.",
    },
    "auto-edits": {
      label: "Auto edits",
      desc: "Apply file edits, ask before commands.",
    },
    plan: {
      label: "Plan only",
      desc: "Read-only. No writes or commands.",
    },
    auto: {
      label: "Auto",
      desc: "Apply edits and run safe commands.",
    },
  };

  const EFFORT_LEVELS = ["off", "low", "medium", "high"];

  const SLASH_COMMANDS = [
    { id: "plan", name: "/plan", desc: "Open the session plan", icon: "list-checks" },
    { id: "review", name: "/review", desc: "Review file changes", icon: "diff" },
    { id: "terminal", name: "/terminal", desc: "Open the terminal dock", icon: "terminal" },
    { id: "model", name: "/model", desc: "Switch model or thinking level", icon: "sparkles" },
    { id: "compact", name: "/compact", desc: "Compact the session context", icon: "compress" },
    { id: "settings", name: "/settings", desc: "Open settings", icon: "settings" },
    { id: "clear", name: "/clear", desc: "Clear the composer", icon: "x" },
  ];

  const state = {
    theme: "moss",
    density: "comfortable",
    motion: "full",
    timestamps: true,
    reasoning: true,
    sidebarOpen: true,
    dockOpen: true,
    autoClosedSidebar: false,
    autoClosedDock: false,
    lastViewportWidth: null,
    dockTab: "changes",
    dockWidth: null,
    activeId: "s1",
    welcome: false,
    projectForNew: "nulo",
    filter: "all",
    query: "",
    collapsedProjects: new Set(),
    selectedFile: {},
    model: DATA.defaultModel,
    thinking: DATA.defaultThinking,
    approval: DATA.defaultApproval,
    compacting: false,
    popover: null,
    paletteOpen: false,
    paletteQuery: "",
    paletteIndex: 0,
    settingsOpen: false,
    settingsSection: "general",
    settingsReturnFocus: null,
    dialogMode: null,
    dialogReturnFocus: null,
    slashOpen: false,
    slashIndex: 0,
    modelQuery: "",
    mentionOpen: false,
    mentionIndex: 0,
    mentionQuery: "",
    mentionStart: -1,
    commentDraft: null,
    reviewCounter: 0,
    messageEdit: null,
    rewindSessionId: null,
    rewindIndex: 0,
    rewindReturnFocus: null,
    attachments: [],
    attachmentCounter: 0,
    sessionCounter: 0,
    timers: new Map(),
  };

  const dom = {
    app: document.getElementById("app"),
    workbench: document.getElementById("workbench"),
    titlebar: document.querySelector(".titlebar"),
    titlebarProject: document.querySelector(".titlebar-project"),
    titlebarBranch: document.querySelector(".titlebar-branch"),
    toggleSidebarBtn: document.getElementById("toggleSidebarBtn"),
    toggleDockBtn: document.getElementById("toggleDockBtn"),
    sidebar: document.getElementById("sidebar"),
    newSessionBtn: document.getElementById("newSessionBtn"),
    sessionSearch: document.getElementById("sessionSearch"),
    filterRow: document.getElementById("filterRow"),
    projectList: document.getElementById("projectList"),
    capabilitiesBtn: document.getElementById("capabilitiesBtn"),
    sidebarSettingsBtn: document.getElementById("sidebarSettingsBtn"),
    sessionbar: document.getElementById("sessionbar"),
    conversationScroll: document.getElementById("conversationScroll"),
    conversation: document.getElementById("conversation"),
    composerDock: document.getElementById("composerDock"),
    composer: document.getElementById("composer"),
    composerInput: document.getElementById("composerInput"),
    composerMeta: document.getElementById("composerMeta"),
    approvalChip: document.getElementById("approvalChip"),
    approvalPopover: document.getElementById("approvalPopover"),
    envChip: document.getElementById("envChip"),
    envPopover: document.getElementById("envPopover"),
    modelChip: document.getElementById("modelChip"),
    modelChipLabel: document.getElementById("modelChipLabel"),
    modelPopover: document.getElementById("modelPopover"),
    ctxPill: document.getElementById("ctxPill"),
    ctxPillLabel: document.getElementById("ctxPillLabel"),
    ctxRing: document.getElementById("ctxRing"),
    sendBtn: document.getElementById("sendBtn"),
    attachBtn: document.getElementById("attachBtn"),
    fileInput: document.getElementById("fileInput"),
    slashMenu: document.getElementById("slashMenu"),
    mentionMenu: document.getElementById("mentionMenu"),
    attachmentTray: document.getElementById("attachmentTray"),
    dock: document.getElementById("dock"),
    dockHeader: document.getElementById("dockHeader"),
    dockBody: document.getElementById("dockBody"),
    dockResizer: document.getElementById("dockResizer"),
    paletteOverlay: document.getElementById("paletteOverlay"),
    paletteInput: document.getElementById("paletteInput"),
    paletteList: document.getElementById("paletteList"),
    settingsOverlay: document.getElementById("settingsOverlay"),
    settingsNav: document.getElementById("settingsNav"),
    settingsContent: document.getElementById("settingsContent"),
    settingsClose: document.getElementById("settingsClose"),
    settingsDone: document.getElementById("settingsDone"),
    dialogOverlay: document.getElementById("dialogOverlay"),
    dialogEyebrow: document.getElementById("dialogEyebrow"),
    dialogTitle: document.getElementById("dialogTitle"),
    dialogDesc: document.getElementById("dialogDesc"),
    dialogField: document.getElementById("dialogField"),
    dialogFieldLabel: document.querySelector("#dialogField > span"),
    dialogInput: document.getElementById("dialogInput"),
    dialogClose: document.getElementById("dialogClose"),
    dialogCancel: document.getElementById("dialogCancel"),
    dialogConfirm: document.getElementById("dialogConfirm"),
    rewindOverlay: document.getElementById("rewindOverlay"),
    rewindList: document.getElementById("rewindList"),
    rewindDesc: document.getElementById("rewindDesc"),
    rewindFiles: document.getElementById("rewindFiles"),
    rewindFilesRow: document.getElementById("rewindFilesRow"),
    rewindClose: document.getElementById("rewindClose"),
    rewindCancel: document.getElementById("rewindCancel"),
    rewindConfirm: document.getElementById("rewindConfirm"),
    toastStack: document.getElementById("toastStack"),
  };

  const icon = (name, cls = "") =>
    `<svg class="icon${cls ? " " + cls : ""}" aria-hidden="true"><use href="#i-${name}"></use></svg>`;

  const esc = (value) =>
    String(value == null ? "" : value).replace(/[&<>"']/g, (ch) => ({
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    }[ch]));

  function inlineMd(text) {
    const codes = [];
    let out = text.replace(/`([^`]+)`/g, (_, code) => {
      codes.push(`<code>${code}</code>`);
      return `\u0001${codes.length - 1}\u0001`;
    });
    out = out
      .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
      .replace(/(^|[\s(])\*([^*\n]+)\*/g, "$1<em>$2</em>")
      .replace(
        /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g,
        '<a href="$2" target="_blank" rel="noreferrer">$1</a>'
      )
      .replace(/\u0001(\d+)\u0001/g, (_, index) => codes[Number(index)]);
    return out;
  }

  function mdToHtml(md) {
    let text = esc(String(md || "").trim());
    const codeBlocks = [];
    text = text.replace(/```([\w+-]*)\n([\s\S]*?)```/g, (_, lang, body) => {
      codeSnippetCounter += 1;
      codeSnippets.set(String(codeSnippetCounter), body.replace(/\n$/, ""));
      codeBlocks.push(
        `<div class="code-block"><div class="code-head"><span>${lang || "text"}</span><button class="code-copy" type="button" data-copy-code="${codeSnippetCounter}">${icon("copy")}copy</button></div><pre><code>${body.replace(/\n$/, "")}</code></pre></div>`
      );
      return `\n@@CODE${codeBlocks.length - 1}@@\n`;
    });

    const lines = text.split(/\r?\n/);
    const out = [];
    let paragraph = [];
    let list = null;

    const flushParagraph = () => {
      if (paragraph.length) {
        out.push(`<p>${inlineMd(paragraph.join(" "))}</p>`);
        paragraph = [];
      }
    };
    const closeList = () => {
      if (list) {
        out.push(`</${list}>`);
        list = null;
      }
    };

    for (const rawLine of lines) {
      const line = rawLine.trimEnd();
      if (!line.trim()) {
        flushParagraph();
        closeList();
        continue;
      }
      const codeMatch = line.trim().match(/^@@CODE(\d+)@@$/);
      if (codeMatch) {
        flushParagraph();
        closeList();
        out.push(codeBlocks[Number(codeMatch[1])]);
        continue;
      }
      if (/^###\s+/.test(line)) {
        flushParagraph();
        closeList();
        out.push(`<h3>${inlineMd(line.replace(/^###\s+/, ""))}</h3>`);
        continue;
      }
      if (/^##\s+/.test(line)) {
        flushParagraph();
        closeList();
        out.push(`<h2>${inlineMd(line.replace(/^##\s+/, ""))}</h2>`);
        continue;
      }
      if (/^#\s+/.test(line)) {
        flushParagraph();
        closeList();
        out.push(`<h1>${inlineMd(line.replace(/^#\s+/, ""))}</h1>`);
        continue;
      }
      if (/^---+\s*$/.test(line)) {
        flushParagraph();
        closeList();
        out.push("<hr>");
        continue;
      }
      if (/^[-*]\s+/.test(line)) {
        flushParagraph();
        if (list !== "ul") {
          closeList();
          list = "ul";
          out.push("<ul>");
        }
        out.push(`<li>${inlineMd(line.replace(/^[-*]\s+/, ""))}</li>`);
        continue;
      }
      if (/^\d+\.\s+/.test(line)) {
        flushParagraph();
        if (list !== "ol") {
          closeList();
          list = "ol";
          out.push("<ol>");
        }
        out.push(`<li>${inlineMd(line.replace(/^\d+\.\s+/, ""))}</li>`);
        continue;
      }
      if (/^&gt;\s?/.test(line)) {
        flushParagraph();
        closeList();
        out.push(`<blockquote>${inlineMd(line.replace(/^&gt;\s?/, ""))}</blockquote>`);
        continue;
      }
      paragraph.push(line.trim());
    }

    flushParagraph();
    closeList();
    return out.join("");
  }

  const formatTokens = (value) => {
    const number = Number(value) || 0;
    if (number >= 1000000) return (number / 1000000).toFixed(1).replace(".0", "") + "M";
    if (number >= 1000) {
      const digits = number >= 100000 ? 0 : 1;
      return (number / 1000).toFixed(digits).replace(".0", "") + "k";
    }
    return String(number);
  };

  const formatPercent = (value) => {
    const number = Math.max(0, Math.min(999, Number(value) || 0));
    return number < 10 ? number.toFixed(1).replace(".0", "") : String(Math.round(number));
  };

  function modelInfo(spec) {
    const [providerId, ...rest] = String(spec || "").split("/");
    const modelId = rest.join("/");
    const provider = DATA.providers.find((item) => item.id === providerId) || null;
    const model = provider
      ? provider.models.find((item) => item.id === modelId) || null
      : null;
    return {
      provider,
      model,
      providerId,
      modelId,
      label: model ? model.name : modelId || "No model",
      context: model ? model.context : 128000,
      maxOut: model ? model.maxOut : 32768,
    };
  }

  const modelLabel = (spec) => modelInfo(spec).label;

  const clock = () => {
    const date = new Date();
    return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
  };

  const activeSession = () =>
    state.welcome ? null : sessions[state.activeId] || null;

  const projectById = (id) => projects.find((item) => item.id === id) || null;

  const projectForSession = (session) =>
    session ? projectById(session.project) : projectById(state.projectForNew) || projects[0];

  const activeProject = () => projectForSession(activeSession());

  const parentSession = (session) => (session && session.parent ? sessions[session.parent] || null : null);

  function treeRows(list) {
    const visible = new Set(list.map((session) => session.id));
    const children = new Map();
    const roots = [];
    for (const session of list) {
      const parentId =
        session.parent && visible.has(session.parent) && session.parent !== session.id
          ? session.parent
          : null;
      if (parentId) {
        if (!children.has(parentId)) children.set(parentId, []);
        children.get(parentId).push(session);
      } else {
        roots.push(session);
      }
    }
    const rows = [];
    const seen = new Set();
    const walk = (session, depth) => {
      if (seen.has(session.id)) return;
      seen.add(session.id);
      rows.push({ session, depth });
      for (const child of children.get(session.id) || []) walk(child, depth + 1);
    };
    roots.forEach((session) => walk(session, 0));
    for (const session of list) if (!seen.has(session.id)) walk(session, 0);
    return rows;
  }

  const currentModelSpec = () => {
    const session = activeSession();
    return session ? session.model : state.model;
  };

  const currentThinking = () => {
    const session = activeSession();
    return session ? session.thinking : state.thinking;
  };

  const currentApproval = () => {
    const session = activeSession();
    return session ? session.approval : state.approval;
  };

  function hunkGroups(file) {
    const rows = parseDiff(file && file.diff);
    const groups = [];
    let current = null;
    rows.forEach((row, rowIndex) => {
      if (row.type === "hunk" || !current) {
        current = {
          index: groups.length,
          header: row.type === "hunk" ? row.text : "Changes",
          rows: [],
          add: 0,
          del: 0,
        };
        groups.push(current);
        if (row.type === "hunk") return;
      }
      if (row.type === "add") current.add += 1;
      if (row.type === "del") current.del += 1;
      current.rows.push({ row, rowIndex });
    });
    return groups;
  }

  function hunkDecision(file, index) {
    return (file && file.hunkDecisions && file.hunkDecisions[index]) || "";
  }

  function effectiveFileStats(file) {
    const stats = { add: Number(file.add) || 0, del: Number(file.del) || 0 };
    const decisions = file.hunkDecisions || {};
    if (Object.keys(decisions).length) {
      const groups = hunkGroups(file);
      for (const group of groups) {
        if (decisions[group.index] !== "revert") continue;
        stats.add -= group.add;
        stats.del -= group.del;
      }
    }
    return {
      add: Math.max(0, stats.add),
      del: Math.max(0, stats.del),
      staged: Boolean(file.staged),
    };
  }

  function changeStats(session) {
    if (!session || !session.changes || !session.changes.files) {
      return { files: 0, add: 0, del: 0, reviewed: 0, staged: 0, comments: 0 };
    }
    const comments = (session.reviewComments || []).length;
    return session.changes.files.reduce(
      (total, file) => {
        const effective = effectiveFileStats(file);
        return {
          files: total.files + 1,
          add: total.add + effective.add,
          del: total.del + effective.del,
          reviewed: total.reviewed + (file.reviewed ? 1 : 0),
          staged: total.staged + (effective.staged ? 1 : 0),
          comments: total.comments + (file.comments || 0),
        };
      },
      { files: 0, add: 0, del: 0, reviewed: 0, staged: 0, comments }
    );
  }

  function allFilesReviewed(session) {
    return Boolean(
      session &&
        session.changes &&
        Array.isArray(session.changes.files) &&
        session.changes.files.every((file) => file.reviewed)
    );
  }

  function fileComments(session, path) {
    return (session.reviewComments || []).filter((comment) => comment.file === path);
  }

  function syncReviewSummary() {
    const session = activeSession();
    if (!session || !session.changes) return;
    const stats = changeStats(session);
    const summary = dom.conversation.querySelector(".changes-card .changes-foot .changes-stats");
    if (summary) summary.textContent = `${stats.reviewed}/${stats.files} reviewed`;
  }

  function findApproval(session, id) {
    if (!session) return null;
    for (const message of session.messages) {
      if (!message.blocks) continue;
      for (const block of message.blocks) {
        if (block.type === "approval" && (!id || block.id === id)) return block;
      }
    }
    return null;
  }

  function pendingApproval(session) {
    if (!session) return null;
    for (const message of session.messages) {
      if (!message.blocks) continue;
      for (const block of message.blocks) {
        if (block.type === "approval" && block.status === "pending") return block;
      }
    }
    return null;
  }

  function contextFor(session) {
    if (session) return session.context;
    const info = modelInfo(state.model);
    return {
      used: 0,
      limit: info.context,
      breakdown: [],
      entries: [],
      skills: 0,
      mcp: 0,
      plugins: 0,
    };
  }

  function statusInfo(session) {
    if (!session) return { label: "Ready", icon: "play", cls: "done" };
    if (session.status === "awaiting") return { label: "Needs approval", icon: "shield", cls: "awaiting" };
    if (session.status === "running") return { label: "Running", icon: "spinner", cls: "running" };
    if (session.status === "paused") return { label: "Paused", icon: "pause", cls: "paused" };
    return { label: "Completed", icon: "check", cls: "done" };
  }

  function messagePlainText(message) {
    if (!message) return "";
    if (message.text) return message.text;
    return (message.blocks || [])
      .map((block) => {
        if (block.type === "text") return block.md || "";
        if (block.type === "thinking") return block.text || "";
        if (block.type === "approval") return block.command || "";
        if (block.type === "tool") return `${block.title || ""}\n${block.output || ""}`;
        return "";
      })
      .filter(Boolean)
      .join("\n\n");
  }

  function copyText(text, note = "Copied to clipboard") {
    const fallback = () => {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy");
        toast(note);
      } catch (error) {
        toast("Clipboard is unavailable in this browser", "warn", "alert");
      }
      area.remove();
    };
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => toast(note)).catch(fallback);
    } else {
      fallback();
    }
  }

  function toast(message, type = "info", iconName = "check") {
    if (!dom.toastStack) return;
    const node = document.createElement("div");
    node.className = `toast${type !== "info" ? " " + type : ""}`;
    node.setAttribute("role", "status");
    node.innerHTML = `
      <span class="toast-icon">${icon(iconName)}</span>
      <span class="toast-message">${esc(message)}</span>
      <button class="toast-close" aria-label="Dismiss">${icon("x")}</button>
    `;
    const remove = () => {
      if (!node.isConnected) return;
      node.classList.add("is-leaving");
      window.setTimeout(() => node.remove(), 150);
    };
    node.querySelector(".toast-close").addEventListener("click", remove);
    dom.toastStack.appendChild(node);
    while (dom.toastStack.children.length > 3) {
      dom.toastStack.firstElementChild.remove();
    }
    window.setTimeout(remove, 3800);
  }

  function loadPrefs() {
    let prefs = {};
    try {
      prefs = JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}") || {};
    } catch (error) {
      prefs = {};
    }
    if (prefs.theme === "paper" || prefs.theme === "moss") state.theme = prefs.theme;
    if (prefs.density === "compact" || prefs.density === "comfortable") state.density = prefs.density;
    if (prefs.motion === "reduced" || prefs.motion === "full") state.motion = prefs.motion;
    if (typeof prefs.timestamps === "boolean") state.timestamps = prefs.timestamps;
    if (typeof prefs.reasoning === "boolean") state.reasoning = prefs.reasoning;
    if (typeof prefs.sidebarOpen === "boolean") state.sidebarOpen = prefs.sidebarOpen;
    if (typeof prefs.dockOpen === "boolean") state.dockOpen = prefs.dockOpen;
    if (["changes", "plan", "terminal", "context"].includes(prefs.dockTab)) state.dockTab = prefs.dockTab;
    const width = Number(prefs.dockWidth);
    if (width >= 320 && width <= 620) state.dockWidth = width;
    if (modelInfo(prefs.model).model) state.model = prefs.model;
    if (EFFORT_LEVELS.includes(prefs.thinking)) state.thinking = prefs.thinking;
    if (PERMISSIONS[prefs.approval]) state.approval = prefs.approval;

    if (!localStorage.getItem(STORAGE_KEY)) {
      if (window.innerWidth <= 1180) state.dockOpen = false;
      if (window.innerWidth <= 900) state.sidebarOpen = false;
    }
  }

  function savePrefs() {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          theme: state.theme,
          density: state.density,
          motion: state.motion,
          timestamps: state.timestamps,
          reasoning: state.reasoning,
          sidebarOpen: state.sidebarOpen,
          dockOpen: state.dockOpen,
          dockTab: state.dockTab,
          dockWidth: state.dockWidth,
          model: state.model,
          thinking: state.thinking,
          approval: state.approval,
        })
      );
    } catch (error) {
      /* Preferences are optional in the prototype. */
    }
  }

  function applyShell() {
    const root = document.documentElement;
    root.dataset.theme = state.theme;
    root.dataset.density = state.density;
    root.dataset.motion = state.motion;
    root.dataset.timestamps = state.timestamps ? "on" : "off";
    root.dataset.reasoning = state.reasoning ? "on" : "off";
    dom.app.classList.toggle("sidebar-collapsed", !state.sidebarOpen);
    dom.app.classList.toggle("dock-collapsed", !state.dockOpen);
    dom.toggleSidebarBtn.classList.toggle("is-on", state.sidebarOpen);
    dom.toggleDockBtn.classList.toggle("is-on", state.dockOpen);
    if (state.dockWidth && state.dockOpen) {
      dom.workbench.style.setProperty("--dock-w", `${state.dockWidth}px`);
    } else {
      dom.workbench.style.removeProperty("--dock-w");
    }
  }

  /* ----------------------------------------------------------------------
     Rendering primitives
     ---------------------------------------------------------------------- */

  function parseDiff(lines) {
    const rows = [];
    let oldNo = 0;
    let newNo = 0;
    for (const line of lines || []) {
      const hunk = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (hunk) {
        oldNo = Number(hunk[1]);
        newNo = Number(hunk[2]);
        rows.push({ type: "hunk", text: line, old: "", new: "" });
        continue;
      }
      if (line.startsWith("\\")) continue;
      if (line.startsWith("+")) {
        rows.push({ type: "add", text: line.slice(1), old: "", new: newNo });
        newNo += 1;
        continue;
      }
      if (line.startsWith("-")) {
        rows.push({ type: "del", text: line.slice(1), old: oldNo, new: "" });
        oldNo += 1;
        continue;
      }
      rows.push({
        type: "ctx",
        text: line.startsWith(" ") ? line.slice(1) : line,
        old: oldNo,
        new: newNo,
      });
      oldNo += 1;
      newNo += 1;
    }
    return rows;
  }

  function statusPill(session) {
    if (!session) {
      return `<span class="status-pill done">${icon("play")}Ready</span>`;
    }
    const status = statusInfo(session);
    const marker =
      status.icon === "spinner"
        ? '<span class="spinner" aria-hidden="true"></span>'
        : icon(status.icon);
    const tag = "span";
    const attrs = session.status === "awaiting" ? ' data-scroll-approval="true" role="button" tabindex="0"' : "";
    return `<${tag} class="status-pill ${status.cls}"${attrs}>${marker}${status.label}</${tag}>`;
  }

  function stepMarker(step, index) {
    if (step.status === "done") return icon("check");
    if (step.status === "blocked") return icon("x");
    if (step.status === "active") return "";
    return String(index + 1);
  }

  function renderPlanCard(plan) {
    if (!plan) return "";
    const done = plan.steps.filter((step) => step.status === "done").length;
    const total = plan.steps.length;
    const percent = total ? Math.round((done / total) * 100) : 0;
    const steps = plan.steps
      .map((step, index) => `
        <div class="plan-step ${esc(step.status || "pending")}">
          <span class="step-marker">${stepMarker(step, index)}</span>
          <span class="step-copy">
            <span class="step-title">${esc(step.title)}</span>
            ${step.detail ? `<span class="step-detail">${esc(step.detail)}</span>` : ""}
          </span>
          ${step.duration ? `<span class="step-duration">${esc(step.duration)}</span>` : ""}
        </div>
      `)
      .join("");
    return `
      <section class="plan-card">
        <header class="plan-head">
          ${icon("list-checks")}
          <span class="plan-title">${esc(plan.title || "Plan")}</span>
          <span class="plan-progress">${done} / ${total}</span>
        </header>
        <div class="plan-bar"><span style="width:${percent}%"></span></div>
        <div class="plan-steps">${steps}</div>
        ${plan.next ? `<div class="plan-next">${icon("arrow-right")}<span>${esc(plan.next)}</span></div>` : ""}
      </section>
    `;
  }

  function renderSubagent(block) {
    return `
      <section class="subagent" data-subagent>
        <button class="subagent-head" type="button" aria-expanded="false">
          <span class="subagent-icon">${icon("sparkles")}</span>
          <span class="subagent-name">${esc(block.name || "scout")}</span>
          <span class="subagent-role">${esc(block.agent || "Explore")}</span>
          <span class="subagent-summary">${esc(block.summary || "")}</span>
          <span class="subagent-status">${icon("check")}${esc(block.duration || "done")}</span>
          ${icon("chevron-down", "subagent-chevron")}
        </button>
        <div class="subagent-body">
          ${(block.details || [])
            .map((detail) => `<div class="subagent-detail">${esc(detail)}</div>`)
            .join("")}
        </div>
      </section>
    `;
  }

  function toolIcon(tool) {
    return {
      read: "file",
      bash: "terminal",
      edit: "diff",
      search: "search",
      task: "layers",
    }[tool] || "code";
  }

  function renderTool(block) {
    const status = block.status || "done";
    const statusLabel =
      status === "running"
        ? '<span class="spinner" aria-hidden="true"></span>running'
        : status === "fail"
          ? `${icon("x")}failed`
          : `${icon("check")}done`;
    return `
      <section class="tool-card${block.open ? " is-open" : ""}">
        <button class="tool-head" type="button" aria-expanded="${block.open ? "true" : "false"}">
          <span class="tool-icon">${icon(toolIcon(block.tool))}</span>
          <span class="tool-title">${esc(block.title || block.tool || "tool")}</span>
          ${block.meta ? `<span class="tool-meta">${esc(block.meta)}</span>` : ""}
          <span class="tool-status ${esc(status)}">${statusLabel}</span>
          ${icon("chevron-down", "tool-chevron")}
        </button>
        ${block.output ? `<pre class="tool-output">${esc(block.output)}</pre>` : ""}
        ${
          status === "fail"
            ? `<div class="tool-recovery">
                 <span>${icon("alert")}${esc(block.error || "This tool call did not finish.")}</span>
                 <button class="text-link" type="button" data-retry-tool="${esc(block.title || "")}">${icon("refresh")}Retry</button>
               </div>`
            : ""
        }
      </section>
    `;
  }

  function renderApproval(block) {
    const resolved = block.status && block.status !== "pending";
    if (resolved) {
      const approved = block.status === "approved";
      return `
        <section class="approval-card is-resolved">
          <div class="approval-head">
            <span class="approval-icon">${icon(approved ? "check" : "x")}</span>
            <span class="approval-copy">
              <span class="approval-kicker">${approved ? "Command approved" : "Command denied"}</span>
              <span class="approval-title">${esc(block.reason || "Command approval")}</span>
            </span>
          </div>
          <div class="approval-command">
            <span class="approval-prompt">❯</span><code>${esc(block.command)}</code>
          </div>
          <div class="approval-resolved ${approved ? "approved" : "denied"}">
            ${icon(approved ? "check" : "x")}
            <span>${approved ? "Approved" : "Denied"}${block.resolvedAt ? " at " + esc(block.resolvedAt) : ""}</span>
            <span class="resolved-note">${esc(block.resolvedAs || "manual")}</span>
          </div>
        </section>
      `;
    }
    return `
      <section class="approval-card" data-approval-card="${esc(block.id)}">
        <div class="approval-head">
          <span class="approval-icon">${icon("shield")}</span>
          <span class="approval-copy">
            <span class="approval-kicker">Approval needed</span>
            <span class="approval-title">Run this command?</span>
            <span class="approval-reason">${esc(block.reason || "")}</span>
          </span>
        </div>
        <div class="approval-command">
          <span class="approval-prompt">❯</span><code>${esc(block.command)}</code>
        </div>
        <div class="approval-risk">
          ${icon("terminal")}<span>${esc(block.cwd || "")}</span>
          <span aria-hidden="true">·</span>
          <span>${esc(block.risk || "runs locally")}</span>
        </div>
        <div class="approval-actions">
          <button class="btn primary" type="button" data-approval="allow" data-approval-id="${esc(block.id)}">
            ${icon("check")}Allow once
          </button>
          <button class="btn ghost" type="button" data-approval="always" data-approval-id="${esc(block.id)}">
            Always allow
          </button>
          <button class="btn danger" type="button" data-approval="deny" data-approval-id="${esc(block.id)}">
            Deny
          </button>
          <span class="spacer"></span>
          <span class="approval-hint">build mode</span>
        </div>
      </section>
    `;
  }

  function renderChangesCard(changes) {
    if (!changes || !changes.files || !changes.files.length) return "";
    const stats = changes.files.reduce(
      (total, file) => {
        const effective = effectiveFileStats(file);
        return {
          add: total.add + effective.add,
          del: total.del + effective.del,
          reviewed: total.reviewed + (file.reviewed ? 1 : 0),
        };
      },
      { add: 0, del: 0, reviewed: 0 }
    );
    const files = changes.files
      .map((file) => {
        const effective = effectiveFileStats(file);
        return `
        <button class="changes-file" type="button" data-open-file="${esc(file.path)}">
          <span class="changes-file-status ${file.status === "A" ? "add" : "mod"}">${esc(file.status)}</span>
          <span class="changes-file-path">${esc(file.path)}</span>
          <span class="changes-file-stat">
            <span class="add">+${effective.add}</span>
            <span class="del">−${effective.del}</span>
          </span>
        </button>
      `;
      })
      .join("");
    return `
      <section class="changes-card">
        <header class="changes-head">
          ${icon("diff")}
          <span class="changes-title">${changes.files.length} file${changes.files.length === 1 ? "" : "s"} changed</span>
          <span class="changes-stats"><span class="add">+${stats.add}</span> <span class="del">−${stats.del}</span></span>
        </header>
        <div class="changes-files">${files}</div>
        <div class="changes-foot">
          <button class="text-link" type="button" data-open-changes>
            Review changes ${icon("arrow-right")}
          </button>
          <span class="spacer"></span>
          <span class="changes-stats">${stats.reviewed}/${changes.files.length} reviewed</span>
        </div>
      </section>
    `;
  }

  function renderNotice(text) {
    return `<div class="notice-line">${icon("info")}<span>${esc(text)}</span></div>`;
  }

  function renderAssistantBlocks(message, session) {
    return (message.blocks || [])
      .map((block) => {
        if (block.type === "thinking") {
          return `
            <details class="thinking">
              <summary>${icon("chevron-right")}Thought for ${esc(block.duration || "a moment")}</summary>
              <div class="thinking-body">${esc(block.text || "")}</div>
            </details>
          `;
        }
        if (block.type === "text") {
          return `<div class="md">${mdToHtml(block.md || "")}</div>`;
        }
        if (block.type === "plan") return renderPlanCard(session.plan);
        if (block.type === "subagent") return renderSubagent(block);
        if (block.type === "tool") return renderTool(block);
        if (block.type === "approval") return renderApproval(block);
        if (block.type === "changes") return renderChangesCard(session.changes);
        if (block.type === "notice") return renderNotice(block.text || "");
        return "";
      })
      .join("");
  }

  function renderMessageAttachments(attachments) {
    if (!attachments || !attachments.length) return "";
    return `
      <div class="message-attachments">
        ${attachments
          .map(
            (file) => `
              <span class="message-attachment">
                ${file.preview ? `<img src="${esc(file.preview)}" alt="">` : icon("file")}
                <span>${esc(file.name)}</span>
                <small>${formatBytes(file.size)}</small>
              </span>
            `
          )
          .join("")}
      </div>
    `;
  }

  function renderMessage(message, session, index) {
    if (!message) return "";
    if (message.role === "notice") {
      return renderNotice(message.text || "");
    }
    if (message.role === "user") {
      const editing =
        state.messageEdit &&
        state.messageEdit.sessionId === session.id &&
        state.messageEdit.index === index;
      return `
        <article class="message user${editing ? " is-editing" : ""}" data-message-index="${index}">
          <div class="user-bubble">
            <div class="user-label">You<span class="user-time">${esc(message.time || "")}</span></div>
            ${
              editing
                ? `<div class="message-edit">
                     <textarea id="messageEditInput" rows="3" aria-label="Edit message">${esc(message.text || "")}</textarea>
                     <div class="message-edit-actions">
                       <span class="message-edit-hint">⌘↵ resend · Esc cancel</span>
                       <button class="btn ghost small" type="button" data-cancel-edit>Cancel</button>
                       <button class="btn primary small" type="button" data-save-edit>${icon("arrow-up")}Save &amp; resend</button>
                     </div>
                   </div>`
                : `<div class="user-text">${esc(message.text || "")}</div>`
            }
            ${renderMessageAttachments(message.attachments)}
          </div>
          <div class="msg-actions">
            <button class="msg-action" type="button" data-copy-message="${index}">${icon("copy")}copy</button>
            ${
              editing
                ? ""
                : `<button class="msg-action" type="button" data-edit-message="${index}">${icon("edit")}edit</button>`
            }
            <button class="msg-action" type="button" data-delete-message="${index}">${icon("trash")}delete</button>
          </div>
        </article>
      `;
    }
    if (message.role !== "assistant") return "";
    return `
      <article class="message assistant" data-message-index="${index}">
        <header class="assistant-head">
          <span class="assistant-name">nulo</span>
          <span class="assistant-sep" aria-hidden="true">·</span>
          <span class="assistant-model">${esc(modelLabel(session.model))}</span>
          <span class="assistant-time">${esc(message.time || "")}</span>
        </header>
        <div class="assistant-body">
          ${renderAssistantBlocks(message, session)}
          <div class="msg-actions">
            <button class="msg-action" type="button" data-copy-message="${index}">${icon("copy")}copy</button>
            <button class="msg-action" type="button" data-regenerate-message="${index}">${icon("refresh")}retry</button>
            <button class="msg-action" type="button" data-branch-message="${index}">${icon("fork")}branch</button>
            <button class="msg-action" type="button" data-delete-message="${index}">${icon("trash")}delete</button>
          </div>
        </div>
      </article>
    `;
  }

  function renderWelcome() {
    const project = activeProject();
    const info = modelInfo(state.model);
    const suggestions = DATA.suggestions
      .map((suggestion, index) => `
        <button class="suggestion" type="button" data-suggestion="${index}">
          <span class="suggestion-icon">${icon(suggestion.icon)}</span>
          <span class="suggestion-copy">
            <span class="suggestion-title">${esc(suggestion.title)}</span>
            <span class="suggestion-desc">${esc(suggestion.desc)}</span>
          </span>
          ${icon("arrow-right", "suggestion-arrow")}
        </button>
      `)
      .join("");
    const recent = (project.sessions || [])
      .map((id) => sessions[id])
      .filter(Boolean)
      .slice(0, 3)
      .map((session) => `
        <button class="recent-row" type="button" data-recent-session="${esc(session.id)}">
          <span class="session-state ${esc(session.status)}" aria-hidden="true"></span>
          <span class="recent-copy">
            <span class="recent-title">${esc(session.title)}</span>
            <span class="recent-meta">${esc(statusInfo(session).label)} · ${esc(session.updated || "")}</span>
          </span>
          <span class="recent-time">${session.changes && session.changes.files ? `${session.changes.files.length} files` : "session"}</span>
          ${icon("chevron-right")}
        </button>
      `)
      .join("");
    return `
      <section class="welcome">
        <div class="welcome-kicker">nulo desktop · ${esc(project.name)}</div>
        <h1 class="welcome-title">What should we build?</h1>
        <p class="welcome-sub">
          A local coding agent for <code>${esc(project.path)}</code>. Sessions, tools,
          approvals, and diffs stay on this machine.
        </p>
        <div class="welcome-chips">
          <span class="welcome-chip accent">${icon("check")}AGENTS.md loaded</span>
          <span class="welcome-chip">${icon("sparkles")}2 skills</span>
          <span class="welcome-chip">${icon("plug")}3 MCP tools</span>
          <span class="welcome-chip">${icon("cpu")}${esc(info.provider ? info.provider.name : "local")}</span>
        </div>
        <div class="suggestions">${suggestions}</div>
        ${
          recent
            ? `<section class="welcome-recent">
                 <div class="welcome-recent-head"><span>RECENT IN ${esc(project.name.toUpperCase())}</span><span>${(project.sessions || []).length} sessions</span></div>
                 <div class="recent-list">${recent}</div>
               </section>`
            : ""
        }
      </section>
    `;
  }

  function renderThreadMeta(session) {
    const project = projectById(session.project);
    const context = session.context;
    const percent = context.limit ? (context.used / context.limit) * 100 : 0;
    const envLabel = session.env === "Worktree" ? session.worktree || "worktree" : session.env || "Local";
    return `
      <div class="thread-meta">
        <span class="meta-item">${icon("folder")}${esc(project ? project.path : "")}</span>
        <span class="meta-item">${icon("git-branch")}${esc(session.branch)}</span>
        <span class="meta-item">${icon("globe")}${esc(envLabel)}</span>
        <span class="meta-item">${icon("clock")}started ${esc(session.started || "just now")}</span>
        <span class="meta-spacer"></span>
        <span class="meta-item meta-model">${esc(modelLabel(session.model))}</span>
        <span class="meta-item">${formatPercent(percent)}% context</span>
      </div>
    `;
  }

  function renderConversation(options = {}) {
    if (state.welcome) {
      dom.conversation.innerHTML = renderWelcome();
      dom.conversation.classList.remove("is-swapping");
      return;
    }
    const session = activeSession();
    if (!session) return;
    const messages = session.messages.map((message, index) => renderMessage(message, session, index)).join("");
    const working =
      session.status === "running"
        ? `<div class="working-line"><span class="spinner" aria-hidden="true"></span><span>nulo is working</span><span class="working-hint">Esc to stop</span></div>`
        : "";
    dom.conversation.innerHTML = `${renderThreadMeta(session)}${messages}${working}`;
    if (options.animate) {
      dom.conversation.classList.remove("is-swapping");
      void dom.conversation.offsetWidth;
      dom.conversation.classList.add("is-swapping");
    }
  }

  function renderTitlebar() {
    const project = activeProject();
    if (project) {
      dom.titlebarProject.textContent = project.name;
      dom.titlebarBranch.textContent = project.branch;
    }
    dom.toggleSidebarBtn.classList.toggle("is-on", state.sidebarOpen);
    dom.toggleDockBtn.classList.toggle("is-on", state.dockOpen);
  }

  function filterCounts() {
    const all = Object.values(sessions);
    return {
      all: all.length,
      active: all.filter((session) => session.status === "running" || session.status === "awaiting").length,
      review: all.filter(
        (session) => session.changes && session.changes.files && session.changes.files.length && !allFilesReviewed(session)
      ).length,
    };
  }

  function renderFilterRow() {
    const counts = filterCounts();
    const items = [
      { id: "all", label: "All", count: counts.all },
      { id: "active", label: "Active", count: counts.active },
      { id: "review", label: "Review", count: counts.review },
    ];
    dom.filterRow.innerHTML = items
      .map(
        (item) => `
          <button class="filter-btn${state.filter === item.id ? " is-active" : ""}" type="button"
            role="tab" aria-selected="${state.filter === item.id}" data-filter="${item.id}">
            ${item.label}<span class="filter-count">${item.count}</span>
          </button>
        `
      )
      .join("");
  }

  function sessionMetaHtml(session) {
    const stats = changeStats(session);
    const parts = [`<span>${esc(session.updated || "")}</span>`];
    if (session.status === "awaiting") {
      parts.push(`<span class="meta-state awaiting">needs approval</span>`);
    } else if (session.status === "running") {
      parts.push(`<span class="meta-state running">running</span>`);
    } else if (session.status === "paused") {
      parts.push(`<span class="meta-state paused">paused</span>`);
    } else if (stats.files) {
      parts.push(`<span>${stats.files} file${stats.files === 1 ? "" : "s"}</span>`);
      parts.push(`<span class="meta-diff-add">+${stats.add}</span>`);
      parts.push(`<span class="meta-diff-del">−${stats.del}</span>`);
    } else if (session.env === "Worktree") {
      parts.push(`<span>worktree</span>`);
    }
    return parts.join('<span class="meta-sep">·</span>');
  }

  function renderSidebar() {
    renderFilterRow();
    const query = state.query.trim().toLowerCase();
    const visible = [];
    for (const project of projects) {
      const list = project.sessions
        .map((id) => sessions[id])
        .filter(Boolean)
        .filter((session) => {
          if (query && !session.title.toLowerCase().includes(query)) return false;
          if (state.filter === "active" && !["running", "awaiting"].includes(session.status)) return false;
          if (
            state.filter === "review" &&
            !(session.changes && session.changes.files && session.changes.files.length && !allFilesReviewed(session))
          ) {
            return false;
          }
          return true;
        });
      if (list.length) visible.push({ project, list });
    }

    if (!visible.length) {
      dom.projectList.innerHTML = `
        <div class="sidebar-empty">
          <strong>No sessions found</strong>
          Clear the filter or start a new session.
        </div>
      `;
      return;
    }

    dom.projectList.innerHTML = visible
      .map(({ project, list }) => {
        const collapsed =
          state.collapsedProjects.has(project.id) && !query && state.filter === "all";
        const rows = treeRows(list)
          .map(({ session, depth }) => {
            const branchCount = list.filter((item) => item.parent === session.id).length;
            return `
            <button class="session-row${state.activeId === session.id && !state.welcome ? " is-active" : ""}${depth ? " is-child" : ""}"
              type="button" data-session="${esc(session.id)}" title="${esc(session.title)}"
              ${depth ? `data-depth="${depth}"` : ""}>
              <span class="session-row-top">
                <span class="session-state ${esc(session.status)}" aria-hidden="true"></span>
                ${session.parent ? `<span class="session-fork-mark" aria-label="Forked session" title="Forked session">${icon("fork")}</span>` : ""}
                <span class="session-title">${esc(session.title)}</span>
                ${branchCount ? `<span class="session-branch-count" title="${branchCount} branch${branchCount === 1 ? "" : "es"}">${icon("fork")}${branchCount}</span>` : ""}
              </span>
              <span class="session-meta">${sessionMetaHtml(session)}</span>
            </button>
          `;
          })
          .join("");
        return `
          <section class="project-group" data-project-group="${esc(project.id)}">
            <div class="project-head">
              <button class="project-toggle" type="button" data-project-toggle="${esc(project.id)}" aria-expanded="${!collapsed}">
                ${icon("chevron-down", `project-chevron${collapsed ? " is-collapsed" : ""}`)}
                <span class="project-name">${esc(project.name)}</span>
                <span class="project-count">${list.length}</span>
              </button>
              <button class="project-new" type="button" data-new-session-project="${esc(project.id)}"
                title="New session in ${esc(project.name)}" aria-label="New session in ${esc(project.name)}">
                ${icon("plus")}
              </button>
            </div>
            ${collapsed ? "" : `<div class="project-sessions">${rows}</div>`}
          </section>
        `;
      })
      .join("");
  }

  function renderSessionbar() {
    const session = activeSession();
    if (!session) {
      const project = activeProject();
      dom.sessionbar.innerHTML = `
        <div class="sessionbar-main">
          <div class="sessionbar-title-row">
            <span class="sessionbar-path">${esc(project.name)}<span class="sessionbar-sep">/</span>${esc(project.branch)}</span>
            <span class="sessionbar-sep">/</span>
            <h1 class="sessionbar-title">New session</h1>
            ${statusPill(null)}
          </div>
        </div>
        <div class="sessionbar-actions">
          <button class="icon-btn" type="button" data-toggle-dock aria-label="Toggle right panel" title="Toggle right panel">
            ${icon("panel-right")}
          </button>
        </div>
      `;
      return;
    }
    const project = projectById(session.project);
    const stats = changeStats(session);
    const parent = parentSession(session);
    const envLabel = session.env === "Worktree" ? session.worktree || "Worktree" : session.env || "Local";
    const lineage = parent
      ? `<button class="crumb-lineage" type="button" data-goto-session="${esc(parent.id)}" title="Forked from ${esc(parent.title)}">${icon("fork")}fork of ${esc(parent.title)}</button>`
      : "";
    const changesButton =
      stats.files > 0
        ? `<button class="changes-pill" type="button" data-open-changes title="Review changes">
             <span class="changes-text">${stats.files} file${stats.files === 1 ? "" : "s"}</span>
             <span class="add">+${stats.add}</span>
             <span class="del">−${stats.del}</span>
           </button>`
        : `<button class="changes-pill is-empty" type="button" data-open-changes title="No file changes yet">
             <span class="changes-text">No changes</span>
           </button>`;
    dom.sessionbar.innerHTML = `
      <div class="sessionbar-main">
        <div class="sessionbar-title-row">
          <span class="sessionbar-path">${esc(project.name)}<span class="sessionbar-sep">/</span>${esc(session.branch)}<span class="sessionbar-sep">·</span><span class="sessionbar-env">${esc(envLabel)}</span></span>
          <span class="sessionbar-sep">/</span>
          <h1 class="sessionbar-title">${esc(session.title)}</h1>
          ${statusPill(session)}
          ${lineage}
        </div>
      </div>
      <div class="sessionbar-actions">
        ${changesButton}
        <div class="session-menu-wrap">
          <button class="icon-btn" type="button" data-session-menu aria-haspopup="menu" aria-expanded="false" aria-label="Session actions" title="Session actions">
            ${icon("more")}
          </button>
          <div class="session-menu-popover" role="menu" hidden>
            <button class="session-menu-item" type="button" role="menuitem" data-session-action="rename">${icon("edit")}Rename</button>
            <button class="session-menu-item" type="button" role="menuitem" data-session-action="fork">${icon("fork")}Fork from here</button>
            <button class="session-menu-item" type="button" role="menuitem" data-session-action="rewind">${icon("corner")}Rewind to checkpoint</button>
            <button class="session-menu-item" type="button" role="menuitem" data-session-action="copy-id">${icon("copy")}Copy session id</button>
            <span class="session-menu-sep"></span>
            <button class="session-menu-item is-danger" type="button" role="menuitem" data-session-action="delete">${icon("trash")}Delete session</button>
          </div>
        </div>
        <button class="icon-btn" type="button" data-rewind aria-label="Rewind to checkpoint" title="Rewind to checkpoint">
          ${icon("corner")}
        </button>
        <button class="icon-btn" type="button" data-fork aria-label="Fork session" title="Fork session">
          ${icon("fork")}
        </button>
        <button class="icon-btn" type="button" data-toggle-dock aria-label="Toggle right panel" title="Toggle right panel · Ctrl J">
          ${icon("panel-right")}
        </button>
      </div>
    `;
  }

  function updateSendState() {
    const session = activeSession();
    const running = session && session.status === "running";
    const hasText = dom.composerInput.value.trim().length > 0;
    dom.sendBtn.classList.toggle("is-running", Boolean(running));
    dom.sendBtn.classList.toggle("is-disabled", !running && !hasText);
    dom.sendBtn.innerHTML = icon(running ? "stop" : "arrow-up");
    dom.sendBtn.setAttribute("aria-label", running ? "Stop nulo" : "Send message");
    dom.sendBtn.title = running ? "Stop · Esc" : "Send · Enter";
  }

  function renderComposerControls() {
    const session = activeSession();
    const info = modelInfo(currentModelSpec());
    const approval = currentApproval();
    const project = activeProject();
    const context = contextFor(session);
    const percent = context.limit ? (context.used / context.limit) * 100 : 0;
    const envLabel = session
      ? session.env === "Worktree"
        ? session.worktree || "Worktree"
        : `${session.env || "Local"} · ${session.branch}`
      : `Local · ${project.branch}`;

    dom.approvalChip.querySelector(".chip-label").textContent = PERMISSIONS[approval].label;
    dom.approvalChip.querySelector(".icon").style.color = approval === "plan" ? "var(--info)" : "";
    dom.envChip.querySelector(".chip-label").textContent = envLabel;
    dom.modelChipLabel.textContent = info.label;
    dom.ctxPillLabel.textContent = `${formatTokens(context.used)} / ${formatTokens(context.limit)}`;
    dom.ctxRing.style.setProperty("--ctx-p", `${Math.min(100, percent)}%`);

    if (session) {
      dom.composerInput.placeholder =
        session.status === "running"
          ? "Steering is available while nulo works…"
          : session.status === "awaiting"
            ? "Approve the command above, or ask a follow-up…"
            : "Ask nulo to build, debug, or explain…";
    } else {
      dom.composerInput.placeholder = "What should we build?";
    }

    const approvalBlock = session ? pendingApproval(session) : null;
    dom.composer.classList.toggle("has-approval", Boolean(approvalBlock));

    const left = [];
    if (approvalBlock) {
      left.push(
        `<button class="meta-approval" type="button" data-scroll-approval>${icon("shield")}1 command awaiting approval</button>`
      );
    } else if (session && session.status === "running") {
      left.push(
        `<span class="meta-working"><span class="spinner" aria-hidden="true"></span>nulo is working</span>`
      );
    } else {
      left.push("<span>Enter send</span>");
      left.push('<span class="meta-sep">·</span>');
      left.push("<span>Shift+Enter newline</span>");
      left.push('<span class="meta-sep">·</span>');
      left.push("<span>/ commands</span>");
      left.push('<span class="meta-sep">·</span>');
      left.push("<span>@ files</span>");
    }
    const right = `<span class="meta-right">${esc(info.provider ? info.provider.name : "local")} · ${esc(currentThinking())} reasoning</span>`;
    dom.composerMeta.innerHTML = `<span class="meta-left">${left.join("")}</span>${right}`;
    renderAttachments();
    updateSendState();
    autoGrowComposer();
  }

  function dockEmpty(iconName, title, desc, action) {
    return `
      <div class="dock-empty">
        <span class="dock-empty-icon">${icon(iconName)}</span>
        <span class="dock-empty-title">${esc(title)}</span>
        <p class="dock-empty-desc">${esc(desc)}</p>
        ${
          action
            ? `<button class="btn ghost small" type="button" data-dock-tab="${esc(action.tab)}">${icon(action.icon || "arrow-right")}${esc(action.label)}</button>`
            : ""
        }
      </div>
    `;
  }

  function renderDockHeader(session) {
    const stats = changeStats(session);
    const context = contextFor(session);
    const percent = context.limit ? (context.used / context.limit) * 100 : 0;
    const done = session && session.plan ? session.plan.steps.filter((step) => step.status === "done").length : 0;
    const total = session && session.plan ? session.plan.steps.length : 0;
    const tabs = [
      { id: "changes", label: "Changes", icon: "diff", count: stats.files || "" },
      { id: "plan", label: "Plan", icon: "list-checks", count: total ? `${done}/${total}` : "" },
      { id: "terminal", label: "Terminal", icon: "terminal", count: "" },
      { id: "context", label: "Context", icon: "layers", count: `${formatPercent(percent)}%` },
    ];
    dom.dockHeader.innerHTML = `
      <div class="dock-tabs" role="tablist" aria-label="Session details">
        ${tabs
          .map(
            (tab) => `
              <button class="dock-tab${state.dockTab === tab.id ? " is-active" : ""}" type="button"
                role="tab" aria-selected="${state.dockTab === tab.id}" data-dock-tab="${tab.id}">
                ${icon(tab.icon)}${tab.label}
                ${tab.count !== "" ? `<span class="dock-tab-count">${esc(tab.count)}</span>` : ""}
              </button>
            `
          )
          .join("")}
      </div>
      <div class="dock-actions">
        <button class="icon-btn" type="button" data-close-dock aria-label="Close right panel" title="Close right panel">
          ${icon("x")}
        </button>
      </div>
    `;
  }

  function renderFileDiff(file, session) {
    if (!file) return "";
    const comments = fileComments(session, file.path);
    const files = session.changes.files;
    const fileIndex = files.findIndex((item) => item.path === file.path);
    const groups = hunkGroups(file);
    const effective = effectiveFileStats(file);
    const rows = groups
      .map((group) => {
        const decision = hunkDecision(file, group.index);
        const head = `
          <div class="diff-hunk-head${decision ? ` is-${decision}` : ""}">
            <span class="diff-hunk-range">${esc(group.header)}</span>
            <span class="diff-hunk-stat"><span class="add">+${group.add}</span><span class="del">−${group.del}</span></span>
            <span class="diff-hunk-actions">
              ${
                decision
                  ? `<span class="diff-hunk-state">${decision === "keep" ? "kept" : "reverted"}</span>
                     <button class="diff-hunk-btn" type="button" data-hunk-action="clear" data-hunk-index="${group.index}" aria-label="Undo hunk decision">undo</button>`
                  : `<button class="diff-hunk-btn" type="button" data-hunk-action="keep" data-hunk-index="${group.index}" title="Keep this hunk applied">keep</button>
                     <button class="diff-hunk-btn" type="button" data-hunk-action="revert" data-hunk-index="${group.index}" title="Discard this hunk">revert</button>`
              }
            </span>
          </div>
        `;
        const body = group.rows
          .map(({ row, rowIndex }) => {
            const label = row.new || row.old || String(rowIndex + 1);
            const rowComments = comments.filter((comment) => comment.row === rowIndex);
            const draft =
              state.commentDraft &&
              state.commentDraft.sessionId === session.id &&
              state.commentDraft.file === file.path &&
              state.commentDraft.row === rowIndex
                ? `<div class="diff-note-composer">
                     <div class="diff-note-head">${icon("message")}<span>Line ${esc(label)}</span></div>
                     <textarea id="diffCommentInput" rows="2" placeholder="Leave a review note…" aria-label="Review note"></textarea>
                     <div class="diff-note-actions">
                       <button class="btn ghost small" type="button" data-cancel-comment>Cancel</button>
                       <button class="btn primary small" type="button" data-save-comment>${icon("check")}Add note</button>
                     </div>
                   </div>`
                : "";
            const notes = rowComments
              .map(
                (comment) => `
                  <div class="diff-note">
                    <span class="diff-note-icon">${icon("message")}</span>
                    <span class="diff-note-copy">
                      <span class="diff-note-meta">line ${esc(comment.label)} · ${esc(comment.time || "")}</span>
                      <span class="diff-note-text">${esc(comment.text)}</span>
                    </span>
                    <button class="diff-note-delete" type="button" data-delete-comment="${esc(comment.id)}" title="Delete note" aria-label="Delete note">${icon("x")}</button>
                  </div>
                `
              )
              .join("");
            return `
              <div class="diff-row-wrap">
                <div class="diff-row ${row.type}">
                  <span class="diff-num">${row.old === "" ? "" : row.old}</span>
                  <span class="diff-num">${row.new === "" ? "" : row.new}</span>
                  <span class="diff-sign">${row.type === "add" ? "+" : row.type === "del" ? "−" : " "}</span>
                  <span class="diff-code">${esc(row.text) || " "}</span>
                  <button class="diff-comment-btn" type="button" data-comment-line="${rowIndex}" data-comment-label="${esc(label)}" title="Add a review note on line ${esc(label)}" aria-label="Add a review note on line ${esc(label)}">${icon("message")}</button>
                </div>
                ${notes}
                ${draft}
              </div>
            `;
          })
          .join("");
        return `<div class="diff-hunk${decision ? ` is-${decision}` : ""}" data-hunk-group="${group.index}">${head}${body}</div>`;
      })
      .join("");
    return `
      <div class="diff-box">
        <div class="diff-head">
          ${icon("file")}
          <span class="diff-path">${esc(file.path)}</span>
          <span class="diff-lang">${esc(file.language || "")}</span>
          <div class="diff-nav" aria-label="File navigation">
            <button class="diff-nav-btn" type="button" data-file-prev title="Previous file" aria-label="Previous file" ${files.length < 2 ? "disabled" : ""}>${icon("chevron-left")}</button>
            <span>${fileIndex + 1}/${files.length}</span>
            <button class="diff-nav-btn" type="button" data-file-next title="Next file" aria-label="Next file" ${files.length < 2 ? "disabled" : ""}>${icon("chevron-right")}</button>
          </div>
        </div>
        <div class="diff-body">${rows}</div>
        <div class="diff-foot">
          <button class="btn ghost small" type="button" data-copy-patch="${esc(file.path)}">${icon("copy")}Copy patch</button>
          <button class="btn ghost small" type="button" data-revert-file="${esc(file.path)}">${icon("corner")}Revert file</button>
          <button class="btn ghost small${file.staged ? " is-staged" : ""}" type="button" data-stage-file="${esc(file.path)}">${file.staged ? `${icon("check")}Staged` : `${icon("plus")}Stage`}</button>
          <span class="spacer"></span>
          <button class="text-link" type="button" data-open-editor>${icon("code")}Open in editor</button>
          <span class="diff-foot-note">${comments.length ? `${comments.length} note${comments.length === 1 ? "" : "s"} · ` : ""}${effective.add} added · ${effective.del} removed</span>
        </div>
      </div>
    `;
  }

  function renderDockChanges(session) {
    if (!session) {
      return dockEmpty("diff", "No changes yet", "Start a session and file edits will land here for review.", {
        tab: "plan",
        icon: "list-checks",
        label: "Open plan",
      });
    }
    if (!session.changes || !session.changes.files || !session.changes.files.length) {
      return dockEmpty("diff", "No file changes", "This session has not touched the working tree yet.", {
        tab: "plan",
        icon: "list-checks",
        label: "Open plan",
      });
    }
    const stats = changeStats(session);
    const allReviewed = allFilesReviewed(session);
    const total = Math.max(1, stats.add + stats.del);
    const addPercent = Math.round((stats.add / total) * 100);
    const selectedPath = state.selectedFile[session.id] || session.changes.files[0].path;
    const selectedFile =
      session.changes.files.find((file) => file.path === selectedPath) || session.changes.files[0];
    state.selectedFile[session.id] = selectedFile.path;
    const approval = pendingApproval(session);
    let banner = "";
    if (approval) {
      banner = `
        <div class="review-banner">
          ${icon("shield")}
          <span><strong>1 command awaiting approval.</strong> Review it in the thread before handoff.</span>
        </div>
      `;
    } else if (!allReviewed) {
      banner = `
        <div class="review-banner">
          ${icon("alert")}
          <span><strong>${stats.files - stats.reviewed} file${stats.files - stats.reviewed === 1 ? "" : "s"} pending review.</strong> Notes and review state stay with this session.</span>
        </div>
      `;
    }
    return `
      <section class="dock-section">
        <div class="changes-summary">
          <div class="changes-summary-top">
            <span class="changes-summary-count">${stats.files} file${stats.files === 1 ? "" : "s"} changed</span>
            <span class="changes-summary-branch">${esc(session.branch)}</span>
          </div>
          <div class="changes-summary-stats">
            <span class="add">+${stats.add}</span>
            <span class="del">−${stats.del}</span>
            <span>·</span>
            <span>${stats.reviewed}/${stats.files} reviewed</span>
          </div>
          <div class="changes-summary-bar">
            <span class="bar-add" style="width:${addPercent}%"></span>
            <span class="bar-del" style="width:${100 - addPercent}%"></span>
          </div>
          <div class="commit-bar">
            <span class="commit-copy">
              <span>${stats.staged}/${stats.files} staged</span>
              ${
                session.lastCommit
                  ? `<span class="commit-receipt" title="${esc(session.lastCommit.message)}">${icon("check")}${session.lastCommit.files} file${session.lastCommit.files === 1 ? "" : "s"} to ${esc(session.lastCommit.branch)} · ${esc(session.lastCommit.time)}</span>`
                  : ""
              }
            </span>
            <button class="btn ${stats.staged ? "primary" : "ghost"} small" type="button" data-commit-changes ${stats.staged ? "" : "disabled"}>
              ${icon("check")}Commit…
            </button>
          </div>
          ${banner}
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Files</span>
          <span class="spacer"></span>
          <span class="dock-section-note">${stats.reviewed}/${stats.files} reviewed</span>
        </div>
        <div class="file-list">
          ${session.changes.files
            .map((file) => {
              const notes = fileComments(session, file.path).length;
              const effective = effectiveFileStats(file);
              return `
                <div class="file-row${file.path === selectedFile.path ? " is-active" : ""}${file.reviewed ? " is-reviewed" : ""}"
                  role="button" tabindex="0" data-file="${esc(file.path)}" aria-label="Open diff for ${esc(file.path)}">
                  <button class="file-review${file.reviewed ? " is-done" : ""}" type="button"
                    data-review-file="${esc(file.path)}" title="${file.reviewed ? "Mark unreviewed" : "Mark reviewed"}"
                    aria-label="${file.reviewed ? "Mark unreviewed" : "Mark reviewed"}">${icon("check")}</button>
                  <span class="file-status ${file.status === "A" ? "add" : "mod"}">${esc(file.status)}</span>
                  <span class="file-path">${esc(file.path)}</span>
                  ${notes ? `<span class="file-comments" title="${notes} review note${notes === 1 ? "" : "s"}">${icon("message")}${notes}</span>` : ""}
                  ${file.staged ? `<span class="file-staged" title="Staged for commit">${icon("check")}</span>` : ""}
                  <span class="file-stats"><span class="add">+${effective.add}</span><span class="del">−${effective.del}</span></span>
                </div>
              `;
            })
            .join("")}
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Diff</span>
          <span class="spacer"></span>
          <span class="dock-section-note">${esc(selectedFile.language || "")}</span>
        </div>
        ${renderFileDiff(selectedFile, session)}
      </section>
      <section class="dock-section">
        <div class="compact-box">
          <div class="compact-copy">
            <span class="compact-title">${allReviewed ? "Reviewed" : "Ready for review"}</span>
            <span class="compact-desc">${
              allReviewed
                ? "Every file was marked reviewed. Nothing else is pending."
                : "Inspect the diff, add notes, then mark each file reviewed before handoff."
            }</span>
          </div>
          <button class="btn ${allReviewed ? "ghost" : "primary"} small" type="button"
            data-mark-reviewed ${allReviewed ? "disabled" : ""}>
            ${icon("check")}${allReviewed ? "All reviewed" : "Mark all reviewed"}
          </button>
        </div>
      </section>
    `;
  }

  function stepReviewFile(direction) {
    const session = activeSession();
    if (!session || !session.changes || !session.changes.files.length) return;
    const files = session.changes.files;
    const current = files.findIndex((file) => file.path === state.selectedFile[session.id]);
    const target = files[(current + direction + files.length) % files.length];
    if (!target) return;
    state.selectedFile[session.id] = target.path;
    state.commentDraft = null;
    if (!state.dockOpen) {
      state.dockOpen = true;
      applyShell();
      renderTitlebar();
    }
    state.dockTab = "changes";
    renderDock();
  }

  function renderDockPlan(session) {
    if (!session || !session.plan) {
      return dockEmpty("list-checks", "No plan yet", "Ask nulo to outline the work. Steps and progress stay here.", {
        tab: "changes",
        icon: "diff",
        label: "Open changes",
      });
    }
    const plan = session.plan;
    const done = plan.steps.filter((step) => step.status === "done").length;
    const total = plan.steps.length;
    const percent = total ? Math.round((done / total) * 100) : 0;
    return `
      <section class="dock-section">
        <div class="plan-summary">
          <span class="plan-ring" style="--progress:${percent}%" aria-hidden="true"></span>
          <span class="plan-ring-copy">
            <span class="plan-ring-title">${esc(plan.title || "Plan")}</span>
            <span class="plan-ring-sub">${done} of ${total} steps complete</span>
          </span>
          <span class="plan-ring-value">${percent}<small>%</small></span>
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Steps</span>
          <span class="spacer"></span>
          <span class="dock-section-note">${done}/${total}</span>
        </div>
        <div class="dock-plan-steps">
          ${plan.steps
            .map(
              (step, index) => `
                <div class="dock-step ${esc(step.status || "pending")}">
                  <span class="step-marker">${stepMarker(step, index)}</span>
                  <span class="dock-step-copy">
                    <span class="dock-step-title">${esc(step.title)}</span>
                    ${step.detail ? `<span class="dock-step-detail">${esc(step.detail)}</span>` : ""}
                  </span>
                  ${step.duration ? `<span class="dock-step-meta">${esc(step.duration)}</span>` : ""}
                </div>
              `
            )
            .join("")}
        </div>
        ${plan.next ? `<div class="plan-note"><strong>Next</strong> · ${esc(plan.next)}</div>` : ""}
      </section>
    `;
  }

  function renderTerminalLine(line) {
    if (line.type === "cmd") {
      return `<div class="terminal-line cmd"><span class="prompt">❯</span><span>${esc(line.text)}</span></div>`;
    }
    if (line.type === "wait") {
      return `<div class="terminal-line waiting">${icon("clock")}<span>${esc(line.text)}</span></div>`;
    }
    if (line.type === "run") {
      return `<div class="terminal-line running"><span class="spinner" aria-hidden="true"></span><span>${esc(line.text)}</span></div>`;
    }
    if (line.type === "muted") {
      return `<div class="terminal-line muted">${esc(line.text)}</div>`;
    }
    return `<div class="terminal-line">${esc(line.text)}</div>`;
  }

  function renderDockTerminal(session) {
    if (!session) {
      return dockEmpty("terminal", "No terminal yet", "Start a session to see the commands nulo runs.", {
        tab: "plan",
        icon: "list-checks",
        label: "Open plan",
      });
    }
    const lines = (session.terminal || []).map(renderTerminalLine).join("");
    const project = projectById(session.project);
    return `
      <section class="dock-section">
        <div class="terminal-box">
          <div class="terminal-head">
            <span class="terminal-dots" aria-hidden="true"><i></i><i></i><i></i></span>
            <span class="terminal-tab is-active">${icon("terminal")}zsh</span>
            <span class="spacer"></span>
            <button class="icon-btn" type="button" data-copy-terminal aria-label="Copy terminal contents" title="Copy terminal">
              ${icon("copy")}
            </button>
          </div>
          <div class="terminal-body">
            ${lines}
            <div class="terminal-line"><span class="prompt">❯</span><span class="terminal-cursor" aria-hidden="true"></span></div>
          </div>
          <div class="terminal-foot">
            <span class="prompt">${esc(project ? project.path : "")}</span>
            <span class="spacer"></span>
            <span>read-only · host-owned</span>
          </div>
        </div>
      </section>
      <section class="dock-section">
        <div class="compact-box">
          <div class="compact-copy">
            <span class="compact-title">Host terminal</span>
            <span class="compact-desc">The desktop host attaches its own pty. The prototype mirrors the command stream; it does not execute shell input.</span>
          </div>
          <button class="btn ghost small" type="button" data-open-folder>${icon("external")}Open workspace</button>
        </div>
      </section>
    `;
  }

  function contextEntriesHtml(entries) {
    const kindIcon = {
      instructions: "book",
      file: "file",
      skill: "sparkles",
      mcp: "plug",
    };
    return (entries || [])
      .map(
        (entry) => `
          <div class="context-entry">
            ${icon(kindIcon[entry.kind] || "file")}
            <span class="context-entry-path">${esc(entry.path)}</span>
            <span class="context-entry-kind">${esc(entry.kind || "file")}</span>
            <span class="context-entry-tokens">${formatTokens(entry.tokens)}</span>
          </div>
        `
      )
      .join("");
  }

  function renderDockContext(session) {
    const context = contextFor(session);
    const info = modelInfo(session ? session.model : state.model);
    const percent = context.limit ? (context.used / context.limit) * 100 : 0;
    const over = context.used > context.limit;
    const scale = over ? context.used : context.limit || 1;
    const legend = (context.breakdown || [])
      .map(
        (item) => `
          <div class="legend-item">
            <span class="legend-dot" style="background:${item.color}"></span>
            <span>${esc(item.label)}</span>
            <span class="legend-value">${formatTokens(item.value)}</span>
          </div>
        `
      )
      .join("");
    const bar = (context.breakdown || [])
      .map(
        (item) =>
          `<span style="width:${Math.max(1, (item.value / scale) * 100)}%;background:${item.color}" title="${esc(item.label)}"></span>`
      )
      .join("");
    return `
      <section class="dock-section">
        <div class="context-hero">
          <div class="context-top">
            <span class="context-used">${formatTokens(context.used)}</span>
            <span class="context-limit">/ ${formatTokens(context.limit)} tokens</span>
            <span class="context-percent${over ? " is-over" : ""}">${formatPercent(percent)}%</span>
          </div>
          <div class="context-bar">${bar}</div>
          <div class="context-legend">${legend}</div>
          ${
            over
              ? `<div class="context-warning">${icon("alert")}<span>Context is over the model limit. Compact this session before the next turn.</span></div>`
              : ""
          }
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Loaded context</span>
          <span class="spacer"></span>
          <span class="dock-section-note">${(context.entries || []).length} entries</span>
        </div>
        <div class="context-entries">
          ${contextEntriesHtml(context.entries)}
          ${
            !(context.entries || []).length
              ? '<div class="context-entry"><span class="context-entry-path">No files or skills loaded yet.</span></div>'
              : ""
          }
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Model</span>
        </div>
        <div class="context-model">
          <div class="context-model-row"><span class="context-model-label">provider</span><span class="context-model-value">${esc(info.provider ? info.provider.name : "—")}</span></div>
          <div class="context-model-row"><span class="context-model-label">model</span><span class="context-model-value">${esc(info.label)}</span></div>
          <div class="context-model-row"><span class="context-model-label">context</span><span class="context-model-value">${formatTokens(info.context)} · max ${formatTokens(info.maxOut)} out</span></div>
          <div class="context-model-row"><span class="context-model-label">thinking</span><span class="context-model-value">${esc(session ? session.thinking : state.thinking)}</span></div>
          <div class="context-model-row"><span class="context-model-label">capabilities</span><span class="context-model-value">${info.model && info.model.reasoning ? "reasoning" : "text"}${info.model && info.model.image ? " · image" : ""}</span></div>
        </div>
      </section>
      <section class="dock-section">
        <div class="compact-box">
          <div class="compact-copy">
            <span class="compact-title">Compaction</span>
            <span class="compact-desc">Manual only. Compaction keeps the recent window and summarizes the rest before the next request.</span>
          </div>
          <button class="btn ghost small" type="button" data-compact ${state.compacting ? "disabled" : ""}>
            ${state.compacting ? '<span class="spinner"></span>Compacting…' : `${icon("compress")}Compact session`}
          </button>
        </div>
      </section>
    `;
  }

  function renderWelcomeDock() {
    const project = activeProject();
    const info = modelInfo(state.model);
    return `
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Workspace</span>
          <span class="spacer"></span>
          <span class="dock-section-note">local</span>
        </div>
        <div class="context-model">
          <div class="context-model-row"><span class="context-model-label">path</span><span class="context-model-value">${esc(project.path)}</span></div>
          <div class="context-model-row"><span class="context-model-label">branch</span><span class="context-model-value">${esc(project.branch)}</span></div>
          <div class="context-model-row"><span class="context-model-label">runtime</span><span class="context-model-value">local · cgo</span></div>
          <div class="context-model-row"><span class="context-model-label">config</span><span class="context-model-value">~/.nulo/agent</span></div>
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Readiness</span>
        </div>
        <div class="context-entries">
          <div class="context-entry">${icon("check")}<span class="context-entry-path">Provider connected</span><span class="context-entry-kind">${esc(info.provider ? info.provider.name : "local")}</span><span class="status-chip ok">ready</span></div>
          <div class="context-entry">${icon("book")}<span class="context-entry-path">AGENTS.md</span><span class="context-entry-kind">loaded</span><span class="status-chip ok">ready</span></div>
          <div class="context-entry">${icon("sparkles")}<span class="context-entry-path">Skills</span><span class="context-entry-kind">2</span><span class="status-chip ok">ready</span></div>
          <div class="context-entry">${icon("server")}<span class="context-entry-path">MCP servers</span><span class="context-entry-kind">3</span><span class="status-chip ok">ready</span></div>
        </div>
      </section>
      <section class="dock-section">
        <div class="dock-section-head">
          <span class="dock-section-title">Loaded context</span>
        </div>
        <div class="context-entries">
          <div class="context-entry">${icon("book")}<span class="context-entry-path">AGENTS.md</span><span class="context-entry-kind">instructions</span><span class="context-entry-tokens">2.1k</span></div>
          <div class="context-entry">${icon("sparkles")}<span class="context-entry-path">2 skills</span><span class="context-entry-kind">skills</span><span class="context-entry-tokens">1.1k</span></div>
          <div class="context-entry">${icon("plug")}<span class="context-entry-path">3 MCP tools</span><span class="context-entry-kind">mcp</span><span class="context-entry-tokens">—</span></div>
          <div class="context-entry">${icon("cpu")}<span class="context-entry-path">${esc(info.label)}</span><span class="context-entry-kind">model</span><span class="context-entry-tokens">${formatTokens(info.context)}</span></div>
        </div>
      </section>
      <section class="dock-section">
        <div class="compact-box">
          <div class="compact-copy">
            <span class="compact-title">New session</span>
            <span class="compact-desc">The first message starts a JSONL session under ~/.nulo/agent/sessions. Nothing is written until you send.</span>
          </div>
        </div>
      </section>
    `;
  }

  function renderDock() {
    const session = activeSession();
    renderDockHeader(session);
    let body = "";
    if (state.welcome) {
      if (state.dockTab === "context") body = renderWelcomeDock();
      else if (state.dockTab === "changes") body = renderDockChanges(null);
      else if (state.dockTab === "plan") body = renderDockPlan(null);
      else body = renderDockTerminal(null);
    } else if (state.dockTab === "changes") {
      body = renderDockChanges(session);
    } else if (state.dockTab === "plan") {
      body = renderDockPlan(session);
    } else if (state.dockTab === "terminal") {
      body = renderDockTerminal(session);
    } else {
      body = renderDockContext(session);
    }
    dom.dockBody.innerHTML = body;
  }

  /* ----------------------------------------------------------------------
     Command palette
     ---------------------------------------------------------------------- */

  function paletteGroups() {
    const groups = [
      {
        label: "Actions",
        items: [
          { id: "new-session", label: "New session", sub: "Start a local coding session", hint: "Ctrl N", icon: "plus", run: () => startWelcome() },
          { id: "toggle-sidebar", label: "Toggle sidebar", sub: "Sessions and projects", hint: "Ctrl B", icon: "panel-left", run: () => toggleSidebar() },
          { id: "toggle-dock", label: "Toggle right panel", sub: "Changes, plan, terminal, context", hint: "Ctrl J", icon: "panel-right", run: () => toggleDock() },
          { id: "review", label: "Review changes", sub: "Open the diff dock", hint: "", icon: "diff", run: () => openDockTab("changes") },
          { id: "plan", label: "Open plan", sub: "Steps and progress", hint: "", icon: "list-checks", run: () => openDockTab("plan") },
          { id: "terminal", label: "Open terminal", sub: "Commands run by this session", hint: "", icon: "terminal", run: () => openDockTab("terminal") },
          { id: "compact", label: "Compact context", sub: "Summarize the older window", hint: "", icon: "compress", run: () => compactActiveSession() },
          { id: "fork", label: "Fork current session", sub: "Branch from the current leaf", hint: "", icon: "fork", run: () => forkActiveSession() },
          { id: "theme", label: "Toggle theme", sub: state.theme === "moss" ? "Switch to paper" : "Switch to moss", hint: "", icon: state.theme === "moss" ? "sun" : "moon", run: () => setTheme(state.theme === "moss" ? "paper" : "moss") },
        ],
      },
      {
        label: "Sessions",
        items: Object.values(sessions).map((session) => ({
          id: `session-${session.id}`,
          label: session.title,
          sub: `${(projectById(session.project) || {}).name || session.project} · ${statusInfo(session).label}`,
          hint: session.updated,
          icon: "message",
          sessionId: session.id,
          run: () => switchSession(session.id),
        })),
      },
      {
        label: "Settings",
        items: [
          { id: "settings", label: "Settings", sub: "General, providers, appearance", hint: "Ctrl ,", icon: "settings", run: () => openSettings("general") },
          { id: "providers", label: "Providers & models", sub: `${DATA.providers.length} providers`, hint: "", icon: "cpu", run: () => openSettings("providers") },
          { id: "shortcuts", label: "Keyboard shortcuts", sub: "All bindings", hint: "Ctrl /", icon: "keyboard", run: () => openSettings("shortcuts") },
          { id: "about", label: "About nulo", sub: `Version ${DATA.meta.version}`, hint: "", icon: "info", run: () => openSettings("about") },
        ],
      },
    ];
    const query = state.paletteQuery.trim().toLowerCase();
    if (!query) return groups;
    return groups
      .map((group) => ({
        ...group,
        items: group.items.filter((item) =>
          `${group.label} ${item.label} ${item.sub}`.toLowerCase().includes(query)
        ),
      }))
      .filter((group) => group.items.length);
  }

  function renderPalette() {
    const groups = paletteGroups();
    const flat = groups.flatMap((group) => group.items);
    state.paletteItems = flat;
    if (state.paletteIndex >= flat.length) state.paletteIndex = Math.max(0, flat.length - 1);
    if (!flat.length) {
      dom.paletteList.innerHTML = `<div class="palette-empty"><strong>No results</strong>Try a session name, action, or setting.</div>`;
      return;
    }
    let index = 0;
    dom.paletteList.innerHTML = groups
      .map(
        (group) => `
          <div class="palette-group">
            <div class="palette-group-title">${esc(group.label)}</div>
            ${group.items
              .map((item) => {
                const itemIndex = index++;
                const selected = itemIndex === state.paletteIndex;
                return `
                  <button class="palette-item${selected ? " is-selected" : ""}" type="button"
                    data-palette-index="${itemIndex}">
                    <span class="palette-item-icon">${icon(item.icon)}</span>
                    <span class="palette-item-main">
                      <span class="palette-item-title">${esc(item.label)}</span>
                      <span class="palette-item-sub">${esc(item.sub)}</span>
                    </span>
                    ${item.hint ? `<span class="palette-item-hint">${esc(item.hint)}</span>` : ""}
                  </button>
                `;
              })
              .join("")}
          </div>
        `
      )
      .join("");
  }

  function setPaletteIndex(index) {
    const flat = state.paletteItems || [];
    if (!flat.length) return;
    state.paletteIndex = (index + flat.length) % flat.length;
    dom.paletteList.querySelectorAll(".palette-item").forEach((item) => {
      const selected = Number(item.dataset.paletteIndex) === state.paletteIndex;
      item.classList.toggle("is-selected", selected);
    });
    const selected = dom.paletteList.querySelector(".palette-item.is-selected");
    if (selected) selected.scrollIntoView({ block: "nearest" });
  }

  function runPaletteItem(index) {
    const item = (state.paletteItems || [])[index];
    if (!item) return;
    closePalette();
    item.run();
  }

  function openPalette() {
    if (state.settingsOpen) closeSettings();
    closePopover();
    state.paletteOpen = true;
    state.paletteQuery = "";
    state.paletteIndex = 0;
    dom.paletteOverlay.hidden = false;
    dom.paletteInput.value = "";
    renderPalette();
    window.setTimeout(() => dom.paletteInput.focus(), 20);
  }

  function closePalette() {
    state.paletteOpen = false;
    dom.paletteOverlay.hidden = true;
    dom.sessionSearch.focus({ preventScroll: true });
  }

  /* ----------------------------------------------------------------------
     Settings
     ---------------------------------------------------------------------- */

  const SETTINGS_SECTIONS = [
    { id: "general", label: "General", icon: "settings" },
    { id: "providers", label: "Providers & models", icon: "cpu", badge: String(DATA.providers.length) },
    { id: "capabilities", label: "Capabilities", icon: "plug", badge: "3" },
    { id: "appearance", label: "Appearance", icon: "sun" },
    { id: "shortcuts", label: "Shortcuts", icon: "keyboard" },
    { id: "about", label: "About", icon: "info" },
  ];

  function renderSettingsNav() {
    dom.settingsNav.innerHTML = SETTINGS_SECTIONS.map(
      (section) => `
        <button class="settings-nav-item${state.settingsSection === section.id ? " is-active" : ""}"
          type="button" data-settings-section="${section.id}">
          ${icon(section.icon)}
          <span>${esc(section.label)}</span>
          ${section.badge ? `<span class="nav-badge">${esc(section.badge)}</span>` : ""}
        </button>
      `
    ).join("");
  }

  function modelOptions(selected) {
    return DATA.providers
      .map(
        (provider) => `
          <optgroup label="${esc(provider.name)}">
            ${provider.models
              .map(
                (model) =>
                  `<option value="${esc(provider.id + "/" + model.id)}"${provider.id + "/" + model.id === selected ? " selected" : ""}>${esc(model.name)}</option>`
              )
              .join("")}
          </optgroup>
        `
      )
      .join("");
  }

  function renderSettingsGeneral() {
    const info = modelInfo(currentModelSpec());
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>General</h3>
          <p>Defaults for new sessions. The active session keeps its own model, thinking level, and permission mode.</p>
        </div>
        <div class="settings-group">
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Default model</span>
              <span class="setting-desc">${esc(info.provider ? info.provider.name : "—")} · ${formatTokens(info.context)} context</span>
            </span>
            <span class="setting-control wide">
              <select class="select" id="settingsDefaultModel" aria-label="Default model">
                ${modelOptions(currentModelSpec())}
              </select>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Thinking level</span>
              <span class="setting-desc">Reasoning budget for new turns.</span>
            </span>
            <span class="setting-control">
              <select class="select" id="settingsThinking" aria-label="Thinking level">
                ${EFFORT_LEVELS.map(
                  (level) => `<option value="${level}"${level === currentThinking() ? " selected" : ""}>${level}</option>`
                ).join("")}
              </select>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Permission mode</span>
              <span class="setting-desc">Controls edits and command approvals.</span>
            </span>
            <span class="setting-control">
              <select class="select" id="settingsPermission" aria-label="Permission mode">
                ${Object.entries(PERMISSIONS)
                  .map(
                    ([id, item]) =>
                      `<option value="${id}"${id === currentApproval() ? " selected" : ""}>${esc(item.label)}</option>`
                  )
                  .join("")}
              </select>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Session directory</span>
              <span class="setting-desc">JSONL sessions, one tree per file.</span>
            </span>
            <span class="setting-control">
              <code class="readonly-code">${esc(DATA.meta.sessionDir)}</code>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Runtime</span>
              <span class="setting-desc">The host embeds the nulo SDK directly.</span>
            </span>
            <span class="setting-control">
              <code class="readonly-code">${esc(DATA.meta.runtime)}</code>
            </span>
          </div>
        </div>
        <p class="setting-note">The host reads and writes <code>~/.nulo/agent/settings.json</code>. Provider credentials live in <code>models.json</code>.</p>
      </section>
    `;
  }

  function renderSettingsProviders() {
    const current = currentModelSpec();
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>Providers &amp; models</h3>
          <p>Providers own their model lists. The default model is derived from the selected leaf, not stored twice.</p>
        </div>
        <div class="provider-stack">
          ${DATA.providers
            .map((provider) => {
              const isDefaultProvider = provider.models.some((model) => `${provider.id}/${model.id}` === current);
              return `
                <article class="provider-card${isDefaultProvider ? " is-default" : ""}">
                  <header class="provider-head">
                    <span class="provider-icon">${esc(provider.name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase())}</span>
                    <span class="provider-copy">
                      <span class="provider-name">${esc(provider.name)}</span>
                      <span class="provider-url">${esc(provider.baseUrl)} · ${esc(provider.api)}</span>
                    </span>
                    <span class="provider-meta">
                      ${provider.models.length} model${provider.models.length === 1 ? "" : "s"}
                      ${isDefaultProvider ? '<span class="provider-default-tag">default</span>' : ""}
                    </span>
                  </header>
                  <div class="model-table">
                    ${provider.models
                      .map((model) => {
                        const spec = `${provider.id}/${model.id}`;
                        const selected = spec === current;
                        return `
                          <button class="model-table-row${selected ? " is-default" : ""}" type="button" data-set-model="${esc(spec)}">
                            <span class="model-radio" aria-hidden="true"></span>
                            <span class="model-table-name">${esc(model.name)}</span>
                            <span class="model-table-id">${esc(model.id)}</span>
                            <span class="model-table-meta">
                              <span class="model-context">${formatTokens(model.context)}</span>
                              ${model.reasoning ? '<span class="cap-tag">reason</span>' : ""}
                              ${model.image ? '<span class="cap-tag">image</span>' : ""}
                            </span>
                          </button>
                        `;
                      })
                      .join("")}
                  </div>
                </article>
              `;
            })
            .join("")}
        </div>
        <p class="setting-note">This prototype keeps provider credentials read-only. In the host, edit <code>~/.nulo/agent/models.json</code> or use the provider plugin API.</p>
        <div class="setting-control" style="margin-top:11px">
          <button class="btn ghost small" type="button" data-setting-action="copy-models-path">${icon("copy")}Copy models.json path</button>
        </div>
      </section>
    `;
  }

  function renderSettingsCapabilities() {
    const skills = [
      { name: "ponytail", desc: "Minimal-change coding mode", on: true },
      { name: "vision", desc: "Delegate image understanding", on: true },
    ];
    const mcp = [
      { name: "filesystem", desc: "stdio · 12 tools", on: true },
      { name: "github", desc: "streamable HTTP · 8 tools", on: true },
      { name: "browser", desc: "stdio · not connected", on: false },
    ];
    const plugins = [
      { name: "scout", desc: "protocol plugin · subagent", on: true },
      { name: "notify", desc: "shared library · hooks", on: true },
    ];
    const capRow = (item, group, iconName) => `
      <div class="cap-row">
        <span class="cap-icon">${icon(iconName)}</span>
        <span class="cap-copy">
          <span class="cap-name">${esc(item.name)}</span>
          <span class="cap-desc">${esc(item.desc)}</span>
        </span>
        <span class="status-chip ${item.on ? "ok" : "off"}">${item.on ? "on" : "off"}</span>
        <label class="toggle" title="Toggle ${esc(item.name)}">
          <input type="checkbox" data-cap="${esc(group)}:${esc(item.name)}"${item.on ? " checked" : ""}>
          <span class="toggle-track"></span>
        </label>
      </div>
    `;
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>Capabilities</h3>
          <p>Skills, MCP servers, and plugins loaded for this workspace. Toggles are visual only in the prototype.</p>
        </div>
        <div class="cap-group-title">Skills</div>
        <div class="cap-list">${skills.map((item) => capRow(item, "skill", "sparkles")).join("")}</div>
        <div class="cap-group-title">MCP servers</div>
        <div class="cap-list">${mcp.map((item) => capRow(item, "mcp", "server")).join("")}</div>
        <div class="cap-group-title">Plugins</div>
        <div class="cap-list">${plugins.map((item) => capRow(item, "plugin", "package")).join("")}</div>
        <p class="setting-note">Discovery follows <code>~/.nulo/agent/plugins</code>, <code>extensions</code>, and the trusted project's <code>.nulo/plugins</code>.</p>
      </section>
    `;
  }

  function renderSettingsAppearance() {
    const themeOption = (id, name, desc, cls) => `
      <button class="theme-option${state.theme === id ? " is-selected" : ""}" type="button" data-theme-option="${id}">
        <span class="theme-preview ${cls}">
          <span class="theme-preview-sidebar"></span>
          <span class="theme-preview-main">
            <span class="theme-preview-line accent"></span>
            <span class="theme-preview-line"></span>
            <span class="theme-preview-line short"></span>
          </span>
        </span>
        <span class="theme-option-copy">
          <span class="theme-option-name">${esc(name)}</span>
          <span class="theme-option-desc">${esc(desc)}</span>
          <span class="theme-option-check">${icon("check")}</span>
        </span>
      </button>
    `;
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>Appearance</h3>
          <p>The shell stays terminal-adjacent: dense metadata, quiet surfaces, one green accent.</p>
        </div>
        <div class="theme-grid">
          ${themeOption("moss", "Moss", "nulo default", "moss")}
          ${themeOption("paper", "Paper", "light", "paper")}
        </div>
        <div class="settings-group" style="margin-top:14px">
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Density</span>
              <span class="setting-desc">Comfortable gives the transcript more air.</span>
            </span>
            <span class="setting-control">
              <select class="select" id="settingsDensity" aria-label="Density">
                <option value="comfortable"${state.density === "comfortable" ? " selected" : ""}>Comfortable</option>
                <option value="compact"${state.density === "compact" ? " selected" : ""}>Compact</option>
              </select>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Reasoning summaries</span>
              <span class="setting-desc">Show the collapsed "thought for" line.</span>
            </span>
            <span class="setting-control">
              <label class="toggle">
                <input type="checkbox" data-pref="reasoning"${state.reasoning ? " checked" : ""}>
                <span class="toggle-track"></span>
              </label>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Timestamps</span>
              <span class="setting-desc">Show message times in the transcript.</span>
            </span>
            <span class="setting-control">
              <label class="toggle">
                <input type="checkbox" data-pref="timestamps"${state.timestamps ? " checked" : ""}>
                <span class="toggle-track"></span>
              </label>
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-copy">
              <span class="setting-label">Reduce motion</span>
              <span class="setting-desc">Disable pulse, caret, and panel transitions.</span>
            </span>
            <span class="setting-control">
              <label class="toggle">
                <input type="checkbox" data-pref="motion"${state.motion === "reduced" ? " checked" : ""}>
                <span class="toggle-track"></span>
              </label>
            </span>
          </div>
        </div>
      </section>
    `;
  }

  function renderSettingsShortcuts() {
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>Keyboard shortcuts</h3>
          <p>Every action in the shell has a keyboard path. Modifiers work with Ctrl or Cmd.</p>
        </div>
        ${DATA.shortcuts
          .map(
            (group) => `
              <div class="shortcut-group">
                <div class="shortcut-group-title">${esc(group.group)}</div>
                <div class="shortcut-table">
                  ${group.items
                    .map(
                      (item) => `
                        <div class="shortcut-row">
                          <span class="shortcut-label">${esc(item.label)}</span>
                          <span class="shortcut-keys">${item.keys.map((key) => `<kbd>${esc(key)}</kbd>`).join("")}</span>
                        </div>
                      `
                    )
                    .join("")}
                </div>
              </div>
            `
          )
          .join("")}
      </section>
    `;
  }

  function renderSettingsAbout() {
    return `
      <section class="settings-section">
        <div class="settings-section-head">
          <h3>About</h3>
          <p>A desktop host for the nulo session SDK — Pi-compatible, local-first, Go-native.</p>
        </div>
        <div class="about-card">
          <div class="about-brand">
            <span class="logo-mark" aria-hidden="true">n</span>
            <span class="about-brand-copy">
              <span class="about-name">nulo</span>
              <span class="about-tagline">coding agent · desktop prototype</span>
            </span>
            <span class="about-version">v${esc(DATA.meta.version)}</span>
          </div>
          <dl class="about-meta">
            <dt>runtime</dt><dd>${esc(DATA.meta.runtime)}</dd>
            <dt>config root</dt><dd>${esc(DATA.meta.agentDir)}</dd>
            <dt>workspace</dt><dd>${esc(DATA.meta.cwd)}</dd>
            <dt>session store</dt><dd>${esc(DATA.meta.sessionDir)}</dd>
            <dt>SDK surface</dt><dd>CreateAgentSessionServices · Session.Subscribe · Send</dd>
            <dt>references</dt><dd>Codex app · Claude Code desktop · OpenCode</dd>
          </dl>
          <p class="about-note">
            This is a design prototype: approvals, diffs, plans, and context meters are local fixtures.
            The desktop host is expected to render the same surfaces from <code>sdk.Session</code> events.
          </p>
        </div>
        <div class="setting-control" style="margin-top:12px">
          <button class="btn ghost small" type="button" data-setting-action="copy-version">${icon("copy")}Copy version</button>
          <button class="btn ghost small" type="button" data-setting-action="open-design">${icon("external")}Design folder</button>
        </div>
      </section>
    `;
  }

  function renderSettings() {
    renderSettingsNav();
    if (state.settingsSection === "providers") dom.settingsContent.innerHTML = renderSettingsProviders();
    else if (state.settingsSection === "capabilities") dom.settingsContent.innerHTML = renderSettingsCapabilities();
    else if (state.settingsSection === "appearance") dom.settingsContent.innerHTML = renderSettingsAppearance();
    else if (state.settingsSection === "shortcuts") dom.settingsContent.innerHTML = renderSettingsShortcuts();
    else if (state.settingsSection === "about") dom.settingsContent.innerHTML = renderSettingsAbout();
    else dom.settingsContent.innerHTML = renderSettingsGeneral();
  }

  function handleSessionAction(action) {
    const session = activeSession();
    if (!session) return;
    if (action === "rename") openSessionDialog("rename");
    else if (action === "delete") openSessionDialog("delete");
    else if (action === "fork") forkActiveSession();
    else if (action === "rewind") openRewind();
    else if (action === "copy-id") copyText(session.id, "Session id copied");
  }

  function openSessionDialog(mode) {
    const session = activeSession();
    if (!session) return;
    state.dialogMode = mode;
    state.dialogReturnFocus = document.activeElement;
    dom.dialogOverlay.hidden = false;
    if (mode === "commit") {
      const stats = changeStats(session);
      dom.dialogEyebrow.textContent = "Changes";
      dom.dialogTitle.textContent = "Commit staged files";
      dom.dialogDesc.textContent = `${stats.staged} of ${stats.files} file${stats.files === 1 ? "" : "s"} staged on ${session.branch}. Committed files leave the review list and land in the session log.`;
      dom.dialogField.hidden = false;
      dom.dialogFieldLabel.textContent = "Commit message";
      dom.dialogInput.value =
        session.lastCommit && session.lastCommit.message
          ? session.lastCommit.message
          : `Update ${stats.files} file${stats.files === 1 ? "" : "s"}`;
      dom.dialogConfirm.textContent = "Commit";
      dom.dialogConfirm.classList.remove("danger");
      dom.dialogConfirm.classList.add("primary");
      window.setTimeout(() => {
        dom.dialogInput.focus();
        dom.dialogInput.select();
      }, 30);
      return;
    }
    dom.dialogEyebrow.textContent = "Session";
    dom.dialogTitle.textContent = mode === "delete" ? "Delete session" : "Rename session";
    dom.dialogDesc.textContent =
      mode === "delete"
        ? `Delete “${session.title}”? The fixture session and its review notes will be removed. Child branches become roots.`
        : "Give this session a short name. The title is local to this prototype.";
    dom.dialogField.hidden = mode === "delete";
    dom.dialogFieldLabel.textContent = "Session name";
    dom.dialogInput.value = session.title;
    dom.dialogConfirm.textContent = mode === "delete" ? "Delete" : "Save";
    dom.dialogConfirm.classList.toggle("danger", mode === "delete");
    dom.dialogConfirm.classList.toggle("primary", mode !== "delete");
    window.setTimeout(() => {
      if (mode === "rename") {
        dom.dialogInput.focus();
        dom.dialogInput.select();
      } else {
        dom.dialogConfirm.focus();
      }
    }, 30);
  }

  function closeSessionDialog() {
    state.dialogMode = null;
    dom.dialogOverlay.hidden = true;
    if (state.dialogReturnFocus && document.body.contains(state.dialogReturnFocus)) {
      state.dialogReturnFocus.focus({ preventScroll: true });
    }
  }

  function deleteSessionById(id) {
    const session = sessions[id];
    if (!session) return;
    const project = projectById(session.project);
    if (project) project.sessions = project.sessions.filter((sessionId) => sessionId !== id);
    for (const child of Object.values(sessions)) {
      if (child.parent === id) child.parent = null;
    }
    delete sessions[id];
    const wasActive = state.activeId === id;
    if (wasActive) {
      const next = (project && project.sessions.map((sessionId) => sessions[sessionId]).filter(Boolean)[0]) ||
        Object.values(sessions)[0];
      if (next) switchSession(next.id);
      else startWelcome(project ? project.id : projects[0].id);
    } else {
      renderSidebar();
    }
    toast("Session deleted", "warn", "trash");
  }

  function confirmSessionDialog() {
    const session = activeSession();
    if (!session || !state.dialogMode) return;
    if (state.dialogMode === "commit") {
      const message = dom.dialogInput.value.replace(/\s+/g, " ").trim();
      const staged = session.changes && session.changes.files
        ? session.changes.files.filter((file) => file.staged)
        : [];
      if (!message) {
        toast("Write a commit message first", "warn", "alert");
        dom.dialogInput.focus();
        return;
      }
      if (!staged.length) {
        toast("Stage at least one file first", "warn", "alert");
        closeSessionDialog();
        return;
      }
      const count = staged.length;
      session.changes.files = session.changes.files.filter((file) => !file.staged);
      session.changes.reviewed = session.changes.files.length ? allFilesReviewed(session) : false;
      session.lastCommit = { message, files: count, branch: session.branch, time: clock() };
      session.messages.push({
        role: "notice",
        text: `Committed ${count} file${count === 1 ? "" : "s"} to ${session.branch} · ${message}`,
        time: clock(),
      });
      session.updated = "now";
      closeSessionDialog();
      refreshSession(session, { scroll: true });
      toast(`Committed ${count} file${count === 1 ? "" : "s"} to ${session.branch}`, "info", "check");
      return;
    }
    if (state.dialogMode === "rename") {
      const title = dom.dialogInput.value.replace(/\s+/g, " ").trim();
      if (!title) {
        toast("Session name cannot be empty", "warn", "alert");
        dom.dialogInput.focus();
        return;
      }
      session.title = title;
      closeSessionDialog();
      renderSidebar();
      renderSessionbar();
      toast("Session renamed", "info", "edit");
      return;
    }
    const id = session.id;
    closeSessionDialog();
    deleteSessionById(id);
  }

  function openSettings(section = "general") {
    if (state.paletteOpen) closePalette();
    closePopover();
    state.settingsOpen = true;
    state.settingsSection = section;
    state.settingsReturnFocus = document.activeElement;
    dom.settingsOverlay.hidden = false;
    renderSettings();
    dom.settingsContent.scrollTop = 0;
    window.setTimeout(() => dom.settingsClose.focus(), 30);
  }

  function closeSettings() {
    state.settingsOpen = false;
    dom.settingsOverlay.hidden = true;
    if (state.settingsReturnFocus && document.body.contains(state.settingsReturnFocus)) {
      state.settingsReturnFocus.focus({ preventScroll: true });
    }
  }

  /* ----------------------------------------------------------------------
     Navigation and session actions
     ---------------------------------------------------------------------- */

  function scrollToBottom(smooth = false) {
    const node = dom.conversationScroll;
    if (!node) return;
    if (smooth && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      node.scrollTo({ top: node.scrollHeight, behavior: "smooth" });
    } else {
      node.scrollTop = node.scrollHeight;
    }
  }

  function renderAll(options = {}) {
    applyShell();
    renderTitlebar();
    renderSidebar();
    renderSessionbar();
    renderConversation({ animate: Boolean(options.animate) });
    renderComposerControls();
    renderDock();
  }

  function switchSession(id) {
    const session = sessions[id];
    if (!session) return;
    closePopover();
    closeSlashMenu();
    closeMentionMenu();
    clearAttachments(true);
    state.commentDraft = null;
    state.messageEdit = null;
    state.welcome = false;
    state.activeId = id;
    state.projectForNew = session.project;
    renderAll({ animate: true });
    scrollToBottom(false);
  }

  function startWelcome(projectId) {
    closePopover();
    closeSlashMenu();
    closeMentionMenu();
    clearAttachments(true);
    state.commentDraft = null;
    state.messageEdit = null;
    const project = projectById(projectId) || activeProject() || projects[0];
    state.welcome = true;
    state.projectForNew = project.id;
    state.dockTab = "context";
    dom.composerInput.value = "";
    renderAll({ animate: true });
    dom.conversationScroll.scrollTop = 0;
    window.setTimeout(() => dom.composerInput.focus(), 40);
  }

  function toggleSidebar() {
    state.sidebarOpen = !state.sidebarOpen;
    state.autoClosedSidebar = false;
    savePrefs();
    applyShell();
    dom.toggleSidebarBtn.classList.toggle("is-on", state.sidebarOpen);
  }

  function toggleDock() {
    state.dockOpen = !state.dockOpen;
    state.autoClosedDock = false;
    savePrefs();
    applyShell();
    if (state.dockOpen) renderDock();
  }

  function syncResponsiveShell() {
    const width = window.innerWidth;
    const prev = state.lastViewportWidth ?? width;
    state.lastViewportWidth = width;
    let changed = false;

    if (prev > 1180 && width <= 1180) {
      if (state.dockOpen) {
        state.dockOpen = false;
        state.autoClosedDock = true;
        changed = true;
      }
    } else if (prev <= 1180 && width > 1180 && state.autoClosedDock && !state.dockOpen) {
      state.dockOpen = true;
      state.autoClosedDock = false;
      changed = true;
    }

    if (prev > 900 && width <= 900) {
      if (state.sidebarOpen) {
        state.sidebarOpen = false;
        state.autoClosedSidebar = true;
        changed = true;
      }
    } else if (prev <= 900 && width > 900 && state.autoClosedSidebar && !state.sidebarOpen) {
      state.sidebarOpen = true;
      state.autoClosedSidebar = false;
      changed = true;
    }

    if (changed) applyShell();
  }

  function openDockTab(tab) {
    if (!state.dockOpen) {
      state.dockOpen = true;
      savePrefs();
    }
    state.dockTab = tab;
    applyShell();
    renderDock();
  }

  function setTheme(theme, options = {}) {
    if (!["moss", "paper"].includes(theme)) return;
    state.theme = theme;
    savePrefs();
    applyShell();
    if (state.settingsOpen) renderSettings();
    if (!options.silent) toast(theme === "moss" ? "Moss theme" : "Paper theme", "info", theme === "moss" ? "moon" : "sun");
  }

  function setModel(spec, options = {}) {
    const info = modelInfo(spec);
    if (!info.model) return;
    state.model = spec;
    const session = activeSession();
    if (session) {
      session.model = spec;
      session.context.limit = info.context;
    }
    savePrefs();
    renderTitlebar();
    renderSessionbar();
    renderComposerControls();
    renderDock();
    if (state.settingsOpen) renderSettings();
    if (!options.silent) toast(`Model · ${info.label}`, "info", "sparkles");
  }

  function setThinking(level, options = {}) {
    if (!EFFORT_LEVELS.includes(level)) return;
    state.thinking = level;
    const session = activeSession();
    if (session) session.thinking = level;
    savePrefs();
    renderComposerControls();
    if (state.settingsOpen) renderSettings();
    if (!options.silent) toast(`Thinking · ${level}`, "info", "cpu");
  }

  function setApproval(mode, options = {}) {
    if (!PERMISSIONS[mode]) return;
    state.approval = mode;
    const session = activeSession();
    if (session) session.approval = mode;
    savePrefs();
    renderComposerControls();
    if (state.settingsOpen) renderSettings();
    if (!options.silent) toast(`Permissions · ${PERMISSIONS[mode].label}`, "info", "shield");
  }

  function setEnv(mode) {
    const session = activeSession();
    if (!session) {
      toast("Environment is chosen when the session starts", "info", "git-branch");
      return;
    }
    if (mode === "worktree") {
      session.env = "Worktree";
      session.worktree = session.worktree || "nulo-wt-desktop";
      toast("Environment preview · " + session.worktree, "info", "git-branch");
    } else {
      session.env = "Local";
      session.worktree = null;
      toast("Environment · local checkout", "info", "git-branch");
    }
    renderSessionbar();
    renderComposerControls();
    renderDock();
  }

  function openFileInDock(path) {
    const session = activeSession();
    if (!session) return;
    state.selectedFile[session.id] = path;
    state.dockTab = "changes";
    state.dockOpen = true;
    savePrefs();
    applyShell();
    renderDock();
  }

  function clearSessionTimers(session) {
    const list = state.timers.get(session.id);
    if (!list) return;
    list.forEach((timer) => window.clearTimeout(timer));
    state.timers.delete(session.id);
  }

  function afterSession(session, delay, callback) {
    const timer = window.setTimeout(() => {
      callback();
      const list = state.timers.get(session.id);
      if (list) {
        const index = list.indexOf(timer);
        if (index !== -1) list.splice(index, 1);
      }
    }, delay);
    const list = state.timers.get(session.id) || [];
    list.push(timer);
    state.timers.set(session.id, list);
  }

  function refreshSession(session, options = {}) {
    renderTitlebar();
    renderSidebar();
    if (!state.welcome && state.activeId === session.id) {
      renderSessionbar();
      renderConversation();
      renderComposerControls();
      renderDock();
      if (options.scroll) scrollToBottom(true);
    }
  }

  function resolveApproval(decision, approvalId) {
    const session = activeSession();
    if (!session) return;
    const block = findApproval(session, approvalId);
    if (!block || block.status !== "pending") return;
    block.status = decision === "deny" ? "denied" : "approved";
    block.resolvedAt = clock();
    block.resolvedAs = decision === "always" ? "always allowed" : "manual";
    closePopover();

    if (decision === "deny") {
      session.status = "paused";
      const activeStep = session.plan && session.plan.steps.find((step) => step.status === "active");
      if (activeStep) activeStep.status = "blocked";
      if (session.plan) session.plan.next = "Command denied. Verification was skipped.";
      session.terminal = (session.terminal || []).filter((line) => line.type !== "wait");
      session.terminal.push({ type: "muted", text: "command denied · nothing was run" });
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [
          {
            type: "text",
            md:
              "Skipped the test run. I won't claim the SDK suite is green. The shell is still ready to inspect; nothing outside `ui/design/desktop/` was touched.",
          },
        ],
      });
      refreshSession(session, { scroll: true });
      toast("Command denied · no tests were run", "warn", "x");
      return;
    }

    session.status = "running";
    session.terminal = (session.terminal || []).filter((line) => line.type !== "wait");
    session.terminal.push({ type: "run", text: "go test ./sdk/..." });
    const toolBlock = {
      type: "tool",
      tool: "bash",
      title: "go test ./sdk/...",
      meta: "running",
      status: "running",
      output: "=== RUN   TestSessionTree\n=== RUN   TestCompactionProjection\n=== RUN   TestPluginHandshake",
    };
    session.messages.push({ role: "assistant", time: clock(), blocks: [toolBlock] });
    refreshSession(session, { scroll: true });
    toast(decision === "always" ? "Command allowed for this session" : "Command approved · running tests", "info", "shield");

    afterSession(session, 1100, () => {
      toolBlock.status = "done";
      toolBlock.meta = "1.8s";
      toolBlock.output =
        "ok  github.com/xiaokuang95/nulo/sdk  1.842s\nok  github.com/xiaokuang95/nulo/sdk/internal/plugin  0.214s";
      session.terminal = session.terminal.filter((line) => line.type !== "run");
      session.terminal.push({ type: "out", text: "ok  github.com/xiaokuang95/nulo/sdk  1.842s" });
      if (session.plan) {
        const step = session.plan.steps.find((item) => item.status === "active");
        if (step) step.status = "done";
        session.plan.next = "Ready for review. Resize the dock, switch themes, or start a parallel session.";
      }
      session.status = "done";
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [
          {
            type: "text",
            md:
              "Tests pass. The desktop prototype is ready at `ui/design/desktop/index.html`: sessions and projects on the left, the thread in the middle, and a resizable dock for changes, plan, terminal, and context. **No SDK files were touched.**\n\nThe approval, diff review, theme switch, and new-session flows are all wired. Open it at 1440px and drag the dock edge.",
          },
        ],
      });
      refreshSession(session, { scroll: true });
      toast("Tests passed · session ready", "info", "check");
    });
  }

  function retryTool(title) {
    const session = activeSession();
    if (!session) return;
    let block = null;
    for (const message of session.messages) {
      for (const item of message.blocks || []) {
        if (item.type === "tool" && item.status === "fail" && (!title || item.title === title)) {
          block = item;
          break;
        }
      }
      if (block) break;
    }
    if (!block) return;
    block.status = "running";
    block.meta = "retrying";
    block.output = `${block.output || ""}

retrying…`.trim();
    session.status = "running";
    session.updated = "now";
    const blockedStep = session.plan && session.plan.steps.find((step) => step.status === "blocked");
    if (blockedStep) blockedStep.status = "active";
    if (session.plan) session.plan.next = "Retrying the failed tool call.";
    refreshSession(session, { scroll: false });
    toast("Retrying tool call", "info", "refresh");
    afterSession(session, 1200, () => {
      block.status = "done";
      block.meta = "1.2s";
      block.output = "ok  github.com/xiaokuang95/nulo/sdk/internal/codemode  1.184s";
      const step = session.plan && session.plan.steps.find((item) => item.status === "active");
      if (step) {
        step.status = "done";
        step.duration = "1.2s";
      }
      if (session.plan) session.plan.next = "Retry passed. Decide the timeout budget to finish the branch.";
      session.status = "paused";
      session.updated = "now";
      refreshSession(session, { scroll: false });
      toast("Retry passed · one decision left", "info", "check");
    });
  }

  function stopActiveSession() {
    const session = activeSession();
    if (!session || session.status !== "running") return;
    clearSessionTimers(session);
    session.status = "paused";
    for (const message of session.messages) {
      for (const block of message.blocks || []) {
        if (block.type === "tool" && block.status === "running") {
          block.status = "fail";
          block.meta = "stopped";
        }
      }
    }
    session.messages.push({ role: "notice", text: "Generation stopped · session paused", time: clock() });
    refreshSession(session, { scroll: true });
    toast("Stopped · session paused", "warn", "pause");
  }

  function compactActiveSession() {
    const session = activeSession();
    if (!session) {
      toast("Start a session before compacting", "warn", "alert");
      return;
    }
    if (state.compacting) return;
    const before = session.context.used;
    state.compacting = true;
    renderDock();
    toast("Compacting session context…", "info", "compress");
    window.setTimeout(() => {
      const after = Math.max(5400, Math.round(session.context.limit * 0.07));
      session.context.used = after;
      if (session.context.breakdown.length) {
        const total = session.context.breakdown.reduce((sum, item) => sum + item.value, 0) || 1;
        let remaining = after;
        session.context.breakdown.forEach((item, index) => {
          const next =
            index === session.context.breakdown.length - 1
              ? remaining
              : Math.round((item.value / total) * after);
          item.value = Math.max(100, next);
          remaining -= item.value;
        });
      }
      session.messages.push({
        role: "notice",
        text: `Context compacted · ${formatTokens(before)} → ${formatTokens(after)} tokens`,
        time: clock(),
      });
      state.compacting = false;
      renderDock();
      renderComposerControls();
      if (state.activeId === session.id && !state.welcome) renderConversation();
      toast(`Session compacted · ${formatTokens(Math.max(0, before - after))} tokens freed`, "info", "compress");
    }, 950);
  }

  function checkpointLabel(text) {
    const clean = String(text || "").replace(/\s+/g, " ").trim();
    if (!clean) return "Untitled turn";
    return clean.length > 68 ? `${clean.slice(0, 65)}…` : clean;
  }

  function ensureCheckpoints(session) {
    if (Array.isArray(session.checkpoints)) return session.checkpoints;
    session.checkpoints = [];
    session.messages.forEach((message, index) => {
      if (message.role !== "user") return;
      session.checkpoints.push({
        id: `cp-seed-${index}`,
        label: checkpointLabel(message.text),
        time: message.time || "",
        index,
        messages: clone(session.messages.slice(0, index)),
        changes: null,
        plan: null,
        lastCommit: null,
      });
    });
    return session.checkpoints;
  }

  function captureCheckpoint(session, label) {
    if (!session) return;
    ensureCheckpoints(session);
    session.checkpoints.push({
      id: `cp-${Date.now()}-${session.checkpoints.length}`,
      label: checkpointLabel(label),
      time: clock(),
      index: session.messages.length,
      messages: clone(session.messages),
      changes: session.changes ? clone(session.changes) : null,
      plan: session.plan ? clone(session.plan) : null,
      lastCommit: session.lastCommit ? clone(session.lastCommit) : null,
    });
    if (session.checkpoints.length > 24) {
      session.checkpoints.splice(0, session.checkpoints.length - 24);
    }
  }

  function renderRewind() {
    const session = sessions[state.rewindSessionId];
    if (!session) return;
    const checkpoints = ensureCheckpoints(session);
    const rows = [...checkpoints]
      .reverse()
      .map((checkpoint) => {
        const index = checkpoints.indexOf(checkpoint);
        const selected = index === state.rewindIndex;
        const removed = Math.max(0, session.messages.length - checkpoint.messages.length);
        const files = checkpoint.changes && checkpoint.changes.files ? checkpoint.changes.files.length : 0;
        return `
          <button class="rewind-row${selected ? " is-selected" : ""}" type="button" role="option"
            aria-selected="${selected}" data-rewind-checkpoint="${index}">
            <span class="rewind-rail" aria-hidden="true"></span>
            <span class="rewind-copy">
              <span class="rewind-label">${esc(checkpoint.label)}</span>
              <span class="rewind-meta">${esc(checkpoint.time || "earlier")} · ${removed} message${removed === 1 ? "" : "s"} after · ${files ? `${files} file${files === 1 ? "" : "s"} in snapshot` : "no file snapshot"}</span>
            </span>
            <span class="rewind-check">${selected ? icon("check") : ""}</span>
          </button>
        `;
      })
      .join("");
    dom.rewindList.innerHTML = rows || `<div class="rewind-empty">This session has no checkpoints yet. Send a message to create one.</div>`;
    const selected = checkpoints[state.rewindIndex];
    const hasFiles = Boolean(selected && selected.changes);
    dom.rewindFiles.disabled = !hasFiles;
    dom.rewindFiles.checked = hasFiles;
    dom.rewindFilesRow.classList.toggle("is-disabled", !hasFiles);
    dom.rewindDesc.textContent = selected
      ? `Restore the thread to just before “${checkpointLabel(selected.label)}”. ${hasFiles ? "File changes from that checkpoint can be restored too." : "This checkpoint has no file snapshot; the current changes stay as they are."}`
      : "Restore the thread to an earlier turn.";
    dom.rewindConfirm.disabled = !selected;
  }

  function openRewind() {
    const session = activeSession();
    if (!session) return;
    ensureCheckpoints(session);
    if (!session.checkpoints.length) {
      toast("No checkpoints yet · send a message first", "warn", "clock");
      return;
    }
    state.rewindSessionId = session.id;
    state.rewindIndex = session.checkpoints.length - 1;
    const opener = document.activeElement;
    state.rewindReturnFocus =
      opener && opener.offsetParent !== null ? opener : dom.sessionbar.querySelector("[data-rewind]");
    dom.rewindOverlay.hidden = false;
    renderRewind();
    window.setTimeout(() => {
      const selected = dom.rewindList.querySelector(".rewind-row.is-selected");
      if (selected) selected.focus({ preventScroll: true });
      else dom.rewindConfirm.focus();
    }, 30);
  }

  function closeRewind() {
    state.rewindSessionId = null;
    dom.rewindOverlay.hidden = true;
    if (state.rewindReturnFocus && document.body.contains(state.rewindReturnFocus)) {
      state.rewindReturnFocus.focus({ preventScroll: true });
    }
    state.rewindReturnFocus = null;
  }

  function rewindToCheckpoint() {
    const session = sessions[state.rewindSessionId];
    const checkpoint = session && session.checkpoints ? session.checkpoints[state.rewindIndex] : null;
    if (!session || !checkpoint) return;
    const removed = Math.max(0, session.messages.length - checkpoint.messages.length);
    const restoreFiles = dom.rewindFiles.checked && Boolean(checkpoint.changes);
    state.messageEdit = null;
    clearSessionTimers(session);
    session.messages = clone(checkpoint.messages);
    if (restoreFiles) session.changes = clone(checkpoint.changes);
    session.plan = checkpoint.plan ? clone(checkpoint.plan) : null;
    session.lastCommit = checkpoint.lastCommit ? clone(checkpoint.lastCommit) : null;
    session.messages.push({
      role: "notice",
      text: `${removed ? `${removed} message${removed === 1 ? "" : "s"} removed · ` : ""}rewound to checkpoint${restoreFiles ? " · file changes restored" : ""}`,
      time: clock(),
    });
    session.status = "paused";
    session.updated = "now";
    session.checkpoints = session.checkpoints.filter((item) => item.index <= checkpoint.index);
    state.rewindIndex = Math.max(0, session.checkpoints.length - 1);
    closeRewind();
    refreshSession(session, { scroll: true });
    toast(`Rewound${removed ? ` · ${removed} message${removed === 1 ? "" : "s"} removed` : ""}`, "info", "corner");
  }

  function forkSessionAt(session, index, label) {
    if (!session) return;
    if (session.status === "running") {
      toast("Branch opens when the current turn settles", "warn", "alert");
      return;
    }
    state.sessionCounter += 1;
    const id = `fork-${state.sessionCounter}`;
    const copy = clone(session);
    copy.id = id;
    copy.parent = session.id;
    copy.forkedFrom = session.title;
    copy.title = `${session.title} · ${label || "fork"}`;
    copy.messages = copy.messages.slice(0, index + 1);
    copy.checkpoints = [];
    copy.status = "done";
    copy.reviewComments = [];
    copy.lastCommit = null;
    if (copy.changes && Array.isArray(copy.changes.files)) {
      for (const file of copy.changes.files) {
        file.reviewed = false;
        file.staged = false;
      }
      copy.changes.reviewed = false;
    }
    copy.started = "just now";
    copy.updated = "now";
    sessions[id] = copy;
    const project = projectById(copy.project);
    if (project) project.sessions.unshift(id);
    switchSession(id);
    return copy;
  }

  function forkActiveSession() {
    const session = activeSession();
    if (!session) {
      toast("Start a session before forking", "warn", "alert");
      return;
    }
    if (session.status === "running" || session.status === "awaiting") {
      toast("Fork opens when the current turn settles", "warn", "alert");
      return;
    }
    if (!forkSessionAt(session, session.messages.length - 1, "fork")) return;
    toast("Forked from the current leaf", "info", "fork");
  }

  function startMessageEdit(index) {
    const session = activeSession();
    const message = session ? session.messages[index] : null;
    if (!session || !message || message.role !== "user") return;
    if (session.status === "running" || session.status === "awaiting") {
      toast("Edit opens when the current turn settles", "warn", "clock");
      return;
    }
    state.messageEdit = { sessionId: session.id, index };
    renderConversation();
    window.requestAnimationFrame(() => {
      const input = document.getElementById("messageEditInput");
      if (!input) return;
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    });
  }

  function cancelMessageEdit() {
    if (!state.messageEdit) return;
    state.messageEdit = null;
    renderConversation();
  }

  function saveMessageEdit() {
    const session = activeSession();
    const edit = state.messageEdit;
    if (!session || !edit || edit.sessionId !== session.id) return;
    const input = document.getElementById("messageEditInput");
    const text = input ? input.value.trim() : "";
    if (!text) {
      toast("Message cannot be empty", "warn", "alert");
      if (input) input.focus();
      return;
    }
    const message = session.messages[edit.index];
    if (!message) return;
    captureCheckpoint(session, `Before edit: ${checkpointLabel(text)}`);
    session.messages = session.messages.slice(0, edit.index + 1);
    message.text = text;
    message.time = clock();
    state.messageEdit = null;
    session.status = "running";
    session.updated = "now";
    renderSessionbar();
    renderConversation();
    renderDock();
    scrollToBottom(true);
    simulateReply(session, text);
    toast("Message updated · reply regenerating", "info", "refresh");
  }

  function regenerateMessage(index) {
    const session = activeSession();
    if (!session) return;
    if (session.status === "running" || session.status === "awaiting") {
      toast("Retry opens when the current turn settles", "warn", "clock");
      return;
    }
    const message = session.messages[index];
    if (!message || message.role !== "assistant") return;
    let prompt = "";
    for (let cursor = index - 1; cursor >= 0; cursor -= 1) {
      if (session.messages[cursor].role === "user") {
        prompt = session.messages[cursor].text || "";
        break;
      }
    }
    if (!prompt) {
      toast("No prompt to retry from", "warn", "alert");
      return;
    }
    state.messageEdit = null;
    const removed = session.messages.length - index;
    captureCheckpoint(session, `Before retry: ${checkpointLabel(prompt)}`);
    session.messages = session.messages.slice(0, index);
    session.status = "running";
    session.updated = "now";
    renderSessionbar();
    renderConversation();
    renderDock();
    scrollToBottom(true);
    simulateReply(session, prompt);
    toast(`Retrying turn${removed > 1 ? ` · ${removed - 1} later message${removed - 1 === 1 ? "" : "s"} dropped` : ""}`, "info", "refresh");
  }

  function branchFromMessage(index) {
    const session = activeSession();
    const message = session ? session.messages[index] : null;
    if (!session || !message) return;
    state.messageEdit = null;
    if (!forkSessionAt(session, index, "branch")) return;
    toast("Branched from this message", "info", "fork");
  }

  function deleteMessage(index) {
    const session = activeSession();
    const message = session ? session.messages[index] : null;
    if (!session || !message) return;
    if (session.status === "running") {
      toast("Delete opens when the current turn settles", "warn", "clock");
      return;
    }
    state.messageEdit = null;
    captureCheckpoint(session, "Before deleting a message");
    session.messages.splice(index, 1);
    if (session.status === "awaiting" && !pendingApproval(session)) session.status = "paused";
    session.updated = "now";
    renderConversation();
    renderSessionbar();
    renderSidebar();
    renderDock();
    toast("Message removed · rewind to restore it", "info", "trash");
  }

  /* ----------------------------------------------------------------------
     Composer, new sessions, and local simulation
     ---------------------------------------------------------------------- */

  function formatBytes(bytes) {
    const value = Number(bytes) || 0;
    if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
    if (value >= 1024) return `${Math.round(value / 1024)} KB`;
    return `${value} B`;
  }

  function attachmentPayload() {
    return state.attachments.map(({ name, size, type, preview }) => ({ name, size, type, preview }));
  }

  function renderAttachments() {
    const tray = dom.attachmentTray;
    if (!tray) return;
    tray.hidden = !state.attachments.length;
    tray.innerHTML = state.attachments
      .map(
        (file) => `
          <div class="attachment-chip" title="${esc(file.name)}">
            <span class="attachment-thumb">${file.preview ? `<img src="${esc(file.preview)}" alt="">` : icon(file.type.startsWith("image/") ? "eye" : "file")}</span>
            <span class="attachment-copy">
              <span class="attachment-name">${esc(file.name)}</span>
              <span class="attachment-size">${formatBytes(file.size)}</span>
            </span>
            <button class="attachment-remove" type="button" data-remove-attachment="${esc(file.id)}" aria-label="Remove ${esc(file.name)}" title="Remove attachment">${icon("x")}</button>
          </div>
        `
      )
      .join("");
  }

  function addAttachments(fileList) {
    const files = Array.from(fileList || []);
    if (!files.length) return;
    let added = 0;
    for (const file of files) {
      if (state.attachments.length >= 6) {
        toast("Up to 6 attachments per message", "warn", "paperclip");
        break;
      }
      if (file.size > 25 * 1024 * 1024) {
        toast(`${file.name} is larger than 25 MB`, "warn", "alert");
        continue;
      }
      if (state.attachments.some((item) => item.name === file.name && item.size === file.size)) continue;
      state.attachmentCounter += 1;
      state.attachments.push({
        id: `att-${state.attachmentCounter}`,
        name: file.name,
        size: file.size,
        type: file.type || "file",
        preview: file.type && file.type.startsWith("image/") ? URL.createObjectURL(file) : null,
      });
      added += 1;
    }
    if (added) {
      renderAttachments();
      toast(`${added} attachment${added === 1 ? "" : "s"} added`, "info", "paperclip");
    }
  }

  function removeAttachment(id) {
    const file = state.attachments.find((item) => item.id === id);
    if (file && file.preview) URL.revokeObjectURL(file.preview);
    state.attachments = state.attachments.filter((item) => item.id !== id);
    renderAttachments();
  }

  function clearAttachments(revoke = false) {
    if (revoke) {
      for (const file of state.attachments) if (file.preview) URL.revokeObjectURL(file.preview);
    }
    state.attachments = [];
    renderAttachments();
  }

  function autoGrowComposer() {
    const input = dom.composerInput;
    input.style.height = "auto";
    input.style.height = `${Math.min(196, Math.max(58, input.scrollHeight))}px`;
  }

  function sessionTitleFromPrompt(prompt) {
    const clean = prompt.replace(/\s+/g, " ").trim();
    if (!clean) return "New session";
    const title = clean.length > 56 ? clean.slice(0, 56).replace(/\s+\S*$/, "") + "…" : clean;
    return title.charAt(0).toUpperCase() + title.slice(1);
  }

  function createSession(prompt, attachments = []) {
    const project = activeProject() || projects[0];
    const info = modelInfo(state.model);
    state.sessionCounter += 1;
    const id = `local-${state.sessionCounter}`;
    const session = {
      id,
      project: project.id,
      title: sessionTitleFromPrompt(prompt),
      status: "running",
      branch: project.branch,
      env: "Local",
      worktree: null,
      model: state.model,
      thinking: state.thinking,
      approval: state.approval,
      started: "just now",
      updated: "now",
      plan: {
        title: "New session",
        steps: [
          { title: "Map the request", detail: "Read the workspace before changing anything.", status: "active" },
          { title: "Show the interaction", detail: "Keep the change small and reviewable.", status: "pending" },
          { title: "Hand off", detail: "Leave the next step explicit.", status: "pending" },
        ],
        next: "Starting with the workspace.",
      },
      changes: null,
      context: {
        used: 8400,
        limit: info.context,
        breakdown: [
          { label: "System prompt", value: 4200, color: "var(--accent)" },
          { label: "Messages", value: 2600, color: "var(--info)" },
          { label: "Tool results", value: 900, color: "var(--warn)" },
          { label: "Files", value: 700, color: "var(--violet)" },
        ],
        entries: [{ kind: "instructions", path: "AGENTS.md", tokens: 2100 }],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "pwd" },
        { type: "out", text: project.path },
      ],
      messages: [{ role: "user", text: prompt, time: clock(), attachments }],
    };
    sessions[id] = session;
    project.sessions.unshift(id);
    state.welcome = false;
    state.activeId = id;
    state.projectForNew = project.id;
    state.dockTab = "plan";
    state.filter = "all";
    state.query = "";
    dom.sessionSearch.value = "";
    renderAll();
    scrollToBottom(false);
    simulateNewSession(session, prompt);
  }

  function simulateNewSession(session, prompt) {
    afterSession(session, 650, () => {
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [
          {
            type: "thinking",
            duration: "2s",
            text: "Mapping the workspace and choosing the smallest surface that shows the interaction.",
          },
          {
            type: "text",
            md:
              "I'll start by mapping the workspace, then keep the change small enough to review in the dock. This thread is simulated in the browser; the real host will stream the same blocks from `sdk.Session.Subscribe`.",
          },
          { type: "plan" },
        ],
      });
      if (session.plan) {
        session.plan.steps[0].status = "done";
        session.plan.steps[1].status = "active";
        session.plan.next = "Wiring the interaction.";
      }
      refreshSession(session, { scroll: true });
    });

    afterSession(session, 1600, () => {
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [
          {
            type: "tool",
            tool: "read",
            title: "Read 3 files",
            meta: "238 lines",
            status: "done",
            output:
              "README.md           embed the SDK once\nPLAN.md             host consumes sdk only\nui/design/index.html  terminal prototype",
          },
          {
            type: "text",
            md:
              "The workspace is mapped. A desktop host only needs the public `sdk` surface: one session, a subscription for events, and the approval gate in front of tool execution.",
          },
        ],
      });
      refreshSession(session, { scroll: true });
    });

    afterSession(session, 2800, () => {
      const lower = prompt.toLowerCase();
      let final =
        "This is a design prototype — the Go host is not attached, so no tools ran for real. In the desktop host, these blocks come from `sdk.Session.Subscribe`: tool results, approvals, plan updates, and diffs all have a home in this shell.";
      if (lower.includes("review")) {
        final =
          "The review path is the right place to start: the right dock lists every changed file, renders a unified diff, and keeps a pending command visible above the composer. This prototype does not execute the review — it shows where the host should attach the diff.";
      } else if (lower.includes("provider")) {
        final =
          "Providers own their models, and the default model derives from the selected leaf. The settings surface is already laid out that way: one provider row, one nested model list, one default badge. Model selection updates the composer and context meter immediately.";
      } else if (lower.includes("sdk") || lower.includes("explain")) {
        final =
          "The public surface is small on purpose: `CreateAgentSessionServices`, `Runtime.New`, one `Session`, and events over `Subscribe`. The host owns presentation and approvals; the SDK owns the loop, tools, sessions, and model calls. This desktop shell is the presentation layer.";
      }
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [{ type: "text", md: final }],
      });
      if (session.plan) {
        session.plan.steps.forEach((step) => {
          step.status = "done";
        });
        session.plan.next = "Ready for a real host connection.";
      }
      session.status = "done";
      session.updated = "now";
      refreshSession(session, { scroll: true });
      toast("Prototype turn complete", "info", "check");
    });
  }

  function simulateReply(session, prompt) {
    const lower = prompt.toLowerCase();
    afterSession(session, 750, () => {
      let text =
        "This prototype is not attached to the nulo host, so I can't execute tools here. The composer, approvals, review dock, and context meter are wired; in the host this turn would stream from `sdk.Session.Subscribe`.";
      if (lower.includes("test")) {
        text =
          "I'd run the narrowest test first, then widen only if the failure crosses a package boundary. In this prototype the terminal dock mirrors commands but does not execute shell input.";
      } else if (lower.includes("review")) {
        text =
          "The change set lives in the right dock: file list, unified diff, and a review state. Nothing is auto-applied by this prototype; the host owns the approval gate.";
      } else if (lower.includes("theme")) {
        text =
          "Moss is the default; Paper is the light counterpart. Both share the same tokens, so the diff, terminal, and settings surfaces switch together.";
      }
      session.messages.push({
        role: "assistant",
        time: clock(),
        blocks: [{ type: "text", md: text }],
      });
      session.status = "done";
      session.updated = "now";
      refreshSession(session, { scroll: true });
    });
  }


  function submitComposer() {
    const prompt = dom.composerInput.value.trim();
    if (!prompt) {
      toast("Write a message first", "warn", "alert");
      return;
    }
    const session = activeSession();
    const attachments = attachmentPayload();
    dom.composerInput.value = "";
    autoGrowComposer();
    closeSlashMenu();
    closeMentionMenu();
    clearAttachments();
    updateSendState();

    if (state.welcome || !session) {
      createSession(prompt, attachments);
      return;
    }

    captureCheckpoint(session, `Before: ${checkpointLabel(prompt)}`);

    if (session.status === "running") {
      session.messages.push({ role: "user", text: prompt, time: clock(), attachments });
      renderConversation();
      scrollToBottom(true);
      refreshSession(session);
      toast("Steering message queued", "info", "arrow-up");
      return;
    }

    if (session.status === "awaiting") {
      session.messages.push({ role: "user", text: prompt, time: clock(), attachments });
      renderConversation();
      scrollToBottom(true);
      refreshSession(session);
      toast("Queued behind the pending approval", "warn", "clock");
      return;
    }

    session.messages.push({ role: "user", text: prompt, time: clock(), attachments });
    session.status = "running";
    session.updated = "now";
    renderSessionbar();
    renderConversation();
    renderComposerControls();
    renderDock();
    scrollToBottom(true);
    simulateReply(session, prompt);
  }

  /* ----------------------------------------------------------------------
     Composer popovers
     ---------------------------------------------------------------------- */

  function renderModelPopover() {
    const current = currentModelSpec();
    const query = state.modelQuery.trim().toLowerCase();
    const groups = DATA.providers
      .map((provider) => ({
        provider,
        models: provider.models.filter((model) =>
          `${model.name} ${model.id}`.toLowerCase().includes(query)
        ),
      }))
      .filter((group) => group.models.length);
    const list = groups.length
      ? groups
          .map(
            (group) => `
              <div class="model-group">
                <div class="model-group-head"><span class="provider-dot"></span>${esc(group.provider.name)}</div>
                ${group.models
                  .map((model) => {
                    const spec = `${group.provider.id}/${model.id}`;
                    const selected = spec === current;
                    return `
                      <button class="model-row${selected ? " is-selected" : ""}" type="button" data-model="${esc(spec)}">
                        <span class="model-icon">${icon("cpu")}</span>
                        <span class="model-copy">
                          <span class="model-name">${esc(model.name)}</span>
                          <span class="model-id">${esc(model.id)}</span>
                        </span>
                        <span class="model-meta">${formatTokens(model.context)}${model.reasoning ? " · reason" : ""}</span>
                        ${selected ? `<span class="pop-check">${icon("check")}</span>` : ""}
                      </button>
                    `;
                  })
                  .join("")}
              </div>
            `
          )
          .join("")
      : `<div class="palette-empty"><strong>No models found</strong>Try another provider or model id.</div>`;

    dom.modelPopover.innerHTML = `
      <div class="popover-head">
        <span class="popover-icon">${icon("sparkles")}</span>
        <span class="popover-heading">
          <span class="popover-title">Model</span>
          <span class="popover-sub">${DATA.providers.length} providers · ${DATA.providers.reduce((sum, provider) => sum + provider.models.length, 0)} models</span>
        </span>
      </div>
      <div class="pop-search">
        ${icon("search")}
        <input id="modelSearchInput" type="text" placeholder="Filter models" aria-label="Filter models" autocomplete="off" spellcheck="false" value="${esc(state.modelQuery)}">
      </div>
      <div class="popover-body" style="max-height:286px;overflow-y:auto">${list}</div>
      <div class="popover-divider"></div>
      <div class="effort-row">
        <span class="effort-label">Thinking</span>
        ${EFFORT_LEVELS.map(
          (level) => `<button class="effort-btn${level === currentThinking() ? " is-active" : ""}" type="button" data-effort="${level}">${level}</button>`
        ).join("")}
      </div>
      <div class="pop-foot">
        <span class="dock-section-note">${formatTokens(modelInfo(current).context)} context</span>
        <span class="spacer"></span>
        <button class="text-link" type="button" data-manage-providers>Manage providers ${icon("arrow-right")}</button>
      </div>
    `;
    const search = dom.modelPopover.querySelector("#modelSearchInput");
    if (search) {
      search.addEventListener("input", () => {
        state.modelQuery = search.value;
        renderModelPopover();
        const next = dom.modelPopover.querySelector("#modelSearchInput");
        if (next) {
          next.focus();
          next.setSelectionRange(next.value.length, next.value.length);
        }
      });
    }
  }

  function renderPermissionPopover() {
    const current = currentApproval();
    dom.approvalPopover.innerHTML = `
      <div class="popover-head">
        <span class="popover-icon">${icon("shield")}</span>
        <span class="popover-heading">
          <span class="popover-title">Permission mode</span>
          <span class="popover-sub">Controls file edits and command approvals.</span>
        </span>
      </div>
      <div class="popover-body">
        ${Object.entries(PERMISSIONS)
          .map(
            ([id, item]) => `
              <button class="pop-item${id === current ? " is-selected" : ""}" type="button" data-permission="${id}">
                <span class="pop-item-icon">${icon(id === "plan" ? "eye" : id === "manual" ? "shield" : id === "auto" ? "sparkles" : "code")}</span>
                <span class="pop-item-copy">
                  <span class="pop-item-title">${esc(item.label)}</span>
                  <span class="pop-item-desc">${esc(item.desc)}</span>
                </span>
                ${id === current ? `<span class="pop-check">${icon("check")}</span>` : ""}
              </button>
            `
          )
          .join("")}
      </div>
      <div class="pop-foot">
        <span class="dock-section-note">Shift+Tab cycles modes</span>
      </div>
    `;
  }

  function renderEnvPopover() {
    const session = activeSession();
    const current = session ? session.env : "Local";
    dom.envPopover.innerHTML = `
      <div class="popover-head">
        <span class="popover-icon">${icon("git-branch")}</span>
        <span class="popover-heading">
          <span class="popover-title">Environment</span>
          <span class="popover-sub">Where this session runs and writes.</span>
        </span>
      </div>
      <div class="popover-body">
        <button class="pop-item${current !== "Worktree" ? " is-selected" : ""}" type="button" data-env="local">
          <span class="pop-item-icon">${icon("folder")}</span>
          <span class="pop-item-copy">
            <span class="pop-item-title">Local checkout</span>
            <span class="pop-item-desc">${esc((activeProject() || {}).path || "")}</span>
          </span>
          ${current !== "Worktree" ? `<span class="pop-check">${icon("check")}</span>` : ""}
        </button>
        <button class="pop-item${current === "Worktree" ? " is-selected" : ""}" type="button" data-env="worktree">
          <span class="pop-item-icon">${icon("fork")}</span>
          <span class="pop-item-copy">
            <span class="pop-item-title">Isolated worktree</span>
            <span class="pop-item-desc">${session && session.worktree ? esc(session.worktree) : "Create a git worktree for this session"}</span>
          </span>
          ${current === "Worktree" ? `<span class="pop-check">${icon("check")}</span>` : ""}
        </button>
      </div>
      <div class="pop-foot">
        <span class="dock-section-note">Worktree creation is host-owned in the prototype</span>
      </div>
    `;
  }

  function openPopover(name) {
    closePopover();
    closeSlashMenu();
    state.popover = name;
    if (name === "model") {
      state.modelQuery = "";
      renderModelPopover();
      dom.modelPopover.hidden = false;
      dom.modelChip.setAttribute("aria-expanded", "true");
      const search = dom.modelPopover.querySelector("#modelSearchInput");
      if (search) window.setTimeout(() => search.focus(), 20);
    } else if (name === "permission") {
      renderPermissionPopover();
      dom.approvalPopover.hidden = false;
      dom.approvalChip.setAttribute("aria-expanded", "true");
    } else if (name === "env") {
      renderEnvPopover();
      dom.envPopover.hidden = false;
      dom.envChip.setAttribute("aria-expanded", "true");
    }
  }

  function closePopover() {
    state.popover = null;
    dom.modelPopover.hidden = true;
    dom.approvalPopover.hidden = true;
    dom.envPopover.hidden = true;
    dom.modelChip.setAttribute("aria-expanded", "false");
    dom.approvalChip.setAttribute("aria-expanded", "false");
    dom.envChip.setAttribute("aria-expanded", "false");
  }

  function togglePopover(name) {
    if (state.popover === name) closePopover();
    else openPopover(name);
  }

  /* ----------------------------------------------------------------------
     Slash menu
     ---------------------------------------------------------------------- */

  function slashQuery() {
    const value = dom.composerInput.value;
    if (!value.startsWith("/") || /\s/.test(value)) return null;
    return value.slice(1).toLowerCase();
  }

  function filteredSlash() {
    const query = slashQuery();
    if (query == null) return [];
    if (!query) return SLASH_COMMANDS;
    return SLASH_COMMANDS.filter((command) =>
      `${command.name} ${command.desc}`.toLowerCase().includes(query)
    );
  }

  function renderSlashMenu() {
    const commands = filteredSlash();
    if (!commands.length) {
      dom.slashMenu.innerHTML = `
        <div class="slash-head">${icon("terminal")}Slash commands</div>
        <div class="palette-empty"><strong>No command matches</strong>Try /plan, /review, or /model.</div>
      `;
      return;
    }
    if (state.slashIndex >= commands.length) state.slashIndex = 0;
    dom.slashMenu.innerHTML = `
      <div class="slash-head">${icon("terminal")}Slash commands<span style="margin-left:auto">${commands.length}</span></div>
      <div class="slash-list">
        ${commands
          .map(
            (command, index) => `
              <button class="slash-item${index === state.slashIndex ? " is-selected" : ""}" type="button" data-slash-index="${index}">
                <span class="slash-name">${esc(command.name)}</span>
                <span class="slash-desc">${esc(command.desc)}</span>
                ${icon("arrow-right")}
              </button>
            `
          )
          .join("")}
      </div>
      <div class="slash-foot"><span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>↵</kbd> run</span><span><kbd>esc</kbd> close</span></div>
    `;
  }

  function syncSlashMenu() {
    const active = slashQuery() != null;
    if (active && state.popover) closePopover();
    if (!active) {
      closeSlashMenu();
      return;
    }
    if (!state.slashOpen) {
      state.slashOpen = true;
      state.slashIndex = 0;
    }
    dom.slashMenu.hidden = false;
    renderSlashMenu();
  }

  function closeSlashMenu() {
    state.slashOpen = false;
    state.slashIndex = 0;
    dom.slashMenu.hidden = true;
  }

  function mentionContext() {
    const input = dom.composerInput;
    const caret = input.selectionStart == null ? input.value.length : input.selectionStart;
    const before = input.value.slice(0, caret);
    const match = before.match(/(?:^|\s)@([\w./-]*)$/);
    if (!match) return null;
    return {
      query: match[1].toLowerCase(),
      start: caret - match[1].length - 1,
      caret,
    };
  }

  function filteredMentions() {
    const query = (state.mentionQuery || "").toLowerCase();
    return (DATA.files || [])
      .filter((file) => !query || file.path.toLowerCase().includes(query) || (file.language || "").includes(query))
      .slice(0, 9);
  }

  function renderMentionMenu() {
    const files = filteredMentions();
    state.mentionIndex = Math.max(0, Math.min(state.mentionIndex, Math.max(0, files.length - 1)));
    const rows = files.length
      ? files
          .map(
            (file, index) => `
              <button class="mention-item${index === state.mentionIndex ? " is-selected" : ""}" type="button" data-mention-index="${index}" data-mention-path="${esc(file.path)}">
                <span class="mention-icon">${icon("file")}</span>
                <span class="mention-copy">
                  <span class="mention-path">${esc(file.path)}</span>
                  <span class="mention-lang">${esc(file.language || "file")}</span>
                </span>
                ${index === state.mentionIndex ? `<span class="mention-enter">↵</span>` : ""}
              </button>
            `
          )
          .join("")
      : `<div class="mention-empty">No files match that mention.</div>`;
    dom.mentionMenu.innerHTML = `
      <div class="slash-head">${icon("at")}<span>Files · @ mention</span><span class="mention-query">${esc(state.mentionQuery ? "@" + state.mentionQuery : "type a path")}</span></div>
      <div class="mention-list">${rows}</div>
      <div class="slash-foot"><span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>↵</kbd> insert</span><span><kbd>esc</kbd> close</span></div>
    `;
  }

  function syncMentionMenu() {
    const context = mentionContext();
    if (!context) {
      closeMentionMenu();
      return;
    }
    if (state.popover) closePopover();
    closeSlashMenu();
    state.mentionOpen = true;
    state.mentionStart = context.start;
    state.mentionQuery = context.query;
    state.mentionIndex = 0;
    dom.mentionMenu.hidden = false;
    renderMentionMenu();
  }

  function closeMentionMenu() {
    state.mentionOpen = false;
    state.mentionIndex = 0;
    state.mentionQuery = "";
    state.mentionStart = -1;
    if (dom.mentionMenu) dom.mentionMenu.hidden = true;
  }

  function setMentionIndex(index) {
    const files = filteredMentions();
    if (!files.length) return;
    state.mentionIndex = (index + files.length) % files.length;
    renderMentionMenu();
  }

  function insertMention(path) {
    const input = dom.composerInput;
    const caret = input.selectionStart == null ? input.value.length : input.selectionStart;
    const before = input.value.slice(0, state.mentionStart);
    const after = input.value.slice(caret);
    const inserted = `@${path} `;
    input.value = `${before}${inserted}${after}`;
    const next = before.length + inserted.length;
    input.setSelectionRange(next, next);
    input.focus();
    closeMentionMenu();
    autoGrowComposer();
    updateSendState();
  }

  function runSlashCommand(id) {
    dom.composerInput.value = "";
    autoGrowComposer();
    updateSendState();
    closeSlashMenu();
    if (id === "plan") openDockTab("plan");
    else if (id === "review") openDockTab("changes");
    else if (id === "terminal") openDockTab("terminal");
    else if (id === "model") window.setTimeout(() => openPopover("model"), 20);
    else if (id === "compact") compactActiveSession();
    else if (id === "settings") openSettings("general");
    else if (id === "clear") dom.composerInput.focus();
  }

  function runSelectedSlash() {
    const commands = filteredSlash();
    const command = commands[state.slashIndex];
    if (command) runSlashCommand(command.id);
  }

  function setSlashIndex(index) {
    const commands = filteredSlash();
    if (!commands.length) return;
    state.slashIndex = (index + commands.length) % commands.length;
    renderSlashMenu();
  }

  /* ----------------------------------------------------------------------
     Event wiring
     ---------------------------------------------------------------------- */

  function bindEvents() {
    dom.toggleSidebarBtn.addEventListener("click", toggleSidebar);
    dom.toggleDockBtn.addEventListener("click", toggleDock);

    dom.newSessionBtn.addEventListener("click", () => startWelcome());
    dom.sessionSearch.addEventListener("input", () => {
      state.query = dom.sessionSearch.value;
      renderSidebar();
    });
    dom.filterRow.addEventListener("click", (event) => {
      const button = event.target.closest("[data-filter]");
      if (!button) return;
      state.filter = button.dataset.filter;
      renderSidebar();
    });
    dom.projectList.addEventListener("click", (event) => {
      const sessionButton = event.target.closest("[data-session]");
      if (sessionButton) {
        switchSession(sessionButton.dataset.session);
        return;
      }
      const newButton = event.target.closest("[data-new-session-project]");
      if (newButton) {
        startWelcome(newButton.dataset.newSessionProject);
        return;
      }
      const projectButton = event.target.closest("[data-project-toggle]");
      if (projectButton) {
        const id = projectButton.dataset.projectToggle;
        if (state.collapsedProjects.has(id)) state.collapsedProjects.delete(id);
        else state.collapsedProjects.add(id);
        renderSidebar();
      }
    });
    dom.capabilitiesBtn.addEventListener("click", () => openSettings("capabilities"));
    dom.sidebarSettingsBtn.addEventListener("click", () => openSettings("general"));

    dom.sessionbar.addEventListener("click", (event) => {
      const menuButton = event.target.closest("[data-session-menu]");
      if (menuButton) {
        const menu = menuButton.parentElement.querySelector(".session-menu-popover");
        menu.hidden = !menu.hidden;
        menuButton.setAttribute("aria-expanded", String(!menu.hidden));
        return;
      }
      const action = event.target.closest("[data-session-action]");
      if (action) {
        const menu = action.closest(".session-menu-popover");
        if (menu) menu.hidden = true;
        const trigger = menu && menu.parentElement.querySelector("[data-session-menu]");
        if (trigger) trigger.setAttribute("aria-expanded", "false");
        handleSessionAction(action.dataset.sessionAction);
        return;
      }
      const lineageLink = event.target.closest("[data-goto-session]");
      if (lineageLink) {
        switchSession(lineageLink.dataset.gotoSession);
        return;
      }
      if (event.target.closest("[data-open-changes]")) {
        openDockTab("changes");
        return;
      }
      if (event.target.closest("[data-rewind]")) {
        openRewind();
        return;
      }
      if (event.target.closest("[data-fork]")) {
        forkActiveSession();
        return;
      }
      if (event.target.closest("[data-toggle-dock]")) {
        toggleDock();
        return;
      }
      if (event.target.closest("[data-scroll-approval]")) {
        scrollToApproval();
      }
    });
    dom.sessionbar.addEventListener("keydown", (event) => {
      if ((event.key === "Enter" || event.key === " ") && event.target.closest("[data-scroll-approval]")) {
        event.preventDefault();
        scrollToApproval();
      }
    });

    dom.composerInput.addEventListener("input", () => {
      autoGrowComposer();
      updateSendState();
      syncMentionMenu();
      syncSlashMenu();
    });
    dom.composerInput.addEventListener("keydown", (event) => {
      if (event.key === "Tab" && event.shiftKey) {
        event.preventDefault();
        cyclePermission();
        return;
      }
      if (state.mentionOpen) {
        const files = filteredMentions();
        if (event.key === "ArrowDown") {
          event.preventDefault();
          setMentionIndex(state.mentionIndex + 1);
          return;
        }
        if (event.key === "ArrowUp") {
          event.preventDefault();
          setMentionIndex(state.mentionIndex - 1);
          return;
        }
        if ((event.key === "Enter" || event.key === "Tab") && files.length) {
          event.preventDefault();
          insertMention(files[state.mentionIndex].path);
          return;
        }
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          closeMentionMenu();
          return;
        }
      }
      if (state.slashOpen) {
        if (event.key === "ArrowDown") {
          event.preventDefault();
          setSlashIndex(state.slashIndex + 1);
          return;
        }
        if (event.key === "ArrowUp") {
          event.preventDefault();
          setSlashIndex(state.slashIndex - 1);
          return;
        }
        if (event.key === "Enter" && !event.shiftKey) {
          event.preventDefault();
          runSelectedSlash();
          return;
        }
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          closeSlashMenu();
          return;
        }
        if (event.key === "Tab") {
          event.preventDefault();
          const commands = filteredSlash();
          const command = commands[state.slashIndex];
          if (command) {
            dom.composerInput.value = command.name + " ";
            autoGrowComposer();
            closeSlashMenu();
          }
          return;
        }
      }
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        submitComposer();
      }
    });
    dom.sendBtn.addEventListener("click", () => {
      if (activeSession() && activeSession().status === "running") stopActiveSession();
      else submitComposer();
    });
    dom.attachBtn.addEventListener("click", () => dom.fileInput.click());
    dom.fileInput.addEventListener("change", () => {
      addAttachments(dom.fileInput.files);
      dom.fileInput.value = "";
    });
    dom.attachmentTray.addEventListener("click", (event) => {
      const remove = event.target.closest("[data-remove-attachment]");
      if (remove) removeAttachment(remove.dataset.removeAttachment);
    });
    dom.composer.addEventListener("dragover", (event) => {
      if (!event.dataTransfer || !Array.from(event.dataTransfer.types || []).includes("Files")) return;
      event.preventDefault();
      dom.composer.classList.add("is-dropping");
    });
    dom.composer.addEventListener("dragleave", (event) => {
      if (event.relatedTarget && dom.composer.contains(event.relatedTarget)) return;
      dom.composer.classList.remove("is-dropping");
    });
    dom.composer.addEventListener("drop", (event) => {
      if (!event.dataTransfer || !event.dataTransfer.files.length) return;
      event.preventDefault();
      dom.composer.classList.remove("is-dropping");
      addAttachments(event.dataTransfer.files);
    });
    dom.composerInput.addEventListener("paste", (event) => {
      const files = Array.from((event.clipboardData && event.clipboardData.files) || []);
      if (files.length) {
        event.preventDefault();
        addAttachments(files);
      }
    });
    dom.mentionMenu.addEventListener("click", (event) => {
      const item = event.target.closest("[data-mention-path]");
      if (item) insertMention(item.dataset.mentionPath);
    });
    dom.mentionMenu.addEventListener("mouseover", (event) => {
      const item = event.target.closest("[data-mention-index]");
      if (item) setMentionIndex(Number(item.dataset.mentionIndex));
    });
    dom.approvalChip.addEventListener("click", () => togglePopover("permission"));
    dom.envChip.addEventListener("click", () => togglePopover("env"));
    dom.modelChip.addEventListener("click", () => togglePopover("model"));
    dom.ctxPill.addEventListener("click", () => openDockTab("context"));
    dom.composerMeta.addEventListener("click", (event) => {
      if (event.target.closest("[data-scroll-approval]")) scrollToApproval();
    });

    dom.approvalPopover.addEventListener("click", (event) => {
      const item = event.target.closest("[data-permission]");
      if (!item) return;
      setApproval(item.dataset.permission);
      closePopover();
    });
    dom.envPopover.addEventListener("click", (event) => {
      const item = event.target.closest("[data-env]");
      if (!item) return;
      setEnv(item.dataset.env);
      closePopover();
    });
    dom.modelPopover.addEventListener("click", (event) => {
      const modelButton = event.target.closest("[data-model]");
      if (modelButton) {
        setModel(modelButton.dataset.model);
        closePopover();
        return;
      }
      const effort = event.target.closest("[data-effort]");
      if (effort) {
        setThinking(effort.dataset.effort, { silent: true });
        renderModelPopover();
        toast(`Thinking · ${effort.dataset.effort}`, "info", "cpu");
        return;
      }
      if (event.target.closest("[data-manage-providers]")) {
        closePopover();
        openSettings("providers");
      }
    });
    dom.slashMenu.addEventListener("click", (event) => {
      const item = event.target.closest("[data-slash-index]");
      if (!item) return;
      state.slashIndex = Number(item.dataset.slashIndex);
      runSelectedSlash();
    });
  }

  function scrollToApproval() {
    const card = dom.conversation.querySelector("[data-approval-card]");
    if (!card) return;
    card.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  function cyclePermission() {
    const modes = Object.keys(PERMISSIONS);
    const current = modes.indexOf(currentApproval());
    setApproval(modes[(current + 1) % modes.length]);
  }

  function bindDocumentAndPanelEvents() {
    dom.conversation.addEventListener("click", (event) => {
      const codeCopy = event.target.closest("[data-copy-code]");
      if (codeCopy) {
        const snippet = codeSnippets.get(codeCopy.dataset.copyCode);
        if (snippet != null) copyText(snippet, "Code copied");
        return;
      }
      const retryButton = event.target.closest("[data-retry-tool]");
      if (retryButton) {
        retryTool(retryButton.dataset.retryTool);
        return;
      }
      const toolHead = event.target.closest(".tool-head");
      if (toolHead) {
        const card = toolHead.closest(".tool-card");
        const open = card.classList.toggle("is-open");
        toolHead.setAttribute("aria-expanded", String(open));
        return;
      }
      const subagentHead = event.target.closest(".subagent-head");
      if (subagentHead) {
        const card = subagentHead.closest(".subagent");
        const open = card.classList.toggle("is-open");
        subagentHead.setAttribute("aria-expanded", String(open));
        return;
      }
      const approvalButton = event.target.closest("[data-approval]");
      if (approvalButton) {
        resolveApproval(approvalButton.dataset.approval, approvalButton.dataset.approvalId);
        return;
      }
      const fileButton = event.target.closest("[data-open-file]");
      if (fileButton) {
        openFileInDock(fileButton.dataset.openFile);
        return;
      }
      if (event.target.closest("[data-open-changes]")) {
        openDockTab("changes");
        return;
      }
      if (event.target.closest("[data-scroll-approval]")) {
        scrollToApproval();
        return;
      }
      if (event.target.closest("[data-fork]")) {
        forkActiveSession();
        return;
      }
      const copyButton = event.target.closest("[data-copy-message]");
      if (copyButton) {
        const session = activeSession();
        const message = session ? session.messages[Number(copyButton.dataset.copyMessage)] : null;
        if (message) copyText(messagePlainText(message), "Message copied");
        return;
      }
      const editButton = event.target.closest("[data-edit-message]");
      if (editButton) {
        startMessageEdit(Number(editButton.dataset.editMessage));
        return;
      }
      if (event.target.closest("[data-save-edit]")) {
        saveMessageEdit();
        return;
      }
      if (event.target.closest("[data-cancel-edit]")) {
        cancelMessageEdit();
        return;
      }
      const regenerateButton = event.target.closest("[data-regenerate-message]");
      if (regenerateButton) {
        regenerateMessage(Number(regenerateButton.dataset.regenerateMessage));
        return;
      }
      const branchButton = event.target.closest("[data-branch-message]");
      if (branchButton) {
        branchFromMessage(Number(branchButton.dataset.branchMessage));
        return;
      }
      const deleteButton = event.target.closest("[data-delete-message]");
      if (deleteButton) {
        deleteMessage(Number(deleteButton.dataset.deleteMessage));
        return;
      }
      const recent = event.target.closest("[data-recent-session]");
      if (recent) {
        switchSession(recent.dataset.recentSession);
        return;
      }
      const suggestion = event.target.closest("[data-suggestion]");
      if (suggestion) {
        const item = DATA.suggestions[Number(suggestion.dataset.suggestion)];
        if (!item) return;
        dom.composerInput.value = item.prompt;
        autoGrowComposer();
        updateSendState();
        dom.composerInput.focus();
        return;
      }
    });

    dom.conversation.addEventListener("keydown", (event) => {
      if (!state.messageEdit || event.target.id !== "messageEditInput") return;
      if (event.key === "Escape") {
        event.preventDefault();
        cancelMessageEdit();
        return;
      }
      if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        saveMessageEdit();
      }
    });

    dom.dockHeader.addEventListener("click", (event) => {
      const tab = event.target.closest("[data-dock-tab]");
      if (tab) openDockTab(tab.dataset.dockTab);
      if (event.target.closest("[data-close-dock]")) toggleDock();
    });

    dom.dockBody.addEventListener("click", (event) => {
      const session = activeSession();

      const hunkButton = event.target.closest("[data-hunk-action]");
      if (hunkButton && session && session.changes) {
        const path = state.selectedFile[session.id];
        const file = session.changes.files.find((item) => item.path === path);
        if (!file) return;
        const index = String(hunkButton.dataset.hunkIndex);
        const action = hunkButton.dataset.hunkAction;
        file.hunkDecisions = file.hunkDecisions || {};
        if (action === "clear") delete file.hunkDecisions[index];
        else file.hunkDecisions[index] = action;
        if (action !== "clear") {
          file.reviewed = false;
          file.staged = false;
        }
        session.changes.reviewed = allFilesReviewed(session);
        renderDock();
        renderSessionbar();
        renderSidebar();
        syncReviewSummary();
        toast(
          action === "revert"
            ? "Hunk reverted · stats updated"
            : action === "keep"
              ? "Hunk kept"
              : "Hunk decision undone",
          action === "revert" ? "warn" : "info",
          action === "revert" ? "corner" : "check"
        );
        return;
      }

      const revertFileButton = event.target.closest("[data-revert-file]");
      if (revertFileButton && session && session.changes) {
        const file = session.changes.files.find((item) => item.path === revertFileButton.dataset.revertFile);
        if (!file) return;
        const groups = hunkGroups(file);
        file.hunkDecisions = {};
        for (const group of groups) file.hunkDecisions[group.index] = "revert";
        file.reviewed = false;
        file.staged = false;
        session.changes.reviewed = allFilesReviewed(session);
        renderDock();
        renderSessionbar();
        renderSidebar();
        syncReviewSummary();
        toast(`${file.path.split("/").pop()} · all hunks reverted`, "warn", "corner");
        return;
      }

      const stageFileButton = event.target.closest("[data-stage-file]");
      if (stageFileButton && session && session.changes) {
        const file = session.changes.files.find((item) => item.path === stageFileButton.dataset.stageFile);
        if (!file) return;
        file.staged = !file.staged;
        renderDock();
        toast(`${file.path.split("/").pop()} · ${file.staged ? "staged" : "unstaged"}`, "info", "check");
        return;
      }

      if (event.target.closest("[data-commit-changes]")) {
        if (!session || !session.changes) return;
        const stats = changeStats(session);
        if (!stats.staged) {
          toast("Stage a file before committing", "warn", "alert");
          return;
        }
        openSessionDialog("commit");
        return;
      }

      if (event.target.closest("[data-open-editor]")) {
        toast("Open in editor is host-owned", "info", "external");
        return;
      }

      const reviewFile = event.target.closest("[data-review-file]");
      if (reviewFile && session && session.changes) {
        const file = session.changes.files.find((item) => item.path === reviewFile.dataset.reviewFile);
        if (!file) return;
        file.reviewed = !file.reviewed;
        session.changes.reviewed = allFilesReviewed(session);
        renderDock();
        renderSessionbar();
        renderSidebar();
        syncReviewSummary();
        toast(`${file.path.split("/").pop()} · ${file.reviewed ? "reviewed" : "reopened"}`, "info", "check");
        return;
      }

      const prevFile = event.target.closest("[data-file-prev]");
      const nextFile = event.target.closest("[data-file-next]");
      if ((prevFile || nextFile) && session && session.changes) {
        stepReviewFile(prevFile ? -1 : 1);
        return;
      }

      const commentButton = event.target.closest("[data-comment-line]");
      if (commentButton && session) {
        state.commentDraft = {
          sessionId: session.id,
          file: state.selectedFile[session.id],
          row: Number(commentButton.dataset.commentLine),
          label: commentButton.dataset.commentLabel,
        };
        renderDock();
        window.requestAnimationFrame(() => document.getElementById("diffCommentInput")?.focus());
        return;
      }

      if (event.target.closest("[data-cancel-comment]")) {
        state.commentDraft = null;
        renderDock();
        return;
      }

      if (event.target.closest("[data-save-comment]")) {
        const draft = state.commentDraft;
        const input = document.getElementById("diffCommentInput");
        const text = input ? input.value.trim() : "";
        if (!draft || !session) return;
        if (!text) {
          toast("Write a review note first", "warn", "message");
          input?.focus();
          return;
        }
        state.reviewCounter += 1;
        session.reviewComments.push({
          id: `note-${state.reviewCounter}`,
          file: draft.file,
          row: draft.row,
          label: draft.label,
          text,
          time: clock(),
        });
        state.commentDraft = null;
        renderDock();
        toast("Review note added", "info", "message");
        return;
      }

      const deleteComment = event.target.closest("[data-delete-comment]");
      if (deleteComment && session) {
        session.reviewComments = (session.reviewComments || []).filter(
          (comment) => comment.id !== deleteComment.dataset.deleteComment
        );
        renderDock();
        toast("Review note deleted", "info", "x");
        return;
      }

      const fileRow = event.target.closest("[data-file]");
      if (fileRow) {
        if (session) {
          state.selectedFile[session.id] = fileRow.dataset.file;
          state.commentDraft = null;
          renderDock();
        }
        return;
      }
      if (event.target.closest("[data-mark-reviewed]")) {
        if (session && session.changes) {
          for (const file of session.changes.files) file.reviewed = true;
          session.changes.reviewed = allFilesReviewed(session);
          renderDock();
          renderSessionbar();
          renderSidebar();
          syncReviewSummary();
          toast("All files marked reviewed", "info", "check");
        }
        return;
      }
      if (event.target.closest("[data-compact]")) {
        compactActiveSession();
        return;
      }
      if (event.target.closest("[data-scroll-approval]")) {
        scrollToApproval();
        return;
      }
      const copyPatch = event.target.closest("[data-copy-patch]");
      if (copyPatch) {
        const file = session && session.changes
          ? session.changes.files.find((item) => item.path === copyPatch.dataset.copyPatch)
          : null;
        if (file) copyText(file.diff.join("\n"), "Patch copied");
        return;
      }
      if (event.target.closest("[data-copy-terminal]")) {
        const text = session ? (session.terminal || []).map((line) => line.text).join("\n") : "";
        copyText(text, "Terminal contents copied");
        return;
      }
      if (event.target.closest("[data-open-folder]")) {
        toast("Open folder is host-owned", "info", "external");
        return;
      }
      const tab = event.target.closest("[data-dock-tab]");
      if (tab) openDockTab(tab.dataset.dockTab);
    });

    dom.dockBody.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && state.commentDraft) {
        event.preventDefault();
        event.stopPropagation();
        state.commentDraft = null;
        renderDock();
        return;
      }
      if ((event.key === "Enter" || event.key === " ") && event.target.matches(".file-row")) {
        event.preventDefault();
        event.target.click();
        return;
      }
      if (event.key === "Enter" && (event.ctrlKey || event.metaKey) && event.target.id === "diffCommentInput") {
        event.preventDefault();
        dom.dockBody.querySelector("[data-save-comment]")?.click();
      }
    });

    dom.paletteOverlay.addEventListener("click", (event) => {
      if (event.target === dom.paletteOverlay) closePalette();
    });
    dom.paletteInput.addEventListener("input", () => {
      state.paletteQuery = dom.paletteInput.value;
      state.paletteIndex = 0;
      renderPalette();
    });
    dom.paletteInput.addEventListener("keydown", (event) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setPaletteIndex(state.paletteIndex + 1);
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        setPaletteIndex(state.paletteIndex - 1);
      } else if (event.key === "Enter") {
        event.preventDefault();
        runPaletteItem(state.paletteIndex);
      } else if (event.key === "Escape") {
        event.preventDefault();
        closePalette();
      }
    });
    dom.paletteList.addEventListener("click", (event) => {
      const item = event.target.closest("[data-palette-index]");
      if (item) runPaletteItem(Number(item.dataset.paletteIndex));
    });
    dom.paletteList.addEventListener("mouseover", (event) => {
      const item = event.target.closest("[data-palette-index]");
      if (item) setPaletteIndex(Number(item.dataset.paletteIndex));
    });

    dom.settingsOverlay.addEventListener("click", (event) => {
      if (event.target === dom.settingsOverlay) closeSettings();
    });
    dom.settingsClose.addEventListener("click", closeSettings);
    dom.settingsDone.addEventListener("click", closeSettings);
    dom.dialogOverlay.addEventListener("click", (event) => {
      if (event.target === dom.dialogOverlay) closeSessionDialog();
    });
    dom.dialogClose.addEventListener("click", closeSessionDialog);
    dom.dialogCancel.addEventListener("click", closeSessionDialog);
    dom.dialogConfirm.addEventListener("click", confirmSessionDialog);
    dom.dialogInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        confirmSessionDialog();
      } else if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        closeSessionDialog();
      }
    });
    dom.rewindOverlay.addEventListener("click", (event) => {
      if (event.target === dom.rewindOverlay) closeRewind();
    });
    dom.rewindList.addEventListener("click", (event) => {
      const row = event.target.closest("[data-rewind-checkpoint]");
      if (!row) return;
      state.rewindIndex = Number(row.dataset.rewindCheckpoint);
      renderRewind();
      const selected = dom.rewindList.querySelector(".rewind-row.is-selected");
      if (selected) selected.focus({ preventScroll: true });
    });
    dom.rewindClose.addEventListener("click", closeRewind);
    dom.rewindCancel.addEventListener("click", closeRewind);
    dom.rewindConfirm.addEventListener("click", rewindToCheckpoint);
    dom.rewindOverlay.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        closeRewind();
        return;
      }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        const session = sessions[state.rewindSessionId];
        if (!session) return;
        event.preventDefault();
        const delta = event.key === "ArrowDown" ? -1 : 1;
        state.rewindIndex = Math.max(0, Math.min(session.checkpoints.length - 1, state.rewindIndex + delta));
        renderRewind();
        const selected = dom.rewindList.querySelector(".rewind-row.is-selected");
        if (selected) selected.focus({ preventScroll: true });
      }
    });
    dom.settingsNav.addEventListener("click", (event) => {
      const item = event.target.closest("[data-settings-section]");
      if (!item) return;
      state.settingsSection = item.dataset.settingsSection;
      renderSettings();
      dom.settingsContent.scrollTop = 0;
    });
    dom.settingsContent.addEventListener("click", (event) => {
      const theme = event.target.closest("[data-theme-option]");
      if (theme) {
        setTheme(theme.dataset.themeOption);
        return;
      }
      const model = event.target.closest("[data-set-model]");
      if (model) {
        setModel(model.dataset.setModel);
        return;
      }
      const action = event.target.closest("[data-setting-action]");
      if (action) {
        if (action.dataset.settingAction === "copy-models-path") copyText("~/.nulo/agent/models.json", "models.json path copied");
        else if (action.dataset.settingAction === "copy-version") copyText(`nulo ${DATA.meta.version}`, "Version copied");
        else if (action.dataset.settingAction === "open-design") toast("Design folder · ui/design/desktop", "info", "external");
      }
    });
    dom.settingsContent.addEventListener("change", (event) => {
      const target = event.target;
      if (target.id === "settingsDefaultModel") setModel(target.value);
      else if (target.id === "settingsThinking") setThinking(target.value);
      else if (target.id === "settingsPermission") setApproval(target.value);
      else if (target.id === "settingsDensity") {
        state.density = target.value;
        savePrefs();
        applyShell();
        toast(`Density · ${target.value}`, "info", "settings");
      } else if (target.dataset.pref) {
        const pref = target.dataset.pref;
        if (pref === "reasoning") state.reasoning = target.checked;
        if (pref === "timestamps") state.timestamps = target.checked;
        if (pref === "motion") state.motion = target.checked ? "reduced" : "full";
        savePrefs();
        applyShell();
        renderConversation();
        toast(`${pref} ${target.checked ? "on" : "off"}`, "info", "settings");
      } else if (target.dataset.cap) {
        toast(`${target.dataset.cap.split(":")[1]} · visual toggle only`, "info", "plug");
      }
    });

    document.addEventListener("click", (event) => {
      if (state.popover && !event.target.closest(".popover-anchor")) closePopover();
      if (state.slashOpen && !event.target.closest(".composer")) closeSlashMenu();
      if (state.mentionOpen && !event.target.closest(".composer")) closeMentionMenu();
      if (!event.target.closest(".session-menu-wrap")) {
        document.querySelectorAll(".session-menu-popover").forEach((menu) => {
          menu.hidden = true;
          const trigger = menu.parentElement.querySelector("[data-session-menu]");
          if (trigger) trigger.setAttribute("aria-expanded", "false");
        });
      }
    });
  }

  /* ----------------------------------------------------------------------
     Global keys, resizing, focus
     ---------------------------------------------------------------------- */

  function trapFocus(event, container) {
    if (event.key !== "Tab") return;
    const focusables = Array.from(
      container.querySelectorAll(
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
    ).filter((node) => node.offsetParent !== null);
    if (!focusables.length) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  function bindGlobalEvents() {
    dom.paletteOverlay.addEventListener("keydown", (event) => trapFocus(event, dom.paletteOverlay));
    dom.settingsOverlay.addEventListener("keydown", (event) => trapFocus(event, dom.settingsOverlay));
    dom.dialogOverlay.addEventListener("keydown", (event) => trapFocus(event, dom.dialogOverlay));
    dom.rewindOverlay.addEventListener("keydown", (event) => trapFocus(event, dom.rewindOverlay));

    document.addEventListener("keydown", (event) => {
      const target = event.target;
      const typing =
        target instanceof Element &&
        (target.matches("input, textarea, select") || target.isContentEditable);
      const modifier = event.ctrlKey || event.metaKey;

      if (modifier && event.key.toLowerCase() === "k") {
        event.preventDefault();
        if (state.paletteOpen) closePalette();
        else openPalette();
        return;
      }
      if (modifier && event.key.toLowerCase() === "n") {
        event.preventDefault();
        startWelcome();
        return;
      }
      if (modifier && event.key.toLowerCase() === "b") {
        event.preventDefault();
        toggleSidebar();
        return;
      }
      if (modifier && event.key.toLowerCase() === "j") {
        event.preventDefault();
        toggleDock();
        return;
      }
      if (modifier && event.key.toLowerCase() === "f") {
        event.preventDefault();
        state.sidebarOpen = true;
        applyShell();
        dom.sessionSearch.focus();
        dom.sessionSearch.select();
        return;
      }
      if (modifier && event.key === ",") {
        event.preventDefault();
        if (state.settingsOpen) closeSettings();
        else openSettings("general");
        return;
      }
      if (modifier && event.key === "/") {
        event.preventDefault();
        openSettings("shortcuts");
        return;
      }
      if (event.altKey && !typing && (event.key === "ArrowLeft" || event.key === "ArrowRight")) {
        event.preventDefault();
        stepReviewFile(event.key === "ArrowLeft" ? -1 : 1);
        return;
      }
      if (event.key === "F1") {
        event.preventDefault();
        openSettings("shortcuts");
        return;
      }
      if (event.key === "Escape") {
        if (state.rewindSessionId) {
          closeRewind();
          return;
        }
        if (state.dialogMode) {
          closeSessionDialog();
          return;
        }
        if (state.paletteOpen) {
          closePalette();
          return;
        }
        if (state.settingsOpen) {
          closeSettings();
          return;
        }
        if (state.popover) {
          closePopover();
          return;
        }
        if (state.slashOpen) {
          closeSlashMenu();
          return;
        }
        const session = activeSession();
        if (session && session.status === "running") {
          stopActiveSession();
          return;
        }
        if (document.activeElement === dom.composerInput) dom.composerInput.blur();
        return;
      }
      if (event.key === "/" && !typing && !modifier) {
        event.preventDefault();
        dom.composerInput.focus();
        dom.composerInput.value = "/";
        autoGrowComposer();
        updateSendState();
        syncSlashMenu();
      }
    });

    window.addEventListener("resize", syncResponsiveShell);

    dom.composerInput.addEventListener("blur", () => {
      window.setTimeout(() => {
        if (!dom.composer.contains(document.activeElement)) {
          closeSlashMenu();
          closeMentionMenu();
        }
      }, 100);
    });
  }

  function bindResizer() {
    let dragging = false;
    let startX = 0;
    let startWidth = 0;

    dom.dockResizer.addEventListener("pointerdown", (event) => {
      if (event.button !== 0) return;
      dragging = true;
      startX = event.clientX;
      startWidth = dom.dock.getBoundingClientRect().width;
      dom.dockResizer.setPointerCapture(event.pointerId);
      dom.workbench.classList.add("is-resizing");
      document.body.style.cursor = "col-resize";
    });

    dom.dockResizer.addEventListener("pointermove", (event) => {
      if (!dragging) return;
      const width = Math.max(320, Math.min(620, startWidth + (startX - event.clientX)));
      state.dockWidth = width;
      dom.workbench.style.setProperty("--dock-w", `${width}px`);
    });

    const finish = (event) => {
      if (!dragging) return;
      dragging = false;
      dom.workbench.classList.remove("is-resizing");
      document.body.style.cursor = "";
      if (event && event.pointerId != null && dom.dockResizer.hasPointerCapture(event.pointerId)) {
        dom.dockResizer.releasePointerCapture(event.pointerId);
      }
      savePrefs();
    };
    dom.dockResizer.addEventListener("pointerup", finish);
    dom.dockResizer.addEventListener("pointercancel", finish);
    dom.dockResizer.addEventListener("dblclick", () => {
      state.dockWidth = 420;
      dom.workbench.style.setProperty("--dock-w", "420px");
      savePrefs();
      toast("Dock width reset", "info", "panel-right");
    });
  }

  function init() {
    loadPrefs();
    if (window.innerWidth <= 1180 && state.dockOpen) {
      state.dockOpen = false;
      state.autoClosedDock = true;
    }
    if (window.innerWidth <= 900 && state.sidebarOpen) {
      state.sidebarOpen = false;
      state.autoClosedSidebar = true;
    }
    state.lastViewportWidth = window.innerWidth;
    applyShell();
    bindEvents();
    bindDocumentAndPanelEvents();
    bindGlobalEvents();
    bindResizer();
    renderAll();
    window.requestAnimationFrame(() => scrollToBottom(false));
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
