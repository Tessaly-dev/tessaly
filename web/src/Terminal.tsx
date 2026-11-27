// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { connectTerminal, terminalUrl } from "./terminalSession";

type Props = {
  workspaceId: number;
  // session changes when the agent starts or stops, so the view reconnects.
  session: string;
  onClose?: () => void;
};

// Terminal shows a workspace's agent terminal live, and sends keystrokes to it.
export function Terminal({ workspaceId, session, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const term = new XTerm({ convertEol: false, cursorBlink: true, fontSize: 13, scrollback: 5000 });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);
    fit.fit();

    const end = connectTerminal(term, new WebSocket(terminalUrl(workspaceId)), () => onCloseRef.current?.());
    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(el);
    return () => {
      observer.disconnect();
      end();
      term.dispose();
    };
  }, [workspaceId, session]);

  return <div className="terminal" ref={ref} aria-label="Agent terminal" />;
}
