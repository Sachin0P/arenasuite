// ArenaSuite browser script. Plain JavaScript, no libraries.
// The site works without it (every action is a normal HTML form); this just
// adds tabs, toasts, confirm prompts, the create-competition wizard and live
// updates.

document.documentElement.classList.add("js");

// ---------- toasts ----------

function toast(message, kind) {
  const box = document.getElementById("toasts");
  const el = document.createElement("div");
  el.className = "toast " + (kind || "");
  el.textContent = message;
  box.appendChild(el);
  setTimeout(() => el.remove(), 4000);
  document.dispatchEvent(new CustomEvent("arena:toast", { detail: { message, kind } }));
}

// The server's flash message (set after a form action) shows as a toast.
const flash = document.getElementById("flash");
if (flash) toast(flash.dataset.msg, flash.dataset.kind);

// ---------- confirm before destructive actions ----------
// <form data-confirm="Delete this?"> asks before submitting.

document.addEventListener("submit", (e) => {
  const msg = e.target.dataset.confirm;
  if (msg && !confirm(msg)) e.preventDefault();
});

// ---------- tabs ----------
// <nav class="tabs" data-tabs> holds links like <a href="#teams">; the
// matching panel is <section class="tab-panel" id="tab-teams">. The tab is
// kept in the URL hash, so a redirect back to "/page#teams" reopens it.

function showTab() {
  const nav = document.querySelector("[data-tabs]");
  if (!nav) return;
  const links = [...nav.querySelectorAll("a")];
  const wanted = location.hash.slice(1);
  const current = links.find((a) => a.hash.slice(1) === wanted) || links[0];
  for (const a of links) {
    const active = a === current;
    a.classList.toggle("active", active);
    const panel = document.getElementById("tab-" + a.hash.slice(1));
    if (panel) panel.classList.toggle("active", active);
  }
}
window.addEventListener("hashchange", showTab);
showTab();

// Forms inside a tab come back to the same tab after submitting.
for (const form of document.querySelectorAll(".tab-panel form[method=post]")) {
  if (form.querySelector("input[name=next]")) continue;
  const panel = form.closest(".tab-panel");
  const next = document.createElement("input");
  next.type = "hidden";
  next.name = "next";
  next.value = location.pathname + location.search + "#" + panel.id.replace("tab-", "");
  form.appendChild(next);
}

// ---------- show/hide helpers ----------
// <input type=checkbox data-toggle="#box"> shows #box only while checked.
// <button data-reveal="#box"> toggles #box.

for (const box of document.querySelectorAll("[data-toggle]")) {
  const target = document.querySelector(box.dataset.toggle);
  const sync = () => (target.hidden = !box.checked);
  box.addEventListener("change", sync);
  sync();
}
for (const btn of document.querySelectorAll("[data-reveal]")) {
  btn.addEventListener("click", () => {
    const target = document.querySelector(btn.dataset.reveal);
    target.hidden = !target.hidden;
  });
}

// ---------- create / edit competition wizard ----------

const wizard = document.querySelector("[data-wizard]");
if (wizard) {
  const steps = [...wizard.querySelectorAll(".wizard-step")];
  const labels = [...document.querySelectorAll(".steps span")];
  let step = 0;

  const show = () => {
    steps.forEach((s, i) => s.classList.toggle("active", i === step));
    labels.forEach((l, i) => l.classList.toggle("active", i === step));
    if (step === steps.length - 1) fillPreview();
  };
  wizard.addEventListener("click", (e) => {
    if (e.target.matches("[data-next]")) { step = Math.min(step + 1, steps.length - 1); show(); }
    if (e.target.matches("[data-back]")) { step = Math.max(step - 1, 0); show(); }
  });

  // Academic competitions use stages instead of a bracket format.
  const category = wizard.querySelector("[name=category]");
  const syncCategory = () => {
    const academic = category.value === "ACADEMIC";
    wizard.querySelector("#format-box").hidden = academic;
    wizard.querySelector("#stages-box").hidden = !academic;
  };
  category.addEventListener("change", syncCategory);
  syncCategory();

  // The last step shows a summary of what was entered.
  function fillPreview() {
    const value = (name) => {
      const el = wizard.querySelector(`[name=${name}]`);
      if (!el) return "";
      if (el.tagName === "SELECT") return el.options[el.selectedIndex]?.text || "";
      return el.value;
    };
    const rows = [
      ["Name", value("name")],
      ["Organization", value("organizationId")],
      ["Category", value("category")],
      ["Type", value("type")],
      ["Dates", value("startDate").replace("T", " ") + " → " + value("endDate").replace("T", " ")],
      ["Register by", value("registrationDeadline").replace("T", " ")],
      ["Mode / venue", value("mode") + (value("venue") ? " · " + value("venue") : "")],
      ["Fee", Number(value("registrationFee")) > 0 ? "₹" + value("registrationFee") : "Free"],
      category.value === "ACADEMIC" ? ["Stages", value("stages")] : ["Format", value("format")],
    ];
    const dl = wizard.querySelector("#preview");
    dl.innerHTML = "";
    for (const [label, text] of rows) {
      const dt = document.createElement("dt");
      dt.textContent = label;
      const dd = document.createElement("dd");
      dd.textContent = text || "—";
      dl.append(dt, dd);
    }
  }

  // If the server sent the form back with an error, start on step 1 anyway.
  show();
}

// ---------- live updates (Server-Sent Events) ----------
// One connection per tab. The server sends personal notifications, and, on
// a competition page, live score / standings / announcement events.

const competitionId = document.body.dataset.competition;
const signedIn = document.body.dataset.user === "1";

if ((signedIn || competitionId) && window.EventSource) {
  const events = new EventSource("/events" + (competitionId ? "?competition=" + competitionId : ""));

  // A new personal notification: bump the badge and show it.
  events.addEventListener("notification", (e) => {
    const n = JSON.parse(e.data);
    const badge = document.getElementById("unread");
    if (badge) {
      const count = Number(badge.dataset.count || 0) + 1;
      badge.dataset.count = count;
      badge.textContent = count > 9 ? "9+" : count;
      badge.hidden = false;
    }
    toast(n.message);
  });

  if (competitionId) {
    // Something changed on this competition: re-fetch the page and swap in
    // the fresh live sections (marked with data-live).
    let pending = null;
    const refresh = () => {
      document.dispatchEvent(new CustomEvent("arena:live")); // fx.js and motion.js react to this
      clearTimeout(pending); // several events at once -> one refresh
      pending = setTimeout(async () => {
        const res = await fetch(location.pathname, { headers: { Accept: "text/html" } });
        if (!res.ok) return;
        const doc = new DOMParser().parseFromString(await res.text(), "text/html");
        for (const el of document.querySelectorAll("[data-live]")) {
          const fresh = doc.getElementById(el.id);
          if (fresh) el.innerHTML = fresh.innerHTML;
        }
      }, 200);
    };
    for (const name of ["scoreUpdated", "matchStatusUpdated", "standingsUpdated", "registrationSubmitted", "registrationApproved", "registrationRejected", "teamJoinRequest"]) {
      events.addEventListener(name, refresh);
    }
    events.addEventListener("announcementCreated", (e) => {
      toast("New announcement: " + JSON.parse(e.data).title);
      refresh();
    });
  }
}
