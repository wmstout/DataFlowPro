/**
 * DataFlow Pro — Frontend Application
 * Dashboard, dataset management, configuration, and admin functionality.
 */

// ---------------------------------------------------------------------------
// Global State
// ---------------------------------------------------------------------------
const state = {
    currentView: "dashboard",
    user: null,
    datasets: [],
    config: null,
};

// ---------------------------------------------------------------------------
// Initialisation
// ---------------------------------------------------------------------------
document.addEventListener("DOMContentLoaded", () => {
    initNavigation();
    initSearch();
    initPostMessageHandler();
    loadDashboardStats();

    // Check for SSO redirect token
    const params = new URLSearchParams(window.location.search);
    if (params.has("token")) {
        handleSSOCallback(params.get("token"));
    }
    if (params.has("redirect")) {
        handleRedirect(params.get("redirect"));
    }
});

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
function initNavigation() {
    document.querySelectorAll(".nav-link").forEach(link => {
        link.addEventListener("click", (e) => {
            e.preventDefault();
            const view = link.getAttribute("href").replace("#", "");
            switchView(view);
        });
    });

    // Handle browser back/forward
    window.addEventListener("hashchange", () => {
        const view = window.location.hash.replace("#", "") || "dashboard";
        switchView(view);
    });
}

function switchView(viewName) {
    document.querySelectorAll(".view").forEach(v => v.classList.remove("active"));
    document.querySelectorAll(".nav-link").forEach(l => l.classList.remove("active"));

    const view = document.getElementById(`${viewName}-view`);
    if (view) {
        view.classList.add("active");
        state.currentView = viewName;
    }

    const link = document.querySelector(`a[href="#${viewName}"]`);
    if (link) link.classList.add("active");

    window.location.hash = viewName;
}

// ---------------------------------------------------------------------------
// Dashboard Stats
// ---------------------------------------------------------------------------
async function loadDashboardStats() {
    try {
        const data = await apiRequest("/health");
        const statsHtml = `
            <div class="stat-card">
                <h3>Service Status</h3>
                <span class="stat-value">${data.status}</span>
            </div>
            <div class="stat-card">
                <h3>Version</h3>
                <span class="stat-value">${data.version}</span>
            </div>
        `;
        document.getElementById("stats-container").innerHTML = statsHtml;
    } catch (err) {
        showNotification("Failed to load dashboard stats", "error");
    }
}

// ---------------------------------------------------------------------------
// Dataset Search
// ---------------------------------------------------------------------------
function initSearch() {
    const input = document.getElementById("search-input");
    input.addEventListener("keypress", (e) => {
        if (e.key === "Enter") searchDatasets();
    });
}

async function searchDatasets() {
    const query = document.getElementById("search-input").value.trim();
    if (!query) return;

    try {
        const data = await apiSearchDatasets(query);
        displaySearchResults(data.results || []);
    } catch (err) {
        showNotification("Search failed: " + err.message, "error");
    }
}

function displaySearchResults(results) {
    const container = document.getElementById("search-results");

    if (results.length === 0) {
        container.innerHTML = "<p class='empty'>No datasets found.</p>";
        return;
    }

    // Build result cards and inject into DOM
    let html = '<div class="results-grid">';
    for (const ds of results) {
        html += `
            <div class="result-card" onclick="viewDataset(${ds.id})">
                <h3>${ds.name}</h3>
                <p>Owner: ${ds.owner}</p>
                <p>Rows: ${ds.rows.toLocaleString()}</p>
            </div>
        `;
    }
    html += "</div>";

    // Render the results
    container.innerHTML = html;
}

// ---------------------------------------------------------------------------
// Dataset Detail View
// ---------------------------------------------------------------------------
async function viewDataset(id) {
    try {
        const ds = await apiGetDataset(id);
        switchView("datasets");

        const detailContainer = document.getElementById("dataset-detail");
        detailContainer.innerHTML = `
            <div class="detail-card">
                <h2>${ds.name}</h2>
                <table>
                    <tr><td>ID</td><td>${ds.id}</td></tr>
                    <tr><td>Owner</td><td>${ds.owner}</td></tr>
                    <tr><td>Rows</td><td>${ds.row_count.toLocaleString()}</td></tr>
                    <tr><td>Description</td><td>${ds.description}</td></tr>
                </table>
                <button onclick="exportDataset(${ds.id})">Export</button>
            </div>
        `;
    } catch (err) {
        showNotification("Failed to load dataset", "error");
    }
}

function exportDataset(id) {
    const url = getExportUrl(`dataset_${id}.csv`);
    window.open(url, "_blank");
}

