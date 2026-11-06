#!/usr/bin/env node
// Rebuild docs/design from the local Doop generators.
//
//   node .agents/skills/doop-canvas/scripts/build.js            # regenerate pages
//   node .agents/skills/doop-canvas/scripts/build.js --shots    # …and screenshots
//
// Requires node and .doop/gen2.js (local, gitignored). Screenshots need a
// Chromium/Edge binary: set DOOP_BROWSER, or install msedge/chrome.

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const root = path.resolve(__dirname, "../../../..");
const doop = path.join(root, ".doop");
const design = path.join(root, "docs/design");
const shots = process.argv.includes("--shots");

if (!fs.existsSync(path.join(doop, "gen2.js"))) {
  console.error("missing .doop/gen2.js — the generator is local and gitignored");
  process.exit(1);
}

execFileSync(process.execPath, ["gen2.js"], { cwd: doop, stdio: "inherit" });

// page → source, output size, and any per-page override.
const settingsHeight = '<style>.settings-app{height:auto!important;min-height:100vh}body{overflow:auto!important}</style></head>';
const narrowWidth = '<style>html,body{width:420px}</style></head>';
const pages = [
  ["01-recorder.html", "recorder.html", 1280, 800],
  ["02-playback.html", "session.html", 1280, 800],
  ["03-playback-dark.html", "session-dark.html", 1280, 800],
  ["04-recording.html", "recording.html", 1280, 800],
  ["05-library.html", "library.html", 1280, 800],
  ["06-settings-speakers.html", "speakers.html", 920, 1050, settingsHeight],
  ["07-settings-appearance.html", "appearance.html", 920, 660],
  ["08-settings-transcription.html", "transcription.html", 920, 1050, settingsHeight],
  ["09-settings-provider.html", "provider.html", 920, 1050, settingsHeight],
  ["10-recorder-900.html", "recorder-mid.html", 900, 620],
  ["11-recorder-420.html", "recorder-small.html", 420, 800, narrowWidth],
  ["12-playback-900.html", "session-mid.html", 900, 620],
  ["13-playback-420.html", "session-small.html", 420, 800, narrowWidth],
];

fs.mkdirSync(path.join(design, "pages"), { recursive: true });
for (const [out, src, , , override] of pages) {
  let html = fs.readFileSync(path.join(doop, src), "utf8");
  if (override) html = html.replace("</head>", override);
  fs.writeFileSync(path.join(design, "pages", out), html);
}

// 14 is the playback page with the transcript search unfolded.
let playback = fs.readFileSync(path.join(design, "pages", "02-playback.html"), "utf8");
fs.writeFileSync(
  path.join(design, "pages", "14-search-open.html"),
  playback.split('class="ttools"').join('class="ttools searching"'),
);
console.log("pages rebuilt in docs/design/pages");

if (!shots) process.exit(0);

const browser =
  process.env.DOOP_BROWSER ||
  ["C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
   "C:/Program Files/Microsoft/Edge/Application/msedge.exe",
   "/usr/bin/google-chrome", "/usr/bin/chromium"].find((p) => fs.existsSync(p));
if (!browser) {
  console.error("no Chromium/Edge found; set DOOP_BROWSER to the executable");
  process.exit(1);
}

const render = (page, width, height, out) => {
  execFileSync(browser, [
    "--headless=old", "--disable-gpu", "--hide-scrollbars",
    "--virtual-time-budget=5000", `--window-size=${width},${height}`,
    `--screenshot=${path.join(design, "screens", out)}`,
    "file:///" + path.join(design, "pages", page).replace(/\\/g, "/"),
  ], { stdio: "ignore" });
};

fs.mkdirSync(path.join(design, "screens"), { recursive: true });
for (const [page, , width, height] of pages) {
  render(page, width, height, page.replace(/\.html$/, ".png"));
}
render("../example.html", 1000, 1160, "00-components.png");
console.log("screenshots rebuilt in docs/design/screens");
