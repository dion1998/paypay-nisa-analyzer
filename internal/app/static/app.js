const yen = new Intl.NumberFormat("ja-JP", { style: "currency", currency: "JPY", maximumFractionDigits: 0 });
const date = new Intl.DateTimeFormat("zh-Hant", { year: "numeric", month: "short", day: "numeric" });
const query = document.querySelector("#fund-query");
const fundID = document.querySelector("#fund-id");
const fundList = document.querySelector("#fund-list");
let selectedFund = null;
let searchFunds = [];
let activeIndex = -1;
let searchTimer;

function escapeHTML(value) { const element = document.createElement("div"); element.textContent = value ?? ""; return element.innerHTML; }
function nisaLabels(fund) { const labels = []; if (fund.nisaTsumitate) labels.push("つみたて投資枠"); if (fund.nisaGrowth) labels.push("成長投資枠"); return labels.length ? labels : ["NISA 對象未確認"]; }
function setComboOpen(open) { query.setAttribute("aria-expanded", String(open)); }
function clearSuggestions() { fundList.innerHTML = ""; searchFunds = []; activeIndex = -1; setComboOpen(false); }
function showFund(fund) {
  if (!fund) return;
  selectedFund = fund; fundID.value = fund.id; query.value = fund.name; clearSuggestions();
  document.querySelector("#fund-meta").innerHTML = `<p class="fund-name">${escapeHTML(fund.name)}</p><p class="muted">${escapeHTML(fund.manager || "發行商資料待同步")}</p>${nisaLabels(fund).map(label => `<span class="tag">${label}</span>`).join("")}`;
}
function renderSuggestions(funds) {
  searchFunds = funds; activeIndex = -1;
  fundList.innerHTML = funds.length ? funds.map((fund, index) => `<button class="fund-option" type="button" role="option" id="fund-option-${index}" aria-selected="false" data-index="${index}"><strong>${escapeHTML(fund.name)}</strong><small>${escapeHTML(fund.manager || "發行商待同步")} · ${nisaLabels(fund).join(" · ")}</small></button>`).join("") : `<p class="field-hint">找不到符合的基金；可先按「更新公開資料」。</p>`;
  setComboOpen(true);
}
function setActiveSuggestion(index) {
  const options = [...fundList.querySelectorAll(".fund-option")];
  if (!options.length) return;
  activeIndex = (index + options.length) % options.length;
  options.forEach((option, i) => { const active = i === activeIndex; option.classList.toggle("is-active", active); option.setAttribute("aria-selected", String(active)); });
  options[activeIndex].scrollIntoView({ block: "nearest" });
}

query.addEventListener("input", () => {
  clearTimeout(searchTimer); fundID.value = ""; selectedFund = null;
  const value = query.value.trim(); if (!value) { clearSuggestions(); return; }
  searchTimer = setTimeout(async () => {
    try { const response = await fetch(`/api/funds?q=${encodeURIComponent(value)}`); if (!response.ok) throw new Error(); renderSuggestions(await response.json()); }
    catch { fundList.innerHTML = `<p class="field-hint">基金清單暫時無法讀取。</p>`; setComboOpen(true); }
  }, 180);
});
query.addEventListener("keydown", event => {
  if (event.key === "ArrowDown") { event.preventDefault(); setActiveSuggestion(activeIndex + 1); }
  if (event.key === "ArrowUp") { event.preventDefault(); setActiveSuggestion(activeIndex - 1); }
  if (event.key === "Enter" && activeIndex >= 0) { event.preventDefault(); showFund(searchFunds[activeIndex]); }
  if (event.key === "Escape") clearSuggestions();
});
fundList.addEventListener("click", event => { const option = event.target.closest("[data-index]"); if (option) showFund(searchFunds[Number(option.dataset.index)]); });
document.addEventListener("click", event => { if (!event.target.closest(".fund-combobox") && !event.target.closest("#fund-list")) clearSuggestions(); });

document.querySelectorAll(".tab").forEach(tab => tab.addEventListener("click", () => {
  document.querySelectorAll(".tab").forEach(item => { const active = item === tab; item.classList.toggle("active", active); item.setAttribute("aria-selected", String(active)); });
  document.querySelectorAll(".view-panel").forEach(panel => { panel.hidden = panel.id !== tab.dataset.view; });
}));

