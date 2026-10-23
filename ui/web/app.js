// Ottr frontend: hash-routed SPA on the raw window.mygo runtime.
const app = document.getElementById("app");
const call = (m, ...a) => mygo.call(m, ...a);

// WebView2 serves custom schemes under http://<scheme>.localhost.
const mediaURL = (p) =>
  (mygo.platform === "win32" ? "http://media.localhost/" : "media://localhost/") + p;

const fmtDuration = (ms) => {
  if (!ms) return "0:00";
  const s = Math.round(ms / 1000);
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
  const mm = h ? String(m).padStart(2, "0") : String(m);
  return (h ? h + ":" : "") + mm + ":" + String(r).padStart(2, "0");
};
const fmtDate = (iso) => new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
const esc = (s) => (s ? s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])) : "");

let search = "";

function route() {
  const m = location.hash.match(/^#\/session\/(\d+)$/);
  if (m) return renderSession(+m[1]);
  if (location.hash === "#/record") return renderRecord();
  renderLibrary();
}

function showError(err) {
  document.querySelector(".error-banner")?.remove();
  if (!err) return;
  const el = document.createElement("div");
  el.className = "error-banner";
  el.textContent = err.message || String(err);
  app.prepend(el);
}

async function renderLibrary() {
  let items = [];
  try {
    items = await call("Library.List", search);
  } catch (err) {
    app.innerHTML = "";
    showError(err);
    return;
  }
  const rows = items.map((m) => `
    <tr class="row" data-id="${m.id}">
      <td>${esc(m.name)}</td>
      <td><span class="badge">${esc(m.source)}</span></td>
      <td class="dim">${fmtDate(m.createdAt)}</td>
      <td class="dim">${fmtDuration(m.durationMs)}</td>
      <td><span class="badge status-${esc(m.transcriptStatus)}">${esc(m.transcriptStatus)}</span></td>
      <td><span class="badge status-${esc(m.summaryStatus)}">${esc(m.summaryStatus)}</span></td>
      <td><button class="danger del" data-id="${m.id}">Delete</button></td>
    </tr>`).join("");
  app.innerHTML = `
    <div class="topbar">
      <h1>Ottr</h1>
      <input type="search" id="search" placeholder="Search" value="${esc(search)}" />
      <div class="spacer"></div>
      <button id="record">Record</button>
      <button id="import" class="primary">Import</button>
    </div>
    <div class="content">
      ${items.length === 0 ? `
        <div class="empty">
          <p class="title">${search ? "No matches" : "No videos yet"}</p>
          <p>${search ? "Try a different search." : "Record your screen or import a media file to start."}</p>
        </div>` : `
        <table class="library">
          <thead><tr>
            <th>Name</th><th>Source</th><th>Created</th><th>Duration</th>
            <th>Transcript</th><th>Summary</th><th></th>
          </tr></thead>
          <tbody>${rows}</tbody>
        </table>`}
    </div>`;
  const si = document.getElementById("search");
  si.oninput = () => { search = si.value; renderLibrary(); };
  si.onkeydown = (e) => { if (e.key === "Escape") { si.value = ""; search = ""; renderLibrary(); } };
  document.getElementById("record").onclick = () => { location.hash = "#/record"; };
  document.getElementById("import").onclick = async () => {
    try {
      await call("Library.Import");
    } catch (err) {
      showError(err);
    }
  };
  app.querySelectorAll("tr.row").forEach((tr) => {
    tr.onclick = (e) => {
      if (e.target.closest("button")) return;
      location.hash = "#/session/" + tr.dataset.id;
    };
  });
  app.querySelectorAll("button.del").forEach((b) => {
    b.onclick = async () => {
      try {
        await call("Library.Remove", +b.dataset.id);
      } catch (err) {
        showError(err);
      }
    };
  });
}

let currentSessionID = 0;
let currentTab = 0; // 0: transcript, 1: translate, 2: summary
let subQuery = "";
let currentTargetLang = "en";
let transQuery = "";
let showTransSettings = false;

async function renderSession(id) {
  currentSessionID = id;
  let m;
  try {
    m = await call("Library.Get", id);
  } catch (err) {
    app.innerHTML = `<div class="topbar"><a class="back" href="#/">← Library</a></div>`;
    showError(err);
    return;
  }

  const playerHTML = m.hasVideo
    ? `<video id="media-player" controls src="${mediaURL(m.id)}"></video>`
    : `<audio id="media-player" controls src="${mediaURL(m.id)}"></audio>`;

  const tabs = ["Transcript", "Translate", "Summary"];

  app.innerHTML = `
    <div class="topbar">
      <a class="back" href="#/">← Library</a>
      <h1>${esc(m.name)}</h1>
      <div class="spacer"></div>
      <span class="dim">${esc(m.source)} · ${fmtDuration(m.durationMs)}</span>
    </div>
    <div class="session">
      <div class="left">
        ${playerHTML}
        <div class="dim">
          Created: ${fmtDate(m.createdAt)}
          ${m.width > 0 ? `· Resolution: ${m.width}x${m.height}` : ""}
        </div>
      </div>
      <div class="right">
        <div class="tabs">
          ${tabs.map((t, i) => `<button data-tab="${i}" class="${i === currentTab ? "active" : ""}">${t}</button>`).join("")}
        </div>
        <div class="tab-body" id="tab-body"></div>
      </div>
    </div>`;

  app.querySelectorAll(".tabs button").forEach((b) => {
    b.onclick = () => {
      currentTab = +b.dataset.tab;
      app.querySelectorAll(".tabs button").forEach((x) => x.classList.remove("active"));
      b.classList.add("active");
      renderTabBody(m);
    };
  });

  renderTabBody(m);
}

async function renderTabBody(m) {
  const body = document.getElementById("tab-body");
  if (!body) return;

  if (currentTab === 1) {
    return renderTranslateTab(m, body);
  }
  if (currentTab === 2) {
    return renderSummaryTab(m, body);
  }

  // Tab 0: Transcript
  if (m.transcriptStatus === "pending") {
    body.innerHTML = `
      <div class="empty">
        <div class="spinner"></div>
        <p class="title">Transcribing Audio…</p>
        <p>Running local sherpa-onnx streaming zipformer ASR.</p>
      </div>`;
    return;
  }

  if (m.transcriptStatus !== "done") {
    body.innerHTML = `
      <div class="empty">
        <p class="title">${m.transcriptStatus === "error" ? "Transcription Failed" : "No Transcript Yet"}</p>
        <p>${m.hasAudio ? "Run local speech recognition to generate timestamped subtitles." : "This media file has no audio stream."}</p>
        ${m.hasAudio ? `<button id="btn-transcribe" class="primary">Transcribe</button>` : ""}
      </div>`;
    const btn = document.getElementById("btn-transcribe");
    if (btn) {
      btn.onclick = async () => {
        btn.disabled = true;
        btn.textContent = "Starting…";
        try {
          await call("Transcription.Transcribe", m.id);
          m.transcriptStatus = "pending";
          renderTabBody(m);
        } catch (err) {
          btn.disabled = false;
          showError(err);
        }
      };
    }
    return;
  }

  // Done: load transcript segments
  let tr;
  try {
    tr = await call("Transcription.Get", m.id);
  } catch (err) {
    body.innerHTML = `<div class="empty"><p class="title">Failed to load transcript</p></div>`;
    showError(err);
    return;
  }

  const segments = tr.segments || [];
  body.innerHTML = `
    <div class="transcript-pane">
      <div class="sub-header">
        <input type="search" id="sub-search" placeholder="Search subtitles…" value="${esc(subQuery)}" />
        <div class="exp-group">
          <span class="dim">Export:</span>
          <button class="btn-sm" id="exp-srt">SRT</button>
          <button class="btn-sm" id="exp-vtt">VTT</button>
          <button class="btn-sm" id="exp-txt">TXT</button>
        </div>
      </div>
      <div class="sub-list" id="sub-list"></div>
    </div>`;

  const listEl = document.getElementById("sub-list");
  const renderList = (filter) => {
    const q = filter.trim().toLowerCase();
    const filtered = q ? segments.filter((s) => s.text.toLowerCase().includes(q)) : segments;
    if (filtered.length === 0) {
      listEl.innerHTML = `<div class="dim" style="text-align:center;padding:24px;">No matching subtitles</div>`;
      return;
    }
    listEl.innerHTML = filtered.map((s) => `
      <div class="sub-item" data-start="${s.startMs}" data-end="${s.endMs}">
        <span class="sub-time">${fmtDuration(s.startMs)}</span>
        <span class="sub-text">${esc(s.text)}</span>
      </div>`).join("");

    const player = document.getElementById("media-player");
    listEl.querySelectorAll(".sub-item").forEach((el) => {
      el.onclick = () => {
        if (!player) return;
        player.currentTime = +el.dataset.start / 1000;
        player.play();
      };
    });
  };

  renderList(subQuery);

  const searchEl = document.getElementById("sub-search");
  searchEl.oninput = () => {
    subQuery = searchEl.value;
    renderList(subQuery);
  };

  const doExport = async (fmt) => {
    try {
      await call("Transcription.Export", m.id, fmt);
    } catch (err) {
      showError(err);
    }
  };
  document.getElementById("exp-srt").onclick = () => doExport("srt");
  document.getElementById("exp-vtt").onclick = () => doExport("vtt");
  document.getElementById("exp-txt").onclick = () => doExport("txt");

  const player = document.getElementById("media-player");
  if (player) {
    player.ontimeupdate = () => {
      const curMs = player.currentTime * 1000;
      let activeEl = null;
      listEl.querySelectorAll(".sub-item").forEach((el) => {
        const start = +el.dataset.start;
        const end = +el.dataset.end;
        if (curMs >= start && curMs < end) {
          el.classList.add("active");
          activeEl = el;
        } else {
          el.classList.remove("active");
        }
      });
      if (activeEl && !listEl.matches(":hover")) {
        activeEl.scrollIntoView({ block: "nearest", behavior: "smooth" });
      }
    };
  }
}

async function renderTranslateTab(m, body) {
  if (m.transcriptStatus !== "done") {
    body.innerHTML = `
      <div class="empty">
        <p class="title">Transcript Required</p>
        <p>Please transcribe the video first before translating subtitles.</p>
        <button id="goto-transcript" class="primary">Go to Transcript</button>
      </div>`;
    document.getElementById("goto-transcript").onclick = () => {
      currentTab = 0;
      app.querySelectorAll(".tabs button").forEach((b, i) => b.classList.toggle("active", i === 0));
      renderTabBody(m);
    };
    return;
  }

  let tr;
  try {
    tr = await call("Translation.Get", m.id, currentTargetLang);
  } catch (err) {
    showError(err);
    return;
  }

  const langs = [
    { code: "en", name: "English" },
    { code: "zh", name: "Chinese (中文)" },
    { code: "ja", name: "Japanese (日本語)" },
  ];

  const langOptions = langs.map((l) =>
    `<option value="${l.code}" ${l.code === currentTargetLang ? "selected" : ""}>${l.name}</option>`
  ).join("");

  const settings = await call("Translation.GetSettings").catch(() => ({ provider: "mymemory" }));

  body.innerHTML = `
    <div class="transcript-pane">
      <div class="sub-header">
        <select id="sel-target-lang" class="lang-select">
          ${langOptions}
        </select>
        <input type="search" id="trans-search" placeholder="Search translation…" value="${esc(transQuery)}" />
        <div class="exp-group">
          ${(tr && tr.segments && tr.segments.length > 0) ? `
            <span class="dim">Export:</span>
            <button class="btn-sm" id="exp-trans-srt" title="Translated subtitles only">SRT</button>
            <button class="btn-sm" id="exp-trans-bi" title="Bilingual original + translated">Bilingual</button>
            <button class="btn-sm" id="exp-trans-vtt">VTT</button>
            <button class="btn-sm" id="exp-trans-txt">TXT</button>
          ` : ""}
          <button class="btn-sm" id="btn-trans-settings" title="Translation Settings">⚙ Config</button>
        </div>
      </div>
      ${showTransSettings ? renderSettingsBox(settings) : ""}
      <div class="sub-list" id="trans-list">
        ${(!tr || !tr.segments || tr.segments.length === 0) ? `
          <div class="empty" style="margin-top:5vh;">
            <p class="title">Not Translated Yet</p>
            <p>Translate subtitles into ${langs.find((l) => l.code === currentTargetLang)?.name || currentTargetLang}.</p>
            <button id="btn-do-translate" class="primary">Translate</button>
          </div>` : ""}
      </div>
    </div>`;

  document.getElementById("sel-target-lang").onchange = (e) => {
    currentTargetLang = e.target.value;
    renderTranslateTab(m, body);
  };

  document.getElementById("btn-trans-settings").onclick = () => {
    showTransSettings = !showTransSettings;
    renderTranslateTab(m, body);
  };

  if (showTransSettings) {
    bindSettingsBox(m, body);
  }

  const btnTrans = document.getElementById("btn-do-translate");
  if (btnTrans) {
    btnTrans.onclick = async () => {
      btnTrans.disabled = true;
      btnTrans.textContent = "Translating online…";
      try {
        await call("Translation.Translate", m.id, currentTargetLang);
        renderTranslateTab(m, body);
      } catch (err) {
        btnTrans.disabled = false;
        btnTrans.textContent = "Translate";
        showError(err);
      }
    };
  }

  if (tr && tr.segments && tr.segments.length > 0) {
    const listEl = document.getElementById("trans-list");
    const renderList = (filter) => {
      const q = filter.trim().toLowerCase();
      const filtered = q
        ? tr.segments.filter((s) => s.translatedText.toLowerCase().includes(q) || s.originalText.toLowerCase().includes(q))
        : tr.segments;

      if (filtered.length === 0) {
        listEl.innerHTML = `<div class="dim" style="text-align:center;padding:24px;">No matching subtitles</div>`;
        return;
      }

      listEl.innerHTML = filtered.map((s) => `
        <div class="sub-item trans-item" data-start="${s.startMs}" data-end="${s.endMs}">
          <span class="sub-time">${fmtDuration(s.startMs)}</span>
          <div class="sub-content">
            <div class="trans-text">${esc(s.translatedText)}</div>
            <div class="orig-text dim">${esc(s.originalText)}</div>
          </div>
        </div>`).join("");

      const player = document.getElementById("media-player");
      listEl.querySelectorAll(".trans-item").forEach((el) => {
        el.onclick = () => {
          if (!player) return;
          player.currentTime = +el.dataset.start / 1000;
          player.play();
        };
      });
    };

    renderList(transQuery);

    const searchEl = document.getElementById("trans-search");
    if (searchEl) {
      searchEl.oninput = () => {
        transQuery = searchEl.value;
        renderList(transQuery);
      };
    }

    const doExport = async (fmt) => {
      try {
        await call("Translation.Export", m.id, currentTargetLang, fmt);
      } catch (err) {
        showError(err);
      }
    };
    document.getElementById("exp-trans-srt")?.addEventListener("click", () => doExport("srt"));
    document.getElementById("exp-trans-bi")?.addEventListener("click", () => doExport("bilingual_srt"));
    document.getElementById("exp-trans-vtt")?.addEventListener("click", () => doExport("vtt"));
    document.getElementById("exp-trans-txt")?.addEventListener("click", () => doExport("txt"));

    const player = document.getElementById("media-player");
    if (player) {
      player.ontimeupdate = () => {
        const curMs = player.currentTime * 1000;
        let activeEl = null;
        listEl.querySelectorAll(".trans-item").forEach((el) => {
          const start = +el.dataset.start;
          const end = +el.dataset.end;
          if (curMs >= start && curMs < end) {
            el.classList.add("active");
            activeEl = el;
          } else {
            el.classList.remove("active");
          }
        });
        if (activeEl && !listEl.matches(":hover")) {
          activeEl.scrollIntoView({ block: "nearest", behavior: "smooth" });
        }
      };
    }
  }
}

let showSumSettings = false;

async function renderSummaryTab(m, body) {
  if (m.transcriptStatus !== "done") {
    body.innerHTML = `
      <div class="empty">
        <p class="title">Transcript Required</p>
        <p>Please transcribe the video first before generating an AI summary.</p>
        <button id="goto-transcript" class="primary">Go to Transcript</button>
      </div>`;
    document.getElementById("goto-transcript").onclick = () => {
      currentTab = 0;
      app.querySelectorAll(".tabs button").forEach((b, i) => b.classList.toggle("active", i === 0));
      renderTabBody(m);
    };
    return;
  }

  let sum;
  try {
    sum = await call("Summary.Get", m.id);
  } catch (err) {
    showError(err);
    return;
  }

  const has = sum && sum.contentMarkdown;

  if (m.summaryStatus === "pending") {
    body.innerHTML = `
      <div class="empty">
        <div class="spinner"></div>
        <p class="title">Generating Summary…</p>
        <p>Analyzing transcript with the remote AI model.</p>
      </div>`;
    return;
  }

  const header = `
    <div class="sub-header">
      <span class="dim" style="flex:1;font-size:12px;">${has ? `Model: ${esc(sum.model)}` : "No summary yet"}</span>
      ${has ? `
        <div class="exp-group">
          <button class="btn-sm" id="btn-copy-sum">Copy</button>
          <button class="btn-sm" id="btn-export-sum">Export</button>
        </div>` : ""}
      <button class="btn-sm" id="btn-sum-settings" title="AI Model Settings">⚙ Config</button>
    </div>`;

  if (!has) {
    body.innerHTML = `
      <div class="transcript-pane">
        ${header}
        ${showSumSettings ? renderSumSettingsBox() : ""}
        <div class="empty" style="margin-top:5vh;">
          <p class="title">${m.summaryStatus === "error" ? "Summary Failed" : "No Summary Yet"}</p>
          <p>Generate an overview, key points, action items, chapters and highlights from the transcript.</p>
          <button id="btn-do-summary" class="primary">Generate</button>
        </div>
      </div>`;
  } else {
    const chapters = (sum.chapters || []).map((c) => `
      <div class="chapter-item" data-start="${c.startMs}">
        <span class="sub-time">${fmtDuration(c.startMs)}</span>
        <div class="sub-content">
          <div class="trans-text">${esc(c.title)}</div>
          <div class="orig-text dim">${esc(c.summary)}</div>
        </div>
      </div>`).join("");

    const highlights = (sum.highlights || []).map((h) => `
      <div class="chapter-item" data-start="${h.startMs}">
        <span class="sub-time">${fmtDuration(h.startMs)}</span>
        <div class="sub-content"><div class="orig-text">${esc(h.note)}</div></div>
      </div>`).join("");

    body.innerHTML = `
      <div class="transcript-pane">
        ${header}
        ${showSumSettings ? renderSumSettingsBox() : ""}
        <div class="sub-list">
          <section class="sum-section">
            <h3>Overview</h3>
            <p>${esc(sum.overview)}</p>
          </section>
          ${(sum.keyPoints || []).length ? `
          <section class="sum-section">
            <h3>Key Points</h3>
            <ul>${sum.keyPoints.map((k) => `<li>${esc(k)}</li>`).join("")}</ul>
          </section>` : ""}
          ${(sum.actionItems || []).length ? `
          <section class="sum-section">
            <h3>Action Items</h3>
            <ul class="todo">${sum.actionItems.map((a) => `<li>${esc(a)}</li>`).join("")}</ul>
          </section>` : ""}
          ${chapters ? `<section class="sum-section"><h3>Chapters</h3>${chapters}</section>` : ""}
          ${highlights ? `<section class="sum-section"><h3>Highlights</h3>${highlights}</section>` : ""}
        </div>
      </div>`;

    body.querySelectorAll(".chapter-item").forEach((el) => {
      el.onclick = () => {
        const player = document.getElementById("media-player");
        if (!player) return;
        player.currentTime = +el.dataset.start / 1000;
        player.play();
      };
    });

    document.getElementById("btn-copy-sum").onclick = async () => {
      try {
        await navigator.clipboard.writeText(sum.contentMarkdown);
        const btn = document.getElementById("btn-copy-sum");
        btn.textContent = "Copied";
        setTimeout(() => { btn.textContent = "Copy"; }, 1500);
      } catch (err) {
        showError(new Error("Clipboard unavailable: " + err.message));
      }
    };
    document.getElementById("btn-export-sum").onclick = async () => {
      try {
        await call("Summary.Export", m.id);
      } catch (err) {
        showError(err);
      }
    };
  }

  document.getElementById("btn-sum-settings").onclick = () => {
    showSumSettings = !showSumSettings;
    renderSummaryTab(m, body);
  };
  if (showSumSettings) bindSumSettingsBox(m, body);

  const btnGen = document.getElementById("btn-do-summary");
  if (btnGen) {
    btnGen.onclick = async () => {
      btnGen.disabled = true;
      btnGen.textContent = "Generating…";
      try {
        await call("Summary.Generate", m.id);
        m.summaryStatus = "done";
        renderSummaryTab(m, body);
      } catch (err) {
        btnGen.disabled = false;
        btnGen.textContent = "Generate";
        showError(err);
      }
    };
  }
}

async function renderSumSettingsBox() {
  const s = await call("Summary.GetSettings").catch(() => ({}));
  return `
    <div class="settings-drawer">
      <div style="font-weight:600;margin-bottom:8px;">Remote AI Model</div>
      <label style="font-size:12px;" class="dim">API Base URL</label>
      <input type="text" id="sum-base-url" placeholder="https://api.deepseek.com/v1" value="${esc(s.baseUrl || "")}" />
      <label style="font-size:12px;" class="dim">API Key</label>
      <input type="password" id="sum-api-key" placeholder="sk-..." value="${esc(s.apiKey || "")}" />
      <label style="font-size:12px;" class="dim">Model</label>
      <input type="text" id="sum-model" placeholder="deepseek-chat / gpt-4o-mini" value="${esc(s.model || "")}" />
      <div><button class="primary btn-sm" id="btn-save-sum-settings">Save Settings</button></div>
    </div>`;
}

function bindSumSettingsBox(m, body) {
  document.getElementById("btn-save-sum-settings").onclick = async () => {
    const cfg = {
      baseUrl: document.getElementById("sum-base-url").value.trim(),
      apiKey: document.getElementById("sum-api-key").value.trim(),
      model: document.getElementById("sum-model").value.trim(),
    };
    try {
      await call("Summary.SaveSettings", cfg);
      showSumSettings = false;
      renderSummaryTab(m, body);
    } catch (err) {
      showError(err);
    }
  };
}

function renderSettingsBox(s) {
  return `
    <div class="settings-drawer">
      <div style="font-weight:600;margin-bottom:8px;">Online Translation Provider</div>
      <div class="check" style="margin-bottom:6px;">
        <label><input type="radio" name="trans-prov" value="mymemory" ${s.provider !== "custom" ? "checked" : ""}> Free Online API (MyMemory, zero key needed)</label>
      </div>
      <div class="check" style="margin-bottom:12px;">
        <label><input type="radio" name="trans-prov" value="custom" ${s.provider === "custom" ? "checked" : ""}> Custom AI Model (OpenAI / DeepSeek / Ollama)</label>
      </div>
      <div id="custom-cfg" style="${s.provider === "custom" ? "" : "display:none;"}display:flex;flex-direction:column;gap:8px;margin-bottom:12px;">
        <label style="font-size:12px;" class="dim">API Base URL</label>
        <input type="text" id="cfg-base-url" placeholder="https://api.deepseek.com/v1" value="${esc(s.baseUrl)}" />
        <label style="font-size:12px;" class="dim">API Key</label>
        <input type="password" id="cfg-api-key" placeholder="sk-..." value="${esc(s.apiKey)}" />
        <label style="font-size:12px;" class="dim">Model</label>
        <input type="text" id="cfg-model" placeholder="deepseek-chat or gpt-4o-mini" value="${esc(s.model)}" />
      </div>
      <div><button class="primary btn-sm" id="btn-save-settings">Save Settings</button></div>
    </div>`;
}

function bindSettingsBox(m, body) {
  const radios = document.querySelectorAll('input[name="trans-prov"]');
  const customBox = document.getElementById("custom-cfg");
  radios.forEach((r) => {
    r.onchange = () => {
      customBox.style.display = r.value === "custom" ? "flex" : "none";
    };
  });
  document.getElementById("btn-save-settings").onclick = async () => {
    const isCustom = document.querySelector('input[name="trans-prov"]:checked')?.value === "custom";
    const cfg = {
      provider: isCustom ? "custom" : "mymemory",
      baseUrl: document.getElementById("cfg-base-url")?.value.trim() || "",
      apiKey: document.getElementById("cfg-api-key")?.value.trim() || "",
      model: document.getElementById("cfg-model")?.value.trim() || "",
    };
    try {
      await call("Translation.SaveSettings", cfg);
      showTransSettings = false;
      renderTranslateTab(m, body);
    } catch (err) {
      showError(err);
    }
  };
}

let recTimer = null;
const stopRecTimer = () => { if (recTimer) { clearInterval(recTimer); recTimer = null; } };

async function renderRecord() {
  stopRecTimer();
  let sources, state;
  try {
    [sources, state] = await Promise.all([call("Recorder.Sources"), call("Recorder.State")]);
  } catch (err) {
    app.innerHTML = `<div class="topbar"><a class="back" href="#/">← Library</a><h1>Record</h1></div>`;
    showError(err);
    return;
  }
  if (state.recording) return renderRecording(state);

  const group = (kind, label) => {
    const items = sources.filter((s) => s.kind === kind);
    if (!items.length) return "";
    return `<optgroup label="${label}">${items.map((s) =>
      `<option value="${esc(s.id)}">${esc(s.name)}</option>`).join("")}</optgroup>`;
  };
  app.innerHTML = `
    <div class="topbar"><a class="back" href="#/">← Library</a><h1>Record</h1></div>
    <div class="content">
      <div class="rec-form">
        <label>Source</label>
        <select id="src">
          ${group("screen", "Screen")}
          ${group("display", "Displays")}
          ${group("window", "Windows")}
        </select>
        <label class="check"><input type="checkbox" id="sys" checked /> System audio</label>
        <label class="check"><input type="checkbox" id="mic" /> Microphone</label>
        <div><button id="start" class="primary">Start</button></div>
      </div>
    </div>`;
  document.getElementById("start").onclick = async () => {
    const cfg = {
      sourceId: document.getElementById("src").value,
      systemAudio: document.getElementById("sys").checked,
      mic: document.getElementById("mic").checked,
    };
    const btn = document.getElementById("start");
    btn.disabled = true;
    try {
      await call("Recorder.Start", cfg);
    } catch (err) {
      btn.disabled = false;
      showError(err);
    }
  };
}

function renderRecording(state) {
  app.innerHTML = `
    <div class="topbar"><a class="back" href="#/">← Library</a><h1>Recording</h1>
      <div class="spacer"></div>
      <span class="rec-dot"></span><span id="elapsed">0:00</span>
    </div>
    <div class="content">
      <img id="preview" class="preview" src="${mediaURL("preview")}" alt="Live preview" />
      <div id="live-subtitle" class="live-subtitle dim">Listening for audio…</div>
      <div style="margin-top:14px;"><button id="stop" class="primary">Stop</button></div>
    </div>`;
  const startedAt = new Date(state.startedAt);
  const tick = () => {
    document.getElementById("elapsed").textContent = fmtDuration(Date.now() - startedAt);
  };
  tick();
  recTimer = setInterval(tick, 500);
  document.getElementById("stop").onclick = async () => {
    const btn = document.getElementById("stop");
    btn.disabled = true;
    btn.textContent = "Finalizing…";
    try {
      const m = await call("Recorder.Stop");
      stopRecTimer();
      location.hash = "#/session/" + m.id;
    } catch (err) {
      stopRecTimer();
      location.hash = "#/";
      showError(err);
    }
  };
}

mygo.on("recording:state", (st) => {
  if (!location.hash.startsWith("#/record")) return;
  if (st.error) {
    stopRecTimer();
    renderRecord().then(() => showError(new Error(st.error)));
  } else if (st.recording) {
    renderRecording(st);
  }
});

mygo.on("transcript:live", (sub) => {
  const el = document.getElementById("live-subtitle");
  if (el && sub && sub.text) {
    el.textContent = sub.text;
    el.classList.remove("dim");
  }
});

mygo.on("transcript:ready", (mediaID) => {
  if (location.hash === `#/session/${mediaID}`) {
    renderSession(mediaID);
  }
});

mygo.on("translation:ready", (tr) => {
  if (location.hash === `#/session/${tr.mediaId}` && currentTab === 1) {
    call("Library.Get", tr.mediaId).then((m) => renderTranslateTab(m, document.getElementById("tab-body")));
  }
});

mygo.on("summary:ready", (s) => {
  if (location.hash === `#/session/${s.mediaId}` && currentTab === 2) {
    call("Library.Get", s.mediaId).then((m) => renderSummaryTab(m, document.getElementById("tab-body")));
  }
});

mygo.on("library:changed", () => {
  if (location.hash === "" || location.hash === "#/") {
    renderLibrary();
  } else if (location.hash.startsWith("#/session/")) {
    const m = location.hash.match(/^#\/session\/(\d+)$/);
    if (m && +m[1] === currentSessionID) {
      call("Library.Get", currentSessionID).then((media) => {
        renderTabBody(media);
      }).catch(() => {});
    }
  }
});

window.addEventListener("hashchange", route);
route();
