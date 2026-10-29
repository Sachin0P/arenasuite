// ArenaSuite motion: everything that moves on the page apart from the 3D
// scenes (see fx.js). Headlines scramble in, cards rise as you scroll and
// tilt under the pointer, primary buttons are magnetic, numbers count up, a
// custom cursor follows you, and successes rain confetti.
//
// It only adds polish. Without JavaScript, or with "reduce motion" switched
// on, the site looks the same apart from the movement.

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;
const finePointer = matchMedia("(pointer: fine)").matches;

const PALETTE = ["#ffe600", "#c6ff00", "#5b8cff", "#ff2e88", "#f4f4ef"];

// ---------- text scramble ----------
// The headline briefly cycles through random characters before settling.

const GLYPHS = "!<>-_\\/[]{}=+*^?#%&ABCDEFGHJKLMNPQRSTUVWXYZ0123456789";

function scramble(el, duration = 750) {
  const final = el.textContent;
  if (!final.trim()) return;
  el.setAttribute("aria-label", final);
  const start = performance.now();

  const frame = (now) => {
    const progress = Math.min((now - start) / duration, 1);
    const settled = Math.floor(progress * final.length);
    let out = final.slice(0, settled);
    for (let i = settled; i < final.length; i++) {
      out += final[i] === " " ? " " : GLYPHS[(Math.random() * GLYPHS.length) | 0];
    }
    el.textContent = out;
    if (progress < 1) requestAnimationFrame(frame);
    else el.removeAttribute("aria-label");
  };
  requestAnimationFrame(frame);
}

// ---------- scroll reveal ----------

function setupReveal() {
  const targets = document.querySelectorAll(
    "main .card, main .stat, main .list-row, main .bracket-match, main section > h2, main .table-wrap, main .empty",
  );

  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        entry.target.classList.add("in");
        observer.unobserve(entry.target);
        countUp(entry.target);
      }
    },
    { threshold: 0.08 },
  );

  // Stagger siblings so a grid ripples in one card after another.
  const indexInParent = new Map();
  for (const el of targets) {
    const i = indexInParent.get(el.parentElement) ?? 0;
    indexInParent.set(el.parentElement, i + 1);
    el.style.setProperty("--delay", `${Math.min(i, 10) * 55}ms`);
    el.classList.add("reveal");
    observer.observe(el);
  }
}

// ---------- counters ----------

function countUp(el) {
  const strong = el.matches(".stat") ? el.querySelector("strong") : null;
  if (!strong) return;
  const target = Number(strong.textContent.trim());
  if (!Number.isFinite(target) || target === 0) return;

  const start = performance.now();
  const duration = 900 + Math.min(target, 200) * 4;
  const frame = (now) => {
    const p = Math.min((now - start) / duration, 1);
    const eased = 1 - Math.pow(1 - p, 4);
    strong.textContent = Math.round(target * eased);
    if (p < 1) requestAnimationFrame(frame);
  };
  requestAnimationFrame(frame);
}

// ---------- tilt cards ----------
// Delegated from the document, so cards swapped in by live updates tilt too.
// Cards holding form fields stay still, so they're easy to type into.

function setupTilt() {
  let current = null;

  const reset = (card) => {
    card.style.removeProperty("--rx");
    card.style.removeProperty("--ry");
    card.classList.remove("tilting");
  };

  document.addEventListener("pointermove", (e) => {
    const card = e.target.closest?.(".card");
    if (current && current !== card) {
      reset(current);
      current = null;
    }
    if (!card || card.querySelector("input, textarea, select")) return;

    const r = card.getBoundingClientRect();
    const x = (e.clientX - r.left) / r.width - 0.5;
    const y = (e.clientY - r.top) / r.height - 0.5;
    card.style.setProperty("--rx", `${(-y * 9).toFixed(2)}deg`);
    card.style.setProperty("--ry", `${(x * 11).toFixed(2)}deg`);
    card.classList.add("tilting");
    current = card;
  });
  document.addEventListener("pointerleave", () => current && reset(current));
}

// ---------- magnetic buttons ----------

function setupMagnetic() {
  for (const btn of document.querySelectorAll(".btn-primary:not(.btn-sm), .hero .btn")) {
    btn.classList.add("magnetic");
    btn.addEventListener("pointermove", (e) => {
      const r = btn.getBoundingClientRect();
      btn.style.setProperty("--mx", `${((e.clientX - r.left - r.width / 2) * 0.35).toFixed(1)}px`);
      btn.style.setProperty("--my", `${((e.clientY - r.top - r.height / 2) * 0.45).toFixed(1)}px`);
    });
    btn.addEventListener("pointerleave", () => {
      btn.style.removeProperty("--mx");
      btn.style.removeProperty("--my");
    });
  }
}

// ---------- custom cursor ----------

