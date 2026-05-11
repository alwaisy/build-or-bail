// Fetches config from /api/config, then controls mock vs real data flow.

let APP_CONFIG = { showMock: true, provider: "openrouter" };

async function fetchConfig() {
    try {
        const res = await fetch("/api/config");
        if (res.ok) APP_CONFIG = await res.json();
    } catch (e) {
        console.warn("Config fetch failed, using defaults:", e.message);
    }
    return APP_CONFIG;
}

async function fetchIdeas(query) {
    const params = new URLSearchParams();
    if (query) params.set("q", query);

    const res = await fetch("/api/ideas?" + params.toString());
    const data = await res.json();

    if (!res.ok) {
        throw {
            type: data.type || "unknown_error",
            message: data.message || data.error || res.statusText,
        };
    }

    return data;
}

async function fetchSavedIdeas() {
    const res = await fetch("/api/saved");
    const data = await res.json();
    if (!res.ok) throw new Error("Failed to fetch saved ideas");
    return data.ideas || [];
}

async function removeSavedIdea(id) {
    const res = await fetch("/api/unsave", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || "Failed to remove saved idea");
    return data;
}

function loadMock() {
    return { ideas: MOCK_IDEAS, source: "mock", query: "mock" };
}

const IDEA_INDEX_DB_NAME = "buildorbail";
const IDEA_INDEX_STORE = "idea_threads";

function ideaThreadKey(idea) {
    const link = (idea?.sampleLink || "").trim();
    if (link) return link;
    return `${(idea?.title || "").trim()}::${(idea?.samplePost || "").trim()}`;
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
