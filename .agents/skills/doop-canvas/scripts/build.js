#!/usr/bin/env node
// Rebuild docs/design from the Doop canvas.
//
//   node .agents/skills/doop-canvas/scripts/build.js            # export pages
//   node .agents/skills/doop-canvas/scripts/build.js --shots    # …and screenshots
//
// The canvas is the source of truth, so the pages are the frames' own HTML.
// They are read through a Doop MCP client named by DOOP_CLIENT: a command that
// takes `call <tool> -` with JSON arguments on stdin and prints the tool's
// result (get_frame prints the frame as JSON, get_frame_screenshot prints
// "IMAGE SAVED: <path>"). Without DOOP_CLIENT the frames are read from
// .doop/frames/<frame id>.html, exported earlier (gitignored, local only).
//
// Screenshots are rendered by a local Chromium/Edge (DOOP_BROWSER, or msedge /
// chrome / chromium found on the machine). Where there is none, Doop renders
// them itself with get_frame_screenshot, which is the canvas's own picture.

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const root = path.resolve(__dirname, "../../../..");
const doop = path.join(root, ".doop");
const design = path.join(root, "docs/design");
const shots = process.argv.includes("--shots");
const agent = "Hark";

// page → frame id and size. Order follows the board: main window, playback
// family, library states, settings, dialogs, responsive variants.
const pages = [
  ["01-recorder.html", "8Gr6-Ve2G5", 1280, 800],
  ["02-recorder-dark.html", "VTN9T8_j04", 1280, 800],
  ["03-recording.html", "y-8zMeAZZu", 1280, 800],
  ["04-recording-dark.html", "Fgx6l8dRMT", 1280, 800],
  ["05-transcribing.html", "7IAHHDxZVe", 1280, 800],
  ["06-playback.html", "0oS1qcVm0G", 1280, 800],
  ["07-playback-dark.html", "eDrrkXjejG", 1280, 800],
  ["08-summary.html", "BHaEGh64p_", 1280, 800],
  ["09-search.html", "aEhVbltVSK", 1280, 800],
  ["10-library-grid.html", "Alz_JKGcah", 1280, 800],
  ["11-library-empty.html", "qzEP255Sdt", 1280, 800],
  ["12-settings-appearance.html", "K5zn4aFdP4", 920, 660],
  ["13-settings-transcription.html", "Ld93QswHO6", 920, 660],
  ["14-settings-provider.html", "oLhMUylrdm", 920, 660],
  ["15-settings-speakers.html", "oM7uRXDq4A", 920, 660],
  ["16-dialog-delete.html", "AwxltPO8hH", 440, 252],
  ["17-dialog-export.html", "iovV_bKivj", 460, 452],
  ["18-dialog-failed.html", "v364cb5VvV", 460, 256],
  ["19-recorder-900.html", "-tyVG4qgpP", 900, 620],
  ["20-recorder-420.html", "LY2G4tQbU7", 420, 800],
  ["21-playback-900.html", "_DbSE_3soq", 900, 620],
  ["22-playback-420.html", "yihV5ej8VG", 420, 800],
];

const client = process.env.DOOP_CLIENT ? process.env.DOOP_CLIENT.split(" ").filter(Boolean) : null;

// call runs one Doop tool through the client and returns what it printed.
const call = (tool, args) => {
  const [cmd, ...rest] = client;
  return execFileSync(cmd, [...rest, "call", tool, "-"], {
    input: JSON.stringify({ ...args, agent_name: agent }),
    encoding: "utf8",
    maxBuffer: 64 << 20,
  });
};

// frameHTML is a frame's document: from Doop when there is a client, else from
// the local export.
const frameHTML = (id) => {
  const local = path.join(doop, "frames", id + ".html");
  if (!client) {
    if (!fs.existsSync(local)) {
      console.error(`missing ${local}: set DOOP_CLIENT, or export the frame first`);
      process.exit(1);
    }
    return fs.readFileSync(local, "utf8");
  }
  const out = call("get_frame", { frame_id: id });
  const frame = JSON.parse(out.slice(out.indexOf("{")));
  if (!frame.html) {
    console.error(`frame ${id} has no html: ${out.slice(0, 200)}`);
    process.exit(1);
  }
  fs.mkdirSync(path.dirname(local), { recursive: true });
  fs.writeFileSync(local, frame.html);
  return frame.html;
};

fs.mkdirSync(path.join(design, "pages"), { recursive: true });
const keep = new Set(pages.map(([out]) => out));
for (const old of fs.readdirSync(path.join(design, "pages"))) {
  if (!keep.has(old)) fs.rmSync(path.join(design, "pages", old));
}
for (const [out, id] of pages) {
  fs.writeFileSync(path.join(design, "pages", out), frameHTML(id));
}
console.log(`${pages.length} pages exported to docs/design/pages`);

if (!shots) process.exit(0);

const browser =
  process.env.DOOP_BROWSER ||
  ["C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
   "C:/Program Files/Microsoft/Edge/Application/msedge.exe",
   "/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"].find((p) => fs.existsSync(p));
if (!browser && !client) {
  console.error("no Chromium/Edge and no DOOP_CLIENT: set one to render screenshots");
  process.exit(1);
}

const render = (page, id, width, height, out) => {
  const target = path.join(design, "screens", out);
  if (browser) {
    // The 420px pages pin their width: Chromium will not lay out below ~500px.
    let file = path.join(design, "pages", page);
    if (width < 500) {
      file = path.join(doop, "narrow-" + page);
      fs.mkdirSync(doop, { recursive: true });
      fs.writeFileSync(file, fs.readFileSync(path.join(design, "pages", page), "utf8")
        .replace("</head>", `<style>html,body{width:${width}px}</style></head>`));
    }
    execFileSync(browser, [
      "--headless=old", "--disable-gpu", "--hide-scrollbars",
      "--virtual-time-budget=5000", `--window-size=${width},${height}`,
      `--screenshot=${target}`, "file:///" + file.replace(/\\/g, "/"),
    ], { stdio: "ignore" });
    return;
  }
  const out2 = call("get_frame_screenshot", { frame_id: id, scale: 1 });
  const m = out2.match(/IMAGE SAVED: (\S+)/);
  if (!m) {
    console.error(`no screenshot for ${id}: ${out2.slice(0, 200)}`);
    process.exit(1);
  }
  fs.copyFileSync(m[1], target);
};

fs.mkdirSync(path.join(design, "screens"), { recursive: true });
const keepShots = new Set(pages.map(([page]) => page.replace(/\.html$/, ".png")));
for (const old of fs.readdirSync(path.join(design, "screens"))) {
  if (!keepShots.has(old)) fs.rmSync(path.join(design, "screens", old));
}
for (const [page, id, width, height] of pages) {
  render(page, id, width, height, page.replace(/\.html$/, ".png"));
}
console.log(`screenshots rebuilt in docs/design/screens (${browser ? "local browser" : "Doop renderer"})`);
