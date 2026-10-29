// ArenaSuite 3D effects, built with three.js (vendored in /static/vendor so
// the Content-Security-Policy can stay at 'self').
//
//   - Sports kit floats behind every page: footballs, basketballs, cricket
//     and tennis balls and shuttlecocks; trophies and medals on Academic
//     pages; game controllers on Esports pages. It follows the mouse and the
//     scroll position, and scatters when a live event (or a goal) happens.
//   - The home page hero is a stadium drawn like a printed poster, with a
//     penalty shoot-out: move to aim, click to shoot, and try to beat
//     KEEPER-BOT, a robot goalkeeper. Goals get slow motion, a camera swoop,
//     paper confetti bursts and a Mexican wave.
//
// The look is a hand-inked illustration in three colours: off-white paper,
// chocolate-brown ink and light sage green. Materials are cel-shaded into
// three hard bands of light (no smooth shading, no gradients), and the main
// objects get chocolate ink outlines that "boil" a little, like hand-drawn
// animation. Nothing here is needed to use the site: without WebGL, or with
// "reduce motion" on, the page just stays still.

import * as THREE from "./vendor/three.module.min.js";

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

// The same palette as app.css.
const COLORS = {
  paper: 0xf4efe4, // off-white
  cream: 0xfbf8f1,
  choc: 0x3e2519, // chocolate brown, the ink
  cocoa: 0x6b4430,
  caramel: 0xd2a77a,
  sage: 0xaebf9a, // light sage green
  sageMid: 0x93a97f,
  moss: 0x587548,
};

function makeRenderer(canvas, opaque = false) {
  try {
    const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: !opaque, powerPreference: "low-power" });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
    renderer.setClearColor(COLORS.paper, opaque ? 1 : 0);
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

// TONES is a three-step light ramp (shadow, mid-tone, lit) read with nearest
// filtering, so toon materials snap between three flat bands of colour
// instead of shading smoothly: cel shading, like an illustration.
const TONES = (() => {
  const texture = new THREE.DataTexture(new Uint8Array([95, 175, 255]), 3, 1, THREE.RedFormat);
  texture.minFilter = THREE.NearestFilter;
  texture.magFilter = THREE.NearestFilter;
  texture.needsUpdate = true;
  return texture;
})();

const flat = (color, extra = {}) => new THREE.MeshToonMaterial({ color, gradientMap: TONES, ...extra });
const solid = (color, extra = {}) => new THREE.MeshBasicMaterial({ color, ...extra });
const ink = solid(COLORS.choc, { side: THREE.BackSide });

// inked gives every mesh in object a chocolate outline: a slightly bigger
// copy drawn inside out behind it (the "inverted hull" trick). The copy is
// stretched so the outline is about `thickness` thick on every side, in the
// mesh's own units. Rings and tubes are skipped, since growing them would
// move the line instead of outlining it. It returns the outlines, so a
// scene can make them wobble.
function inked(object, thickness = 0.05, skip = () => false) {
  const meshes = [];
  object.traverse((o) => {
    if (!o.isMesh || o.material === ink || o.material.wireframe || skip(o)) return;
    const type = o.geometry.type;
    if (type === "TorusGeometry" || type === "TubeGeometry" || type === "RingGeometry" || type === "CircleGeometry") return;
    meshes.push(o);
  });
  return meshes.map((o) => {
    if (!o.geometry.boundingBox) o.geometry.computeBoundingBox();
    const size = o.geometry.boundingBox.getSize(new THREE.Vector3()).max(new THREE.Vector3(0.02, 0.02, 0.02));
    const hull = new THREE.Mesh(o.geometry, ink);
    hull.userData = { size, thickness };
    hull.scale.set(1 + (2 * thickness) / size.x, 1 + (2 * thickness) / size.y, 1 + (2 * thickness) / size.z);
    o.add(hull);
    return hull;
  });
}

