// Fetches config from /api/config, then controls mock vs real data flow.

let APP_CONFIG = { showMock: true, provider: "openrouter" };
const AUTH_STORAGE_KEY = "buildorbail_auth_v1";
let AUTH_SESSION = { email: "", accessToken: "" };

function normalizeEmail(email) {
    return String(email || "").trim().toLowerCase();
}

function loadAuthSession() {
    try {
        const raw = localStorage.getItem(AUTH_STORAGE_KEY);
        if (!raw) return;
        const parsed = JSON.parse(raw);
        AUTH_SESSION = {
            email: normalizeEmail(parsed.email),
            accessToken: String(parsed.accessToken || "").trim(),
        };
    } catch (_) {
        AUTH_SESSION = { email: "", accessToken: "" };
    }
}

function hasAuthSession() {
    return !!(AUTH_SESSION.email && AUTH_SESSION.accessToken);
}

function getAuthSession() {
    return { ...AUTH_SESSION };
}

function setAuthSession(email, accessToken) {
    AUTH_SESSION = {
        email: normalizeEmail(email),
        accessToken: String(accessToken || "").trim(),
    };
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(AUTH_SESSION));
    return getAuthSession();
}

function clearAuthSession() {
    AUTH_SESSION = { email: "", accessToken: "" };
    localStorage.removeItem(AUTH_STORAGE_KEY);
}

function authHeaders() {
    if (!hasAuthSession()) return {};
    return {
        "X-User-Email": AUTH_SESSION.email,
        "X-User-Token": AUTH_SESSION.accessToken,
    };
}

loadAuthSession();

async function parseApiResponse(res, fallbackType = "unknown_error") {
    const raw = await res.text();
    let data = null;

    if (raw) {
        try {
            data = JSON.parse(raw);
        } catch (_) {
            data = null;
        }
    }

    if (!res.ok) {
        if (data && typeof data === "object") {
            throw {
                type: data.type || fallbackType,
                message: data.message || data.error || res.statusText,
            };
        }

        const compact = (raw || "").replace(/\s+/g, " ").trim();
        throw {
            type: fallbackType,
            message: compact ? compact.slice(0, 220) : "We hit a temporary server issue. Please try again.",
        };
    }

    if (!data || typeof data !== "object") {
        throw {
            type: "invalid_json",
            message: "We hit a temporary server issue. Please try again.",
        };
    }

    return data;
}

async function fetchConfig() {
    try {
        const res = await fetch("/api/config");
        APP_CONFIG = await parseApiResponse(res, "config_error");
    } catch (e) {
        console.warn("Config fetch failed, using defaults:", e.message || e);
    }
    return APP_CONFIG;
}

async function fetchIdeas(query) {
    const params = new URLSearchParams();
    if (query) params.set("q", query);

    const res = await fetch("/api/ideas?" + params.toString(), {
        headers: authHeaders(),
    });
    return parseApiResponse(res, "unknown_error");
}

async function fetchSavedIdeas() {
    const res = await fetch("/api/saved", {
        headers: authHeaders(),
    });
    const data = await parseApiResponse(res, "saved_fetch_error");
    return data.ideas || [];
}

async function removeSavedIdea(id) {
    const res = await fetch("/api/unsave", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            ...authHeaders(),
        },
        body: JSON.stringify({ id }),
    });
    const data = await parseApiResponse(res, "unsave_error");
    return data;
}

async function recordDecision(idea, action) {
    const res = await fetch("/api/decision", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            ...authHeaders(),
        },
        body: JSON.stringify({
            action,
            idea,
        }),
    });
    return parseApiResponse(res, "decision_error");
}

async function registerUser(email) {
    const res = await fetch("/api/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: normalizeEmail(email) }),
    });
    const data = await parseApiResponse(res, "auth_error");
    return data;
}

async function loginUser(email, accessToken) {
    const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            email: normalizeEmail(email),
            accessToken: String(accessToken || "").trim(),
        }),
    });
    const data = await parseApiResponse(res, "auth_error");
    setAuthSession(data.email, data.accessToken);
    return data;
}

function loadMock() {
    return { ideas: MOCK_IDEAS, source: "mock", query: "mock" };
}

const IDEA_INDEX_DB_NAME = "buildorbail";
const IDEA_INDEX_STORE = "idea_threads";

function ideaThreadKey(idea) {
    const link = (idea?.sampleLink || "").trim();
    const title = (idea?.title || "").trim();
    const post = (idea?.samplePost || "").trim();
    // Use link+title so two ideas referencing the same thread are still distinct
    return `${link}::${title}::${post}`;
}

function openIdeaIndexDb() {
    return new Promise((resolve, reject) => {
        const req = indexedDB.open(IDEA_INDEX_DB_NAME, 1);
        req.onupgradeneeded = () => {
            const db = req.result;
            if (!db.objectStoreNames.contains(IDEA_INDEX_STORE)) {
                db.createObjectStore(IDEA_INDEX_STORE, { keyPath: "threadKey" });
            }
        };
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
    });
}

async function getSeenThreadKeys() {
    const db = await openIdeaIndexDb();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(IDEA_INDEX_STORE, "readonly");
        const store = tx.objectStore(IDEA_INDEX_STORE);
        const req = store.getAllKeys();
        req.onsuccess = () => resolve(new Set((req.result || []).map((k) => String(k))));
        req.onerror = () => reject(req.error);
        tx.oncomplete = () => db.close();
    });
}

async function markIdeasSeen(ideas) {
    const db = await openIdeaIndexDb();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(IDEA_INDEX_STORE, "readwrite");
        const store = tx.objectStore(IDEA_INDEX_STORE);
        const now = new Date().toISOString();
        ideas.forEach((idea) => {
            const key = ideaThreadKey(idea);
            if (!key) return;
            store.put({
                threadKey: key,
                seenAt: now,
                title: idea.title || "",
            });
        });
        tx.oncomplete = () => {
            db.close();
            resolve();
        };
        tx.onerror = () => {
            db.close();
            reject(tx.error);
        };
    });
}
