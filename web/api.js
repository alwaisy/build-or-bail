// Fetches config from /api/config, then controls mock vs real data flow.

let APP_CONFIG = { showMock: true, provider: "openrouter" };

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

    const res = await fetch("/api/ideas?" + params.toString());
    return parseApiResponse(res, "unknown_error");
}

async function fetchSavedIdeas() {
    const res = await fetch("/api/saved");
    const data = await parseApiResponse(res, "saved_fetch_error");
    return data.ideas || [];
}

async function removeSavedIdea(id) {
    const res = await fetch("/api/unsave", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id }),
    });
    const data = await parseApiResponse(res, "unsave_error");
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
