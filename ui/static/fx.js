// ArenaSuite 3D effects, built with three.js (vendored in /static/vendor so
// the Content-Security-Policy can stay at 'self').
//
//   - Sports kit floats behind every page: footballs, basketballs, cricket
//     and tennis balls and shuttlecocks; trophies and medals on Academic
//     pages; game controllers on Esports pages. It follows the mouse and the
//     scroll position, and scatters when a live event (or a goal) happens.
//   - The home page hero is a floodlit stadium with a penalty shoot-out:
//     move to aim, click to shoot, and try to beat the keeper.
//
// Every material is flat-shaded, wireframe or a single flat colour, so each
// face is one solid colour. Nothing here is needed to use the site: without
// WebGL, or with "reduce motion" on, the page just stays still.

import * as THREE from "./vendor/three.module.min.js";

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

const COLORS = {
  volt: 0xffe600,
  sports: 0xc6ff00,
  academic: 0x5b8cff,
  esports: 0xff2e88,
  white: 0xf4f4ef,
  steel: 0x22222b,
  night: 0x07070a,
};

function makeRenderer(canvas, opaque = false) {
  try {
    const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: !opaque, powerPreference: "low-power" });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
    renderer.setClearColor(COLORS.night, opaque ? 1 : 0);
    return renderer;
  } catch {
    return null; // no WebGL: the page works fine without the scene
  }
}

// A tiny animation loop that pauses while the tab is hidden.
function loop(tick) {
  let last = performance.now();
  let running = false;
  const frame = (now) => {
    if (!running) return;
    const dt = Math.min((now - last) / 1000, 0.05);
    last = now;
    tick(dt, now / 1000);
    requestAnimationFrame(frame);
  };
  const start = () => {
    if (running || reducedMotion) return;
    running = true;
    last = performance.now();
    requestAnimationFrame(frame);
  };
  const stop = () => (running = false);
  document.addEventListener("visibilitychange", () => (document.hidden ? stop() : start()));
  return { start, stop };
}

// The pointer position in -1..1, shared by both scenes.
const pointer = { x: 0, y: 0 };
window.addEventListener("pointermove", (e) => {
  pointer.x = (e.clientX / window.innerWidth) * 2 - 1;
  pointer.y = (e.clientY / window.innerHeight) * 2 - 1;
});

// ---------- the sports kit ----------
// Each maker returns a Group about 2 units across, centred on the origin.
// Geometries and materials are built once and shared between copies.

const flat = (color, extra = {}) => new THREE.MeshStandardMaterial({ color, flatShading: true, roughness: 0.55, metalness: 0.1, ...extra });
const solid = (color, extra = {}) => new THREE.MeshBasicMaterial({ color, ...extra });

// footballGeometry builds a truncated icosahedron, the classic football: cut
// every corner off an icosahedron at a third of each edge. The 20 faces
// become white hexagons and the 12 corners become black pentagons.
function footballGeometry() {
  const ico = new THREE.IcosahedronGeometry(1, 0);
  const position = ico.attributes.position;

  // IcosahedronGeometry repeats shared corners, so find the 12 unique ones.
  const corners = [];
  const faces = [];
  const find = (v) => {
    const i = corners.findIndex((c) => c.distanceTo(v) < 1e-4);
    if (i >= 0) return i;
    corners.push(v.clone());
    return corners.length - 1;
  };
  for (let i = 0; i < position.count; i += 3) {
    faces.push([0, 1, 2].map((j) => find(new THREE.Vector3().fromBufferAttribute(position, i + j))));
  }

  const third = (a, b) => corners[a].clone().lerp(corners[b], 1 / 3);
  const positions = [];
  const colors = [];
  const white = new THREE.Color(COLORS.white);
  const black = new THREE.Color(0x111114);

  // Fan-triangulate a flat polygon around its centre, facing outwards.
  const polygon = (points, color) => {
    const centre = points.reduce((sum, p) => sum.add(p), new THREE.Vector3()).divideScalar(points.length);
    for (let i = 0; i < points.length; i++) {
      let a = points[i];
      let b = points[(i + 1) % points.length];
      const normal = new THREE.Vector3().subVectors(a, centre).cross(new THREE.Vector3().subVectors(b, centre));
      if (normal.dot(centre) < 0) [a, b] = [b, a];
      positions.push(centre.x, centre.y, centre.z, a.x, a.y, a.z, b.x, b.y, b.z);
      for (let k = 0; k < 3; k++) colors.push(color.r, color.g, color.b);
    }
  };

  for (const [a, b, c] of faces) {
    polygon([third(a, b), third(b, a), third(b, c), third(c, b), third(c, a), third(a, c)], white);
  }

  corners.forEach((corner, i) => {
    const neighbours = new Set();
    for (const f of faces) if (f.includes(i)) for (const n of f) if (n !== i) neighbours.add(n);
    const points = [...neighbours].map((n) => third(i, n));
    // Sort the five points by angle around the corner.
    const normal = corner.clone().normalize();
    const ref = points[0].clone().sub(corner).projectOnPlane(normal).normalize();
    const side = new THREE.Vector3().crossVectors(normal, ref);
    const angle = (p) => {
      const d = p.clone().sub(corner);
      return Math.atan2(d.dot(side), d.dot(ref));
    };
    points.sort((p, q) => angle(p) - angle(q));
    polygon(points, black);
  });

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(positions, 3));
  geometry.setAttribute("color", new THREE.Float32BufferAttribute(colors, 3));
  // The cut corners sit a little inside the unit sphere; scale back up to radius 1.
  const radius = new THREE.Vector3().fromArray(positions, 3).length();
  geometry.scale(1 / radius, 1 / radius, 1 / radius);
  return geometry;
}