document.querySelector("#analysis-form").addEventListener("submit", async event => {
  event.preventDefault();
  if (!fundID.value) { fundList.innerHTML = `<p class="field-hint">請從下拉結果選擇一檔基金。</p>`; setComboOpen(true); return; }
  const payload = { fundId: fundID.value, initialAmount: Number(document.querySelector("#initial").value), monthlyAmount: Number(document.querySelector("#monthly").value) };
  try {
    const response = await fetch("/api/analysis", { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(payload) });
    const result = await response.json(); if (!response.ok) throw new Error(result.error || "分析失敗"); renderResult(result); loadInsights(result.fund.id);
  } catch (error) {
    document.querySelector("#empty-state").textContent = error.message; document.querySelector("#empty-state").hidden = false; document.querySelector("#result").hidden = true; document.querySelector("#data-status").textContent = "資料不足";
  }
});

function relativeReturn(value, contributions) { const rate = contributions ? (value / contributions - 1) * 100 : 0; return `${rate >= 0 ? "+" : ""}${rate.toFixed(1)}%`; }
function renderResult(result) {
  document.querySelector("#empty-state").hidden = true; document.querySelector("#result").hidden = false;
  document.querySelector("#result-fund-name").textContent = result.fund.name;
  [["#p10",result.p10],["#table-p10",result.p10],["#p50",result.p50],["#p50-small",result.p50],["#table-p50",result.p50],["#p90",result.p90],["#table-p90",result.p90]].forEach(([selector,value]) => document.querySelector(selector).textContent = yen.format(value));
  document.querySelector("#return-p10").textContent = relativeReturn(result.p10, result.totalContributions);
  document.querySelector("#return-p50").textContent = relativeReturn(result.p50, result.totalContributions);
  document.querySelector("#return-p90").textContent = relativeReturn(result.p90, result.totalContributions);
  document.querySelector("#contributions").textContent = `5 年投入總額：${yen.format(result.totalContributions)} · 資料截至 ${date.format(new Date(result.dataAsOf))}`;
  document.querySelector("#holding").textContent = result.recommendedYears ? `${result.recommendedYears} 年` : "無合格年限";
  document.querySelector("#drawdown").textContent = `${(result.maxDrawdown * 100).toFixed(1)}%`;
  document.querySelector("#samples").textContent = `${result.sampleCount} 組`;
  document.querySelector("#methodology").textContent = result.methodology;
  document.querySelector("#nisa-note").textContent = result.nisaNote;
  document.querySelector("#data-status").textContent = `資料截至 ${date.format(new Date(result.dataAsOf))}`;
  showFund(result.fund);
}

async function loadInsights(id) {
  const container = document.querySelector("#insights"); container.textContent = "載入已驗證資料…";
  try { const response = await fetch(`/api/insights/${encodeURIComponent(id)}`); const items = await response.json(); container.innerHTML = items.length ? items.map(item => `<article class="insight"><a href="${escapeHTML(item.sourceUrl)}" target="_blank" rel="noreferrer">${escapeHTML(item.title)}</a><p>${escapeHTML(item.publisher)} · ${date.format(new Date(item.publishedAt))}</p><p>${escapeHTML(item.summary)}</p></article>`).join("") : "尚無可驗證的發行商／官方觀點資料。這不影響歷史金額計算。"; }
  catch { container.textContent = "觀點資料暫時無法讀取；這不影響歷史金額計算。"; }
}

document.querySelector("#refresh").addEventListener("click", async event => {
  const button = event.currentTarget; button.disabled = true; button.textContent = "更新中…";
  try { const response = await fetch("/api/data/refresh", {method:"POST"}); const result = await response.json(); if (!response.ok) throw new Error(result.error); button.textContent = `已更新 ${result.fundsUpdated} 檔`; setTimeout(() => button.textContent = "更新公開資料", 2200); }
  catch { button.textContent = "更新失敗"; setTimeout(() => button.textContent = "更新公開資料", 2200); }
  finally { button.disabled = false; }
});
