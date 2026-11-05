import * as T from "./office-vendor/three.module.min.js";

// All furniture and characters are original, procedural geometry. No remote assets.
const palette = {
  terracotta: 0xc98057,
  sage: 0x8d9d76,
  lilac: 0xa295ba,
  honey: 0xd1b25d,
  ocean: 0x739ba4,
};
export class OfficeScene {
  constructor(
    root,
    { select, open, failed, quality = "balanced", reducedMotion = false },
  ) {
    this.root = root;
    this.select = select;
    this.open = open;
    this.failed = failed;
    this.reducedMotion = reducedMotion;
    this.labels = [];
    this.picks = [];
    this.disposed = false;
    this.angle = 0.72;
    this.zoom = 1;
    this.materials = new Map();
    this.geometries = new Set();
    this.scene = new T.Scene();
    this.scene.background = new T.Color(0xeeeade);
    this.camera = new T.OrthographicCamera(-14, 14, 10, -10, 0.1, 150);
    this.renderer = new T.WebGLRenderer({
      antialias: quality !== "low",
      alpha: false,
    });
    this.renderer.setPixelRatio(
      Math.min(devicePixelRatio, quality === "low" ? 1 : 1.6),
    );
    this.renderer.shadowMap.enabled = quality !== "low";
    this.renderer.shadowMap.type = T.PCFSoftShadowMap;
    this.renderer.outputColorSpace = T.SRGBColorSpace;
    this.renderer.setClearColor(0xeeeade);
    root.append(this.renderer.domElement);
    this.overlay = document.createElement("div");
    this.overlay.className = "scene-labels";
    root.append(this.overlay);
    this.renderer.domElement.setAttribute(
      "aria-label",
      "Escritório tridimensional; use Ver equipe para as mesmas ações pelo teclado",
    );
    this.renderer.domElement.setAttribute("role", "img");
    this.scene.add(new T.HemisphereLight(0xfff7e5, 0x8e9982, 2.2));
    const sun = new T.DirectionalLight(0xffedd1, 3);
    sun.position.set(-6, 14, 8);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    Object.assign(sun.shadow.camera, {
      left: -16,
      right: 16,
      top: 14,
      bottom: -14,
    });
    sun.shadow.normalBias = 0.05;
    this.scene.add(sun);
    this.room = new T.Group();
    this.scene.add(this.room);
    this.people = new T.Group();
    this.scene.add(this.people);
    this.buildRoom();
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(root);
    this.pointerDown = (e) => {
      this.down = { x: e.clientX, y: e.clientY, angle: this.angle };
    };
    this.pointerMove = (e) => {
      if (!this.down || !(e.buttons & 1)) return;
      this.angle = this.down.angle + (e.clientX - this.down.x) * 0.004;
      this.resize();
    };
    this.pointerUp = (e) => {
      if (
        this.down &&
        Math.hypot(e.clientX - this.down.x, e.clientY - this.down.y) < 6
      )
        this.pick(e);
      this.down = null;
    };
    this.contextLost = (e) => {
      e.preventDefault();
      this.failed(
        "O contexto gráfico foi perdido. A equipe continua acessível em lista.",
      );
    };
    this.renderer.domElement.addEventListener("pointerdown", this.pointerDown);
    this.renderer.domElement.addEventListener("pointermove", this.pointerMove);
    this.renderer.domElement.addEventListener("pointerup", this.pointerUp);
    this.renderer.domElement.addEventListener(
      "webglcontextlost",
      this.contextLost,
    );
    this.visibility = () => {
      cancelAnimationFrame(this.frame);
      if (!document.hidden) {
        this.render();
        if (!this.reducedMotion && this.labels.some((l) => l.runtime.running))
          this.animate();
      }
    };
    document.addEventListener("visibilitychange", this.visibility);
    this.resize();
  }
  mat(color) {
    if (!this.materials.has(color))
      this.materials.set(
        color,
        new T.MeshStandardMaterial({
          color,
          roughness: 0.85,
          flatShading: true,
        }),
      );
    return this.materials.get(color);
  }
  mesh(parent, geometry, color, x, y, z) {
    this.geometries.add(geometry);
    const m = new T.Mesh(geometry, this.mat(color));
    m.position.set(x, y, z);
    m.castShadow = true;
    m.receiveShadow = true;
    parent.add(m);
    return m;
  }
  box(p, w, h, d, c, x, y, z) {
    return this.mesh(p, new T.BoxGeometry(w, h, d), c, x, y, z);
  }
  cyl(p, r, h, c, x, y, z, r2 = r, n = 12) {
    return this.mesh(p, new T.CylinderGeometry(r2, r, h, n), c, x, y, z);
  }
  ball(p, r, c, x, y, z) {
    return this.mesh(p, new T.IcosahedronGeometry(r, 1), c, x, y, z);
  }
  plant(x, z, scale = 1) {
    const g = new T.Group();
    g.position.set(x, 0, z);
    g.scale.setScalar(scale);
    this.room.add(g);
    this.cyl(g, 0.32, 0.55, 0xc28c68, 0, 0.28, 0, 0.38);
    this.cyl(g, 0.04, 1.2, 0x6c7952, 0, 1, 0);
    for (let i = 0; i < 5; i++) {
      const a = i * 2.4;
      const b = this.ball(
        g,
        0.38,
        0x7d9667,
        Math.cos(a) * 0.22,
        1.05 + i * 0.15,
        Math.sin(a) * 0.22,
      );
      b.scale.set(0.8, 1.3, 0.8);
    }
  }
  buildRoom() {
    const r = this.room;
    this.box(r, 19, 0.4, 13, 0xccb99a, 0, -0.25, 0);
    this.box(r, 19, 0.08, 13, 0xdfcfb2, 0, 0, 0);
    for (let i = -9; i <= 9; i++)
      this.box(r, 0.014, 0.012, 13, 0xc9b896, i, 0.045, 0);
    this.box(r, 19, 3.7, 0.16, 0xf1e9d7, 0, 1.8, -6.5);
    this.box(r, 0.16, 3.7, 13, 0xe4dfcb, -9.5, 1.8, 0);
    this.box(r, 19, 0.12, 0.18, 0xb4a184, 0, 3.7, -6.5);
    this.box(r, 0.18, 0.12, 13, 0xb4a184, -9.5, 3.7, 0);
    // Window frame, garden glimpse, and mullions.
    this.box(r, 6, 2, 0.12, 0xb6c9b7, -4.8, 2.15, -6.36);
    this.box(r, 6.25, 0.12, 0.24, 0xa69474, -4.8, 1.12, -6.25);
    for (const x of [-7.9, -5.85, -3.8, -1.7])
      this.box(r, 0.09, 2.15, 0.15, 0xf8f4e8, x, 2.16, -6.2);
    this.box(r, 6.25, 0.08, 0.15, 0xf8f4e8, -4.8, 2.3, -6.2);
    // Team board with deliberately decorative lines, never fabricated tasks.
    this.box(r, 3.6, 1.9, 0.12, 0x9b8467, 4.2, 2.1, -6.25);
    this.box(r, 3.4, 1.7, 0.1, 0xe9e7d5, 4.2, 2.1, -6.15);
    for (let i = 0; i < 3; i++) {
      this.box(r, 0.8, 0.13, 0.03, 0xa5b494, 3.1 + i * 1.1, 2.65, -6.07);
      this.box(r, 0.6, 0.025, 0.03, 0xc0c7b4, 3.1 + i * 1.1, 2.35, -6.07);
    }
    const board = this.box(r, 3.6, 1.9, 0.1, 0xe9e7d5, 4.2, 2.1, -6.08);
    board.visible = false;
    this.picks.push({ object: board, action: "tarefas" });
    board.visible = true;
    // Coffee bar and machine.
    this.box(r, 3.7, 1.1, 0.85, 0x9caa8c, -7.1, 0.55, -4.6);
    this.box(r, 3.95, 0.15, 1.05, 0xc9aa81, -7.1, 1.13, -4.6);
    for (let i = 0; i < 3; i++)
      this.box(r, 0.025, 0.75, 0.05, 0x71856a, -8.2 + i * 1.1, 0.6, -4.15);
    this.box(r, 0.65, 0.7, 0.48, 0x4a5045, -7.4, 1.55, -4.6);
    this.box(r, 0.4, 0.32, 0.04, 0xaeb6a1, -7.4, 1.58, -4.33);
    this.cyl(r, 0.12, 0.22, 0xf6efda, -7.4, 1.32, -4.15);
    this.cyl(r, 0.2, 0.22, 0xc99c6b, -6.3, 1.34, -4.6);
    this.plant(-8.65, -5.7, 1.1);
    this.plant(8.25, -5.4, 1.2);
    this.plant(-8, 5, 1.3);
    // Meeting carpet, table, four upholstered chairs.
    this.box(r, 5, 0.04, 3.7, 0xadb997, -5.3, 0.08, 2.4);
    this.cyl(r, 1.1, 0.16, 0xc5a17a, -5.3, 1.13, 2.4, 1.1, 24).scale.set(
      1.45,
      1,
      0.86,
    );
    this.cyl(r, 0.16, 1, 0x8f795e, -5.3, 0.55, 2.4);
    for (const [x, z] of [
      [-6.7, 1.5],
      [-3.9, 1.5],
      [-6.7, 3.5],
      [-3.9, 3.5],
    ]) {
      this.box(r, 0.8, 0.14, 0.75, 0x85977a, x, 0.6, z);
      this.box(r, 0.8, 0.65, 0.13, 0x85977a, x, 0.95, z + 0.33);
      this.cyl(r, 0.07, 0.58, 0x7b7668, x, 0.3, z);
    }
    this.cyl(r, 0.14, 0.18, 0xf6f0df, -5, 1.3, 2.2);
    this.box(r, 0.5, 0.025, 0.65, 0xece6d5, -5.7, 1.24, 2.5);
    this.box(r, 0.8, 2.6, 2.2, 0xae9879, -8.9, 1.3, -1.3);
    for (let j = 0; j < 3; j++) {
      this.box(r, 0.9, 0.1, 2.25, 0xc6ae8b, -8.9, 0.6 + j * 0.85, -1.3);
      for (let i = 0; i < 5; i++)
        this.box(
          r,
          0.52,
          0.48,
          0.18,
          [0x879578, 0xc89374, 0xb4a087][i % 3],
          -8.75,
          0.9 + j * 0.85,
          -2.1 + i * 0.35,
        );
    }
    const meeting = this.box(r, 3, 0.16, 1.9, 0xc5a17a, -5.3, 1.13, 2.4);
    this.picks.push({ object: meeting, action: "meeting" });
  }
  desk(parent, x, z, agent) {
    const g = new T.Group();
    g.position.set(x, 0, z);
    parent.add(g);
    const wood = agent ? 0xc6a27a : 0xcbb38f;
    this.box(g, 2.3, 0.14, 1.25, wood, 0, 1.1, 0);
    for (const a of [-0.9, 0.9])
      for (const b of [-0.43, 0.43])
        this.box(g, 0.1, 1.02, 0.1, 0x8e927e, a, 0.5, b);
    this.box(g, 0.95, 0.65, 0.09, 0x454e44, 0, 1.63, -0.28);
    this.box(g, 0.82, 0.51, 0.03, agent ? 0xa3b8a0 : 0x81917d, 0, 1.65, -0.22);
    this.box(g, 0.08, 0.28, 0.08, 0x5e6759, 0, 1.25, -0.28);
    this.box(g, 0.4, 0.04, 0.24, 0x5e6759, 0, 1.2, -0.28);
    this.box(g, 0.72, 0.035, 0.23, 0x7d8874, 0, 1.2, 0.22);
    this.box(g, 0.25, 0.07, 0.38, 0x5b6757, 0.85, 1.22, -0.05);
    this.box(g, 0.28, 0.1, 0.1, 0x3b483b, 0.85, 1.32, -0.08);
    this.cyl(g, 0.12, 0.22, 0xf6eed9, -0.85, 1.3, 0.12);
    this.box(g, 0.75, 0.14, 0.7, 0x859479, 0, 0.6, 1.05);
    this.box(g, 0.75, 0.7, 0.13, 0x859479, 0, 0.96, 1.4);
    this.cyl(g, 0.065, 0.5, 0x656f5e, 0, 0.3, 1.05);
    if (agent) {
      const pet = this.avatar(agent);
      pet.position.set(0, 0.73, 0.97);
      g.add(pet);
      g.traverse((o) => {
        if (o.isMesh) this.picks.push({ object: o, agentId: agent.id });
      });
      const monitor = this.box(g, 0.86, 0.54, 0.035, 0x91a98d, 0, 1.65, -0.195);
      this.picks.push({
        object: monitor,
        agentId: agent.id,
        action: "activity",
      });
      const phone = this.box(g, 0.3, 0.12, 0.4, 0x5b6757, 0.85, 1.27, -0.05);
      this.picks.push({ object: phone, agentId: agent.id, action: "phone" });
      const runtime = agent._visual || {};
      if (runtime.approval) {
        this.ball(g, 0.1, 0xd59455, 0.85, 1.6, -0.05);
      }
      if (runtime.meeting) {
        pet.position.set(-5.3 - x, 0.73, 2.1 - z);
      }
      this.labels.push({
        agent,
        pet,
        runtime,
        point: new T.Vector3(x, 2.55, z),
        node: null,
      });
    }
    return g;
  }
  avatar(agent) {
    const g = new T.Group(),
      f = agent.avatar?.familyId || "fox",
      c = palette[agent.avatar?.color] || 0xc98057;
    const body = this.ball(g, 0.4, c, 0, 0.2, 0);
    body.scale.set(0.85, 1.1, 0.7);
    const head = this.ball(g, 0.43, c, 0, 0.8, 0);
    if (f === "capybara") head.scale.set(1.2, 0.85, 1);
    if (f === "robot") {
      head.geometry.dispose();
      this.geometries.delete(head.geometry);
      head.geometry = new T.BoxGeometry(0.7, 0.65, 0.6);
      this.geometries.add(head.geometry);
      this.cyl(g, 0.025, 0.35, 0x6e7a62, 0, 1.3, 0);
      this.ball(g, 0.08, 0xd5b661, 0, 1.5, 0);
    } else {
      for (const x of [-0.28, 0.28]) {
        if (f === "fox" || f === "cat") {
          const ear = this.mesh(
            g,
            new T.ConeGeometry(0.16, 0.4, 3),
            c,
            x,
            1.2,
            0,
          );
          ear.rotation.z = x > 0 ? -0.18 : 0.18;
        } else this.ball(g, f === "owl" ? 0.12 : 0.11, c, x, 1.12, 0);
      }
    }
    for (const x of [-0.15, 0.15]) {
      this.ball(g, f === "owl" ? 0.15 : 0.09, 0xf7eed8, x, 0.88, 0.34);
      const pupil = this.ball(g, 0.04, 0x303e32, x, 0.88, 0.415);
      if (agent.avatar?.eyes === "soft") pupil.scale.set(1.1, 0.4, 1);
    }
    this.ball(g, 0.08, 0x584b3c, 0, 0.72, 0.4);
    for (const x of [-0.34, 0.34]) {
      const arm = this.ball(g, 0.15, c, x, 0.32, 0);
      arm.scale.set(0.7, 1.4, 0.8);
    }
    if (agent.avatar?.accessories?.includes("headphones")) {
      this.box(g, 0.83, 0.09, 0.12, 0x495747, 0, 1.17, 0);
      for (const x of [-0.42, 0.42])
        this.box(g, 0.12, 0.25, 0.25, 0x495747, x, 0.96, 0);
    }
    return g;
  }
  update(agents, snapshot = {}) {
    const signature = JSON.stringify([
      agents,
      snapshot.tasks,
      snapshot.approvals,
      snapshot.intents,
      snapshot.stale,
    ]);
    if (signature === this.signature) return;
    this.signature = signature;
    cancelAnimationFrame(this.frame);
    this.labels.forEach((l) => l.node?.remove());
    this.labels = [];
    this.picks = this.picks.filter((p) => !p.agentId);
    this.people.traverse((o) => {
      if (o.isMesh) {
        o.geometry.dispose();
        this.geometries.delete(o.geometry);
      }
    });
    this.people.clear();
    const list = agents.filter(
      (a) => !["archived", "draft"].includes(a.status),
    );
    // Desk allocation comes from the persisted desk ID, so archiving a person
    // never silently moves the other desks into a new slot.
    const slots = new Map();
    for (const [index, agent] of list.entries()) {
      const number = Number(String(agent.deskId || "").match(/(\d+)$/)?.[1]);
      slots.set(
        Number.isSafeInteger(number) && number > 0 ? number - 1 : index,
        agent,
      );
    }
    const capacity = Math.max(8, ...[...slots.keys()].map((i) => i + 1));
    if (capacity > 8) {
      const extraRows = Math.ceil(capacity / 4) - 2;
      this.box(
        this.people,
        13.5,
        0.4,
        extraRows * 3.6,
        0xccb99a,
        2.2,
        -0.25,
        5.5 + extraRows * 1.8,
      );
      this.box(
        this.people,
        13.5,
        0.08,
        extraRows * 3.6,
        0xdfcfb2,
        2.2,
        0,
        5.5 + extraRows * 1.8,
      );
    }
    for (let i = 0; i < capacity; i++) {
      const source = slots.get(i);
      let a = null;
      if (source) {
        const task = (snapshot.tasks || []).find(
          (t) =>
            t.agentId === source.id &&
            !["DONE", "COMPLETED", "CANCELLED", "FAILED"].includes(
              String(t.status).toUpperCase(),
            ),
        );
        const status = String(task?.status || "").toUpperCase();
        const approval = (snapshot.approvals || []).some(
          (p) =>
            p.agentId === source.id &&
            String(p.status).toUpperCase() === "PENDING",
        );
        const running =
          !snapshot.stale &&
          ["RUNNING", "EXECUTING", "VERIFYING"].includes(status);
        const meeting =
          !snapshot.stale && ["PLANNING", "MEETING"].includes(status);
        a = {
          ...source,
          _visual: {
            running,
            meeting,
            approval,
            label: snapshot.stale
              ? "Estado desatualizado"
              : approval
                ? "Aguardando aprovação"
                : task
                  ? {
                      RUNNING: "Trabalhando",
                      EXECUTING: "Trabalhando",
                      VERIFYING: "Verificando",
                      BLOCKED: "Bloqueado",
                      PLANNING: "Planejando",
                      MEETING: "Em reunião",
                      UNKNOWN: "Estado desconhecido",
                    }[status] || status
                  : source.status === "paused"
                    ? "Pausado"
                    : "Sem sessão",
          },
        };
      }
      const x = (i % 4) * 3.2 - 2.2,
        z = Math.floor(i / 4) * 3.6 - 2.8;
      this.desk(this.people, x, z, a);
    }
    for (const l of this.labels) {
      const b = document.createElement("button");
      b.className = "scene-label";
      b.textContent = l.agent.name;
      const s = document.createElement("small");
      s.textContent = (l.agent.role || "Integrante") + " · " + l.runtime.label;
      b.append(s);
      b.addEventListener("click", () => this.select(l.agent.id));
      this.overlay.append(b);
      l.node = b;
    }
    this.render();
    if (!this.reducedMotion && this.labels.some((l) => l.runtime.running))
      this.animate();
  }
  animate() {
    if (this.disposed || document.hidden) return;
    const now = performance.now();
    for (const l of this.labels)
      if (l.runtime.running) {
        l.pet.rotation.z = Math.sin(now * 0.006) * 0.025;
        l.pet.rotation.x = Math.sin(now * 0.009) * 0.025;
      }
    this.render();
    this.frame = requestAnimationFrame(() => this.animate());
  }
  resize() {
    if (this.disposed) return;
    const w = this.root.clientWidth,
      h = this.root.clientHeight;
    if (!w || !h) return;
    this.renderer.setSize(w, h);
    const aspect = w / h,
      span = (aspect < 1 ? 18 : 10) / this.zoom;
    this.camera.left = -span * aspect;
    this.camera.right = span * aspect;
    this.camera.top = span;
    this.camera.bottom = -span;
    this.camera.position.set(
      Math.sin(this.angle) * 23,
      22,
      Math.cos(this.angle) * 23,
    );
    this.camera.lookAt(0, 0, 0);
    this.camera.updateProjectionMatrix();
    this.render();
  }
  control(action) {
    if (action === "in") this.zoom = Math.min(1.55, this.zoom + 0.15);
    if (action === "out") this.zoom = Math.max(0.65, this.zoom - 0.15);
    if (action === "rotate") this.angle += 0.3;
    if (action === "reset") {
      this.angle = 0.72;
      this.zoom = 1;
    }
    if (action === "meeting") {
      this.angle = 0.9;
      this.zoom = 1.3;
    }
    this.resize();
  }
  render() {
    if (this.disposed || document.hidden) return;
    this.renderer.render(this.scene, this.camera);
    for (const l of this.labels) {
      const p = l.point.clone().project(this.camera);
      l.node.style.left = (p.x * 0.5 + 0.5) * this.root.clientWidth + "px";
      l.node.style.top = (-p.y * 0.5 + 0.5) * this.root.clientHeight + "px";
      l.node.hidden = p.z > 1;
    }
  }
  pick(e) {
    const r = this.renderer.domElement.getBoundingClientRect(),
      ray = new T.Raycaster();
    ray.setFromCamera(
      new T.Vector2(
        ((e.clientX - r.left) / r.width) * 2 - 1,
        (-(e.clientY - r.top) / r.height) * 2 + 1,
      ),
      this.camera,
    );
    const hit = ray.intersectObjects(
      this.picks.map((p) => p.object),
      false,
    )[0];
    const p = this.picks.find((p) => p.object === hit?.object);
    if (p?.action) this.open(p.action, p.agentId);
    else if (p?.agentId) this.select(p.agentId);
  }
  dispose() {
    this.disposed = true;
    cancelAnimationFrame(this.frame);
    this.resizeObserver.disconnect();
    document.removeEventListener("visibilitychange", this.visibility);
    this.renderer.domElement.removeEventListener(
      "pointerdown",
      this.pointerDown,
    );
    this.renderer.domElement.removeEventListener(
      "pointermove",
      this.pointerMove,
    );
    this.renderer.domElement.removeEventListener("pointerup", this.pointerUp);
    this.renderer.domElement.removeEventListener(
      "webglcontextlost",
      this.contextLost,
    );
    this.geometries.forEach((g) => g.dispose());
    this.materials.forEach((m) => m.dispose());
    this.renderer.dispose();
    this.root.replaceChildren();
  }
}
