// Argus status plugin for opencode. Written by `argus install`; do not edit.
// Writes "state\nfolder" to __OPENCODE_DIR__/<conversation id> when a
// conversation starts working, asks for permission or finishes. It only writes
// files.
import { mkdirSync, writeFileSync } from "node:fs";

const DIR = "__OPENCODE_DIR__";

const ArgusStatus = {
  id: "argus-status",
  setup: async (ctx: any) => {
    const directory: string = ctx.location.directory;
    const unload = new AbortController();
    const dirs = new Map<string, Promise<string | undefined>>();
    try {
      mkdirSync(DIR, { recursive: true });
    } catch {}

    // The same event stream reaches every loaded plugin; each one only writes
    // the conversations of its own folder.
    function dirOf(id: string): Promise<string | undefined> {
      let d = dirs.get(id);
      if (!d) {
        d = ctx.session
          .get({ sessionID: id }, { signal: unload.signal })
          .then((s: any) => s?.location?.directory)
          .catch(() => undefined);
        dirs.set(id, d);
      }
      return d;
    }

    async function write(id: unknown, status: string) {
      if (typeof id !== "string" || !/^[A-Za-z0-9_-]+$/.test(id)) return;
      if ((await dirOf(id)) !== directory) return;
      try {
        writeFileSync(`${DIR}/${id}`, `${status}\n${directory}\n`);
      } catch {}
    }

    const task = (async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: unload.signal })) {
          const id = event?.data?.sessionID;
          switch (event.type) {
            case "session.execution.started":
            case "permission.replied":
              await write(id, "working");
              break;
            case "permission.asked":
              await write(id, "waiting");
              break;
            case "session.execution.succeeded":
            case "session.execution.failed":
            case "session.execution.interrupted":
              await write(id, "done");
              break;
          }
        }
      } catch {}
    })();

    return async () => {
      unload.abort();
      await Promise.race([task, new Promise((r) => setTimeout(r, 500))]);
    };
  },
};

export default ArgusStatus;
