import { cp, mkdir, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

const projectRoot = fileURLToPath(new URL("..", import.meta.url));
const sourceDir = fileURLToPath(new URL("../src", import.meta.url));
const staticDir = fileURLToPath(new URL("../static", import.meta.url));
const distDir = fileURLToPath(new URL("../dist", import.meta.url));
const assetsDir = fileURLToPath(new URL("../dist/assets", import.meta.url));

await rm(distDir, { recursive: true, force: true });
await mkdir(assetsDir, { recursive: true });

await Promise.all([
  cp(`${staticDir}/app.html`, `${distDir}/app.html`),
  cp(`${staticDir}/privacy.html`, `${distDir}/privacy.html`),
  cp(`${sourceDir}/styles.css`, `${assetsDir}/app.css`),
  build({
    entryPoints: [`${sourceDir}/app.js`],
    outfile: `${assetsDir}/app.js`,
    bundle: true,
    format: "iife",
    platform: "browser",
    target: "es2022",
    sourcemap: false,
    legalComments: "none",
  }),
  build({
    entryPoints: [`${sourceDir}/storage-diagnostics.js`],
    outfile: `${assetsDir}/storage-diagnostics.js`,
    bundle: true,
    format: "iife",
    platform: "browser",
    target: "es2022",
    sourcemap: false,
    legalComments: "none",
  }),
]);

console.log("Partner H5 built in dist/");