// tennisSeam is the curve of a tennis ball's seam, pushed onto the sphere.
function tennisSeam() {
  const points = [];
  for (let i = 0; i < 64; i++) {
    const t = (i / 64) * Math.PI * 2;
    const a = 0.75, b = 0.25;
    const p = new THREE.Vector3(a * Math.cos(t) + b * Math.cos(3 * t), a * Math.sin(t) - b * Math.sin(3 * t), 2 * Math.sqrt(a * b) * Math.sin(2 * t));
    points.push(p.normalize().multiplyScalar(1.01));
  }
  return new THREE.TubeGeometry(new THREE.CatmullRomCurve3(points, true), 96, 0.045, 4, true);
}

const KIT = (() => {
  const g = {
    football: footballGeometry(),
    ball: new THREE.SphereGeometry(1, 16, 12),
    seam: new THREE.TorusGeometry(1.0, 0.035, 4, 48),
    sideSeam: new THREE.TorusGeometry(0.75, 0.035, 4, 40),
    tennisSeam: tennisSeam(),
    cork: new THREE.SphereGeometry(0.3, 10, 6, 0, Math.PI * 2, 0, Math.PI / 2),
    skirt: new THREE.ConeGeometry(0.7, 1.3, 12, 2, true),
    rim: new THREE.TorusGeometry(0.7, 0.03, 4, 24),
    cup: new THREE.LatheGeometry(
      [[0.12, 0.3], [0.1, 0.62], [0.16, 0.74], [0.5, 0.9], [0.62, 1.25], [0.66, 1.6]].map(([x, y]) => new THREE.Vector2(x, y)),
      10,
    ),
    stem: new THREE.CylinderGeometry(0.42, 0.55, 0.3, 10),
    handle: new THREE.TorusGeometry(0.28, 0.06, 6, 12, Math.PI),
    plinth: new THREE.BoxGeometry(1.2, 0.28, 1.2),
    disc: new THREE.CylinderGeometry(0.62, 0.62, 0.12, 10),
    discRing: new THREE.TorusGeometry(0.46, 0.05, 4, 20),
    ribbon: new THREE.BoxGeometry(0.3, 1.1, 0.04),
    body: new THREE.CapsuleGeometry(0.42, 1.0, 3, 8),
    grip: new THREE.CapsuleGeometry(0.3, 0.4, 3, 8),
    button: new THREE.SphereGeometry(0.085, 6, 4),
    padBar: new THREE.BoxGeometry(0.34, 0.1, 0.08),
    stick: new THREE.CylinderGeometry(0.11, 0.11, 0.1, 8),
  };
  const m = {
    football: new THREE.MeshStandardMaterial({ vertexColors: true, flatShading: true, roughness: 0.45 }),
    basketball: flat(0xff7a1a),
    cricket: flat(0xb5121b, { roughness: 0.35 }),
    tennis: flat(0xd8ff3a),
    seamDark: flat(0x111114),
    seamLight: flat(0xf1e3c6),
    white: flat(COLORS.white),
    feathers: solid(COLORS.white, { wireframe: true }),
    gold: flat(0xffd400, { metalness: 0.55, roughness: 0.3, side: THREE.DoubleSide }),
    darkGold: flat(0xb38f00, { metalness: 0.5 }),
    plinth: flat(0x1b1b22),
    ribbonA: flat(COLORS.academic),
    ribbonB: flat(COLORS.esports),
    pad: flat(0xe4e4ec, { roughness: 0.6 }), // a white console pad shows up on the dark page
    padDetail: flat(0x22222b),
    buttons: [COLORS.sports, COLORS.academic, COLORS.esports, COLORS.volt].map((c) => flat(c, { emissive: c, emissiveIntensity: 0.3 })),
  };

  const mesh = (geometry, material, setup) => {
    const object = new THREE.Mesh(geometry, material);
    setup?.(object);
    return object;
  };

  return {
    football() {
      const group = new THREE.Group();
      group.add(mesh(g.football, m.football));
      return group;
    },
    basketball() {
      const group = new THREE.Group();
      group.add(
        mesh(g.ball, m.basketball),
        mesh(g.seam, m.seamDark, (o) => (o.rotation.x = Math.PI / 2)),
        mesh(g.seam, m.seamDark),
        mesh(g.sideSeam, m.seamDark, (o) => { o.rotation.y = Math.PI / 2; o.position.x = 0.66; }),
        mesh(g.sideSeam, m.seamDark, (o) => { o.rotation.y = Math.PI / 2; o.position.x = -0.66; }),
      );
      return group;
    },
    cricket() {
      const group = new THREE.Group();
      group.add(
        mesh(g.ball, m.cricket),
        mesh(g.seam, m.seamLight, (o) => { o.rotation.y = Math.PI / 2; o.position.x = 0.05; }),
        mesh(g.seam, m.seamLight, (o) => { o.rotation.y = Math.PI / 2; o.position.x = -0.05; }),
      );
      return group;
    },
    tennis() {
      const group = new THREE.Group();
      group.add(mesh(g.ball, m.tennis), mesh(g.tennisSeam, m.white));
      return group;
    },
    shuttlecock() {
      const group = new THREE.Group();
      group.add(
        mesh(g.cork, m.white, (o) => { o.rotation.x = Math.PI; o.position.y = -0.55; }),
        mesh(g.skirt, m.feathers, (o) => { o.rotation.x = Math.PI; o.position.y = 0.1; }),
        mesh(g.rim, m.white, (o) => { o.rotation.x = Math.PI / 2; o.position.y = 0.75; }),
      );
      return group;
    },
    trophy() {
      const group = new THREE.Group();
      group.add(
        mesh(g.cup, m.gold),
        mesh(g.stem, m.gold, (o) => (o.position.y = 0.18)),
        mesh(g.handle, m.gold, (o) => { o.rotation.z = Math.PI / 2; o.position.set(-0.66, 1.15, 0); }),
        mesh(g.handle, m.gold, (o) => { o.rotation.z = -Math.PI / 2; o.position.set(0.66, 1.15, 0); }),
        mesh(g.plinth, m.plinth, (o) => (o.position.y = -0.1)),
      );
      group.children.forEach((c) => (c.position.y -= 0.75));
      return group;
    },
    medal() {
      const group = new THREE.Group();
      group.add(
        mesh(g.disc, m.gold, (o) => { o.rotation.x = Math.PI / 2; o.position.y = -0.35; }),
        mesh(g.discRing, m.darkGold, (o) => (o.position.set(0, -0.35, 0.07))),
        mesh(g.ribbon, m.ribbonA, (o) => { o.rotation.z = 0.35; o.position.set(-0.2, 0.55, -0.02); }),
        mesh(g.ribbon, m.ribbonB, (o) => { o.rotation.z = -0.35; o.position.set(0.2, 0.55, -0.03); }),
      );
      return group;
    },
    controller() {
      const group = new THREE.Group();
      group.add(
        mesh(g.body, m.pad, (o) => { o.rotation.z = Math.PI / 2; o.scale.z = 0.55; }),
        mesh(g.grip, m.pad, (o) => { o.rotation.z = 0.5; o.position.set(-0.68, -0.32, 0); o.scale.z = 0.7; }),
        mesh(g.grip, m.pad, (o) => { o.rotation.z = -0.5; o.position.set(0.68, -0.32, 0); o.scale.z = 0.7; }),
        mesh(g.padBar, m.padDetail, (o) => o.position.set(-0.52, 0.08, 0.22)),
        mesh(g.padBar, m.padDetail, (o) => { o.rotation.z = Math.PI / 2; o.position.set(-0.52, 0.08, 0.22); }),
        mesh(g.stick, m.padDetail, (o) => { o.rotation.x = Math.PI / 2; o.position.set(-0.2, -0.2, 0.24); }),
        mesh(g.stick, m.padDetail, (o) => { o.rotation.x = Math.PI / 2; o.position.set(0.2, -0.2, 0.24); }),
        ...[[0.52, 0.2], [0.64, 0.08], [0.52, -0.04], [0.4, 0.08]].map(([x, y], i) => mesh(g.button, m.buttons[i], (o) => o.position.set(x, y, 0.22))),
      );
      return group;
    },
  };
})();

