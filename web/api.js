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