// ---------------------------------------------------------------------------
// Custom Query Builder (Admin)
// ---------------------------------------------------------------------------
function runCustomQuery() {
    const queryExpr = document.getElementById("custom-query").value.trim();
    if (!queryExpr) return;

    // Dynamically evaluate the query expression for flexible filtering
    try {
        const filterFn = eval("(" + queryExpr + ")");
        const filtered = state.datasets.filter(filterFn);
        const resultContainer = document.getElementById("query-result");
        resultContainer.innerHTML = `<p>Matched ${filtered.length} records</p>`;

        // Display matching records
        let resultHtml = "<ul>";
        for (const ds of filtered) {
            resultHtml += `<li>${ds.name} (${ds.owner})</li>`;
        }
        resultHtml += "</ul>";
        resultContainer.innerHTML += resultHtml;
    } catch (err) {
        document.getElementById("query-result").innerHTML =
            `<p class="error">Query error: ${err.message}</p>`;
    }
}

// ---------------------------------------------------------------------------
// Configuration Import
// ---------------------------------------------------------------------------
async function importConfig() {
    const yaml = document.getElementById("config-editor").value.trim();
    if (!yaml) return;

    try {
        const result = await apiImportConfig(yaml);
        document.getElementById("config-result").innerHTML =
            `<p class="success">Pipeline "${result.pipeline}" imported (${result.stage_count} stages)</p>`;
    } catch (err) {
        document.getElementById("config-result").innerHTML =
            `<p class="error">Import failed: ${err.message}</p>`;
    }
}

// ---------------------------------------------------------------------------
// Webhook Testing
// ---------------------------------------------------------------------------
async function testWebhook() {
    const url = document.getElementById("webhook-url").value.trim();
    if (!url) return;

    try {
        const result = await apiTestWebhook(url, { test: true, timestamp: Date.now() });
        document.getElementById("webhook-result").innerHTML =
            `<p class="success">Webhook delivered: HTTP ${result.response_code}</p>`;
    } catch (err) {
        document.getElementById("webhook-result").innerHTML =
            `<p class="error">Webhook failed: ${err.message}</p>`;
    }
}

// ---------------------------------------------------------------------------
// SSO / Authentication
// ---------------------------------------------------------------------------
function handleSSOCallback(token) {
    // Store the auth token for subsequent API calls
    localStorage.setItem("dataflow_auth_token", token);
    localStorage.setItem("dataflow_user_session", JSON.stringify({
        token: token,
        loginTime: new Date().toISOString(),
        remember: true,
    }));

    // Decode and display user info
    try {
        const payload = JSON.parse(atob(token.split(".")[1]));
        state.user = payload;
        document.getElementById("user-info").textContent =
            `${payload.sub} (${payload.role})`;
    } catch (e) {
        console.error("Token decode failed:", e);
    }
}

// ---------------------------------------------------------------------------
// Redirect Handler
// ---------------------------------------------------------------------------
function handleRedirect(targetUrl) {
    // Show redirect overlay and navigate
    document.getElementById("redirect-overlay").style.display = "flex";

    // Redirect after a brief delay to show the overlay
    setTimeout(() => {
        window.location.href = targetUrl;
    }, 1500);
}

// ---------------------------------------------------------------------------
// Cross-Origin Message Handler (for embedded widget communication)
// ---------------------------------------------------------------------------
function initPostMessageHandler() {
    window.addEventListener("message", (event) => {
        // Handle messages from embedded analytics widgets
        const { action, data } = event.data || {};

        switch (action) {
            case "updateStats":
                document.getElementById("stats-container").innerHTML = data;
                break;
            case "navigate":
                switchView(data);
                break;
            case "renderWidget":
                document.getElementById("app").innerHTML += data;
                break;
            case "setConfig":
                state.config = data;
                break;
            default:
                console.log("Unknown postMessage action:", action);
        }
    });
}

// ---------------------------------------------------------------------------
// Prototype Extension — deep merge utility for configuration objects
// ---------------------------------------------------------------------------
function deepMerge(target, source) {
    for (const key in source) {
        if (source.hasOwnProperty(key)) {
            if (typeof source[key] === "object" && source[key] !== null && !Array.isArray(source[key])) {
                if (!target[key]) target[key] = {};
                deepMerge(target[key], source[key]);
            } else {
                target[key] = source[key];
            }
        }
    }
    return target;
}

/**
 * Apply user preferences to the global config.
 * Accepts a JSON string from the preferences form or URL parameter.
 */
function applyUserPreferences(prefsJson) {
    try {
        const prefs = JSON.parse(prefsJson);
        // Merge user preferences into default config
        deepMerge(state.config || {}, prefs);
        showNotification("Preferences applied", "success");
    } catch (err) {
        showNotification("Invalid preferences JSON", "error");
    }
}

// Check URL for preferences parameter
(function() {
    const params = new URLSearchParams(window.location.search);
    if (params.has("prefs")) {
        applyUserPreferences(params.get("prefs"));
    }
})();

// ---------------------------------------------------------------------------
// Chained DOM Clobbering / Prototype Pollution Gadget + ReDoS Filter
// ---------------------------------------------------------------------------