// pageCategory is the category of a category or competition page, or "".
function pageCategory() {
  const el = document.querySelector("main > [data-category]");
  return el ? el.dataset.category : "";
}

const KIT_FOR = {
  SPORTS: ["football", "basketball", "cricket", "tennis", "shuttlecock"],
  ACADEMIC: ["trophy", "medal", "trophy", "medal"],
  ESPORTS: ["controller", "controller", "medal", "trophy"],
  "": ["football", "basketball", "trophy", "controller", "cricket", "tennis", "medal", "shuttlecock"],
};

// ---------- the floating kit behind every page ----------

function backgroundScene() {
  const canvas = document.createElement("canvas");
  canvas.id = "bg-3d";
  canvas.setAttribute("aria-hidden", "true");
  document.body.prepend(canvas);

  const renderer = makeRenderer(canvas);
  if (!renderer) {
    canvas.remove();
    return null;
  }

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(55, 1, 0.1, 100);
  camera.position.z = 14;

  scene.add(new THREE.AmbientLight(0xffffff, 0.8));
  const sun = new THREE.DirectionalLight(0xffffff, 2);
  sun.position.set(6, 9, 8);
  scene.add(sun);

  const kinds = KIT_FOR[pageCategory()] ?? KIT_FOR[""];
  const count = window.innerWidth < 700 ? 12 : 24;
  const items = [];

  for (let i = 0; i < count; i++) {
    const item = KIT[kinds[i % kinds.length]]();
    item.scale.setScalar(0.45 + Math.random() * 0.6);
    item.position.set((Math.random() - 0.5) * 34, (Math.random() - 0.5) * 24, -Math.random() * 16 + 2);
    item.rotation.set(Math.random() * 6, Math.random() * 6, 0);
    item.userData = {
      home: item.position.clone(),
      spin: new THREE.Vector3(Math.random() - 0.5, Math.random() - 0.5, Math.random() - 0.5).multiplyScalar(1.2),
      phase: Math.random() * Math.PI * 2,
      kick: new THREE.Vector3(),
    };
    scene.add(item);
    items.push(item);
  }

  const resize = () => {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight, false);
  };
  window.addEventListener("resize", resize);
  resize();

  const look = new THREE.Vector2();

  const { start } = loop((dt, t) => {
    // The camera eases towards the pointer and sinks as you scroll.
    look.x += (pointer.x * 2.2 - look.x) * 0.04;
    look.y += (-pointer.y * 1.6 - look.y) * 0.04;
    camera.position.x = look.x;
    camera.position.y = look.y - window.scrollY * 0.004;
    camera.lookAt(0, camera.position.y * 0.6, 0);

    for (const item of items) {
      const u = item.userData;
      item.rotation.x += u.spin.x * dt;
      item.rotation.y += u.spin.y * dt;
      item.rotation.z += u.spin.z * dt;

      // Spring back home after a shockwave, plus a gentle bounce.
      u.kick.multiplyScalar(0.94);
      item.position.x += (u.home.x + u.kick.x - item.position.x) * 0.06;
      item.position.y += (u.home.y + u.kick.y + Math.abs(Math.sin(t * 0.9 + u.phase)) * 0.8 - item.position.y) * 0.06;
      item.position.z += (u.home.z + u.kick.z - item.position.z) * 0.06;
    }

    renderer.render(scene, camera);
  });

  renderer.render(scene, camera);
  start();

  return {
    // shockwave throws everything away from the centre of the screen.
    shockwave(strength = 1) {
      for (const item of items) {
        const dir = item.position.clone().setZ(0).normalize();
        item.userData.kick.add(dir.multiplyScalar(4 + Math.random() * 6 * strength));
        item.userData.spin.multiplyScalar(-1.5);
      }
    },
  };
}

