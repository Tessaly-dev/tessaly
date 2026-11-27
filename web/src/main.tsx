// SPDX-License-Identifier: Apache-2.0
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { takeTokenFromHash } from "./api";
import "./styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root element");

// Read the token outside React, so StrictMode's double effects in dev cannot
// race on the fragment, which this call clears.
const token = takeTokenFromHash();

// Pasting a sign-in link into an open tab changes only the fragment, which
// does not reload the page. Reload so the token above is read.
window.addEventListener("hashchange", () => {
  if (window.location.hash.startsWith("#token=")) window.location.reload();
});

createRoot(root).render(
  <StrictMode>
    <App initialToken={token} />
  </StrictMode>,
);