// boil redraws outlines a touch thicker or thinner and a hair off-register,
// a few times a second, so they shimmer like lines drawn by hand on every
// frame of a cartoon.
function boil(hulls) {
  for (const hull of hulls) {
    const { size, thickness } = hull.userData;
    const t = thickness * (0.7 + Math.random() * 0.6);
    hull.scale.set(1 + (2 * t) / size.x, 1 + (2 * t) / size.y, 1 + (2 * t) / size.z);
    hull.position.set((Math.random() - 0.5) * thickness * 0.5, (Math.random() - 0.5) * thickness * 0.5, 0);
  }
}

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
  const white = new THREE.Color(COLORS.cream);
  const black = new THREE.Color(COLORS.choc);

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
  // Every triangle gets its own normal, so each panel is lit as one flat face.
  geometry.computeVertexNormals();
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
    football: flat(0xffffff, { vertexColors: true }),
    basketball: flat(COLORS.caramel),
    cricket: flat(COLORS.cocoa),
    tennis: flat(COLORS.sage),
    seamDark: flat(COLORS.choc),
    seamLight: flat(COLORS.cream),
    white: flat(COLORS.cream),
    feathers: solid(COLORS.choc, { wireframe: true }),
    gold: flat(COLORS.caramel, { side: THREE.DoubleSide }),
    darkGold: flat(COLORS.cocoa),
    plinth: flat(COLORS.choc),
    ribbonA: flat(COLORS.sage),
    ribbonB: flat(COLORS.moss),
    pad: flat(COLORS.cream),
    padDetail: flat(COLORS.choc),
    buttons: [COLORS.sage, COLORS.caramel, COLORS.moss, COLORS.cocoa].map((c) => flat(c)),
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

  scene.add(new THREE.AmbientLight(0xfff6e8, 1.1));
  const sun = new THREE.DirectionalLight(0xfff6e8, 2.4);
  sun.position.set(6, 9, 8);
  scene.add(sun);

  const kinds = KIT_FOR[pageCategory()] ?? KIT_FOR[""];
  const count = window.innerWidth < 700 ? 12 : 24;
  const items = [];

  for (let i = 0; i < count; i++) {
    const item = KIT[kinds[i % kinds.length]]();
    inked(item, 0.06);
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

// makeKeeperBot builds KEEPER-BOT, the robot goalkeeper: a jointed rig
// (hips, torso, head, shoulders, elbows, hips and knees) so it can crouch,
// shuffle, dive and react. Its LED visor has three faces: normal, happy
// (^ ^, after a save) and sad (X X, after a goal). It's cream with a sage
// kit and caramel mitts, and inked like the rest of the illustration.
function makeKeeperBot() {
  const shell = flat(COLORS.cream);
  const trim = flat(COLORS.sageMid);
  const dark = flat(COLORS.choc);
  const mitt = flat(COLORS.caramel);
  const led = solid(0xd9e6c6);
  const capsule = (r, l) => new THREE.CapsuleGeometry(r, l, 3, 8);

  const add = (parent, geometry, material, x = 0, y = 0, z = 0) => {
    const m = new THREE.Mesh(geometry, material);
    m.position.set(x, y, z);
    parent.add(m);
    return m;
  };
  const joint = (parent, x, y, z = 0) => {
    const g = new THREE.Group();
    g.position.set(x, y, z);
    parent.add(g);
    return g;
  };

  const root = new THREE.Group(); // stands on the goal line
  const hips = joint(root, 0, 0.92); // the whole body rolls around the hips when diving
  add(hips, new THREE.BoxGeometry(0.5, 0.22, 0.3), dark);

  const torso = joint(hips, 0, 0.1);
  add(torso, capsule(0.3, 0.32), shell, 0, 0.42).scale.set(1.2, 1, 0.75);
  add(torso, new THREE.BoxGeometry(0.64, 0.09, 0.44), trim, 0, 0.36); // chest stripe
  add(torso, new THREE.BoxGeometry(0.16, 0.16, 0.05), trim, 0, 0.58, 0.22); // badge
  add(torso, new THREE.CylinderGeometry(0.06, 0.08, 0.14, 6), dark, 0, 0.86); // neck

  const head = joint(torso, 0, 1.06);
  add(head, new THREE.IcosahedronGeometry(0.27, 1), shell);
  add(head, new THREE.BoxGeometry(0.44, 0.15, 0.1), dark, 0, 0.02, 0.21); // visor
  add(head, new THREE.CylinderGeometry(0.012, 0.012, 0.24, 4), dark, 0.12, 0.33).rotation.z = -0.3; // antenna
  add(head, new THREE.SphereGeometry(0.04, 6, 4), trim, 0.16, 0.45);

  const faces = { normal: new THREE.Group(), happy: new THREE.Group(), sad: new THREE.Group() };
  const bar = (w) => new THREE.BoxGeometry(w, 0.02, 0.01);
  for (const face of Object.values(faces)) {
    face.position.set(0, 0.02, 0.265);
    head.add(face);
  }
  faces.happy.visible = false;
  faces.sad.visible = false;
  for (const x of [-0.1, 0.1]) {
    add(faces.normal, new THREE.BoxGeometry(0.08, 0.06, 0.01), led, x, 0);
    add(faces.happy, bar(0.06), led, x - 0.02, 0.005).rotation.z = 0.7; // ^
    add(faces.happy, bar(0.06), led, x + 0.02, 0.005).rotation.z = -0.7;
    add(faces.sad, bar(0.085), led, x, 0).rotation.z = 0.8; // X
    add(faces.sad, bar(0.085), led, x, 0).rotation.z = -0.8;
  }

  const limbs = {};
  for (const [name, side] of [["L", -1], ["R", 1]]) {
    const shoulder = joint(torso, side * 0.45, 0.62);
    add(shoulder, capsule(0.085, 0.26), shell, 0, -0.2);
    const elbow = joint(shoulder, 0, -0.4);
    add(elbow, capsule(0.075, 0.24), trim, 0, -0.18);
    add(elbow, capsule(0.15, 0.1), mitt, 0, -0.42).scale.z = 0.6;

    const hip = joint(hips, side * 0.15, -0.08);
    add(hip, capsule(0.1, 0.26), dark, 0, -0.2);
    const knee = joint(hip, 0, -0.42);
    add(knee, capsule(0.09, 0.26), shell, 0, -0.2);
    add(knee, new THREE.BoxGeometry(0.18, 0.12, 0.3), trim, 0, -0.43, 0.05);

    limbs[name] = { side, shoulder, elbow, hip, knee };
  }

  // Outline everything except the chocolate parts (the ink would vanish
  // into them anyway) and the LED faces.
  const outlines = inked(root, 0.022, (o) => o.material === dark || o.material === led);

  // The pose is a handful of numbers that ease towards a target each frame.
  const pose = { x: 0, lift: 0, roll: 0, crouch: 0.5, lean: 0.2, armL: -0.9, armR: 0.9, elbow: -0.7, stance: 0.18, yaw: 0, pitch: 0 };
  let face = "normal";

  return {
    root,
    pose,
    outlines,
    // apply eases the pose towards target (rate = how snappy) and moves the joints.
    apply(target, rate, dt) {
      const k = 1 - Math.exp(-rate * dt);
      for (const key of Object.keys(pose)) pose[key] += (target[key] - pose[key]) * k;

      root.position.x = pose.x;
      root.position.y = pose.lift - pose.crouch * 0.2;
      hips.rotation.z = pose.roll;
      torso.rotation.x = pose.lean;
      head.rotation.set(pose.pitch, pose.yaw, 0);
      for (const limb of Object.values(limbs)) {
        limb.shoulder.rotation.z = limb.side < 0 ? pose.armL : pose.armR;
        limb.elbow.rotation.x = pose.elbow;
        limb.hip.rotation.set(-pose.crouch * 0.8, 0, limb.side * pose.stance);
        limb.knee.rotation.x = pose.crouch * 1.5;
      }
    },
    setFace(name) {
      if (name === face) return;
      face = name;
      for (const [key, group] of Object.entries(faces)) group.visible = key === name;
    },
  };
}

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

  // A warm afternoon sun over the paper.
  scene.add(new THREE.AmbientLight(0xfff6e8, 1.05));
  const sun = new THREE.DirectionalLight(0xfff3e0, 2.5);
  sun.position.set(9, 16, 6);
  scene.add(sun);

  // Everything inked in the stadium wobbles together (see boil()).
  const outlines = [];

  // ----- the sky: an art deco sunburst behind a paper sun -----
  // Long flat rays fan out from the sun in two shades of paper and turn
  // very slowly. The stands cover the bottom half.
  const sky = new THREE.Group();
  sky.position.set(0, 7, GOAL_Z - 46);
  scene.add(sky);
  const rays = new THREE.Group();
  sky.add(rays);
  const RAYS = 28;
  for (let i = 0; i < RAYS; i += 2) {
    const a = (i / RAYS) * Math.PI * 2;
    const b = ((i + 1) / RAYS) * Math.PI * 2;
    const shape = new THREE.Shape([new THREE.Vector2(0, 0), new THREE.Vector2(Math.cos(a) * 140, Math.sin(a) * 140), new THREE.Vector2(Math.cos(b) * 140, Math.sin(b) * 140)]);
    rays.add(new THREE.Mesh(new THREE.ShapeGeometry(shape), solid(0xebe2cf)));
  }
  const sunDisc = new THREE.Mesh(new THREE.CircleGeometry(7, 48), solid(COLORS.caramel));
  sunDisc.position.z = 0.1;
  const sunRim = new THREE.Mesh(new THREE.RingGeometry(7, 7.5, 48), solid(COLORS.choc));
  sunRim.position.z = 0.12;
  const sunHalo = new THREE.Mesh(new THREE.RingGeometry(8.6, 9, 48), solid(COLORS.sage));
  sunHalo.position.z = 0.11;
  sky.add(sunDisc, sunRim, sunHalo);

  // Paper-cut clouds: overlapping cream circles on slightly bigger
  // chocolate ones, so the outline follows the whole puffy shape.
  const clouds = [];
  const puff = new THREE.CircleGeometry(1, 28);
  const cloudCream = solid(COLORS.cream);
  for (let i = 0; i < 4; i++) {
    const cloud = new THREE.Group();
    for (const [x, y, r] of [[0, 0, 1.6], [1.7, -0.2, 1.25], [-1.7, -0.3, 1.1], [0.8, 0.9, 1.15], [-0.7, 0.7, 0.95]]) {
      const edge = new THREE.Mesh(puff, solid(COLORS.choc));
      edge.position.set(x, y, 0);
      edge.scale.setScalar(r + 0.16);
      const fill = new THREE.Mesh(puff, cloudCream);
      fill.position.set(x, y, 0.02);
      fill.scale.setScalar(r);
      cloud.add(edge, fill);
    }
    cloud.position.set(-40 + i * 24, 12 + (i % 2) * 5, GOAL_Z - 34 + (i % 2) * 3);
    cloud.scale.setScalar(1 + (i % 3) * 0.35);
    cloud.userData.speed = 0.6 + Math.random() * 0.7;
    scene.add(cloud);
    clouds.push(cloud);
  }

  // A few birds: ink ticks that flap as they cross the sky.
  const birds = Array.from({ length: 6 }, (_, i) => {
    const bird = new THREE.Group();
    const wing = new THREE.BoxGeometry(0.7, 0.07, 0.02);
    wing.translate(0.35, 0, 0);
    const left = new THREE.Mesh(wing, solid(COLORS.choc));
    const right = new THREE.Mesh(wing, solid(COLORS.choc));
    right.rotation.y = Math.PI;
    bird.add(left, right);
    bird.position.set(-30 + i * 9, 15 + Math.random() * 6, GOAL_Z - 26 - Math.random() * 6);
    bird.userData = { left, right, phase: Math.random() * 6, speed: 1.6 + Math.random() };
    scene.add(bird);
    return bird;
  });

  // ----- the pitch: mown stripes in two flat sages -----
  const stripe = new THREE.PlaneGeometry(40, 2.9);
  for (let i = 0; i < 12; i++) {
    const band = new THREE.Mesh(stripe, solid(i % 2 ? 0xb3c3a0 : 0xa5b791));
    band.rotation.x = -Math.PI / 2;
    band.position.set(0, 0, 4 - i * 2.9);
    scene.add(band);
  }
  const apron = new THREE.Mesh(new THREE.PlaneGeometry(60, 10), solid(COLORS.sageMid));
  apron.rotation.x = -Math.PI / 2;
  apron.position.set(0, -0.01, GOAL_Z - 5);
  scene.add(apron);

  // ----- line markings, painted on as flat cream strips -----
  const chalk = solid(COLORS.cream);
  const segment = (x1, z1, x2, z2, width = 0.12) => {
    const length = Math.hypot(x2 - x1, z2 - z1);
    const strip = new THREE.Mesh(new THREE.PlaneGeometry(width, length + width), chalk);
    strip.rotation.set(-Math.PI / 2, 0, -Math.atan2(x2 - x1, z2 - z1));
    strip.position.set((x1 + x2) / 2, 0.02, (z1 + z2) / 2);
    scene.add(strip);
  };
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
  const spotMark = new THREE.Mesh(new THREE.CircleGeometry(0.16, 16), chalk);
  spotMark.rotation.x = -Math.PI / 2;
  spotMark.position.set(SPOT.x, 0.02, SPOT.z);
  scene.add(spotMark);

  // ----- the goal and its net -----
  const postMaterial = flat(COLORS.cream);
  const goalFrame = new THREE.Group();
  const post = new THREE.CylinderGeometry(0.08, 0.08, GOAL_H, 10);
  for (const x of [-GOAL_W / 2, GOAL_W / 2]) {
    const p = new THREE.Mesh(post, postMaterial);
    p.position.set(x, GOAL_H / 2, GOAL_Z);
    goalFrame.add(p);
  }
  const crossbar = new THREE.Mesh(new THREE.CylinderGeometry(0.08, 0.08, GOAL_W + 0.16, 10), postMaterial);
  crossbar.rotation.z = Math.PI / 2;
  crossbar.position.set(0, GOAL_H, GOAL_Z);
  goalFrame.add(crossbar);
  scene.add(goalFrame);
  outlines.push(...inked(goalFrame, 0.035));

  // The net is drawn in fine ink lines.
  const netMaterial = solid(COLORS.choc, { wireframe: true, transparent: true, opacity: 0.45 });
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
  // The stands are chocolate paper-cut tiers, and the crowd is confetti
  // dots in the house colours. Each fan remembers their seat height and
  // their angle around the stadium, so a Mexican wave can travel round
  // after a goal.
  const crowdPositions = [];
  const crowdColors = [];
  const crowdSeatY = [];
  const crowdAngle = [];
  const crowdPalette = [COLORS.cream, COLORS.sage, COLORS.caramel, COLORS.paper, COLORS.sageMid, 0xe7d3b8].map((c) => new THREE.Color(c));
  const stadiumCentre = GOAL_Z + 6;
  const tierMaterials = [solid(COLORS.choc), solid(0x55331f)];
  const tierEdge = solid(COLORS.caramel);
  const tier = (i, w, d, x, y, z, along) => {
    const box = new THREE.Mesh(new THREE.BoxGeometry(w, 1.1, d), tierMaterials[i % 2]);
    box.position.set(x, y, z);
    scene.add(box);
    // A thin caramel lip along the front edge of every tier, like a printed highlight.
    const lip = new THREE.Mesh(new THREE.BoxGeometry(along === "x" ? w : 0.08, 0.08, along === "x" ? 0.08 : d), tierEdge);
    lip.position.set(along === "x" ? x : x + (x < 0 ? 0.92 : -0.92), y + 0.56, along === "x" ? z + 0.92 : z);
    scene.add(lip);
    const n = Math.floor((along === "x" ? w : d) * 3.2);
    for (let k = 0; k < n; k++) {
      const offset = (Math.random() - 0.5) * (along === "x" ? w : d);
      const px = along === "x" ? x + offset : x + (Math.random() - 0.5) * w;
      const pz = along === "x" ? z + (Math.random() - 0.5) * d : z + offset;
      crowdPositions.push(px, y + 0.7, pz);
      crowdSeatY.push(y + 0.7);
      crowdAngle.push(Math.atan2(px, pz - stadiumCentre));
      const c = crowdPalette[(Math.random() * crowdPalette.length) | 0];
      crowdColors.push(c.r, c.g, c.b);
    }
  };
  for (let i = 0; i < 6; i++) {
    tier(i, 52, 1.8, 0, 0.55 + i * 1.15, GOAL_Z - 5.5 - i * 1.8, "x"); // behind the goal
    tier(i, 1.8, 30, -17 - i * 1.8, 0.55 + i * 1.15, GOAL_Z + 9, "z"); // left side
    tier(i, 1.8, 30, 17 + i * 1.8, 0.55 + i * 1.15, GOAL_Z + 9, "z"); // right side
  }
  const crowdGeometry = new THREE.BufferGeometry();
  crowdGeometry.setAttribute("position", new THREE.Float32BufferAttribute(crowdPositions, 3));
  crowdGeometry.setAttribute("color", new THREE.Float32BufferAttribute(crowdColors, 3));
  scene.add(new THREE.Points(crowdGeometry, new THREE.PointsMaterial({ size: 0.2, vertexColors: true })));
  const crowdBase = crowdColors.slice();
  const flashes = []; // fans waving flags: { index, until }
  const flagColor = new THREE.Color(COLORS.sage);
  let waveStart = -Infinity;

  // ----- bunting: a string of pennants across the stand behind the goal -----
  // They hang from a sagging string and flutter in the breeze.
  const pennants = [];
  const pennantShape = new THREE.Shape([new THREE.Vector2(-0.38, 0), new THREE.Vector2(0.38, 0), new THREE.Vector2(0, -0.95)]);
  const pennantGeometry = new THREE.ShapeGeometry(pennantShape);
  const pennantColors = [COLORS.sage, COLORS.cream, COLORS.caramel, COLORS.sageMid].map((c) => solid(c, { side: THREE.DoubleSide }));
  const BUNTING_Y = 6.4;
  const buntingZ = GOAL_Z - 4.2;
  const sag = (x) => BUNTING_Y - 1.4 * (1 - (x / 22) ** 2);
  const stringPoints = [];
  for (let x = -22; x <= 22; x += 1) stringPoints.push(new THREE.Vector3(x, sag(x), buntingZ));
  scene.add(new THREE.Line(new THREE.BufferGeometry().setFromPoints(stringPoints), new THREE.LineBasicMaterial({ color: COLORS.choc })));
  for (let x = -21; x <= 21; x += 1.05) {
    const flag = new THREE.Mesh(pennantGeometry, pennantColors[pennants.length % pennantColors.length]);
    flag.position.set(x, sag(x), buntingZ);
    flag.userData.phase = x * 0.5;
    scene.add(flag);
    pennants.push(flag);
  }

  // ----- floodlight towers, silhouetted in chocolate -----
  for (const [x, z] of [[-16, GOAL_Z - 10], [16, GOAL_Z - 10], [-22, GOAL_Z + 6], [22, GOAL_Z + 6]]) {
    const pole = new THREE.Mesh(new THREE.CylinderGeometry(0.15, 0.25, 15, 6), solid(COLORS.choc));
    pole.position.set(x, 7.5, z);
    scene.add(pole);
    const lamp = new THREE.Mesh(new THREE.BoxGeometry(3, 1.6, 0.3), solid(COLORS.cream));
    lamp.position.set(x, 15.2, z);
    lamp.lookAt(0, 0, GOAL_Z + 4);
    scene.add(lamp);
    const lampEdge = new THREE.Mesh(new THREE.BoxGeometry(3.3, 1.9, 0.2), solid(COLORS.choc));
    lampEdge.position.set(0, 0, -0.1);
    lamp.add(lampEdge);
  }

  // ----- confetti bursts: a pool of paper squares that pop over the stands -----
  const SPARKS = 480;
  const sparkPositions = new Float32Array(SPARKS * 3).fill(-999);
  const sparkColors = new Float32Array(SPARKS * 3);
  const sparks = Array.from({ length: SPARKS }, () => ({ velocity: new THREE.Vector3(), life: 0 }));
  const sparkGeometry = new THREE.BufferGeometry();
  sparkGeometry.setAttribute("position", new THREE.BufferAttribute(sparkPositions, 3));
  sparkGeometry.setAttribute("color", new THREE.BufferAttribute(sparkColors, 3));
  scene.add(new THREE.Points(sparkGeometry, new THREE.PointsMaterial({ size: 0.38, vertexColors: true })));
  let nextSpark = 0;

  const firework = (origin, color) => {
    const c = new THREE.Color(color);
    for (let i = 0; i < 70; i++) {
      const s = sparks[nextSpark];
      sparkPositions.set([origin.x, origin.y, origin.z], nextSpark * 3);
      sparkColors.set([c.r, c.g, c.b], nextSpark * 3);
      s.velocity.randomDirection().multiplyScalar(5 + Math.random() * 4);
      s.life = 1.4 + Math.random() * 0.5;
      nextSpark = (nextSpark + 1) % SPARKS;
    }
  };

  // ----- KEEPER-BOT -----
  const keeper = makeKeeperBot();
  keeper.root.position.z = GOAL_Z + 0.5;
  scene.add(keeper.root);
  outlines.push(...keeper.outlines);
  const keeperShadow = new THREE.Mesh(new THREE.CircleGeometry(0.7, 20), solid(COLORS.choc, { transparent: true, opacity: 0.22, depthWrite: false }));
  keeperShadow.rotation.x = -Math.PI / 2;
  keeperShadow.scale.y = 0.45;
  keeperShadow.position.set(0, 0.025, GOAL_Z + 0.5);
  scene.add(keeperShadow);

  // ----- the ball, its shadow, its trail and the aiming ring -----
  const ball = KIT.football();
  outlines.push(...inked(ball, 0.07));
  ball.scale.setScalar(BALL_R);
  ball.position.copy(SPOT);
  scene.add(ball);

  const shadow = new THREE.Mesh(new THREE.CircleGeometry(BALL_R * 1.1, 16), solid(COLORS.choc, { transparent: true, opacity: 0.3, depthWrite: false }));
  shadow.rotation.x = -Math.PI / 2;
  scene.add(shadow);

  // The trail is a row of shrinking sage ghosts of the ball, like the
  // motion marks behind a ball in a comic.
  const trailGeometry = new THREE.IcosahedronGeometry(BALL_R, 1);
  const trailMaterial = solid(COLORS.sageMid, { transparent: true, opacity: 0.5, depthWrite: false });
  const trail = Array.from({ length: 10 }, (_, i) => {
    const ghost = new THREE.Mesh(trailGeometry, trailMaterial);
    ghost.scale.setScalar(0.85 - i * 0.07);
    ghost.visible = false;
    scene.add(ghost);
    return ghost;
  });
  const history = [];

  const aimRing = new THREE.Mesh(new THREE.RingGeometry(0.26, 0.36, 28), solid(COLORS.choc, { transparent: true, opacity: 0.9, depthTest: false }));
  aimRing.renderOrder = 10;
  const aimDot = new THREE.Mesh(new THREE.CircleGeometry(0.1, 16), solid(COLORS.caramel, { depthTest: false }));
  aimDot.renderOrder = 11;
  aimRing.add(aimDot);
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

  // ----- the scoreboard, result banner and slow-mo tag (HTML, see home.tmpl) -----
  const goalsEl = document.getElementById("sb-goals");
  const shotsEl = document.getElementById("sb-shots");
  const burst = document.getElementById("goal-burst");
  const slowmoTag = document.getElementById("slowmo-tag");
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
  let resetAt = 0;

  // What the keeper is doing: "ready", "dive", "celebrate", "sad" or "watch".
  let keeperMode = "ready";
  let diveDir = 0;
  let diveAt = 0;

  // Goals get the full treatment: slow motion and a camera swoop.
  let timeScale = 1;
  let slowmoUntil = 0;
  let replayUntil = 0;

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
  // Aim keeps following the pointer between shots, so it's right when the
  // ball is back on the spot. A shot in the air keeps its own target.
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
    history.length = 0;

    // The keeper guesses: usually towards the side you're aiming at.
    const side = Math.sign(aim.x) || 1;
    const roll = Math.random();
    diveDir = roll < 0.45 ? side : roll < 0.75 ? -side : 0;
    diveAt = performance.now() + 110; // a split second to react
  }

  // Where the keeper's mitts can reach at the moment the ball arrives.
  function keeperReaches(x, y) {
    const kx = keeper.pose.x;
    if (diveDir === 0) return Math.abs(x - kx) < 0.9 && y < 2.7;
    return Math.abs(x - kx) < 1.3 && y < 2.0;
  }

  function finishShot() {
    const now = performance.now();
    const { x, y } = ball.position;
    const dir = flight.to.clone().sub(flight.from).normalize();
    const inside = Math.abs(x) < GOAL_W / 2 - BALL_R * 0.6 && y < GOAL_H - BALL_R * 0.6;
    const hitsFrame = !inside && Math.abs(Math.abs(x) - GOAL_W / 2) < BALL_R + 0.15 && y < GOAL_H + BALL_R;
    const hitsBar = !inside && Math.abs(y - GOAL_H) < BALL_R + 0.15 && Math.abs(x) < GOAL_W / 2 + BALL_R;

    let text;
    let scored = false;
    if (inside && keeperReaches(x, y)) {
      text = "Saved!";
      keeperMode = "celebrate";
      velocity.set((Math.random() - 0.5) * 8, 3.5, 7);
    } else if (inside) {
      text = "Goal!";
      scored = true;
      keeperMode = "sad";
      velocity.copy(dir).multiplyScalar(9);
      netHit.set(x, y);
    } else if (hitsFrame || hitsBar) {
      text = hitsBar ? "Off the bar!" : "Off the post!";
      keeperMode = "watch";
      velocity.set(-Math.sign(x) * 3 + (Math.random() - 0.5) * 3, hitsBar ? 4 : 2, 6);
    } else {
      text = y >= GOAL_H ? "Over!" : "Wide!";
      keeperMode = "watch";
      velocity.copy(dir).multiplyScalar(14);
    }

    if (scored) {
      goals += 1;
      bump(goalsEl, goals);
      background?.shockwave(1.5);
      document.dispatchEvent(new CustomEvent("arena:goal"));

      // Slow motion and a camera swoop to the side of the goal...
      slowmoUntil = now + 1100;
      replayUntil = now + 2000;
      slowmoTag?.classList.add("on");
      // ...the crowd stands up in a wave, waving sage flags everywhere...
      waveStart = now;
      for (let i = 0; i < 90; i++) flashes.push({ index: (Math.random() * (crowdPositions.length / 3)) | 0, until: now + 150 + Math.random() * 1200 });
      // ...and confetti pops over the stands.
      [COLORS.sage, COLORS.caramel, COLORS.choc, COLORS.moss, COLORS.cream].forEach((color, i) => {
        setTimeout(() => firework(new THREE.Vector3((Math.random() - 0.5) * 30, 12 + Math.random() * 6, GOAL_Z - 8 - Math.random() * 6), color), 250 + i * 260);
      });
    }
    announce(text, scored);

    state = "loose";
    resetAt = now + (scored ? 2600 : 1800);
  }

  function resetShot() {
    state = "ready";
    keeperMode = "ready";
    diveDir = 0;
    ball.position.copy(SPOT);
    ball.rotation.set(0, 0, 0);
    ball.scale.setScalar(0.01);
    velocity.set(0, 0, 0);
    history.length = 0;
  }

  // ----- keeper choreography -----
  // Each mode is a target pose; KEEPER-BOT eases towards it.
  function keeperTarget(t) {
    const ready = {
      x: Math.sin(t * 1.7) * 0.7, // shuffling along the line
      lift: Math.abs(Math.sin(t * 3.4)) * 0.06,
      roll: Math.sin(t * 1.7) * -0.05,
      crouch: 0.55,
      lean: 0.22,
      armL: -0.95,
      armR: 0.95,
      elbow: -0.7,
      stance: 0.2,
      // The head follows your aim.
      yaw: THREE.MathUtils.clamp((aim.x - keeper.pose.x) * 0.12, -0.6, 0.6),
      pitch: THREE.MathUtils.clamp((1.4 - aim.y) * 0.12, -0.3, 0.3),
    };

    switch (keeperMode) {
      case "dive":
        return diveDir === 0
          ? { ...ready, x: 0, lift: 0.8, roll: 0, crouch: 0, lean: 0, armL: -2.95, armR: 2.95, elbow: 0, stance: 0.08, yaw: 0, pitch: -0.3 }
          : { ...ready, x: diveDir * 2.3, lift: 0.55, roll: -diveDir * 1.35, crouch: 0.05, lean: 0, armL: -2.8, armR: 2.8, elbow: 0, stance: 0.05, yaw: 0, pitch: 0 };
      case "celebrate": {
        // Fist pumps on the spot.
        const pump = Math.abs(Math.sin(t * 9));
        return { ...ready, x: keeper.pose.x * 0.98, lift: pump * 0.45, roll: 0, crouch: 0.15, lean: -0.1, armL: -2.6 - pump * 0.3, armR: 2.6 + pump * 0.3, elbow: -0.6 * pump, yaw: 0, pitch: -0.25 };
      }
      case "sad":
        // Slumped on its knees, head down.
        return { ...ready, x: keeper.pose.x, lift: -0.08, roll: 0, crouch: 0.95, lean: 0.55, armL: -0.15, armR: 0.15, elbow: -0.2, stance: 0.25, yaw: 0, pitch: 0.65 };
      case "watch":
        // Turns to watch the ball go by.
        return { ...ready, x: keeper.pose.x, crouch: 0.3, armL: -1.3, armR: 1.3, elbow: -1.4, yaw: THREE.MathUtils.clamp(ball.position.x * 0.2, -0.8, 0.8), pitch: -0.2 };
      default:
        return ready;
    }
  }

  // ----- animation -----
  const cameraPosition = new THREE.Vector3(0, 2.1, -3.4);
  const cameraLook = new THREE.Vector3(0, 1.1, GOAL_Z);
  const wantPosition = new THREE.Vector3();
  const wantLook = new THREE.Vector3();

  let lastBoil = 0;
  const { start, stop } = loop(tick);
  new IntersectionObserver(([entry]) => (entry.isIntersecting ? start() : stop())).observe(host);

  function tick(realDt, t) {
    const now = performance.now();

    // Slow motion just after a goal.
    const slowmo = now < slowmoUntil;
    timeScale += ((slowmo ? 0.28 : 1) - timeScale) * 0.15;
    if (!slowmo) slowmoTag?.classList.remove("on");
    const dt = realDt * timeScale;

    if (state === "flying" && keeperMode === "ready" && now > diveAt) keeperMode = "dive";

    // The ball pops back onto the spot after a reset.
    if (ball.scale.x < BALL_R) ball.scale.setScalar(Math.min(BALL_R, ball.scale.x + realDt * 1.6));

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
      ball.rotation.y += realDt * 0.6; // idle on the spot
    }

    shadow.position.set(ball.position.x, 0.03, ball.position.z);
    shadow.scale.setScalar(Math.max(0.3, 1 - (ball.position.y - BALL_R) * 0.25));

    // The trail follows the ball while it's in the air.
    history.unshift(ball.position.clone());
    history.length = Math.min(history.length, trail.length * 2);
    trail.forEach((ghost, i) => {
      const p = history[(i + 1) * 2];
      ghost.visible = state !== "ready" && Boolean(p) && ball.position.y > BALL_R + 0.05;
      if (p) ghost.position.copy(p);
    });

    // KEEPER-BOT: dives are snappy, everything else is smooth.
    keeper.apply(keeperTarget(t), keeperMode === "dive" ? 14 : 7, dt);
    keeper.setFace(keeperMode === "celebrate" ? "happy" : keeperMode === "sad" ? "sad" : "normal");

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

    // The crowd: fans wave sage flags now and then, and do a Mexican wave
    // after a goal.
    const colors = crowdGeometry.attributes.color;
    if (Math.random() < 0.5) flashes.push({ index: (Math.random() * colors.count) | 0, until: now + 220 });
    for (let i = flashes.length - 1; i >= 0; i--) {
      const f = flashes[i];
      const done = now > f.until;
      colors.array[f.index * 3] = done ? crowdBase[f.index * 3] : flagColor.r;
      colors.array[f.index * 3 + 1] = done ? crowdBase[f.index * 3 + 1] : flagColor.g;
      colors.array[f.index * 3 + 2] = done ? crowdBase[f.index * 3 + 2] : flagColor.b;
      if (done) flashes.splice(i, 1);
    }
    colors.needsUpdate = true;

    const waveAge = (now - waveStart) / 1000;
    if (waveAge < 4.5) {
      const positions = crowdGeometry.attributes.position;
      for (let i = 0; i < positions.count; i++) {
        const phase = crowdAngle[i] * 2.2 - waveAge * 5;
        const up = waveAge < 4 ? Math.max(0, Math.cos(phase)) ** 6 * 0.9 : 0;
        positions.array[i * 3 + 1] = crowdSeatY[i] + up;
      }
      positions.needsUpdate = true;
    }

    // Confetti: the squares fly out, flutter down and drop out of sight.
    for (let i = 0; i < SPARKS; i++) {
      const s = sparks[i];
      if (s.life <= 0) continue;
      s.life -= realDt;
      s.velocity.y -= 6 * realDt;
      s.velocity.multiplyScalar(0.975);
      s.velocity.x += Math.sin(t * 7 + i) * 0.05; // a little flutter
      sparkPositions[i * 3] += s.velocity.x * realDt;
      sparkPositions[i * 3 + 1] += s.velocity.y * realDt;
      sparkPositions[i * 3 + 2] += s.velocity.z * realDt;
      if (s.life <= 0) sparkPositions[i * 3 + 1] = -999;
    }
    sparkGeometry.attributes.position.needsUpdate = true;
    sparkGeometry.attributes.color.needsUpdate = true;

    // The sky drifts: the sunburst turns, clouds float by, birds flap past
    // and the bunting flutters.
    rays.rotation.z += realDt * 0.025;
    for (const cloud of clouds) {
      cloud.position.x += cloud.userData.speed * realDt;
      if (cloud.position.x > 48) cloud.position.x = -48;
    }
    for (const bird of birds) {
      const u = bird.userData;
      bird.position.x += u.speed * realDt;
      bird.position.y += Math.sin(t * 0.8 + u.phase) * 0.004;
      if (bird.position.x > 40) bird.position.x = -40;
      const flap = Math.sin(t * 9 + u.phase) * 0.55;
      // The right wing is the left one turned round, so both use the same angle.
      u.left.rotation.z = 0.35 + flap;
      u.right.rotation.z = 0.35 + flap;
    }
    for (const flag of pennants) {
      flag.rotation.x = Math.sin(t * 2.6 + flag.userData.phase) * 0.45 - 0.15;
      flag.rotation.z = Math.sin(t * 1.9 + flag.userData.phase) * 0.12;
    }

    // The ink outlines are redrawn about eight times a second.
    if (t - lastBoil > 0.125) {
      lastBoil = t;
      boil(outlines);
    }

    keeperShadow.position.x = keeper.root.position.x;

    aimRing.position.set(aim.x, aim.y, GOAL_Z + 0.05);
    aimRing.scale.setScalar(1 + Math.sin(t * 6) * 0.08);
    aimRing.visible = state === "ready";

    // The camera: a handheld drift behind the ball, or a swoop to the side
    // of the goal for the slow-motion replay.
    if (now < replayUntil) {
      wantPosition.set(7.5, 2.2, GOAL_Z + 6.5);
      wantLook.set(ball.position.x * 0.6 - 1.5, 1.0, GOAL_Z - 0.6);
    } else {
      wantPosition.set(pointer.x * 0.6, 2.1 - pointer.y * 0.3, -3.4);
      wantLook.set(0, 1.1, GOAL_Z);
    }
    const ease = now < replayUntil ? 0.06 : 0.05;
    cameraPosition.lerp(wantPosition, ease);
    cameraLook.lerp(wantLook, ease);
    camera.position.copy(cameraPosition);
    camera.lookAt(cameraLook);

    renderer.render(scene, camera);
  }

  keeper.apply(keeperTarget(0), 100, 1);
  camera.lookAt(cameraLook);
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
