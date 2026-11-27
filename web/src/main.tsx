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

createRoot(root).render(
  <StrictMode>
    <App initialToken={token} />
  </StrictMode>,
);