// ---------- the home page stadium ----------

function stadiumScene(host, background) {
  const canvas = document.createElement("canvas");
  host.append(canvas);

  const renderer = makeRenderer(canvas, true);
  if (!renderer) {
    canvas.remove();
    return;
  }

  // Sizes are loosely in metres: a 7m goal, a spot 5.5m out (well, roughly).
  const GOAL_Z = -16;
  const GOAL_W = 7;
  const GOAL_H = 2.5;
  const SPOT = new THREE.Vector3(0, 0.42, -10.5);
  const BALL_R = 0.42;
  const NET_DEPTH = 1.6;

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 200);
  camera.position.set(0, 2.1, -3.4);
  const lookAt = new THREE.Vector3(0, 1.1, GOAL_Z);

  scene.add(new THREE.AmbientLight(0xffffff, 0.7));
  const flood = new THREE.DirectionalLight(0xffffff, 2.4);
  flood.position.set(9, 16, 4);
  scene.add(flood);
  const backFlood = new THREE.DirectionalLight(0xfff3c4, 1.2);
  backFlood.position.set(-10, 12, -24);
  scene.add(backFlood);

  // ----- the pitch: mown stripes in two flat greens -----
  const stripe = new THREE.PlaneGeometry(40, 2.9);
  for (let i = 0; i < 12; i++) {
    const band = new THREE.Mesh(stripe, solid(i % 2 ? 0x0f3d1f : 0x0b3118));
    band.rotation.x = -Math.PI / 2;
    band.position.set(0, 0, 4 - i * 2.9);
    scene.add(band);
  }
  // Grass beyond the goal line, before the stands.
  const apron = new THREE.Mesh(new THREE.PlaneGeometry(60, 10), solid(0x092a14));
  apron.rotation.x = -Math.PI / 2;
  apron.position.set(0, -0.01, GOAL_Z - 5);
  scene.add(apron);

  // ----- line markings -----
  const lines = [];
  const segment = (x1, z1, x2, z2) => lines.push(x1, 0.02, z1, x2, 0.02, z2);
  segment(-20, GOAL_Z, 20, GOAL_Z); // goal line
  segment(-9, GOAL_Z, -9, GOAL_Z + 8); // penalty box
  segment(9, GOAL_Z, 9, GOAL_Z + 8);
  segment(-9, GOAL_Z + 8, 9, GOAL_Z + 8);
  segment(-4.5, GOAL_Z, -4.5, GOAL_Z + 2.5); // six-yard box
  segment(4.5, GOAL_Z, 4.5, GOAL_Z + 2.5);
  segment(-4.5, GOAL_Z + 2.5, 4.5, GOAL_Z + 2.5);
  // The "D": the part of a circle round the spot that's outside the box.
  let prev = null;
  for (let a = -0.93; a <= 0.93; a += 0.06) {
    const p = [Math.sin(a) * 4.6, SPOT.z + Math.cos(a) * 4.6];
    if (prev) segment(prev[0], prev[1], p[0], p[1]);
    prev = p;
  }
  const lineGeometry = new THREE.BufferGeometry();
  lineGeometry.setAttribute("position", new THREE.Float32BufferAttribute(lines, 3));
  scene.add(new THREE.LineSegments(lineGeometry, new THREE.LineBasicMaterial({ color: COLORS.white })));
  const spotMark = new THREE.Mesh(new THREE.CircleGeometry(0.14, 12), solid(COLORS.white));
  spotMark.rotation.x = -Math.PI / 2;
  spotMark.position.set(SPOT.x, 0.02, SPOT.z);
  scene.add(spotMark);

  // ----- the goal and its net -----
  const postMaterial = flat(COLORS.white, { roughness: 0.3 });
  const post = new THREE.CylinderGeometry(0.07, 0.07, GOAL_H, 8);
  for (const x of [-GOAL_W / 2, GOAL_W / 2]) {
    const p = new THREE.Mesh(post, postMaterial);
    p.position.set(x, GOAL_H / 2, GOAL_Z);
    scene.add(p);
  }
  const bar = new THREE.Mesh(new THREE.CylinderGeometry(0.07, 0.07, GOAL_W + 0.14, 8), postMaterial);
  bar.rotation.z = Math.PI / 2;
  bar.position.set(0, GOAL_H, GOAL_Z);
  scene.add(bar);

  const netMaterial = solid(COLORS.white, { wireframe: true, transparent: true, opacity: 0.4 });
  const backNet = new THREE.Mesh(new THREE.PlaneGeometry(GOAL_W, GOAL_H, 16, 6), netMaterial);
  backNet.position.set(0, GOAL_H / 2, GOAL_Z - NET_DEPTH);
  scene.add(backNet);
  const netRest = backNet.geometry.attributes.position.array.slice();
  const roofNet = new THREE.Mesh(new THREE.PlaneGeometry(GOAL_W, NET_DEPTH, 16, 4), netMaterial);
  roofNet.rotation.x = -Math.PI / 2;
  roofNet.position.set(0, GOAL_H, GOAL_Z - NET_DEPTH / 2);
  scene.add(roofNet);
  for (const x of [-GOAL_W / 2, GOAL_W / 2]) {
    const side = new THREE.Mesh(new THREE.PlaneGeometry(NET_DEPTH, GOAL_H, 4, 6), netMaterial);
    side.rotation.y = Math.PI / 2;
    side.position.set(x, GOAL_H / 2, GOAL_Z - NET_DEPTH / 2);
    scene.add(side);
  }

  // ----- the stands and the crowd -----
  const crowdPositions = [];
  const crowdColors = [];
  const crowdPalette = [COLORS.volt, COLORS.sports, COLORS.academic, COLORS.esports, COLORS.white, 0x555560].map((c) => new THREE.Color(c));
  const tier = (w, d, x, y, z, along) => {
    const box = new THREE.Mesh(new THREE.BoxGeometry(w, 1.1, d), flat(y % 2 ? 0x17171d : 0x1d1d25));
    box.position.set(x, y, z);
    scene.add(box);
    const n = Math.floor((along === "x" ? w : d) * 3.2);
    for (let i = 0; i < n; i++) {
      const offset = (Math.random() - 0.5) * (along === "x" ? w : d);
      crowdPositions.push(along === "x" ? x + offset : x + (Math.random() - 0.5) * w, y + 0.7, along === "x" ? z + (Math.random() - 0.5) * d : z + offset);
      const c = crowdPalette[(Math.random() * crowdPalette.length) | 0];
      crowdColors.push(c.r, c.g, c.b);
    }
  };
  for (let i = 0; i < 6; i++) {
    tier(52, 1.8, 0, 0.55 + i * 1.15, GOAL_Z - 5.5 - i * 1.8, "x"); // behind the goal
    tier(1.8, 30, -17 - i * 1.8, 0.55 + i * 1.15, GOAL_Z + 9, "z"); // left side
    tier(1.8, 30, 17 + i * 1.8, 0.55 + i * 1.15, GOAL_Z + 9, "z"); // right side
  }
  const crowdGeometry = new THREE.BufferGeometry();
  crowdGeometry.setAttribute("position", new THREE.Float32BufferAttribute(crowdPositions, 3));
  crowdGeometry.setAttribute("color", new THREE.Float32BufferAttribute(crowdColors, 3));
  const crowd = new THREE.Points(crowdGeometry, new THREE.PointsMaterial({ size: 0.16, vertexColors: true }));
  scene.add(crowd);
  const crowdBase = crowdColors.slice();
  const flashes = []; // camera flashes in the crowd: { index, until }

  // ----- floodlight towers, each throwing a flat cone of light -----
  const beamGeometry = new THREE.ConeGeometry(4.5, 22, 20, 1, true);
  beamGeometry.translate(0, -11, 0); // apex at the origin
  beamGeometry.rotateX(-Math.PI / 2); // pointing along +z, so lookAt() aims it
  const beamMaterial = solid(0xfff8d6, { transparent: true, opacity: 0.05, depthWrite: false, side: THREE.DoubleSide });
  for (const [x, z] of [[-16, GOAL_Z - 10], [16, GOAL_Z - 10], [-22, GOAL_Z + 6], [22, GOAL_Z + 6]]) {
    const pole = new THREE.Mesh(new THREE.CylinderGeometry(0.15, 0.25, 15, 6), flat(0x2a2a33));
    pole.position.set(x, 7.5, z);
    scene.add(pole);
    const head = new THREE.Mesh(new THREE.BoxGeometry(3, 1.6, 0.3), solid(0xfffbe6));
    head.position.set(x, 15.2, z);
    head.lookAt(0, 0, GOAL_Z + 4);
    scene.add(head);
    const beam = new THREE.Mesh(beamGeometry, beamMaterial);
    beam.position.copy(head.position);
    beam.lookAt(x * 0.3, 0, GOAL_Z + 3);
    scene.add(beam);
  }

  // ----- a starry sky (square points, no texture) -----
  const starPositions = [];
  for (let i = 0; i < 500; i++) starPositions.push((Math.random() - 0.5) * 140, 14 + Math.random() * 40, GOAL_Z - 30 - Math.random() * 30);
  const starGeometry = new THREE.BufferGeometry();
  starGeometry.setAttribute("position", new THREE.Float32BufferAttribute(starPositions, 3));
  scene.add(new THREE.Points(starGeometry, new THREE.PointsMaterial({ color: COLORS.white, size: 0.18 })));

  // ----- the keeper, in the Esports magenta kit -----
  const keeper = new THREE.Group();
  const kit = flat(COLORS.esports);
  const dark = flat(0x111114);
  const glove = flat(COLORS.volt);
  const part = (geometry, material, x, y, z = 0) => {
    const m = new THREE.Mesh(geometry, material);
    m.position.set(x, y, z);
    keeper.add(m);
    return m;
  };
  part(new THREE.BoxGeometry(0.75, 0.85, 0.38), kit, 0, 1.25);
  part(new THREE.SphereGeometry(0.22, 8, 6), flat(0xd9a77c), 0, 1.92);
  part(new THREE.BoxGeometry(0.26, 0.85, 0.3), dark, -0.2, 0.42);
  part(new THREE.BoxGeometry(0.26, 0.85, 0.3), dark, 0.2, 0.42);
  const armL = part(new THREE.BoxGeometry(0.2, 0.8, 0.22), kit, -0.55, 1.55);
  const armR = part(new THREE.BoxGeometry(0.2, 0.8, 0.22), kit, 0.55, 1.55);
  armL.rotation.z = -0.5;
  armR.rotation.z = 0.5;
  part(new THREE.SphereGeometry(0.16, 6, 4), glove, -0.78, 1.95);
  part(new THREE.SphereGeometry(0.16, 6, 4), glove, 0.78, 1.95);
  keeper.position.set(0, 0, GOAL_Z + 0.5);
  scene.add(keeper);

  // ----- the ball, its shadow and the aiming ring -----
  const ball = KIT.football();
  ball.scale.setScalar(BALL_R);
  ball.position.copy(SPOT);
  scene.add(ball);

  const shadow = new THREE.Mesh(new THREE.CircleGeometry(BALL_R * 1.1, 16), solid(0x000000, { transparent: true, opacity: 0.45, depthWrite: false }));
  shadow.rotation.x = -Math.PI / 2;
  scene.add(shadow);

  const aimRing = new THREE.Mesh(new THREE.RingGeometry(0.26, 0.36, 28), solid(COLORS.volt, { transparent: true, opacity: 0.95, depthTest: false }));
  aimRing.renderOrder = 10;
  scene.add(aimRing);
  const aim = new THREE.Vector3(1.6, 1.2, GOAL_Z);

  // ----- layout -----
  // On wide screens the view is shifted so the goal sits to the right of
  // the headline; on phones it stays centred behind it.
  const resize = () => {
    const { width, height } = host.getBoundingClientRect();
    camera.aspect = width / height;
    if (width > 860) camera.setViewOffset(width, height, -width * 0.2, 0, width, height);
    else camera.clearViewOffset();
    camera.updateProjectionMatrix();
    renderer.setSize(width, height, false);
  };
  new ResizeObserver(resize).observe(host);
  resize();

  // ----- the scoreboard and result banner (HTML, see home.tmpl) -----
  const goalsEl = document.getElementById("sb-goals");
  const shotsEl = document.getElementById("sb-shots");
  const burst = document.getElementById("goal-burst");
  let goals = 0;
  let shots = 0;

  const bump = (el, value) => {
    if (!el) return;
    el.textContent = value;
    el.classList.remove("bump");
    void el.offsetWidth; // restart the animation
    el.classList.add("bump");
  };
  const announce = (text, isGoal) => {
    if (!burst) return;
    burst.textContent = text;
    burst.classList.toggle("miss", !isGoal);
    burst.classList.remove("show");
    void burst.offsetWidth;
    burst.classList.add("show");
  };

  // ----- the penalty -----
  // state: "ready" -> "flying" (towards the goal) -> "loose" (after the result)
  let state = "ready";
  let flight = null;
  const velocity = new THREE.Vector3();
  let netPush = 0;
  let netVelocity = 0;
  const netHit = new THREE.Vector2();
  let dive = { dir: 0, t: 0, active: false };
  let resetAt = 0;

  const raycaster = new THREE.Raycaster();
  const goalPlane = new THREE.Plane(new THREE.Vector3(0, 0, 1), -GOAL_Z);

  // Aim follows the pointer, projected onto the plane of the goal.
  const aimAt = (e) => {
    const r = host.getBoundingClientRect();
    const ndc = new THREE.Vector2(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1);
    raycaster.setFromCamera(ndc, camera);
    const hit = new THREE.Vector3();
    if (raycaster.ray.intersectPlane(goalPlane, hit)) {
      aim.set(THREE.MathUtils.clamp(hit.x, -6, 6), THREE.MathUtils.clamp(hit.y, 0.3, 4.2), GOAL_Z);
    }
  };

  let press = null;
  host.addEventListener("pointermove", aimAt);
  host.addEventListener("pointerdown", (e) => {
    aimAt(e);
    press = { x: e.clientX, y: e.clientY };
  });
  host.addEventListener("pointerup", (e) => {
    if (press && Math.hypot(e.clientX - press.x, e.clientY - press.y) < 12) shoot();
    press = null;
  });

  function shoot() {
    if (state !== "ready" || reducedMotion) return;
    state = "flying";
    shots += 1;
    bump(shotsEl, shots);

    // A little error on every shot, and some curl.
    const target = aim.clone().add(new THREE.Vector3((Math.random() - 0.5) * 0.7, (Math.random() - 0.5) * 0.4, 0));
    flight = { from: ball.position.clone(), to: target, curl: (Math.random() - 0.5) * 1.6, t: 0, duration: 0.62 };

    // The keeper guesses: usually towards the side you're aiming at.
    const side = Math.sign(aim.x) || 1;
    const roll = Math.random();
    dive = { dir: roll < 0.45 ? side : roll < 0.75 ? -side : 0, t: 0, active: true };
  }

  // Where the keeper's gloves can reach, given how far the dive has gone.
  function keeperReaches(x, y) {
    const kx = keeper.position.x;
    if (dive.dir === 0) return Math.abs(x - kx) < 0.9 && y < 2.6;
    return Math.abs(x - kx) < 1.25 && y < 1.9;
  }

  function finishShot() {
    const { x, y } = ball.position;
    const dir = flight.to.clone().sub(flight.from).normalize();
    const inside = Math.abs(x) < GOAL_W / 2 - BALL_R * 0.6 && y < GOAL_H - BALL_R * 0.6;
    const hitsFrame = !inside && Math.abs(Math.abs(x) - GOAL_W / 2) < BALL_R + 0.15 && y < GOAL_H + BALL_R;
    const hitsBar = !inside && Math.abs(y - GOAL_H) < BALL_R + 0.15 && Math.abs(x) < GOAL_W / 2 + BALL_R;

    let text;
    let scored = false;
    if (inside && keeperReaches(x, y)) {
      text = "Saved!";
      velocity.set((Math.random() - 0.5) * 8, 3.5, 7);
    } else if (inside) {
      text = "Goal!";
      scored = true;
      velocity.copy(dir).multiplyScalar(9);
      netHit.set(x, y);
    } else if (hitsFrame || hitsBar) {
      text = hitsBar ? "Off the bar!" : "Off the post!";
      velocity.set(-Math.sign(x) * 3 + (Math.random() - 0.5) * 3, hitsBar ? 4 : 2, 6);
    } else {
      text = y >= GOAL_H ? "Over!" : "Wide!";
      velocity.copy(dir).multiplyScalar(14);
    }

    if (scored) {
      goals += 1;
      bump(goalsEl, goals);
      background?.shockwave(1.5);
      document.dispatchEvent(new CustomEvent("arena:goal"));
      // The crowd goes wild: a burst of camera flashes.
      for (let i = 0; i < 80; i++) flashes.push({ index: (Math.random() * (crowdPositions.length / 3)) | 0, until: performance.now() + 150 + Math.random() * 900 });
    }
    announce(text, scored);

    state = "loose";
    resetAt = performance.now() + 1700;
  }

  function resetShot() {
    state = "ready";
    ball.position.copy(SPOT);
    ball.rotation.set(0, 0, 0);
    ball.scale.setScalar(0.01);
    velocity.set(0, 0, 0);
    dive = { dir: 0, t: 0, active: false };
  }

  // ----- animation -----
  const { start, stop } = loop(tick);
  new IntersectionObserver(([entry]) => (entry.isIntersecting ? start() : stop())).observe(host);

  function tick(dt, t) {
    const now = performance.now();

    // The ball pops back onto the spot after a reset.
    if (ball.scale.x < BALL_R) ball.scale.setScalar(Math.min(BALL_R, ball.scale.x + dt * 1.6));

    if (state === "flying") {
      flight.t += dt / flight.duration;
      const s = Math.min(flight.t, 1);
      ball.position.lerpVectors(flight.from, flight.to, s);
      ball.position.x += Math.sin(Math.PI * s) * flight.curl;
      ball.position.y += Math.sin(Math.PI * s) * 0.5;
      ball.rotation.x -= dt * 22;
      ball.rotation.y += flight.curl * dt * 8;
      if (s >= 1) finishShot();
    } else if (state === "loose") {
      velocity.y -= 18 * dt; // gravity
      ball.position.addScaledVector(velocity, dt);
      ball.rotation.x -= velocity.z * dt;
      if (ball.position.y < BALL_R) {
        ball.position.y = BALL_R;
        velocity.y *= -0.45;
        velocity.x *= 0.8;
        velocity.z *= 0.8;
      }
      // A goal stops in the back of the net and makes it bulge.
      if (ball.position.z < GOAL_Z - NET_DEPTH + BALL_R && Math.abs(ball.position.x) < GOAL_W / 2 && ball.position.y < GOAL_H) {
        ball.position.z = GOAL_Z - NET_DEPTH + BALL_R;
        netVelocity -= velocity.z * 0.25;
        velocity.z *= -0.15;
      }
      if (now > resetAt) resetShot();
    } else {
      ball.rotation.y += dt * 0.6; // idle on the spot
    }

    shadow.position.set(ball.position.x, 0.03, ball.position.z);
    shadow.scale.setScalar(Math.max(0.3, 1 - (ball.position.y - BALL_R) * 0.25));

    // The keeper sways on the line, then dives.
    if (dive.active) {
      dive.t = Math.min(dive.t + dt / 0.42, 1);
      const e = 1 - Math.pow(1 - dive.t, 3);
      if (dive.dir === 0) {
        keeper.position.set(0, Math.sin(e * Math.PI) * 0.8, GOAL_Z + 0.5);
        keeper.rotation.z = 0;
      } else {
        keeper.position.set(dive.dir * 2.3 * e, Math.sin(e * Math.PI) * 0.6 + e * 0.15, GOAL_Z + 0.5);
        keeper.rotation.z = -dive.dir * 1.25 * e;
      }
    } else {
      keeper.position.set(Math.sin(t * 1.7) * 0.7, Math.abs(Math.sin(t * 3.4)) * 0.08, GOAL_Z + 0.5);
      keeper.rotation.z = Math.sin(t * 1.7) * -0.06;
    }

    // The net bulges where the ball hit it, then springs back.
    netVelocity += (-60 * netPush - 7 * netVelocity) * dt;
    netPush += netVelocity * dt;
    const net = backNet.geometry.attributes.position;
    for (let i = 0; i < net.count; i++) {
      const x = netRest[i * 3];
      const y = netRest[i * 3 + 1] + GOAL_H / 2;
      const falloff = Math.max(0, 1 - Math.hypot(x - netHit.x, y - netHit.y) / 2.4);
      net.array[i * 3 + 2] = netRest[i * 3 + 2] - netPush * falloff;
    }
    net.needsUpdate = true;

    // Camera flashes twinkle across the crowd.
    const colors = crowdGeometry.attributes.color;
    if (Math.random() < 0.5) flashes.push({ index: (Math.random() * colors.count) | 0, until: now + 120 });
    for (let i = flashes.length - 1; i >= 0; i--) {
      const f = flashes[i];
      const done = now > f.until;
      colors.array[f.index * 3] = done ? crowdBase[f.index * 3] : 1;
      colors.array[f.index * 3 + 1] = done ? crowdBase[f.index * 3 + 1] : 1;
      colors.array[f.index * 3 + 2] = done ? crowdBase[f.index * 3 + 2] : 1;
      if (done) flashes.splice(i, 1);
    }
    colors.needsUpdate = true;

    aimRing.position.set(aim.x, aim.y, GOAL_Z + 0.05);
    aimRing.scale.setScalar(1 + Math.sin(t * 6) * 0.08);
    aimRing.visible = state === "ready";

    // A gentle handheld-camera drift that follows the pointer.
    camera.position.x += (pointer.x * 0.6 - camera.position.x) * 0.04;
    camera.position.y += (2.1 - pointer.y * 0.3 - camera.position.y) * 0.04;
    camera.lookAt(lookAt);

    renderer.render(scene, camera);
  }

  camera.lookAt(lookAt);
  tick(0, 0);

  // The match clock on the scoreboard ticks over once a second.
  const clock = document.getElementById("sb-clock");
  if (clock && !reducedMotion) {
    const kickoff = Date.now();
    const update = () => {
      const minute = Math.min(90, 1 + Math.floor((Date.now() - kickoff) / 1000));
      clock.textContent = minute < 90 ? `Live ${minute}'` : "Full time";
    };
    update();
    setInterval(update, 1000);
  }
}

// ---------- start ----------

const background = backgroundScene();
document.addEventListener("arena:live", () => background?.shockwave(0.6));
document.addEventListener("arena:party", () => background?.shockwave(2));

const stadiumHost = document.getElementById("hero-stage");
if (stadiumHost) stadiumScene(stadiumHost, background);
