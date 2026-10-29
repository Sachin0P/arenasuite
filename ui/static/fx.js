// ArenaSuite 3D effects, built with three.js (vendored in /static/vendor so
// the Content-Security-Policy can stay at 'self').
//
//   - A field of low-poly shapes floats behind every page. It follows the
//     mouse and the scroll position, and scatters when a live event arrives.
//   - The home page hero has a faceted "arena orb": drag it to spin it, click
//     it to smash it apart, and it springs back together.
//
// Every material is flat-shaded or wireframe, so each face is one solid
// colour. Nothing here is needed to use the site: if WebGL isn't available,
// or the visitor prefers reduced motion, the page just stays still.

import * as THREE from "./vendor/three.module.min.js";

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

const COLORS = {
  volt: 0xffe600,
  sports: 0xc6ff00,
  academic: 0x5b8cff,
  esports: 0xff2e88,
  steel: 0x2a2a33,
};

function makeRenderer(canvas) {
  try {
    const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true, powerPreference: "low-power" });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
    renderer.setClearColor(0x000000, 0);
    return renderer;
  } catch {
    return null; // no WebGL: the page works fine without the scene
  }
}

// pagePalette picks the colours for this page: the category's accent on a
// category page, all three category colours elsewhere.
function pagePalette() {
  const el = document.querySelector("main [data-category]");
  if (el && location.pathname !== "/") {
    const accent = getComputedStyle(el).getPropertyValue("--accent").trim();
    if (accent) return [new THREE.Color(accent), new THREE.Color(COLORS.volt)];
  }
  return [COLORS.sports, COLORS.academic, COLORS.esports, COLORS.volt].map((c) => new THREE.Color(c));
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

// ---------- the floating field behind every page ----------

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

  scene.add(new THREE.AmbientLight(0xffffff, 0.55));
  const sun = new THREE.DirectionalLight(0xffffff, 1.6);
  sun.position.set(6, 9, 8);
  scene.add(sun);

  const palette = pagePalette();
  const geometries = [
    new THREE.IcosahedronGeometry(1, 0),
    new THREE.OctahedronGeometry(1, 0),
    new THREE.TetrahedronGeometry(1.1, 0),
    new THREE.BoxGeometry(1.3, 1.3, 1.3),
    new THREE.TorusKnotGeometry(0.7, 0.22, 48, 6),
    new THREE.DodecahedronGeometry(1, 0),
  ];

  const floaters = [];
  const count = window.innerWidth < 700 ? 18 : 34;

  for (let i = 0; i < count; i++) {
    const color = palette[i % palette.length];
    const wire = i % 3 === 0;
    const material = wire
      ? new THREE.MeshBasicMaterial({ color, wireframe: true, transparent: true, opacity: 0.55 })
      : new THREE.MeshStandardMaterial({ color: i % 2 ? color : COLORS.steel, flatShading: true, roughness: 0.55, metalness: 0.2 });

    const mesh = new THREE.Mesh(geometries[i % geometries.length], material);
    const scale = 0.35 + Math.random() * 0.9;
    mesh.scale.setScalar(scale);
    mesh.position.set((Math.random() - 0.5) * 34, (Math.random() - 0.5) * 24, -Math.random() * 16 + 2);
    mesh.rotation.set(Math.random() * 6, Math.random() * 6, 0);
    mesh.userData = {
      home: mesh.position.clone(),
      spin: new THREE.Vector3(Math.random() - 0.5, Math.random() - 0.5, Math.random() - 0.5).multiplyScalar(0.9),
      phase: Math.random() * Math.PI * 2,
      kick: new THREE.Vector3(),
    };
    scene.add(mesh);
    floaters.push(mesh);
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

    for (const m of floaters) {
      const u = m.userData;
      m.rotation.x += u.spin.x * dt;
      m.rotation.y += u.spin.y * dt;
      m.rotation.z += u.spin.z * dt;

      // Spring back home after a shockwave, plus a gentle bob.
      u.kick.multiplyScalar(0.94);
      m.position.x += (u.home.x + u.kick.x - m.position.x) * 0.06;
      m.position.y += (u.home.y + u.kick.y + Math.sin(t * 0.6 + u.phase) * 0.5 - m.position.y) * 0.06;
      m.position.z += (u.home.z + u.kick.z - m.position.z) * 0.06;
    }

    renderer.render(scene, camera);
  });

  renderer.render(scene, camera);
  start();

  return {
    // shockwave pushes every shape away from the centre of the screen.
    shockwave(strength = 1) {
      for (const m of floaters) {
        const dir = m.position.clone().setZ(0).normalize();
        m.userData.kick.add(dir.multiplyScalar(4 + Math.random() * 6 * strength));
        m.userData.spin.multiplyScalar(-1);
      }
    },
  };
}