function setupCursor() {
  const dot = document.createElement("div");
  const ring = document.createElement("div");
  dot.className = "cursor-dot";
  ring.className = "cursor-ring";
  document.body.append(dot, ring);
  document.body.classList.add("has-cursor");

  let x = -100, y = -100, rx = -100, ry = -100;
  window.addEventListener("pointermove", (e) => {
    x = e.clientX;
    y = e.clientY;
    const interactive = e.target.closest?.("a, button, summary, .card, label.check, [data-reveal], .hero-stage");
    ring.classList.toggle("hover", Boolean(interactive));
  });
  window.addEventListener("pointerdown", () => ring.classList.add("down"));
  window.addEventListener("pointerup", () => ring.classList.remove("down"));
  document.addEventListener("pointerleave", () => (x = y = -100));

  const frame = () => {
    rx += (x - rx) * 0.18;
    ry += (y - ry) * 0.18;
    dot.style.transform = `translate(${x}px, ${y}px)`;
    ring.style.transform = `translate(${rx}px, ${ry}px)`;
    requestAnimationFrame(frame);
  };
  frame();
}

// ---------- confetti ----------
// Plain DOM squares with a little physics: no images, no gradients.

function confetti(originX = window.innerWidth / 2, originY = window.innerHeight * 0.35, count = 90) {
  const pieces = [];
  for (let i = 0; i < count; i++) {
    const el = document.createElement("div");
    el.className = "confetti";
    el.style.background = PALETTE[i % PALETTE.length];
    if (i % 3 === 0) el.style.borderRadius = "50%";
    document.body.append(el);
    const angle = Math.random() * Math.PI * 2;
    const speed = 6 + Math.random() * 12;
    pieces.push({
      el,
      x: originX,
      y: originY,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed - 9,
      spin: (Math.random() - 0.5) * 30,
      rot: Math.random() * 360,
    });
  }

  const start = performance.now();
  const frame = (now) => {
    const alive = now - start < 2600;
    for (const p of pieces) {
      p.vy += 0.45; // gravity
      p.vx *= 0.985;
      p.x += p.vx;
      p.y += p.vy;
      p.rot += p.spin;
      p.el.style.transform = `translate(${p.x}px, ${p.y}px) rotate(${p.rot}deg) rotateX(${p.rot * 2}deg)`;
      if (!alive) p.el.remove();
    }
    if (alive) requestAnimationFrame(frame);
  };
  requestAnimationFrame(frame);
}

// ---------- page transitions ----------
// Links fade the page out for a moment before navigating.

function setupPageTransitions() {
  document.addEventListener("click", (e) => {
    const link = e.target.closest?.("a[href]");
    if (!link || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (link.target || link.hasAttribute("download") || link.origin !== location.origin) return;
    if (link.pathname === location.pathname && link.search === location.search && link.hash) return; // same-page tab
    e.preventDefault();
    document.body.classList.add("leaving");
    setTimeout(() => (location.href = link.href), 170);
  });
  // Coming back with the back button restores the page from the cache.
  window.addEventListener("pageshow", (e) => e.persisted && document.body.classList.remove("leaving"));
}

// ---------- live updates ----------
// app.js fires "arena:live" when a competition event arrives over /events.

function setupLiveFlash() {
  document.addEventListener("arena:live", () => {
    for (const el of document.querySelectorAll("[data-live]")) {
      el.classList.remove("flash-hit");
      void el.offsetWidth; // restart the animation
      el.classList.add("flash-hit");
    }
  });
}

// ---------- easter egg ----------
// Type "arena" anywhere (outside a form field) to start a party.

function setupEasterEgg() {
  let typed = "";
  document.addEventListener("keydown", (e) => {
    if (e.target.closest?.("input, textarea, select")) return;
    typed = (typed + e.key.toLowerCase()).slice(-5);
    if (typed !== "arena") return;
    typed = "";
    document.dispatchEvent(new CustomEvent("arena:party"));
    for (let i = 0; i < 4; i++) {
      setTimeout(() => confetti(Math.random() * window.innerWidth, window.innerHeight * 0.3, 70), i * 250);
    }
  });
}

// ---------- start ----------

if (!reducedMotion) {
  for (const el of document.querySelectorAll(".hero h1 .line, .band h1, main .page > h1, main h1.mb, main h1:only-child")) {
    scramble(el);
  }
  setupReveal();
  setupMagnetic();
  setupPageTransitions();
  setupLiveFlash();
  setupEasterEgg();
  if (finePointer) {
    setupTilt();
    setupCursor();
  }

  // Successes get confetti: the flash message from the last form action
  // (shown by app.js before this module ran), later toasts, and smashing
  // the home page orb.
  const flash = document.getElementById("flash");
  if (flash?.dataset.kind === "success") confetti(window.innerWidth - 120, window.innerHeight - 60, 60);
  document.addEventListener("arena:toast", (e) => {
    if (e.detail.kind === "success") confetti(window.innerWidth - 120, window.innerHeight - 60, 60);
  });
  document.addEventListener("arena:smash", (e) => {
    const stage = document.getElementById("hero-stage")?.getBoundingClientRect();
    if (stage) confetti(stage.left + stage.width * 0.7, stage.top + stage.height * 0.5, 50);
  });
}
