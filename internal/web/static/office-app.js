const $ = (s) => document.querySelector(s);
const pets = [
  ["fox", "Raposa", "🦊"],
  ["cat", "Gato", "🐱"],
  ["owl", "Coruja", "🦉"],
  ["capybara", "Capivara", "🦫"],
  ["robot", "Robô", "🤖"],
];
const colors = [
  ["terracotta", "Terracota"],
  ["sage", "Sálvia"],
  ["lilac", "Lavanda"],
  ["honey", "Mel"],
  ["ocean", "Oceano"],
];
const state = {
  projects: [],
  projectId: "",
  snapshot: null,
  tools: [],
  capabilities: {},
  route: "escritorio",
  selected: null,
  scene: null,
  loading: false,
  stale: true,
  wizard: null,
  wizardStep: 0,
};
const page = $("#page");
let refreshGeneration = 0,
  noticeTimer;
function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined) n.textContent = text;
  return n;
}
function button(text, fn, cls = "") {
  const b = el("button", cls, text);
  b.type = "button";
  b.addEventListener("click", fn);
  return b;
}
function link(text, route, cls = "button") {
  const a = el("a", cls, text);
  a.href = "#/" + route;
  return a;
}
function note(text) {
  $("#notice").textContent = text;
  $("#notice").hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => ($("#notice").hidden = true), 7000);
}
function errorMessage(error) {
  if (error.status === 409)
    return "A configuração mudou em outra janela. Atualize os dados antes de salvar novamente.";
  return (
    error.message ||
    "Não foi possível concluir. Atualize para consultar o estado confirmado antes de repetir."
  );
}
async function api(path, method = "GET", body) {
  const r = await fetch(
    path.startsWith("/api/") ? path : "/api/office" + path,
    {
      method,
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    },
  );
  let value;
  try {
    value = await r.json();
  } catch {
    value = {};
  }
  if (!r.ok) {
    const e = new Error(
      value.error?.message ||
        value.error ||
        value.message ||
        "A operação não foi confirmada (HTTP " + r.status + ").",
    );
    e.status = r.status;
    throw e;
  }
  return value;
}
const agents = () => state.snapshot?.agents || [];
const teams = () => state.snapshot?.teams || [];
const tasks = () => state.snapshot?.tasks || [];
const messages = () => state.snapshot?.messages || [];
const activeProject = () =>
  state.projects.find((p) => p.id === state.projectId);
const symbol = (a) => pets.find((p) => p[0] === a.avatar?.familyId)?.[2] || "◈";
function avatarIcon(agent) {
  const family = agent.avatar?.familyId || "fox";
  const shades = {
    terracotta: "#c98057",
    sage: "#8d9d76",
    lilac: "#a295ba",
    honey: "#d1b25d",
    ocean: "#739ba4",
  };
  const color = shades[agent.avatar?.color] || shades.terracotta;
  function svgNode(tag, attrs) {
    const n = document.createElementNS("http://www.w3.org/2000/svg", tag);
    for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, String(v));
    return n;
  }
  const svg = svgNode("svg", {
    viewBox: "0 0 100 100",
    width: 100,
    height: 100,
    role: "img",
    "aria-label": pets.find((p) => p[0] === family)?.[1] || "Personagem",
  });
  svg.append(
    svgNode("ellipse", {
      cx: 50,
      cy: 89,
      rx: 31,
      ry: 5,
      fill: "#728061",
      opacity: 0.15,
    }),
    svgNode("ellipse", { cx: 50, cy: 69, rx: 22, ry: 23, fill: color }),
  );
  if (family === "robot") {
    svg.append(
      svgNode("path", { d: "M50 22V10", stroke: "#5a6a54", "stroke-width": 4 }),
      svgNode("circle", { cx: 50, cy: 9, r: 5, fill: "#d1b25d" }),
      svgNode("rect", {
        x: 22,
        y: 22,
        width: 56,
        height: 47,
        rx: 10,
        fill: color,
      }),
    );
  } else {
    if (family === "fox" || family === "cat")
      for (const d of ["M24 40L20 11L44 30Z", "M56 30L80 11L76 40Z"])
        svg.append(svgNode("path", { d, fill: color }));
    else
      for (const x of [29, 71])
        svg.append(svgNode("circle", { cx: x, cy: 29, r: 9, fill: color }));
    svg.append(
      svgNode("ellipse", {
        cx: 50,
        cy: 47,
        rx: family === "capybara" ? 31 : 29,
        ry: family === "capybara" ? 23 : 28,
        fill: color,
      }),
    );
  }
  for (const x of [39, 61]) {
    svg.append(
      svgNode("circle", {
        cx: x,
        cy: 44,
        r: family === "owl" ? 12 : 8,
        fill: "#f7eed8",
      }),
    );
    if (agent.avatar?.eyes === "soft")
      svg.append(
        svgNode("path", {
          d: `M${x - 3} 45Q${x} 41 ${x + 3} 45`,
          fill: "none",
          stroke: "#303e32",
          "stroke-width": 2.5,
        }),
      );
    else
      svg.append(svgNode("circle", { cx: x, cy: 45, r: 3, fill: "#303e32" }));
  }
  svg.append(
    svgNode("ellipse", { cx: 50, cy: 57, rx: 5, ry: 3, fill: "#584b3c" }),
  );
  if (agent.avatar?.accessories?.includes("headphones"))
    svg.append(
      svgNode("path", {
        d: "M19 47V33Q50 3 81 33V47M19 37V53M81 37V53",
        fill: "none",
        stroke: "#495747",
        "stroke-width": 7,
        "stroke-linecap": "round",
      }),
    );
  return svg;
}
const toolName = (id) =>
  state.tools.find((t) => t.id === id)?.name || id || "Não configurada";
const readyTools = () =>
  state.tools.filter(
    (t) =>
      t.selectable === true &&
      t.kind !== "utility" &&
      t.kind !== "infrastructure",
  );
const date = (value) =>
  value ? new Date(value).toLocaleString("pt-BR") : "Não informado";
