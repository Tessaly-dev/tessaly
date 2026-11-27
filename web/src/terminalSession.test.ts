// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it, vi } from "vitest";
import { connectTerminal, terminalProtocols, terminalUrl, type SocketLike, type TermLike } from "./terminalSession";

function fakeTerm() {
  const written: string[] = [];
  let dataCb: (d: string) => void = () => undefined;
  let resizeCb: (s: { rows: number; cols: number }) => void = () => undefined;
  const disposed: string[] = [];
  const term: TermLike = {
    rows: 30,
    cols: 120,
    write: (d) => written.push(new TextDecoder().decode(d)),
    onData: (cb) => ((dataCb = cb), { dispose: () => disposed.push("data") }),
    onResize: (cb) => ((resizeCb = cb), { dispose: () => disposed.push("resize") }),
  };
  return { term, written, disposed, type: (d: string) => dataCb(d), resize: (rows: number, cols: number) => resizeCb({ rows, cols }) };
}

function fakeSocket() {
  const sent: unknown[] = [];
  const socket: SocketLike & { closed: boolean } = {
    binaryType: "blob",
    readyState: 0,
    onopen: null,
    onmessage: null,
    onclose: null,
    closed: false,
    send: (d) => sent.push(JSON.parse(d)),
    close() {
      this.closed = true;
    },
  };
  return { socket, sent };
}

describe("terminalUrl", () => {
  it("uses ws or wss to match the page", () => {
    expect(terminalUrl(3, { protocol: "http:", host: "127.0.0.1:7777" } as Location)).toBe(
      "ws://127.0.0.1:7777/api/v1/workspaces/3/terminal",
    );
    expect(terminalUrl(3, { protocol: "https:", host: "tess.example" } as Location)).toBe(
      "wss://tess.example/api/v1/workspaces/3/terminal",
    );
  });
});

describe("connectTerminal", () => {
  it("wires output, input and resizes", () => {
    const t = fakeTerm();
    const { socket, sent } = fakeSocket();
    connectTerminal(t.term, socket);
    expect(socket.binaryType).toBe("arraybuffer");

    t.type("before open");
    expect(sent).toEqual([]);

    socket.readyState = 1;
    socket.onopen?.(new Event("open"));
    expect(sent).toEqual([{ type: "resize", rows: 30, cols: 120 }]);

    socket.onmessage?.(new MessageEvent("message", { data: new TextEncoder().encode("hi\r\n").buffer }));
    socket.onmessage?.(new MessageEvent("message", { data: "<script>text is ignored</script>" }));
    expect(t.written).toEqual(["hi\r\n"]);

    t.type("ls\r");
    t.resize(40, 100);
    expect(sent.slice(1)).toEqual([
      { type: "input", data: "ls\r" },
      { type: "resize", rows: 40, cols: 100 },
    ]);
  });

  it("reports a close from the server, and cleans up", () => {
    const t = fakeTerm();
    const { socket } = fakeSocket();
    const onClose = vi.fn();
    const end = connectTerminal(t.term, socket, onClose);

    socket.onclose?.(new CloseEvent("close"));
    expect(onClose).toHaveBeenCalledTimes(1);

    end();
    expect(socket.closed).toBe(true);
    expect(t.disposed).toEqual(["data", "resize"]);
    expect(socket.onclose).toBeNull();
  });
});

describe("terminalProtocols", () => {
  it("offers the session key after the plain protocol", () => {
    expect(terminalProtocols("abc")).toEqual(["tessaly", "tessaly.key.abc"]);
    expect(terminalProtocols(null)).toEqual(["tessaly"]);
  });
});