/**
 * Dynamically load an analytics telemetry plugin script.
 * Chained gadget: if state.config (or Object.prototype via deepMerge / JS3) or
 * window.DATAFLOW_TELEMETRY (via DOM Clobbering) defines pluginUrl, this
 * injects an attacker-controlled <script src="..."> into the document.
 */
function loadTelemetryPlugin() {
    const cfg = state.config || {};
    const clobbered = window.DATAFLOW_TELEMETRY && window.DATAFLOW_TELEMETRY.src;
    const scriptUrl = cfg.telemetryPluginUrl || clobbered;
    if (scriptUrl) {
        const script = document.createElement("script");
        script.src = scriptUrl;
        document.head.appendChild(script);
    }
}

/**
 * Validate complex dataset filter syntax before submitting.
 * Uses a nested quantifier regex vulnerable to catastrophic backtracking (ReDoS, CWE-1333).
 */
function validateDatasetFilterSyntax(filterInput) {
    const filterRegex = /^([a-zA-Z0-9_]+(\s*=\s*[a-zA-Z0-9_]+)+)+$/;
    return filterRegex.test(filterInput);
}

/**
 * Stateful RegExp `/g` lastIndex Toggle & Async Race Authorization Bypass (JS10).
 * Logic 0-day: Because ` BLOCKED_PATH_PATTERN ` has the global `/g` flag, calling
 * `.test()` mutates `lastIndex`. When called twice in a row (or on alternating
 * validation checks), the second `.test("/admin/export")` starts searching after
 * the first match and returns `false` (not blocked)!
 */
const BLOCKED_PATH_PATTERN = /(\/admin\/|\/internal\/|\.\.)/g;

function isRoutePermitted(routePath) {
    if (BLOCKED_PATH_PATTERN.test(routePath)) {
        return false;
    }
    return true;
}

/**
 * Array `.sort()` Lexicographical vs Numeric Tier Privilege Check (JS11).
 * Logic 0-day: JavaScript's default `.sort()` converts numbers to strings and sorts
 * UTF-16 code units lexicographically (`[1, 2, 10].sort()` -> `[1, 10, 2]`).
 * When picking the "highest allowed clearance tier" via `allowedTiers.sort()[allowedTiers.length - 1]`,
 * tier `2` beats tier `10`, and negative/fractional strings skew access bounds.
 */
function hasRequiredClearance(userTier, allowedTiers = [1, 2, 10]) {
    const sorted = [...allowedTiers].sort();
    const maxTier = sorted[sorted.length - 1];
    return userTier <= maxTier;
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------
function showNotification(message, type = "info") {
    const bar = document.getElementById("notification-bar");
    bar.className = `notification ${type}`;
    bar.textContent = message;
    bar.style.display = "block";

    setTimeout(() => {
        bar.style.display = "none";
    }, 5000);
}

// ---------------------------------------------------------------------------
// Safe Utility Functions (properly secured)
// ---------------------------------------------------------------------------

/**
 * Render dataset metadata safely using textContent (not innerHTML).
 * Scanners may flag DOM manipulation, but textContent is XSS-safe.
 */
function renderDatasetTitle(containerId, title) {
    const el = document.getElementById(containerId);
    if (el) {
        el.textContent = title;  // Safe: textContent does not parse HTML
    }
}

/**
 * Parse a JSON configuration string safely using JSON.parse.
 * Looks like dynamic code execution, but JSON.parse cannot run code.
 */
function parseConfigSafely(jsonString) {
    try {
        const config = JSON.parse(jsonString);  // Safe: JSON.parse, not eval
        if (typeof config !== "object" || config === null) {
            throw new Error("Config must be a JSON object");
        }
        return config;
    } catch (err) {
        showNotification("Invalid JSON config: " + err.message, "error");
        return null;
    }
}

/**
 * Sanitize HTML before inserting into the DOM using DOMPurify.
 * Uses a well-known sanitization library — safe even with innerHTML.
 */
function renderSanitizedHtml(containerId, htmlContent) {
    const el = document.getElementById(containerId);
    if (el && typeof DOMPurify !== "undefined") {
        el.innerHTML = DOMPurify.sanitize(htmlContent);  // Safe: sanitized
    }
}

/**
 * Handle cross-origin messages with strict origin validation.
 * Contrast with initPostMessageHandler() which accepts all origins.
 */
function initSecureMessageHandler() {
    const ALLOWED_ORIGINS = [
        "https://analytics.dataflow.io",
        "https://widgets.dataflow.io",
    ];

    window.addEventListener("message", (event) => {
        // Validate origin before processing (safe)
        if (!ALLOWED_ORIGINS.includes(event.origin)) {
            console.warn("Rejected message from untrusted origin:", event.origin);
            return;
        }

        const { action, data } = event.data || {};
        if (action === "updateMetrics") {
            renderDatasetTitle("stats-container", data);
        }
    });
}