function summary(rows) {
  const dl = el("dl", "summary");
  for (const [k, v] of rows)
    dl.append(
      el("dt", "", k),
      el("dd", "", v === null || v === undefined ? "Não informado" : String(v)),
    );
  return dl;
}
function field(label, input, hint) {
  const l = el("label", "field", label);
  l.append(input);
  if (hint) l.append(el("small", "", hint));
  return l;
}
function input(value = "", type = "text") {
  const i = el("input");
  i.type = type;
  i.value = value;
  return i;
}
function select(options, value) {
  const s = el("select");
  for (const [id, label, disabled] of options) {
    const o = el("option", "", label);
    o.value = id;
    o.disabled = !!disabled;
    s.append(o);
  }
  s.value = value ?? "";
  return s;
}
function empty(root, title, text, action) {
  const e = el("div", "empty");
  e.append(el("h2", "", title), el("p", "", text));
  if (action) e.append(action);
  root.append(e);
}
function heading(root, title, description, action) {
  const h = el("div", "page-head"),
    t = el("div");
  t.append(
    el(
      "span",
      "eyebrow",
      "GasaCLI · " + (activeProject()?.name || "Seu estúdio"),
    ),
    el("h1", "", title),
  );
  if (description) t.append(el("p", "", description));
  h.append(t);
  if (action) h.append(action);
  root.append(h);
}
function modal(title, build) {
  const content = $("#modal-content");
  content.replaceChildren(el("h2", "", title));
  build(content);
  if (!$("#modal").open) $("#modal").showModal();
}
$("#close-modal").addEventListener("click", () => $("#modal").close());
$("#modal").addEventListener("click", (e) => {
  if (e.target === $("#modal")) {
    const r = $("#modal").getBoundingClientRect();
    if (
      e.clientX < r.left ||
      e.clientX > r.right ||
      e.clientY < r.top ||
      e.clientY > r.bottom
    )
      $("#modal").close();
  }
});
async function bootstrap() {
  const hash = location.hash.slice(1);
  if (!hash.startsWith("/")) {
    const code = new URLSearchParams(hash).get("code");
    if (code) {
      history.replaceState(
        null,
        "",
        location.pathname + location.search + "#/escritorio",
      );
      await api("/api/session", "POST", { code });
    }
  }
  const [projectsResult, toolResult, capabilities] = await Promise.allSettled([
    api("/api/projects"),
    api("/tools"),
    api("/capabilities"),
  ]);
  if (projectsResult.status !== "fulfilled") throw projectsResult.reason;
  state.projects = Array.isArray(projectsResult.value)
    ? projectsResult.value
    : projectsResult.value.projects || [];
  if (toolResult.status === "fulfilled")
    state.tools = toolResult.value.tools || [];
  if (capabilities.status === "fulfilled")
    state.capabilities = capabilities.value;
  const picker = $("#project");
  picker.replaceChildren(el("option", "", "Escolha um projeto"));
  picker.firstChild.value = "";
  for (const p of state.projects) {
    const o = el("option", "", p.name + (p.trusted ? "" : " · não confiável"));
    o.value = p.id;
    picker.append(o);
  }
  let saved;
  try {
    saved = sessionStorage.getItem("gasa-office-project");
  } catch {}
  state.projectId =
    state.projects.find((p) => p.id === saved)?.id ||
    state.projects.find((p) => p.current && p.trusted)?.id ||
    state.projects.find((p) => p.trusted)?.id ||
    "";
  picker.value = state.projectId;
  await refresh();
  render();
  setInterval(() => {
    if (!document.hidden) refresh();
  }, 5000);
}
$("#project").addEventListener("change", async (e) => {
  if (
    state.wizard &&
    !confirm(
      "Trocar o projeto? O formulário atual será preservado como rascunho local.",
    )
  ) {
    e.target.value = state.projectId;
    return;
  }
  saveLocalDraft();
  state.wizard = null;
  state.projectId = e.target.value;
  state.selected = null;
  state.snapshot = null;
  state.stale = true;
  try {
    sessionStorage.setItem("gasa-office-project", state.projectId);
  } catch {}
  await refresh();
  render();
});
async function refresh() {
  if (!state.projectId) {
    state.stale = true;
    return;
  }
  const generation = ++refreshGeneration,
    projectId = state.projectId;
  try {
    const snapshot = await api(
      "/snapshot?projectId=" + encodeURIComponent(projectId),
    );
    if (generation !== refreshGeneration || projectId !== state.projectId)
      return;
    const changed = JSON.stringify(snapshot) !== JSON.stringify(state.snapshot);
    state.snapshot = snapshot;
    state.stale = false;
    $("#connection").textContent = "Sincronizado · local";
    $("#connection").className = "connection online";
    $("#notification-count").textContent = (snapshot.intents || []).filter(
      (i) => /pending|awaiting/.test(String(i.status).toLowerCase()),
    ).length;
    state.scene?.update(agents(), { ...state.snapshot, stale: state.stale });
    if (
      changed &&
      !state.wizard &&
      !$("#modal").open &&
      state.route !== "escritorio" &&
      !state.route.startsWith("conversas") &&
      !page.contains(document.activeElement)
    )
      render();
    if (changed) {
      rootChatRefresh();
      if (state.route.startsWith("conversas/")) {
        const recipient = state.route.split("/")[1];
        const timeline = page.querySelector(".timeline");
        if (timeline) {
          const current = messages().filter(
            (m) =>
              recipient === "time" ||
              m.recipientId === recipient ||
              m.agentId === recipient,
          );
          if (current.length)
            timeline.replaceChildren(...current.map(messageNode));
        }
      }
    }
  } catch (e) {
    if (projectId !== state.projectId) return;
    state.stale = true;
    $("#connection").textContent = "Estado desatualizado";
    $("#connection").className = "connection";
    if (!state.snapshot) note(errorMessage(e));
    state.scene?.update(agents(), { ...state.snapshot, stale: true });
  }
}
function navigate(route) {
  location.hash = "/" + route;
}
window.addEventListener("hashchange", () => {
  const next = location.hash.slice(2);
  if (state.wizard && !next.startsWith("agentes/")) {
    saveLocalDraft();
    state.wizard = null;
  }
  render();
});
function render() {
  state.scene?.dispose();
  state.scene = null;
  state.route = location.hash.startsWith("#/")
    ? location.hash.slice(2)
    : "escritorio";
  document.querySelectorAll("[data-nav]").forEach((a) => {
    const active = state.route.split("/")[0] === a.dataset.nav;
    a.classList.toggle("active", active);
    if (active) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  });
  page.replaceChildren();
  if (!state.projectId) {
    const root = el("section", "page");
    heading(
      root,
      "O seu próximo grande projeto começa aqui.",
      "Selecione um projeto na barra superior para abrir seu escritório.",
    );
    empty(
      root,
      "Um lugar para seu time de IA",
      "Agentes e conversas pertencem ao projeto escolhido. Nenhuma ferramenta é executada ao abrir o estúdio.",
    );
    page.append(root);
    return;
  }
  if (state.route === "escritorio") {
    office();
    return;
  }
  const root = el("section", "page");
  page.append(root);
  if (state.stale)
    root.append(
      el(
        "p",
        "warning",
        "Estado desatualizado. Alterações e execução precisam de confirmação do servidor.",
      ),
    );
  if (state.route === "equipe") teamPage(root);
  else if (state.route === "equipes") groupsPage(root);
  else if (state.route.startsWith("agentes/")) wizardPage(root);
  else if (state.route.startsWith("tarefas/"))
    taskDetail(root, state.route.split("/")[1]);
  else if (state.route === "tarefas") taskPage(root);
  else if (state.route === "dashboard") dashboard(root);
  else if (state.route.startsWith("ajustes/"))
    settingsPage(root, state.route.split("/")[1]);
  else if (state.route.startsWith("conversas/"))
    chatPage(root, state.route.split("/")[1]);
  else if (state.route === "copiloto") copilot(root);
  else {
    heading(root, "Página não encontrada", "Escolha um destino na navegação.");
    root.append(link("Voltar ao escritório", "escritorio"));
  }
}
async function office() {
  const section = el("section", "office"),
    scene = el("div", "scene");
  section.append(scene);
  const intro = el("div", "office-heading");
  intro.append(
    el("span", "eyebrow", "IDEIAS GANHAM COMPANHIA"),
    el("h1", "", activeProject()?.name || "Seu escritório"),
    el(
      "p",
      "",
      agents().filter((a) => !["draft", "archived"].includes(a.status)).length +
        " integrantes · " +
        (state.stale ? "Estado desatualizado" : "Seu time, no mesmo lugar."),
    ),
  );
  section.append(intro);
  const actions = el("div", "office-actions");
  actions.append(
    link("♧ Ver equipe", "equipe"),
    link("+ Criar agente", "agentes/novo", "button primary"),
  );
  section.append(actions);
  const controls = el("div", "camera-controls");
  for (const [label, action] of [
    ["−", "out"],
    ["+", "in"],
    ["↻", "rotate"],
    ["Visão geral", "reset"],
  ]) {
    const b = button(label, () => state.scene?.control(action));
    b.setAttribute(
      "aria-label",
      {
        out: "Afastar câmera",
        in: "Aproximar câmera",
        rotate: "Girar câmera",
        reset: "Restaurar câmera",
      }[action],
    );
    controls.append(b);
  }
  section.append(
    controls,
    el(
      "span",
      "office-key",
      "Arraste para girar · Selecione uma pessoa para conversar",
    ),
  );
  if (!agents().some((a) => !["draft", "archived"].includes(a.status))) {
    const invite = el("div", "empty-invite");
    invite.append(
      el("h2", "", "Seu escritório está pronto."),
      el(
        "p",
        "",
        "Agora, dê vida ao seu time. Escolha um bichinho, defina um papel e conecte uma IA da sua máquina.",
      ),
      link("Criar primeiro integrante", "agentes/novo", "button primary"),
    );
    section.append(invite);
  }
  section.append(composer(false));
  page.append(section);
  if (state.selected) chatPanel(section);
  try {
    const { OfficeScene } = await import("./office-scene.js");
    if (!section.isConnected) return;
    state.scene = new OfficeScene(scene, {
      select: (id) => {
        state.selected = id;
        section.querySelector(".chat-panel")?.remove();
        chatPanel(section);
        section
          .querySelector(".composer")
          .dispatchEvent(
            new CustomEvent("office-recipient", { detail: "agent:" + id }),
          );
      },
      open: sceneAction,
      failed: (msg) => fallbackScene(scene, msg),
      quality: state.snapshot?.settings?.quality,
      reducedMotion:
        state.snapshot?.settings?.reducedMotion ||
        matchMedia("(prefers-reduced-motion: reduce)").matches,
    });
    state.scene.update(agents(), { ...state.snapshot, stale: state.stale });
  } catch {
    fallbackScene(
      scene,
      "O 3D não está disponível neste navegador. Você pode usar todos os controles pela visão de equipe.",
    );
  }
}
function fallbackScene(root, msg) {
  state.scene?.dispose();
  state.scene = null;
  root.replaceChildren();
  const panel = el("div", "scene-fallback");
  panel.append(
    el("h2", "", "Vamos pela visão de equipe"),
    el("p", "", msg),
    link("Abrir equipe acessível", "equipe"),
    button("Tentar 3D novamente", render),
  );
  root.append(panel);
}
function chatPanel(root) {
  const a = agents().find((a) => a.id === state.selected);
  if (!a) return;
  const panel = el("aside", "chat-panel");
  panel.setAttribute("aria-label", "Conversa com " + a.name);
  const h = el("div", "inline-title");
  h.append(
    el("h2", "", symbol(a) + " " + a.name),
    button(
      "×",
      () => {
        state.selected = null;
        panel.remove();
      },
      "quiet",
    ),
  );
  panel.append(
    h,
    el("p", "muted", a.role + " · " + toolName(a.primary?.toolId)),
  );
  const recent = messages()
    .filter((m) => m.recipientId === a.id || m.agentId === a.id)
    .slice(-3);
  if (!recent.length)
    panel.append(
      el("p", "muted", "Uma nova conversa começa com uma boa ideia."),
    );
  for (const m of recent) panel.append(messageNode(m));
  panel.append(
    link("Ampliar conversa", "conversas/" + a.id, "button"),
    link("Editar perfil", "agentes/" + a.id, "button"),
  );
  root.append(panel);
}
function messageNode(m) {
  const n = el("article", "message");
  n.append(
    el(
      "small",
      "",
      (m.authorName ||
        m.author ||
        { human: "Você", user: "Você", agent: "Agente", system: "Sistema" }[
          m.senderKind
        ] ||
        m.kind ||
        "Sistema") +
        " · " +
        date(m.createdAt),
    ),
    el(
      "div",
      "",
      m.text || m.content || m.message || "Evento sem texto público",
    ),
  );
  if (m.status) n.append(el("small", "", m.status));
  if (m.id && m.senderKind !== "agent" && m.senderKind !== "system")
    n.append(
      button("Encaminhar ao coordenador", () => coordinatePlan(m.id), "quiet"),
    );
  return n;
}
const messageDrafts = new Map();
function messageDraft(projectId, recipient) {
  const storageKey = "gasa-message-draft:" + projectId + ":" + recipient;
  if (messageDrafts.has(storageKey)) return messageDrafts.get(storageKey);
  let saved;
  try {
    saved = JSON.parse(sessionStorage.getItem(storageKey));
  } catch {}
  const draft = {
    storageKey,
    projectId,
    recipient,
    key: saved?.key || crypto.randomUUID(),
    text: saved?.text || "",
    phase: saved?.phase === "sending" ? "unknown" : saved?.phase || "draft",
    payload: saved?.payload || null,
    inFlight: false,
  };
  messageDrafts.set(storageKey, draft);
  return draft;
}
function persistMessageDraft(draft) {
  try {
    sessionStorage.setItem(
      draft.storageKey,
      JSON.stringify({
        key: draft.key,
        text: draft.text,
        phase: draft.phase,
        payload: draft.payload,
      }),
    );
    return true;
  } catch {
    return false;
  }
}
function notifyMessageDraft(storageKey) {
  document
    .querySelectorAll(".composer")
    .forEach((form) =>
      form.dispatchEvent(
        new CustomEvent("office-draft-update", { detail: storageKey }),
      ),
    );
}
function composer(expanded, recipient) {
  const projectId = state.projectId;
  const form = el("form", "composer"),
    head = el("div", "composer-head");
  const choices = [
    ["team:", "Time · via coordenador"],
    ["coordinator:", "Coordenador"],
    ...agents()
      .filter((a) => a.status === "active")
      .map((a) => ["agent:" + a.id, a.name]),
  ];
  const initial = recipient
    ? "agent:" + recipient
    : state.selected
      ? "agent:" + state.selected
      : "team:";
  if (!choices.some(([id]) => id === initial))
    choices.push([initial, "Integrante indisponível"]);
  const to = select(choices, initial);
  to.setAttribute("aria-label", "Destinatário");
  head.append(el("span", "", "CONVERSAR COM"), to);
  const text = el("textarea");
  text.rows = expanded ? 3 : 2;
  text.placeholder = "Qual ideia vamos tirar do papel?";
  text.setAttribute("aria-label", "Mensagem");
  text.maxLength = 16000;
  const bottom = el("div", "composer-foot"),
    send = button("Enviar ↗", () => form.requestSubmit(), "primary");
  bottom.append(
    el(
      "small",
      "",
      state.capabilities.dispatch === true
        ? "Time encaminha ao coordenador, com as permissões do projeto."
        : "Mensagem persistida; execução depende de coordenação disponível.",
    ),
    send,
  );
  const error = el("p", "error");
  error.setAttribute("role", "alert");
  form.append(head, text, bottom, error);
  let draft = messageDraft(projectId, to.value);
  function bindDraft() {
    text.value = draft.text;
    text.readOnly = !!draft.payload;
    send.disabled = draft.inFlight;
    send.textContent = draft.inFlight
      ? "Enviando…"
      : draft.payload
        ? "Reconciliar mesmo envio"
        : "Enviar ↗";
    error.textContent = draft.payload
      ? "Este pedido já foi submetido. Consulte o mesmo recibo, com o texto e a chave originais; nenhum novo pedido será criado."
      : "";
  }
  bindDraft();
  text.addEventListener("input", () => {
    if (!draft.payload) {
      draft.text = text.value;
      persistMessageDraft(draft);
    }
  });
  to.addEventListener("change", () => {
    draft = messageDraft(projectId, to.value);
    bindDraft();
  });
  form.addEventListener("office-recipient", (event) => {
    if (!choices.some(([id]) => id === event.detail)) return;
    to.value = event.detail;
    draft = messageDraft(projectId, to.value);
    bindDraft();
  });
  form.addEventListener("office-draft-update", (event) => {
    if (draft.storageKey !== event.detail) return;
    draft = messageDraft(projectId, to.value);
    bindDraft();
  });
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const submitted = draft;
    if (submitted.inFlight || !submitted.text.trim()) return;
    if (!submitted.payload) {
      const [recipientKind, recipientId] = submitted.recipient.split(":");
      submitted.payload = {
        projectId,
        idempotencyKey: submitted.key,
        recipientKind,
        recipientId,
        text: submitted.text.trim(),
      };
    }
    submitted.phase = "sending";
    if (!persistMessageDraft(submitted)) {
      submitted.phase = "draft";
      submitted.payload = null;
      bindDraft();
      error.textContent =
        "O navegador não permitiu preservar o recibo deste pedido. Nenhuma mensagem foi enviada. Habilite armazenamento de sessão para enviar com recuperação segura.";
      return;
    }
    submitted.inFlight = true;
    bindDraft();
    notifyMessageDraft(submitted.storageKey);
    try {
      const value = await api("/messages", "POST", submitted.payload);
      // Clear exactly the confirmed operation, even if another recipient/project
      // became active while the request was in flight.
      messageDrafts.delete(submitted.storageKey);
      try {
        sessionStorage.removeItem(submitted.storageKey);
      } catch {}
      submitted.phase = "confirmed";
      submitted.text = "";
      submitted.payload = null;
      if (draft === submitted) {
        draft = messageDraft(projectId, to.value);
        bindDraft();
      }
      if (state.projectId === projectId) {
        await refresh();
        rootChatRefresh();
      }
      note(
        value.status === "accepted"
          ? "Pedido aceito para coordenação."
          : "Mensagem registrada. A execução ainda exige coordenação.",
      );
    } catch (errorResult) {
      submitted.phase = "unknown";
      if (
        errorResult.status >= 400 &&
        errorResult.status < 500 &&
        errorResult.status !== 409
      ) {
        submitted.phase = "draft";
        submitted.payload = null;
      }
      persistMessageDraft(submitted);
      if (draft === submitted) {
        bindDraft();
        error.textContent =
          errorMessage(errorResult) +
          " O texto e a chave originais foram preservados; use Reconciliar mesmo envio.";
      }
    } finally {
      submitted.inFlight = false;
      notifyMessageDraft(submitted.storageKey);
      if (draft === submitted || submitted.phase === "confirmed") bindDraft();
    }
  });
  return form;
}
function rootChatRefresh() {
  const section = page.querySelector(".office");
  if (section && state.selected) {
    section.querySelector(".chat-panel")?.remove();
    chatPanel(section);
  }
}
function sceneAction(action, agentId) {
  if (action === "activity") {
    const a = agents().find((a) => a.id === agentId);
    modal("Monitor · " + (a?.name || "Integrante"), (body) => {
      const list = tasks().filter((t) => t.agentId === agentId);
      if (!list.length)
        body.append(
          el(
            "p",
            "",
            "Nenhuma atividade de execução reportada para este integrante.",
          ),
        );
      for (const t of list)
        body.append(
          link(
            statusLabel(String(t.status).toLowerCase()) +
              " · " +
              (t.title || t.id),
            "tarefas/" + t.id,
          ),
        );
      body.append(link("Abrir conversa", "conversas/" + agentId));
    });
  } else if (action === "phone") {
    $("#notifications").click();
  } else if (action === "meeting") {
    modal("Mesa de reunião", (body) => {
      body.append(
        el(
          "p",
          "",
          "O planejamento usa os pedidos e decisões persistidos, sem iniciar uma conversa artificial entre agentes.",
        ),
      );
      const intents = state.snapshot?.intents || [];
      if (!intents.length)
        body.append(el("p", "", "Nenhum pedido aguardando coordenação."));
      for (const i of intents) {
        const m = messages().find((m) => m.id === i.messageId);
        body.append(el("p", "", i.status + " · " + (m?.text || i.messageId)));
        if (m)
          body.append(button("Ver encaminhamento", () => coordinatePlan(m.id)));
      }
      body.append(link("Abrir conversa do time", "conversas/time"));
    });
  } else navigate(action);
}
async function coordinatePlan(messageId) {
  try {
    const runtime = await api(
      "/runtime?projectId=" + encodeURIComponent(state.projectId),
    );
    const cap = runtime.capability || {};
    if (!cap.canLaunchCoordinator && !cap.canSubmitCoordinator) {
      modal("Coordenador indisponível", (body) =>
        body.append(
          el(
            "p",
            "",
            runtime.reason ||
              cap.reason ||
              "O host não comprovou capacidade de encaminhar ao coordenador. A mensagem permanece registrada.",
          ),
        ),
      );
      return;
    }
    const plan = await api("/coordinate-plan", "POST", {
      projectId: state.projectId,
      messageId,
    });
    modal("Encaminhar ao coordenador", (body) => {
      body.append(
        el(
          "p",
          "",
          "Este encaminhamento pode abrir ou continuar um agente real no host. A aceitação pelo host não comprova autorização de trabalho, execução ou conclusão.",
        ),
        summary([
          ["Projeto", activeProject()?.name],
          [
            "Coordenador",
            agents().find((a) => a.id === plan.coordinatorId)?.name ||
              plan.coordinatorId,
          ],
          ["Ação", plan.action],
          ["Runtime", plan.runtimeId],
          ["Worktree", plan.worktree],
          ["Validade", date(plan.expiresAt)],
          ["Hash", plan.hash],
        ]),
      );
      const d = el("details");
      d.append(
        el("summary", "", "Contexto público que será enviado"),
        el("pre", "", plan.prompt || "Não informado"),
      );
      body.append(d);
      const confirm = button(
        "Confirmar encaminhamento deste plano",
        async () => {
          confirm.disabled = true;
          try {
            const key = "gasa-coordinate:" + state.projectId + ":" + plan.id;
            let idempotencyKey;
            try {
              idempotencyKey =
                sessionStorage.getItem(key) || crypto.randomUUID();
              sessionStorage.setItem(key, idempotencyKey);
            } catch {
              idempotencyKey = crypto.randomUUID();
            }
            const op = await api("/coordinate", "POST", {
              projectId: state.projectId,
              planId: plan.id,
              planHash: plan.hash,
              confirmation: true,
              idempotencyKey,
            });
            showCoordination(op);
          } catch (e) {
            body.append(
              el(
                "p",
                "error",
                errorMessage(e) +
                  " Consulte o runtime antes de tentar outro encaminhamento.",
              ),
            );
          }
        },
        "primary",
      );
      body.append(confirm);
    });
  } catch (e) {
    note(errorMessage(e));
  }
}
function showCoordination(operation) {
  modal("Encaminhamento ao coordenador", (body) => {
    body.append(
      summary([
        ["Operação", operation.id],
        ["Estado", operation.status],
        ["Entrega", operation.deliveryStatus || "Não confirmada"],
        ["Terminal", operation.terminal || "Não informado"],
        ["Atualizado", date(operation.updatedAt)],
      ]),
    );
    body.append(
      el(
        "p",
        "warning",
        operation.status === "accepted"
          ? "O host aceitou a entrega pública. A decisão do coordenador e os gates continuam separados."
          : operation.status === "unknown"
            ? "Resultado desconhecido. Não reenvie sem conciliar o host."
            : operation.reason ||
              "Solicitação persistida. Aguarde a observação do host.",
      ),
    );
    body.append(
      button("Consultar estado observado", async () => {
        try {
          const runtime = await api(
            "/runtime?projectId=" + encodeURIComponent(state.projectId),
          );
          const current = (runtime.operations || []).find(
            (o) => o.id === operation.id,
          );
          if (current) showCoordination(current);
          else note("Operação ainda não observada; não repetir o envio.");
        } catch (e) {
          note(errorMessage(e));
        }
      }),
    );
  });
}
function teamPage(root) {
  heading(
    root,
    "Gente boa, ideias grandes.",
    "Cada personagem tem seu papel. A ferramenta é só parte da história.",
    link("+ Criar agente", "agentes/novo", "button primary"),
  );
  root.append(link("Gerenciar duplas e trios", "equipes", "button"));
  const filter = select(
    [
      ["active", "Ativos e pausados"],
      ["draft", "Rascunhos"],
      ["archived", "Arquivados"],
      ["all", "Todos"],
    ],
    "active",
  );
  filter.setAttribute("aria-label", "Filtrar integrantes");
  const search = input("", "search");
  search.placeholder = "Buscar nome ou papel";
  search.setAttribute("aria-label", "Buscar integrantes");
  const filters = el("div", "filters");
  filters.append(search, filter);
  root.append(filters);
  const grid = el("div", "grid");
  root.append(grid);
  const draw = () => {
    grid.replaceChildren();
    const list = agents().filter(
      (a) =>
        (filter.value === "all" || filter.value === "active"
          ? !["draft", "archived"].includes(a.status) || filter.value === "all"
          : a.status === filter.value) &&
        (a.name + " " + a.role)
          .toLowerCase()
          .includes(search.value.toLowerCase()),
    );
    if (!list.length) {
      empty(
        grid,
        "Espaço para novas ideias",
        "Crie seu primeiro agente ou altere o filtro.",
        link("Criar agente", "agentes/novo", "button primary"),
      );
      return;
    }
    for (const a of list) {
      const card = el("article", "card agent-card"),
        top = el("div", "card-top");
      const art = el("div", "agent-art");
      art.append(avatarIcon(a));
      top.append(
        art,
        el(
          "span",
          "badge",
          {
            active: "Sem sessão",
            paused: "Pausado",
            draft: "Rascunho",
            archived: "Arquivado",
          }[a.status] || a.status,
        ),
      );
      card.append(
        top,
        el("h2", "", a.name),
        el("p", "", a.role),
        el("p", "muted", a.whenToCall || "Quando chamar: não informado"),
        summary([
          ["IA principal", toolName(a.primary?.toolId)],
          [
            "Fallback",
            (a.fallbacks || []).map((f) => toolName(f.toolId)).join(" → ") ||
              "Sem fallback",
          ],
          ["Mesa", a.deskId || "Não alocada"],
        ]),
      );
      const actions = el("div", "actions");
      actions.append(
        link("Conversar", "conversas/" + a.id),
        link("Editar", "agentes/" + a.id),
      );
      card.append(actions);
      grid.append(card);
    }
  };
  search.addEventListener("input", draw);
  filter.addEventListener("change", draw);
  draw();
  let draft;
  try {
    draft = localStorage.getItem(draftKey());
  } catch {}
  if (draft) {
    const b = button("Continuar rascunho deste navegador", () =>
      navigate("agentes/novo"),
    );
    root.append(b);
  }
}
const draftKey = () => "gasa-office-draft:" + state.projectId;
function blankAgent() {
  return {
    name: "",
    role: agents().length ? "desenvolvimento" : "coordinator",
    whenToCall: "",
    instructions: "",
    avatar: {
      familyId: "fox",
      color: "terracotta",
      eyes: "round",
      accessories: [],
    },
    primary: { toolId: "", model: "", effort: "" },
    fallbacks: [],
    fallbackMode: "confirm",
    allowedCapabilities: [],
    teamIds: [],
    status: "active",
  };
}
function saveLocalDraft() {
  if (!state.wizard) return;
  try {
    localStorage.setItem(
      draftKey() +
        (state.wizard._editing ? ":edit:" + state.wizard._editing : ""),
      JSON.stringify({ agent: state.wizard, step: state.wizardStep }),
    );
  } catch {}
}
function wizardPage(root) {
  root.classList.add("wizard");
  const id = state.route.split("/")[1],
    editing = id !== "novo";
  if (!state.wizard || state.wizard._editing !== (editing ? id : null)) {
    if (editing) {
      const a = agents().find((a) => a.id === id);
      if (!a) {
        empty(
          root,
          "Integrante não encontrado",
          "Atualize o escritório para consultar o estado mais recente.",
          button("Atualizar", async () => {
            await refresh();
            render();
          }),
        );
        return;
      }
      state.wizard = {
        ...structuredClone(a),
        _editing: id,
        _key: crypto.randomUUID(),
      };
      state.wizardStep = 0;
      try {
        const draft = JSON.parse(
          localStorage.getItem(draftKey() + ":edit:" + id),
        );
        if (draft?.agent?.revision === a.revision) {
          state.wizard = { ...draft.agent, _editing: id };
          state.wizardStep = draft.step || 0;
        }
      } catch {}
    } else {
      let draft;
      try {
        draft = JSON.parse(localStorage.getItem(draftKey()));
      } catch {}
      state.wizard = {
        ...(draft?.agent || blankAgent()),
        _editing: null,
        _key: draft?.agent?._key || crypto.randomUUID(),
      };
      state.wizardStep = draft?.step || 0;
    }
  }
  const a = state.wizard;
  heading(
    root,
    editing ? "Um toque seu." : "Conheça seu próximo integrante.",
    "Personalize, configure e coloque no escritório. Salvar não inicia uma tarefa.",
  );
  const steps = el("div", "steps");
  [
    "Bichinho",
    "Nome e função",
    "Principal e fallback",
    "Permissões",
    "Revisar",
  ].forEach((s, i) =>
    steps.append(
      el(
        "div",
        "step" + (i === state.wizardStep ? " current" : ""),
        i + 1 + ". " + s,
      ),
    ),
  );
  root.append(steps);
  const layout = el("div", "wizard-layout"),
    preview = el("aside", "wizard-preview");
  const art = el("div", "preview-art");
  art.append(avatarIcon(a));
  preview.append(art);
  const details = el("div");
  details.append(
    el("h2", "", a.name || "Seu integrante"),
    el("p", "", a.role || "Um papel a definir"),
    el("p", "muted", toolName(a.primary?.toolId)),
    el("p", "source", "Geometria original · prévia ilustrativa"),
  );
  preview.append(details);
  const content = el("div", "card fields");
  layout.append(preview, content);
  root.append(layout);
  const err = el("p", "error");
  err.setAttribute("role", "alert");
  const step = state.wizardStep;
  if (step === 0) petStep(content, a);
  if (step === 1) identityStep(content, a);
  if (step === 2) engineStep(content, a);
  if (step === 3) permissionsStep(content, a);
  if (step === 4) {
    content.append(
      el("h2", "", "Tudo pronto para fazer parte."),
      summary([
        ["Nome e papel", a.name + " · " + a.role],
        ["Escritório", activeProject()?.name],
        [
          "Personagem",
          pets.find((p) => p[0] === a.avatar.familyId)?.[1] +
            " · " +
            colors.find((c) => c[0] === a.avatar.color)?.[1],
        ],
        [
          "IA principal",
          toolName(a.primary.toolId) +
            (a.primary.model
              ? " / " + a.primary.model
              : " / padrão da ferramenta"),
        ],
        [
          "Fallback",
          (a.fallbacks || []).map((f) => toolName(f.toolId)).join(" → ") ||
            "Sem fallback",
        ],
        [
          "Política",
          a.fallbackMode === "confirm"
            ? "Pedir confirmação"
            : "Automático nos casos seguros",
        ],
        [
          "Permissões",
          a.allowedCapabilities.join(", ") || "Nenhuma capacidade autorizada",
        ],
        [
          "Equipes",
          a.teamIds
            .map((id) => teams().find((t) => t.id === id)?.name || id)
            .join(", ") || "Nenhuma equipe fixa",
        ],
      ]),
      el(
        "p",
        "warning",
        "A mesa aparece após confirmação do servidor. Nenhum processo, login ou chamada de IA é iniciado por este cadastro.",
      ),
    );
  }
  content.append(err);
  const footer = el("div", "wizard-footer");
  if (step > 0)
    footer.append(
      button("← Voltar", () => {
        state.wizardStep--;
        render();
      }),
    );
  else footer.append(link("Cancelar", "equipe"));
  footer.append(
    button("Salvar rascunho", () => {
      saveLocalDraft();
      note("Rascunho salvo neste navegador. Nenhuma mesa foi reservada.");
    }),
  );
  const next = button(
    step === 4
      ? editing
        ? "Salvar alterações"
        : "Criar e colocar no escritório"
      : "Próximo →",
    async () => {
      const invalid = validateStep(a, step);
      if (invalid) {
        err.textContent = invalid;
        return;
      }
      if (step < 4) {
        state.wizardStep++;
        saveLocalDraft();
        render();
        return;
      }
      next.disabled = true;
      try {
        const payload = agentPayload(a);
        if (editing)
          await api("/agents/" + encodeURIComponent(id), "PUT", {
            projectId: state.projectId,
            revision: a.revision,
            ...payload,
          });
        else {
          const created = await api("/agents", "POST", {
            projectId: state.projectId,
            idempotencyKey: a._key,
            ...payload,
          });
          state.selected = created.id;
        }
        try {
          localStorage.removeItem(draftKey() + (editing ? ":edit:" + id : ""));
        } catch {}
        state.wizard = null;
        await refresh();
        navigate("escritorio");
        note(
          editing
            ? "Configuração atualizada."
            : "Integrante criado. Sua mesa está no escritório.",
        );
      } catch (e) {
        err.textContent = errorMessage(e);
        saveLocalDraft();
      } finally {
        next.disabled = false;
      }
    },
    "primary",
  );
  footer.append(next);
  content.append(footer);
  if (editing) {
    const actions = el("div", "actions");
    for (const [status, label] of [
      ["paused", "Pausar"],
      ["active", "Restaurar / ativar"],
      ["archived", "Arquivar"],
    ]) {
      if (a.status === status) continue;
      actions.append(
        button(label, () =>
          modal(label + " " + a.name, (body) => {
            body.append(
              el(
                "p",
                "",
                "A mudança preserva a identidade e o histórico. O servidor valida tarefas e permissões antes de confirmar.",
              ),
            );
            body.append(
              button(
                "Confirmar " + label.toLowerCase(),
                async () => {
                  try {
                    await api("/agents/" + encodeURIComponent(id), "PUT", {
                      projectId: state.projectId,
                      revision: a.revision,
                      ...agentPayload(a),
                      status,
                    });
                    $("#modal").close();
                    state.wizard = null;
                    await refresh();
                    render();
                    note("Estado atualizado.");
                  } catch (e) {
                    note(errorMessage(e));
                  }
                },
                status === "archived" ? "danger" : "primary",
              ),
            );
          }),
        ),
      );
    }
    root.append(actions);
  }
}
function agentPayload(a) {
  return {
    name: a.name.trim(),
    role: a.role.trim(),
    whenToCall: a.whenToCall.trim(),
    instructions: a.instructions,
    avatar: a.avatar,
    primary: a.primary,
    fallbacks: a.fallbacks,
    fallbackMode: a.fallbackMode,
    allowedCapabilities: a.allowedCapabilities,
    teamIds: a.teamIds,
    status: a.status,
  };
}
function validateStep(a, step) {
  if (
    step === 1 &&
    (!a.name.trim() || a.name.trim().length > 60 || !a.role.trim())
  )
    return "Preencha um nome (até 60 caracteres) e um papel.";
  if (step === 2 || step === 4) {
    if (!readyTools().some((t) => t.id === a.primary.toolId))
      return "Escolha uma IA principal verificada como pronta. Você pode salvar um rascunho enquanto configura a ferramenta.";
    const signatures = [a.primary, ...a.fallbacks].map((f) =>
      JSON.stringify(f),
    );
    if (new Set(signatures).size !== signatures.length)
      return "Uma combinação de ferramenta, modelo e esforço não pode se repetir.";
    if (a.fallbacks.some((f) => !readyTools().some((t) => t.id === f.toolId)))
      return "Um fallback está indisponível. Atualize a detecção ou remova essa alternativa.";
  }
  return "";
}
function petStep(root, a) {
  root.append(
    el("h2", "", "Quem vai ocupar essa mesa?"),
    el(
      "p",
      "",
      "A aparência é sua. Ela não define a ferramenta ou o que o integrante pode fazer.",
    ),
  );
  const gallery = el("div", "pet-grid");
  for (const [id, name, emoji] of pets) {
    const b = button(
      "",
      () => {
        a.avatar.familyId = id;
        render();
      },
      "pet" + (a.avatar.familyId === id ? " selected" : ""),
    );
    b.setAttribute("aria-pressed", a.avatar.familyId === id);
    b.append(
      avatarIcon({ avatar: { ...a.avatar, familyId: id } }),
      el("strong", "", name),
    );
    gallery.append(b);
  }
  root.append(gallery, el("h3", "", "Uma cor para chamar de sua"));
  const swatches = el("div", "swatches");
  for (const [id, name] of colors) {
    const b = button(
      "",
      () => {
        a.avatar.color = id;
        render();
      },
      "swatch-" + id + (a.avatar.color === id ? " selected" : ""),
    );
    b.setAttribute("aria-label", name);
    b.setAttribute("aria-pressed", a.avatar.color === id);
    swatches.append(b);
  }
  root.append(swatches);
  const eyes = select(
    [
      ["round", "Olhar curioso"],
      ["soft", "Olhar tranquilo"],
    ],
    a.avatar.eyes || "round",
  );
  eyes.addEventListener("change", () => {
    a.avatar.eyes = eyes.value;
    render();
  });
  root.append(field("Expressão", eyes));
  const accessory = input("", "checkbox");
  accessory.checked = a.avatar.accessories?.includes("headphones");
  accessory.addEventListener("change", () => {
    a.avatar.accessories = accessory.checked ? ["headphones"] : [];
    render();
  });
  const l = field("Fones de ouvido", accessory);
  l.classList.add("check");
  root.append(
    l,
    button(
      "Surpreenda-me",
      () => {
        a.avatar.familyId = pets[Math.floor(Math.random() * pets.length)][0];
        a.avatar.color = colors[Math.floor(Math.random() * colors.length)][0];
        render();
      },
      "quiet",
    ),
  );
}
function identityStep(root, a) {
  root.append(el("h2", "", "Um nome. Um talento."));
  for (const [key, label, max] of [
    ["name", "Nome", 60],
    ["role", "Papel", 100],
    ["whenToCall", "Quando chamar", 300],
  ]) {
    const i = input(a[key]);
    i.maxLength = max;
    i.required = key !== "whenToCall";
    i.addEventListener("input", () => {
      a[key] = i.value;
      if (key === "name")
        page.querySelector(".wizard-preview h2").textContent =
          i.value || "Seu integrante";
    });
    root.append(field(label, i));
  }
  const instructions = el("textarea");
  instructions.value = a.instructions;
  instructions.maxLength = 16384;
  instructions.addEventListener(
    "input",
    () => (a.instructions = instructions.value),
  );
  root.append(
    field(
      "Instruções de trabalho",
      instructions,
      "Instruções não ampliam permissões do projeto.",
    ),
  );
}
function engineChoice(root, label, choice) {
  const choices = [
    ["", "Selecione uma IA pronta"],
    ...state.tools.map((t) => [
      t.id,
      t.name + " · " + (t.selectable ? "Pronta" : toolStatus(t)),
      !t.selectable,
    ]),
  ];
  if (choice.toolId && !state.tools.some((t) => t.id === choice.toolId))
    choices.push([choice.toolId, choice.toolId + " · indisponível", true]);
  const tool = select(choices, choice.toolId);
  tool.addEventListener("change", () => {
    choice.toolId = tool.value;
    choice.model = "";
    choice.effort = "";
    render();
  });
  root.append(field(label, tool));
  const t = state.tools.find((t) => t.id === choice.toolId);
  for (const [key, title, cap] of [
    ["model", "Modelo", "modelSelection"],
    ["effort", "Esforço", "effortSelection"],
  ]) {
    const values = t?.[key === "model" ? "models" : "efforts"];
    if (t?.capabilities?.[cap] && Array.isArray(values) && values.length) {
      const s = select(
        [
          ["", "Padrão da ferramenta"],
          ...values.map((v) =>
            typeof v === "string" ? [v, v] : [v.id, v.name || v.id],
          ),
        ],
        choice[key],
      );
      s.addEventListener("change", () => (choice[key] = s.value));
      root.append(field(title, s));
    } else
      root.append(
        el(
          "p",
          "source",
          title +
            ": padrão da ferramenta. Seleção específica não reportada pelo host.",
        ),
      );
  }
}
function engineStep(root, a) {
  root.append(el("h2", "", "A IA certa para cada ideia."));
  engineChoice(root, "IA principal", a.primary);
  if (!readyTools().length)
    root.append(
      el(
        "p",
        "warning",
        "Nenhuma ferramenta pronta foi verificada. Salve seu rascunho e configure as IAs da máquina.",
      ),
    );
  root.append(
    button("Configurar IAs da máquina", () =>
      modal("IAs da máquina", (body) => renderTools(body, true)),
    ),
  );
  root.append(el("h3", "", "Se precisar de um plano B"));
  a.fallbacks.forEach((f, i) => {
    const row = el("div", "fallback");
    row.append(
      el("span", "", i + 1 + ". " + toolName(f.toolId)),
      button("↑", () => {
        if (i > 0) {
          [a.fallbacks[i - 1], a.fallbacks[i]] = [
            a.fallbacks[i],
            a.fallbacks[i - 1],
          ];
          render();
        }
      }),
      button("↓", () => {
        if (i < a.fallbacks.length - 1) {
          [a.fallbacks[i + 1], a.fallbacks[i]] = [
            a.fallbacks[i],
            a.fallbacks[i + 1],
          ];
          render();
        }
      }),
      button("×", () => {
        a.fallbacks.splice(i, 1);
        render();
      }),
    );
    root.append(row);
    engineChoice(root, "Alternativa " + (i + 1), f);
  });
  root.append(
    button("+ Adicionar fallback", () => {
      a.fallbacks.push({ toolId: "", model: "", effort: "" });
      render();
    }),
  );
  const mode = select(
    [
      ["confirm", "Pedir confirmação"],
      ["automatic_safe", "Automático nos casos seguros"],
    ],
    a.fallbackMode,
  );
  mode.addEventListener("change", () => (a.fallbackMode = mode.value));
  root.append(
    field(
      "Política de troca",
      mode,
      "Uma tentativa incerta precisa ser conciliada antes de usar outra IA. Fallback não amplia permissões.",
    ),
  );
}
function permissionsStep(root, a) {
  root.append(
    el("h2", "", "Liberdade, com limites claros."),
    el(
      "p",
      "",
      "Autorize apenas capacidades verificadas na ferramenta. O servidor aplica os limites do projeto.",
    ),
  );
  const tool = state.tools.find((t) => t.id === a.primary.toolId);
  const names = {
    read: "Ler arquivos",
    search: "Buscar no projeto",
    edit: "Editar e criar arquivos",
    shell: "Executar comandos",
    web: "Acessar a web",
  };
  for (const [key, label] of Object.entries(names)) {
    const i = input("", "checkbox");
    i.checked = a.allowedCapabilities.includes(key);
    i.disabled = tool?.capabilities?.[key] !== true;
    i.addEventListener(
      "change",
      () =>
        (a.allowedCapabilities = i.checked
          ? [...new Set([...a.allowedCapabilities, key])]
          : a.allowedCapabilities.filter((c) => c !== key)),
    );
    const f = field(
      label,
      i,
      i.disabled ? "Capacidade não comprovada pelo host." : undefined,
    );
    f.classList.add("check");
    root.append(f);
  }
  root.append(el("h3", "", "Equipes fixas"));
  if (!teams().length)
    root.append(
      el(
        "p",
        "muted",
        "Nenhuma dupla ou trio criado. Você pode organizar os integrantes depois.",
      ),
    );
  for (const t of teams()) {
    const i = input("", "checkbox");
    i.checked = a.teamIds.includes(t.id);
    i.addEventListener(
      "change",
      () =>
        (a.teamIds = i.checked
          ? [...a.teamIds, t.id]
          : a.teamIds.filter((id) => id !== t.id)),
    );
    const f = field(t.name, i);
    f.classList.add("check");
    root.append(f);
  }
  root.append(
    el(
      "p",
      "warning",
      "Autonomia herdada do escritório. Esta configuração não dá acesso superior à política do projeto.",
    ),
  );
}
function groupsPage(root) {
  heading(
    root,
    "Boas combinações.",
    "Duplas e trios para um propósito. Convocar continua sendo uma decisão do coordenador.",
    button("+ Criar equipe", () => groupEditor()),
  );
  const grid = el("div", "grid");
  root.append(grid);
  for (const t of teams()) {
    const c = el("article", "card");
    c.append(
      el("span", "badge", t.archived ? "Arquivada" : "Equipe fixa"),
      el("h2", "", t.name),
      el("p", "", t.purpose || "Propósito não informado"),
      summary([
        [
          "Integrantes",
          (t.memberIds || [])
            .map((id) => agents().find((a) => a.id === id)?.name || id)
            .join(", ") || "Nenhum",
        ],
        [
          "Líder",
          agents().find((a) => a.id === t.leaderId)?.name || "Não definido",
        ],
      ]),
    );
    c.append(button("Editar equipe", () => groupEditor(t)));
    grid.append(c);
  }
  if (!teams().length)
    empty(
      grid,
      "Melhor em companhia",
      "Organize seus integrantes por afinidade e objetivo. Criar uma equipe não inicia trabalho.",
    );
  root.append(link("← Ver integrantes", "equipe", "button"));
}
function groupEditor(team) {
  modal(team ? "Editar equipe" : "Uma nova combinação", (body) => {
    const form = el("form", "fields"),
      name = input(team?.name || ""),
      purpose = el("textarea"),
      chosen = new Set(team?.memberIds || []);
    purpose.value = team?.purpose || "";
    name.required = true;
    name.maxLength = 60;
    form.append(field("Nome da equipe", name), field("Propósito", purpose));
    for (const a of agents().filter((a) => a.status !== "archived")) {
      const i = input("", "checkbox");
      i.checked = chosen.has(a.id);
      i.addEventListener("change", () =>
        i.checked ? chosen.add(a.id) : chosen.delete(a.id),
      );
      const f = field(a.name + " · " + a.role, i);
      f.classList.add("check");
      form.append(f);
    }
    const leader = select(
      [["", "Selecione um líder"], ...agents().map((a) => [a.id, a.name])],
      team?.leaderId,
    );
    form.append(field("Líder", leader));
    const error = el("p", "error"),
      save = button("Salvar equipe", () => form.requestSubmit(), "primary");
    form.append(error, save);
    const key = crypto.randomUUID();
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (chosen.size < 2 || chosen.size > 3) {
        error.textContent =
          "Selecione dois ou três integrantes para uma dupla ou trio.";
        return;
      }
      if (!chosen.has(leader.value)) {
        error.textContent = "Escolha um líder entre os integrantes.";
        return;
      }
      save.disabled = true;
      try {
        await api(
          team ? "/teams/" + encodeURIComponent(team.id) : "/teams",
          team ? "PUT" : "POST",
          {
            projectId: state.projectId,
            ...(team ? { revision: team.revision } : { idempotencyKey: key }),
            name: name.value.trim(),
            purpose: purpose.value,
            memberIds: [...chosen],
            leaderId: leader.value,
            archived: !!team?.archived,
          },
        );
        await refresh();
        $("#modal").close();
        render();
        note("Equipe salva.");
      } catch (e) {
        error.textContent = errorMessage(e);
      } finally {
        save.disabled = false;
      }
    });
    if (team && !team.archived)
      form.append(
        button(
          "Arquivar equipe",
          async () => {
            try {
              await api("/teams/" + encodeURIComponent(team.id), "PUT", {
                projectId: state.projectId,
                revision: team.revision,
                name: team.name,
                purpose: team.purpose,
                memberIds: team.memberIds,
                leaderId: team.leaderId,
                archived: true,
              });
              await refresh();
              $("#modal").close();
              render();
            } catch (e) {
              error.textContent = errorMessage(e);
            }
          },
          "danger",
        ),
      );
    body.append(form);
  });
}
function chatPage(root, id) {
  root.classList.add("chat-page");
  const a = agents().find((a) => a.id === id);
  heading(
    root,
    a ? "Uma conversa com " + a.name : "A conversa do time",
    a
      ? a.role + " · " + toolName(a.primary?.toolId)
      : "Pedidos do time passam pelo coordenador.",
    link("Voltar ao escritório", "escritorio"),
  );
  const timeline = el("div", "timeline");
  const list = messages().filter(
    (m) => !a || m.recipientId === a.id || m.agentId === a.id,
  );
  for (const m of list) timeline.append(messageNode(m));
  if (!list.length)
    empty(
      timeline,
      "Ainda não há mensagens",
      "Descreva o que você quer construir. A persistência de uma mensagem não comprova uma execução.",
    );
  root.append(timeline, composer(true, a?.id));
}
const statusLabel = (s) =>
  ({
    queued: "Na fila",
    pending: "Aguardando coordenação",
    running: "Em execução",
    blocked: "Bloqueada",
    review: "Em revisão",
    done: "Concluída",
    completed: "Concluída",
    failed: "Falhou",
    cancelled: "Cancelada",
    unknown: "Estado desconhecido",
  })[s] ||
  s ||
  "Não informado";