// ---------- the home page orb ----------

function heroScene(host, background) {
  const canvas = document.createElement("canvas");
  host.append(canvas);

  const renderer = makeRenderer(canvas);
  if (!renderer) {
    canvas.remove();
    return;
  }

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
  camera.position.set(0, 0, 10);

  scene.add(new THREE.AmbientLight(0xffffff, 0.35));
  const key = new THREE.DirectionalLight(0xffffff, 1.4);
  key.position.set(-4, 6, 8);
  scene.add(key);

  // Three coloured lights circle the orb, so its facets keep changing colour.
  const orbitLights = [COLORS.sports, COLORS.academic, COLORS.esports].map((c) => {
    const light = new THREE.PointLight(c, 40, 20, 1.6);
    scene.add(light);
    return light;
  });

  const rig = new THREE.Group();
  scene.add(rig);

  // The orb is a geodesic sphere. A few faces are painted in the category
  // colours; the rest are dark steel.
  let geometry = new THREE.IcosahedronGeometry(2.3, 3);
  if (geometry.index) geometry = geometry.toNonIndexed();

  const position = geometry.attributes.position;
  const base = position.array.slice();
  const faceCount = position.count / 3;
  const push = new Float32Array(position.count * 3); // per-vertex: face normal * random strength
  const colors = new Float32Array(position.count * 3);
  const paint = [COLORS.sports, COLORS.academic, COLORS.esports, COLORS.volt].map((c) => new THREE.Color(c));
  const steel = new THREE.Color(0x1d1d24);

  const a = new THREE.Vector3(), b = new THREE.Vector3(), c = new THREE.Vector3();
  const normal = new THREE.Vector3();
  for (let f = 0; f < faceCount; f++) {
    a.fromBufferAttribute(position, f * 3);
    b.fromBufferAttribute(position, f * 3 + 1);
    c.fromBufferAttribute(position, f * 3 + 2);
    normal.copy(a).add(b).add(c).normalize();
    const strength = 0.4 + Math.random() * 1.6;
    const color = Math.random() < 0.16 ? paint[f % paint.length] : steel;
    for (let v = 0; v < 3; v++) {
      const i = (f * 3 + v) * 3;
      push[i] = normal.x * strength;
      push[i + 1] = normal.y * strength;
      push[i + 2] = normal.z * strength;
      colors[i] = color.r;
      colors[i + 1] = color.g;
      colors[i + 2] = color.b;
    }
  }
  geometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));

  const orb = new THREE.Mesh(
    geometry,
    new THREE.MeshStandardMaterial({ vertexColors: true, flatShading: true, roughness: 0.38, metalness: 0.35, side: THREE.DoubleSide }),
  );
  rig.add(orb);

  const cage = new THREE.LineSegments(
    new THREE.EdgesGeometry(new THREE.IcosahedronGeometry(2.85, 1)),
    new THREE.LineBasicMaterial({ color: COLORS.volt, transparent: true, opacity: 0.6 }),
  );
  rig.add(cage);

  // Three tilted orbits, one per category, each with a little satellite.
  const satellites = [
    { color: COLORS.sports, geo: new THREE.IcosahedronGeometry(0.3, 0), radius: 3.6, tilt: [1.2, 0.2], speed: 0.8 },
    { color: COLORS.academic, geo: new THREE.OctahedronGeometry(0.34, 0), radius: 4.2, tilt: [0.5, -0.9], speed: -0.6 },
    { color: COLORS.esports, geo: new THREE.TorusGeometry(0.22, 0.09, 6, 12), radius: 4.8, tilt: [-0.7, 0.6], speed: 0.45 },
  ].map((s, i) => {
    const orbit = new THREE.Group();
    orbit.rotation.set(s.tilt[0], s.tilt[1], 0);
    const ring = new THREE.Mesh(
      new THREE.TorusGeometry(s.radius, 0.012, 4, 160),
      new THREE.MeshBasicMaterial({ color: s.color, transparent: true, opacity: 0.45 }),
    );
    const body = new THREE.Mesh(
      s.geo,
      new THREE.MeshStandardMaterial({ color: s.color, emissive: s.color, emissiveIntensity: 0.4, flatShading: true }),
    );
    orbit.add(ring, body);
    rig.add(orbit);
    return { ...s, body, angle: i * 2.1 };
  });

  // A square starfield (PointsMaterial without a texture draws squares).
  const starCount = 700;
  const stars = new Float32Array(starCount * 3);
  for (let i = 0; i < starCount; i++) {
    stars[i * 3] = (Math.random() - 0.5) * 40;
    stars[i * 3 + 1] = (Math.random() - 0.5) * 26;
    stars[i * 3 + 2] = -Math.random() * 20 - 2;
  }
  const starGeometry = new THREE.BufferGeometry();
  starGeometry.setAttribute("position", new THREE.BufferAttribute(stars, 3));
  const starField = new THREE.Points(starGeometry, new THREE.PointsMaterial({ color: 0xf4f4ef, size: 0.06 }));
  scene.add(starField);

  // ----- layout -----
  const resize = () => {
    const { width, height } = host.getBoundingClientRect();
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
    renderer.setSize(width, height, false);
    // Beside the headline on wide screens, behind it on phones.
    const wide = width > 860;
    rig.position.set(wide ? 3.1 : 0, wide ? 0 : 0.6, 0);
    rig.scale.setScalar(wide ? 1 : 0.72);
  };
  new ResizeObserver(resize).observe(host);
  resize();

  // ----- interaction: drag to spin, click to smash -----
  const spin = new THREE.Vector2(0.15, 0.35); // angular velocity
  let drag = null;
  let explode = 0; // how far the faces have flown out
  let explodeVelocity = 0;

  host.addEventListener("pointerdown", (e) => {
    drag = { x: e.clientX, y: e.clientY, startX: e.clientX, startY: e.clientY, at: performance.now() };
    host.setPointerCapture(e.pointerId);
  });
  host.addEventListener("pointermove", (e) => {
    if (!drag) return;
    spin.y = (e.clientX - drag.x) * 0.012 * 60;
    spin.x = (e.clientY - drag.y) * 0.012 * 60;
    rig.rotation.y += (e.clientX - drag.x) * 0.012;
    rig.rotation.x += (e.clientY - drag.y) * 0.012;
    drag.x = e.clientX;
    drag.y = e.clientY;
  });
  host.addEventListener("pointerup", (e) => {
    if (!drag) return;
    const moved = Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY);
    if (moved < 6 && performance.now() - drag.at < 400) smash();
    drag = null;
  });

  function smash() {
    explodeVelocity += 9;
    spin.multiplyScalar(-1).addScalar(2.5);
    background?.shockwave(1.5);
    document.dispatchEvent(new CustomEvent("arena:smash"));
  }
  document.addEventListener("arena:live", () => (explodeVelocity += 3));

  // Pause the hero while it's scrolled out of view.
  const { start, stop } = loop(tick);
  new IntersectionObserver(([entry]) => (entry.isIntersecting ? start() : stop())).observe(host);

  function tick(dt, t) {
    // A damped spring pulls the shattered faces back together.
    explodeVelocity += (-38 * explode - 5.5 * explodeVelocity) * dt;
    explode += explodeVelocity * dt;
    const amount = explode + Math.sin(t * 1.3) * 0.04;

    const p = position.array;
    for (let i = 0; i < p.length; i++) p[i] = base[i] + push[i] * amount;
    position.needsUpdate = true;

    if (!drag) {
      spin.lerp(new THREE.Vector2(0.12, 0.3), 0.02);
      rig.rotation.x += spin.x * dt * 0.4;
      rig.rotation.y += spin.y * dt * 0.4;
    }
    cage.rotation.y -= dt * 0.25;
    cage.rotation.z += dt * 0.1;

    for (const s of satellites) {
      s.angle += s.speed * dt;
      s.body.position.set(Math.cos(s.angle) * s.radius, Math.sin(s.angle) * s.radius, 0);
      s.body.rotation.x += dt * 2;
      s.body.rotation.y += dt * 1.5;
    }

    orbitLights.forEach((light, i) => {
      const angle = t * 0.7 + (i * Math.PI * 2) / 3;
      light.position.set(rig.position.x + Math.cos(angle) * 5, Math.sin(angle * 1.3) * 3, Math.sin(angle) * 5);
    });

    starField.rotation.y = pointer.x * 0.08;
    starField.rotation.x = pointer.y * 0.05;
    camera.position.x += (pointer.x * 0.8 - camera.position.x) * 0.05;
    camera.position.y += (-pointer.y * 0.5 - camera.position.y) * 0.05;
    camera.lookAt(rig.position.x * 0.5, 0, 0);

    renderer.render(scene, camera);
  }

  tick(0, 0);
}

// ---------- start ----------

const background = backgroundScene();
document.addEventListener("arena:live", () => background?.shockwave(0.6));
document.addEventListener("arena:party", () => background?.shockwave(2));

const heroHost = document.getElementById("hero-stage");
if (heroHost) heroScene(heroHost, background);
