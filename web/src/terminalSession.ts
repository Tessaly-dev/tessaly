// SPDX-License-Identifier: Apache-2.0

import { sessionKey } from "./http";

// The parts of xterm.js and WebSocket that a terminal session uses, so tests
// can pass fakes.
export type TermLike = {
  write(data: Uint8Array): void;
  onData(cb: (data: string) => void): { dispose(): void };
  onResize(cb: (size: { rows: number; cols: number }) => void): { dispose(): void };
  readonly rows: number;
  readonly cols: number;
};

export type SocketLike = {
  binaryType: string;
  readyState: number;
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  send(data: string): void;
  close(): void;
};

const OPEN = 1;

// terminalProtocols returns the WebSocket subprotocols to offer. Browsers
// cannot set headers on a WebSocket, so the session key rides as the second
// one. The daemon picks the plain "tessaly", so the key is never echoed.
export function terminalProtocols(key: string | null = sessionKey()): string[] {
  return key ? ["tessaly", `tessaly.key.${key}`] : ["tessaly"];
}

// terminalUrl returns the socket URL for a workspace's terminal on this origin.
export function terminalUrl(id: number, loc: Location = window.location): string {
  const scheme = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${loc.host}/api/v1/workspaces/${encodeURIComponent(String(id))}/terminal`;
}

// connectTerminal joins a terminal to a socket: output from the socket is
// written to the terminal, and keystrokes and size changes go to the socket.
// It returns a function that ends the session.
export function connectTerminal(term: TermLike, socket: SocketLike, onClose?: () => void): () => void {
  socket.binaryType = "arraybuffer";
  const send = (msg: object) => {
    if (socket.readyState === OPEN) socket.send(JSON.stringify(msg));
  };

  socket.onopen = () => send({ type: "resize", rows: term.rows, cols: term.cols });
  socket.onmessage = (ev) => {
    // Only binary output is expected. Text is ignored, never interpreted.
    if (typeof ev.data !== "string") term.write(new Uint8Array(ev.data as ArrayBuffer));
  };
  socket.onclose = () => onClose?.();

  const data = term.onData((d) => send({ type: "input", data: d }));
  const resize = term.onResize(({ rows, cols }) => send({ type: "resize", rows, cols }));

  return () => {
    data.dispose();
    resize.dispose();
    socket.onclose = null;
    socket.close();
  };
}