function taskPage(root) {
  heading(
    root,
    "Do pedido à entrega.",
    "Acompanhe o que foi confirmado. Uma execução encerrada ainda precisa passar pelos gates.",
  );
  const filter = select(
    [
      ["all", "Todos os estados"],
      ["queued", "Na fila"],
      ["running", "Em execução"],
      ["blocked", "Bloqueios"],
      ["done", "Encerradas"],
    ],
    "all",
  );
  filter.setAttribute("aria-label", "Filtrar tarefas");
  const search = input("", "search");
  search.placeholder = "Buscar tarefa";
  search.setAttribute("aria-label", "Buscar tarefa");
  const filters = el("div", "filters");
  filters.append(search, filter);
  root.append(filters);
  const board = el("div", "board");
  root.append(board);
  function draw() {
    board.replaceChildren();
    const groups = [
      ["A organizar", ["queued", "pending", "planning", "coordination"]],
      ["Em andamento", ["running", "blocked", "review", "unknown"]],
      ["Encerradas", ["done", "completed", "failed", "cancelled"]],
    ];
    for (const [title, states] of groups) {
      const col = el("section", "board-col");
      col.append(el("h2", "", title));
      const list = tasks().filter(
        (t) =>
          states.includes(String(t.status).toLowerCase()) &&
          (filter.value === "all" ||
            (filter.value === "done" &&
              groups[2][1].includes(String(t.status).toLowerCase())) ||
            String(t.status).toLowerCase() === filter.value) &&
          (t.title || t.request || t.id)
            .toLowerCase()
            .includes(search.value.toLowerCase()),
      );
      for (const t of list) {
        const a = link("", "tarefas/" + t.id, "task");
        a.append(
          el("span", "badge", statusLabel(String(t.status).toLowerCase())),
          el("h3", "", t.title || t.request || t.id),
          el(
            "p",
            "muted",
            agents().find((a) => a.id === t.agentId)?.name ||
              "Responsável não informado",
          ),
        );
        col.append(a);
      }
      if (!list.length)
        col.append(el("p", "muted", "Nenhuma tarefa neste filtro."));
      board.append(col);
    }
  }
  search.addEventListener("input", draw);
  filter.addEventListener("change", draw);
  draw();
}
function taskDetail(root, id) {
  const t = tasks().find((t) => t.id === id);
  if (!t) {
    empty(
      root,
      "Tarefa não encontrada",
      "Atualize os dados ou volte ao quadro.",
      link("Ver tarefas", "tarefas"),
    );
    return;
  }
  heading(
    root,
    t.title || "Detalhe da tarefa",
    statusLabel(String(t.status).toLowerCase()),
    link("← Tarefas", "tarefas"),
  );
  const card = el("article", "card");
  card.append(
    summary([
      ["Pedido", t.request || t.description || "Não informado"],
      [
        "Responsável",
        agents().find((a) => a.id === t.agentId)?.name || t.agentId,
      ],
      ["Estado", statusLabel(String(t.status).toLowerCase())],
      ["Criada", date(t.createdAt)],
      ["Atualizada", date(t.updatedAt)],
      ["Ferramenta efetiva", t.toolId ? toolName(t.toolId) : "Não informada"],
      ["Plano", t.plan || "Não registrado"],
      ["Decisão", t.decision || "Não registrada"],
      ["Fallback", t.fallbackReason || "Não informado"],
    ]),
  );
  for (const [title, value] of [
    ["Tentativas", t.attempts],
    ["Mudanças", t.changes || t.diff],
    ["Checks e evidências", t.checks || t.evidence],
  ]) {
    const d = el("details");
    d.append(
      el("summary", "", title),
      el(
        "pre",
        "",
        value
          ? typeof value === "string"
            ? value
            : JSON.stringify(value, null, 2)
          : "Nenhuma evidência registrada.",
      ),
    );
    card.append(d);
  }
  if (state.capabilities.taskStop === true)
    card.append(
      button(
        "Solicitar parada desta tarefa",
        () => requestStop(t.id),
        "danger",
      ),
    );
  else
    card.append(
      el(
        "p",
        "source",
        "Parada individual não comprovada pelo backend. A barreira do projeto fica disponível na barra superior.",
      ),
    );
  root.append(card);
  for (const m of messages().filter((m) => m.taskId === id))
    root.append(messageNode(m));
}
function dashboard(root) {
  heading(
    root,
    "Seu time, em perspectiva.",
    "Métricas do projeto derivadas de registros confirmados. Fuso: " +
      Intl.DateTimeFormat().resolvedOptions().timeZone,
  );
  const filters = el("div", "filters");
  const from = input("", "date"),
    to = input("", "date");
  const agent = select(
    [["", "Todos os agentes"], ...agents().map((a) => [a.id, a.name])],
    "",
  );
  const tool = select(
    [["", "Todas as ferramentas"], ...state.tools.map((t) => [t.id, t.name])],
    "",
  );
  from.setAttribute("aria-label", "Data inicial");
  to.setAttribute("aria-label", "Data final");
  agent.setAttribute("aria-label", "Agente");
  tool.setAttribute("aria-label", "Ferramenta");
  from.disabled = true;
  to.disabled = true;
  tool.disabled = true;
  for (const control of [from, to, tool])
    control.title =
      "O snapshot atual não reporta os campos necessários para este recorte.";
  filters.append(from, to, agent, tool);
  root.append(
    filters,
    el(
      "p",
      "source",
      "Recorte de data e ferramenta indisponível: o host ainda não fornece datas e ferramenta efetiva por tarefa.",
    ),
  );
  const result = el("div");
  root.append(result);
  function draw() {
    result.replaceChildren();
    const all = agent.value === "";
    const list = tasks().filter((t) => all || t.agentId === agent.value);
    const metrics = state.snapshot?.metrics;
    const known = all && metrics;
    const grid = el("div", "grid");
    const values = [
      [
        "Pedidos registrados",
        known ? String(metrics.requests) : "Não informado",
        "Intenções únicas registradas no projeto",
      ],
      [
        "Tarefas registradas",
        String(list.length),
        "IDs únicos no snapshot do projeto",
      ],
      [
        "Conclusões verificadas",
        known ? String(metrics.verified) : "Não informado",
        "Tarefa e run encerrados como DONE, conforme projeção do backend",
      ],
      [
        "Taxa de sucesso",
        known &&
        metrics.successRate !== null &&
        metrics.successRate !== undefined
          ? Math.round(metrics.successRate * 100) + "%"
          : "Sem amostra",
        known
          ? metrics.verified +
            " verificadas / " +
            metrics.closed +
            " encerradas; canceladas: " +
            metrics.cancelled
          : "Não há agregação de gates por agente disponível",
      ],
      [
        "Assinatura / quota",
        "Não informado",
        "O host não reportou limite ou consumo da conta",
      ],
      [
        "Tokens",
        known && metrics.tokens !== null && metrics.tokens !== undefined
          ? String(metrics.tokens)
          : "Não informado",
        "Somente valor com fonte verificável",
      ],
      [
        "Tempo decorrido",
        "Não informado",
        "Sem intervalos confirmados para este recorte",
      ],
    ];
    for (const [label, value, description] of values) {
      const c = el("article", "card metric");
      c.append(
        el("span", "muted", label),
        el("strong", "", value),
        el("p", "source", description),
      );
      grid.append(c);
    }
    result.append(grid);
    const counts = new Map();
    for (const t of list) {
      const key =
        agents().find((a) => a.id === t.agentId)?.name ||
        "Sem responsável informado";
      counts.set(key, (counts.get(key) || 0) + 1);
    }
    const breakdown = el("div", "card");
    breakdown.append(el("h2", "", "Distribuição de tarefas"));
    if (!counts.size)
      breakdown.append(el("p", "", "Nenhuma tarefa registrada."));
    for (const [label, count] of counts)
      breakdown.append(el("p", "", label + " · " + count + " tarefa(s)"));
    result.append(
      breakdown,
      el(
        "p",
        "source",
        "Fonte: snapshot local · cursor " +
          (state.snapshot?.cursor ?? "não informado") +
          ". Consumo de assinatura pode incluir atividades externas ao projeto; não é inferido a partir de tokens.",
      ),
    );
  }
  agent.addEventListener("change", draw);
  draw();
}
const settingsTabs = [
  ["ferramentas", "IAs da máquina"],
  ["orquestracao", "Orquestração"],
  ["integracoes", "Integrações"],
  ["voz", "Voz"],
  ["aparencia", "Aparência"],
];
function settingsPage(root, tab) {
  heading(
    root,
    "Um estúdio do seu jeito.",
    "Preferências, ferramentas e limites claros para seu time.",
  );
  const nav = el("nav", "settings-nav");
  nav.setAttribute("aria-label", "Ajustes");
  for (const [id, title] of settingsTabs)
    nav.append(link(title, "ajustes/" + id, id === tab ? "active" : ""));
  nav.append(link("Copiloto", "copiloto", ""));
  root.append(nav);
  if (tab === "ferramentas") renderTools(root);
  else if (tab === "orquestracao") orchestration(root);
  else if (tab === "integracoes") integrations(root);
  else if (tab === "voz") voice(root);
  else if (tab === "aparencia") appearance(root);
}
function toolStatus(t) {
  const labels = {
    ready: "Pronta",
    missing: "Não instalada",
    not_installed: "Não instalada",
    needs_login: "Conectar conta",
    installed: "Instalada · verificar autenticação",
    unknown: "Verificação inconclusiva",
    error: "Erro de verificação",
    unsupported: "Indisponível",
    incompatible: "Incompatível",
    unavailable: "Indisponível",
  };
  return labels[t.status] || t.status || "Não verificada";
}
function renderTools(root, compact = false) {
  const top = el("div", "page-head");
  top.append(el("h2", "", "IAs desta máquina"));
  const discover = button("↻ Atualizar detecção", async () => {
    discover.disabled = true;
    discover.textContent = "Verificando esta máquina…";
    try {
      const data = await api("/tool-discovery", "POST", {
        projectId: state.projectId,
      });
      state.tools = data.tools || [];
      note("Detecção concluída.");
      if (compact) {
        $("#modal-content").replaceChildren(el("h2", "", "IAs da máquina"));
        renderTools($("#modal-content"), true);
      } else render();
    } catch (e) {
      note(errorMessage(e));
    } finally {
      discover.disabled = false;
      discover.textContent = "↻ Atualizar detecção";
    }
  });
  top.append(discover);
  root.append(top);
  let savedInstallation;
  try {
    savedInstallation = JSON.parse(
      sessionStorage.getItem("gasa-install-active:" + state.projectId),
    );
  } catch {}
  if (savedInstallation)
    root.append(
      button("Consultar instalação anterior", async () => {
        try {
          if (savedInstallation.id)
            showInstallJob(
              await api(
                "/tool-install-jobs/" +
                  encodeURIComponent(savedInstallation.id) +
                  "?projectId=" +
                  encodeURIComponent(state.projectId),
              ),
            );
          else if (savedInstallation.request)
            showInstallJob(
              await api(
                "/tool-install-jobs",
                "POST",
                savedInstallation.request,
              ),
            );
        } catch (error) {
          note(
            errorMessage(error) +
              " O pedido original foi preservado; não iniciar outra instalação sem conciliação.",
          );
        }
      }),
    );
  const search = input("", "search");
  search.placeholder = "Buscar ferramenta";
  search.setAttribute("aria-label", "Buscar ferramenta");
  const filter = select(
    [
      ["all", "Todas"],
      ["ready", "Prontas"],
      ["pending", "Com pendências"],
      ["missing", "Não instaladas"],
    ],
    "all",
  );
  filter.setAttribute("aria-label", "Filtrar ferramentas");
  const filters = el("div", "filters");
  filters.append(search, filter);
  root.append(filters);
  const grid = el("div", "grid");
  root.append(grid);
  const draw = () => {
    grid.replaceChildren();
    const list = state.tools.filter(
      (t) =>
        t.name.toLowerCase().includes(search.value.toLowerCase()) &&
        (filter.value === "all" ||
          (filter.value === "ready" && t.selectable) ||
          (filter.value === "missing" &&
            ["missing", "not_installed"].includes(
              String(t.status).toLowerCase(),
            )) ||
          (filter.value === "pending" &&
            !t.selectable &&
            !["missing", "not_installed"].includes(
              String(t.status).toLowerCase(),
            ))),
    );
    for (const t of list) {
      const c = el("article", "card");
      c.append(
        el("span", "badge", toolStatus(t)),
        el("h2", "", t.name),
        el("p", "", t.note || "Sem diagnóstico adicional."),
        summary([
          ["Host", t.hostId],
          ["Versão", t.version || "Não informada"],
          ["Autenticação", t.authStatus || "Não verificada"],
          ["Tipo", t.kind || "Não informado"],
        ]),
      );
      const capabilities = Object.entries(t.capabilities || {})
        .filter(([, v]) => v === true)
        .map(([key]) => key);
      c.append(
        el(
          "p",
          "source",
          "Capacidades reportadas: " + (capabilities.join(", ") || "nenhuma"),
        ),
      );
      const details = el("details");
      details.append(
        el("summary", "", "Detalhes técnicos"),
        el(
          "pre",
          "",
          JSON.stringify(
            {
              id: t.id,
              discoveryId: t.discoveryId,
              onboardingId: t.onboardingId,
              orcaId: t.orcaId,
              path: t.path || "Não informado",
            },
            null,
            2,
          ),
        ),
      );
      c.append(details);
      const actions = el("div", "actions");
      if ((t.methods || []).length)
        actions.append(
          button(
            t.status === "missing" || t.status === "not_installed"
              ? "Ver plano de instalação"
              : "Instalação e instruções",
            () => installPlan(t),
          ),
        );
      if (t.selectable && compact)
        actions.append(
          button(
            "Usar como principal",
            () => {
              state.wizard.primary = { toolId: t.id, model: "", effort: "" };
              $("#modal").close();
              render();
            },
            "primary",
          ),
        );
      c.append(actions);
      grid.append(c);
    }
    if (!list.length)
      empty(
        grid,
        "Nenhuma ferramenta neste filtro",
        "Atualize a detecção para consultar o host. Falha de probe não significa ausência.",
      );
  };
  search.addEventListener("input", draw);
  filter.addEventListener("change", draw);
  draw();
}
async function installPlan(tool) {
  saveLocalDraft();
  try {
    const plan = await api("/tool-install-plans", "POST", {
      projectId: state.projectId,
      toolId: tool.id,
    });
    modal("Instalar " + tool.name, (body) => {
      body.append(
        el(
          "p",
          "",
          "Confira a origem, o destino e o método antes de autorizar. Selecionar uma IA nunca instala automaticamente.",
        ),
        summary([
          ["Host", plan.hostId],
          [
            "Método",
            typeof plan.method === "object"
              ? plan.method.id || plan.method.name
              : plan.method,
          ],
          ["Validade", date(plan.expiresAt)],
          ["ID do plano", plan.id],
          ["Revisão / hash", plan.hash],
        ]),
      );
      if (plan.instructions) body.append(el("p", "", plan.instructions));
      if (plan.executable)
        body.append(
          el("pre", "", [plan.executable, ...(plan.args || [])].join(" ")),
        );
      if (plan.url) {
        try {
          const url = new URL(plan.url);
          if (url.protocol === "https:") {
            const a = el("a", "button", "Abrir instrução oficial ↗");
            a.href = url.href;
            a.target = "_blank";
            a.rel = "noopener noreferrer";
            body.append(a);
          }
        } catch {}
      }
      if (!plan.automatic) {
        body.append(
          el(
            "p",
            "warning",
            "Este método requer instalação manual. O GasaCLI não vai executar um instalador por este painel.",
          ),
        );
        return;
      }
      const confirmInstall = button(
        "Confirmar instalação deste plano",
        async () => {
          confirmInstall.disabled = true;
          try {
            const key = "gasa-install:" + state.projectId + ":" + plan.id;
            let idempotencyKey;
            try {
              idempotencyKey =
                sessionStorage.getItem(key) || crypto.randomUUID();
              sessionStorage.setItem(key, idempotencyKey);
            } catch {
              idempotencyKey = crypto.randomUUID();
            }
            const request = {
              projectId: state.projectId,
              planId: plan.id,
              planHash: plan.hash,
              confirmation: true,
              idempotencyKey,
            };
            sessionStorage.setItem(
              "gasa-install-active:" + state.projectId,
              JSON.stringify({ request }),
            );
            const job = await api("/tool-install-jobs", "POST", request);
            showInstallJob(job);
          } catch (e) {
            body.append(el("p", "error", errorMessage(e)));
            confirmInstall.disabled = false;
          }
        },
        "primary",
      );
      body.append(confirmInstall);
    });
  } catch (e) {
    note(errorMessage(e));
  }
}
function showInstallJob(job) {
  try {
    sessionStorage.setItem(
      "gasa-install-active:" + state.projectId,
      JSON.stringify({ id: job.id }),
    );
  } catch {}
  modal("Instalação · " + toolName(job.toolId), (body) => {
    body.append(
      summary([
        ["Estado", job.status],
        ["ID", job.id],
        ["Atualizado", date(job.updatedAt)],
        [
          "Saída",
          job.exitCode === null || job.exitCode === undefined
            ? "Não informada"
            : job.exitCode,
        ],
      ]),
    );
    if (job.error) body.append(el("p", "error", job.error));
    body.append(
      el(
        "p",
        "",
        "A instalação só está pronta após verificação do binário e capacidades. Login e assinatura são etapas separadas.",
      ),
    );
    body.append(
      button("Consultar andamento", async () => {
        try {
          showInstallJob(
            await api(
              "/tool-install-jobs/" +
                encodeURIComponent(job.id) +
                "?projectId=" +
                encodeURIComponent(state.projectId),
            ),
          );
        } catch (e) {
          note(errorMessage(e));
        }
      }),
    );
    if (String(job.status).toLowerCase() === "unknown")
      body.append(
        button("Conciliar resultado desconhecido", async () => {
          try {
            showInstallJob(
              await api(
                "/tool-install-jobs/" +
                  encodeURIComponent(job.id) +
                  "/reconcile",
                "POST",
                { projectId: state.projectId },
              ),
            );
          } catch (error) {
            note(errorMessage(error));
          }
        }),
      );
    if (job.canCancel === true)
      body.append(
        button(
          "Solicitar cancelamento",
          async () => {
            try {
              showInstallJob(
                await api(
                  "/tool-install-jobs/" +
                    encodeURIComponent(job.id) +
                    "/cancel",
                  "POST",
                  { projectId: state.projectId },
                ),
              );
            } catch (e) {
              note(errorMessage(e));
            }
          },
          "danger",
        ),
      );
  });
}
async function saveSettings(update) {
  const current = state.snapshot?.settings || {};
  await api("/settings", "PUT", {
    projectId: state.projectId,
    ...current,
    ...update,
    revision: current.revision ?? 0,
  });
  await refresh();
  note("Preferências salvas.");
}
function orchestration(root) {
  const settings = state.snapshot?.settings || {},
    card = el("section", "card fields");
  card.append(
    el("h2", "", "Cada ideia precisa de direção."),
    el(
      "p",
      "",
      "O coordenador recebe os pedidos do time, decide o escopo e convoca quem precisa participar.",
    ),
  );
  const coordinator = select(
    [
      ["", "Sem coordenador"],
      ...agents()
        .filter((a) => a.status === "active")
        .map((a) => [a.id, a.name + " · " + toolName(a.primary?.toolId)]),
    ],
    settings.coordinatorId,
  );
  card.append(field("Coordenador ativo", coordinator));
  const autonomy = select(
    [
      ["ask_all", "Perguntar tudo"],
      ["accepted_plan", "Plano aceito"],
      ["scoped", "Dentro do escopo"],
    ],
    settings.autonomy || "accepted_plan",
  );
  card.append(
    field(
      "Autonomia do escritório",
      autonomy,
      "A preferência nunca amplia permissões existentes. O backend aplica os limites efetivos.",
    ),
  );
  const error = el("p", "error");
  card.append(
    error,
    button(
      "Salvar coordenador",
      async () => {
        try {
          await saveSettings({
            coordinatorId: coordinator.value,
            autonomy: autonomy.value,
          });
        } catch (e) {
          error.textContent = errorMessage(e);
        }
      },
      "primary",
    ),
  );
  if (settings.coordinatorId)
    card.append(
      link(
        "Editar principal e fallback do coordenador",
        "agentes/" + settings.coordinatorId,
      ),
    );
  card.append(
    summary([
      [
        "Novas atribuições",
        settings.paused
          ? "Pausadas por barreira do projeto"
          : "Sujeitas à coordenação e política",
      ],
      [
        "Dispatch",
        state.capabilities.dispatch === true
          ? "Disponível conforme capacidades verificadas"
          : "Não comprovado",
      ],
      ["Retries / concorrência", "Não configuráveis por esta versão do host"],
      ["Autonomia", "Política vigente do projeto; não ampliada pela interface"],
    ]),
  );
  if (settings.paused)
    card.append(
      button("Revisar retomada", () =>
        modal("Retomar novas atribuições", (body) => {
          body.append(
            el(
              "p",
              "",
              "Verifique tarefas e workers pendentes antes de remover a barreira. Retomar não reenvia pedidos antigos.",
            ),
          );
          body.append(
            button(
              "Remover barreira de novas atribuições",
              async () => {
                try {
                  await saveSettings({ paused: false });
                  $("#modal").close();
                  render();
                } catch (e) {
                  note(errorMessage(e));
                }
              },
              "primary",
            ),
          );
        }),
      ),
    );
  const runtimeState = el("div");
  card.append(
    button("Consultar runtime e entregas", async () => {
      try {
        const data = await api(
          "/runtime?projectId=" + encodeURIComponent(state.projectId),
        );
        runtimeState.replaceChildren(
          summary([
            ["Runtime", data.capability?.runtimeId],
            ["Host", data.capability?.hostId],
            [
              "Estado",
              data.reason || data.capability?.reason || "Observação disponível",
            ],
            [
              "Coordenador vinculado",
              agents().find((a) => a.id === data.binding?.coordinatorId)
                ?.name || "Nenhum vínculo confirmado",
            ],
          ]),
        );
        for (const operation of data.operations || [])
          runtimeState.append(
            button(operation.status + " · " + operation.messageId, () =>
              showCoordination(operation),
            ),
          );
        if (!(data.operations || []).length)
          runtimeState.append(
            el(
              "p",
              "source",
              "Nenhuma operação de entrega registrada para este projeto.",
            ),
          );
      } catch (error) {
        runtimeState.replaceChildren(el("p", "error", errorMessage(error)));
      }
    }),
    runtimeState,
  );
  root.append(card);
}
function integrations(root) {
  const card = el("section", "card");
  card.append(
    el("h2", "", "Conexões com origem conhecida."),
    el(
      "p",
      "",
      "Plugins e skills dependem do host. Uma integração não recebe permissão apenas por aparecer no catálogo.",
    ),
  );
  const result = el("div");
  card.append(
    button("Consultar integrações do host", async (event) => {
      const control = event.currentTarget;
      control.disabled = true;
      result.replaceChildren(el("p", "", "Consultando o host…"));
      try {
        const data = await api(
          "/integrations?projectId=" + encodeURIComponent(state.projectId),
        );
        result.replaceChildren(
          summary([
            ["Host", data.capability?.hostId],
            ["Runtime", data.capability?.runtimeId],
            ["Verificado", date(data.checkedAt)],
            [
              "Diagnóstico",
              data.reason ||
                data.capability?.reason ||
                "Sem diagnóstico adicional",
            ],
          ]),
        );
        const list = Array.isArray(data.plugins)
          ? data.plugins
          : data.plugins?.plugins || [];
        if (!list.length)
          result.append(
            el(
              "p",
              "warning",
              "Nenhum plugin foi reportado pelo host. Habilitação indisponível nesta interface.",
            ),
          );
        for (const item of list) {
          const detail = el("details");
          detail.append(
            el("summary", "", item.name || item.id || "Plugin do host"),
            el("pre", "", JSON.stringify(item, null, 2)),
          );
          result.append(detail);
        }
        const workspaces = Array.isArray(data.workspaces)
          ? data.workspaces
          : [];
        if (workspaces.length)
          result.append(
            el(
              "p",
              "source",
              workspaces.length +
                " vínculo(s) de workspace observado(s) no escopo.",
            ),
          );
      } catch (error) {
        result.replaceChildren(el("p", "error", errorMessage(error)));
      } finally {
        control.disabled = false;
      }
    }),
    result,
  );
  root.append(card);
}
function appearance(root) {
  const settings = state.snapshot?.settings || {},
    card = el("section", "card fields");
  card.append(el("h2", "", "Mais calma. Mais foco."));
  const quality = select(
    [
      ["low", "Baixa · menos sombras"],
      ["balanced", "Equilibrada"],
      ["high", "Alta · resolução limitada"],
    ],
    settings.quality || "balanced",
  );
  card.append(
    field(
      "Qualidade 3D",
      quality,
      "Meta visual, sem garantia de FPS. A cena estática não mantém um loop de renderização ocioso.",
    ),
  );
  const reduced = input("", "checkbox");
  reduced.checked =
    settings.reducedMotion ||
    matchMedia("(prefers-reduced-motion: reduce)").matches;
  const r = field(
    "Movimento reduzido",
    reduced,
    "A cena respeita a preferência do sistema e não simula trabalho.",
  );
  r.classList.add("check");
  card.append(r);
  const sound = input("", "checkbox");
  sound.checked = !!settings.sound;
  const s = field(
    "Permitir avisos sonoros",
    sound,
    "Nenhum áudio toca automaticamente; pedidos continuam visíveis ao silenciar.",
  );
  s.classList.add("check");
  card.append(s);
  const error = el("p", "error");
  card.append(
    error,
    button(
      "Salvar aparência",
      async () => {
        try {
          await saveSettings({
            quality: quality.value,
            reducedMotion: reduced.checked,
            sound: sound.checked,
          });
        } catch (e) {
          error.textContent = errorMessage(e);
        }
      },
      "primary",
    ),
    button("Restaurar enquadramento do escritório", () => {
      state.scene?.control("reset");
      navigate("escritorio");
    }),
  );
  root.append(card);
}
function voice(root) {
  const card = el("section", "card fields");
  card.append(
    el("h2", "", "Uma voz para cada personalidade."),
    el(
      "p",
      "",
      "Prévia voluntária com síntese do navegador. A origem e a disponibilidade dependem do sistema; vozes externas podem exigir rede. Isso não ativa fala automática dos agentes.",
    ),
  );
  if (!("speechSynthesis" in window)) {
    card.append(
      el("p", "warning", "Síntese de voz não está disponível neste navegador."),
    );
    root.append(card);
    return;
  }
  const person = select(
      agents().map((a) => [a.id, a.name]),
      agents()[0]?.id,
    ),
    voices = el("select");
  voices.setAttribute("aria-label", "Voz do navegador");
  const volume = input("1", "range");
  volume.min = "0";
  volume.max = "1";
  volume.step = ".1";
  const preview = input("Olá! Vamos tirar uma boa ideia do papel?");
  function load() {
    const available = speechSynthesis.getVoices();
    voices.replaceChildren();
    for (const v of available) {
      const o = el(
        "option",
        "",
        v.name +
          " · " +
          v.lang +
          (v.localService ? " · sistema local" : " · serviço externo"),
      );
      o.value = v.voiceURI;
      voices.append(o);
    }
  }
  load();
  speechSynthesis.addEventListener("voiceschanged", load, { once: true });
  card.append(
    field("Integrante", person),
    field("Voz", voices),
    field("Volume", volume),
    field("Texto da prévia", preview),
  );
  const error = el("p", "error");
  card.append(
    error,
    button("▶ Ouvir prévia", () => {
      const selected = speechSynthesis
        .getVoices()
        .find((v) => v.voiceURI === voices.value);
      if (!selected) {
        error.textContent =
          "Nenhuma voz disponível. Aguarde o catálogo do navegador.";
        return;
      }
      speechSynthesis.cancel();
      const u = new SpeechSynthesisUtterance(preview.value);
      u.voice = selected;
      u.volume = Number(volume.value);
      u.onerror = () =>
        (error.textContent = "Não foi possível reproduzir esta voz.");
      speechSynthesis.speak(u);
    }),
    button("Silenciar prévia", () => speechSynthesis.cancel()),
    button("Salvar preferência neste navegador", () => {
      try {
        localStorage.setItem(
          "gasa-office-voice:" + state.projectId + ":" + person.value,
          JSON.stringify({
            voiceURI: voices.value,
            volume: Number(volume.value),
          }),
        );
        note(
          "Preferência local salva. A voz não será reproduzida automaticamente.",
        );
      } catch {
        error.textContent = "O navegador não permitiu salvar a preferência.";
      }
    }),
  );
  root.append(card);
}
function copilot(root) {
  heading(root, "Um segundo olhar.", "Copiloto de tela");
  const card = el("section", "card fields");
  card.append(
    el("h2", "", "Observe primeiro. Controle, só com permissão."),
    el(
      "p",
      "",
      "O acesso à janela, escopo, duração e revogação precisam de suporte explícito do host.",
    ),
  );
  const objective = el("textarea");
  objective.placeholder = "Qual resultado você quer observar?";
  card.append(field("Objetivo", objective));
  const capture = button("Escolher janela ou tela", () => {});
  capture.disabled = true;
  const control = button("Autorizar controle", () => {});
  control.disabled = true;
  card.append(
    capture,
    control,
    el(
      "p",
      "warning",
      "Captura e controle não estão implementados pelo contrato desta interface. Nenhum compartilhamento, câmera ou microfone foi solicitado.",
    ),
  );
  root.append(card);
}
$("#notifications").addEventListener("click", () =>
  modal("Solicitações do escritório", (body) => {
    const intents = state.snapshot?.intents || [];
    const pending = intents.filter((i) =>
      /pending|awaiting/.test(String(i.status).toLowerCase()),
    );
    if (!pending.length) {
      body.append(
        el("p", "", "Nenhuma solicitação pendente reportada pelo escritório."),
      );
      return;
    }
    for (const intent of pending) {
      const c = el("article", "card");
      const message = messages().find((m) => m.id === intent.messageId);
      c.append(
        summary([
          ["Tipo", "Pedido de coordenação"],
          ["Estado", intent.status],
          ["Pedido", message?.text || intent.messageId],
          ["Decisão", intent.decision?.reason || "Aguardando coordenador"],
          ["ID", intent.id],
        ]),
      );
      if (message)
        c.append(
          button("Ver encaminhamento", () => coordinatePlan(message.id)),
        );
      c.append(
        el(
          "p",
          "source",
          "Um pedido aguardando coordenação não é uma solicitação de aprovação humana. As aprovações de execução permanecem no canal do host.",
        ),
      );
      body.append(c);
    }
  }),
);
async function requestStop(taskId) {
  modal(taskId ? "Parar esta tarefa" : "Parar novas atribuições", (body) => {
    body.append(
      el(
        "p",
        "",
        "Escopo: " +
          (activeProject()?.name || state.projectId) +
          ". O pedido preserva o histórico. Uma barreira de dispatch não comprova que processos em execução foram encerrados.",
      ),
    );
    const result = el("p", "warning");
    body.append(result);
    const confirmStop = button(
      "Confirmar solicitação de parada",
      async () => {
        confirmStop.disabled = true;
        try {
          const value = await api(
            taskId ? "/tasks/" + encodeURIComponent(taskId) + "/stop" : "/stop",
            "POST",
            { projectId: state.projectId },
          );
          await refresh();
          result.textContent =
            value.workersConfirmedStopped === true
              ? "Parada confirmada para os workers reportados."
              : value.reason ||
                "Barreira de novas atribuições registrada. Parada dos workers ainda não confirmada.";
          body.append(link("Consultar tarefas", "tarefas"));
        } catch (e) {
          result.textContent = errorMessage(e);
          confirmStop.disabled = false;
        }
      },
      "danger",
    );
    body.append(confirmStop);
  });
}
$("#stop-team").addEventListener("click", () => requestStop());
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) refresh();
  else if ("speechSynthesis" in window) speechSynthesis.cancel();
});
bootstrap().catch((e) => {
  state.stale = true;
  $("#connection").textContent = "Conexão indisponível";
  page.replaceChildren();
  const root = el("section", "page");
  heading(
    root,
    "Vamos reconectar seu escritório.",
    "A sessão local não foi confirmada. Abra novamente o endereço fornecido pelo GasaCLI.",
  );
  root.append(
    el("p", "error", errorMessage(e)),
    button("Tentar novamente", () => location.reload()),
  );
  page.append(root);
});
