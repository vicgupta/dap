export class Host {
    init;
    constructor(init) {
        this.init = init;
    }
    get config() {
        return this.init.config;
    }
    get dataDir() {
        return this.init.data_dir;
    }
    async call(method, path, body) {
        const res = await fetch(this.init.api_base.replace(/\/$/, "") + path, {
            method,
            headers: {
                Authorization: "Bearer " + this.init.token,
                "Content-Type": "application/json",
            },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const data = (await res.json().catch(() => ({})));
        if (!res.ok || data.ok === false) {
            throw new Error(data.error || data.reason || "host call failed");
        }
        return data;
    }
    context() {
        return this.call("GET", "/context");
    }
    state() {
        return this.call("GET", "/state");
    }
    putState(state) {
        return this.call("PUT", "/state", state);
    }
    log(message, evidence) {
        return this.call("POST", "/log", { message, evidence });
    }
    tile(tile) {
        return this.call("POST", "/tile", tile);
    }
    propose(card) {
        return this.call("POST", "/propose", card);
    }
    check(req) {
        return this.call("POST", "/check", req);
    }
    secretGet(account) {
        return this.call("POST", "/secret/get", { account });
    }
    secretSet(account, value) {
        return this.call("POST", "/secret/set", { account, value });
    }
    secretDelete(account) {
        return this.call("POST", "/secret/delete", { account });
    }
    mailThreads(provider) {
        return this.call("GET", "/mail/threads?provider=" + encodeURIComponent(provider));
    }
    mailSend(req) {
        return this.call("POST", "/mail/send", req);
    }
}
