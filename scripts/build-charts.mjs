#!/usr/bin/env node
/**
 * Bundles tree-shaken Chart.js entry points into dist/js/ for lazy loading.
 * Set CHART_BUNDLE_META=1 to write dist/bundle-charts-meta.json for bundle analysis.
 */
import * as esbuild from "esbuild";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const outdir = path.join(root, "dist", "js");

fs.mkdirSync(outdir, { recursive: true });

const metafile = process.env.CHART_BUNDLE_META === "1";

const result = await esbuild.build({
  entryPoints: {
    "chart-metrics": path.join(root, "src/js/bundles/chart-metrics.mjs"),
    "chart-dashboard": path.join(root, "src/js/bundles/chart-dashboard.mjs"),
  },
  bundle: true,
  format: "esm",
  platform: "browser",
  target: ["es2020"],
  outdir,
  minify: true,
  sourcemap: false,
  metafile,
  logLevel: "info",
});

if (metafile && result.metafile) {
  const metaPath = path.join(root, "dist", "bundle-charts-meta.json");
  fs.writeFileSync(metaPath, JSON.stringify(result.metafile), "utf8");
  process.stderr.write(`build-charts: wrote ${metaPath}\n`);
}

process.stderr.write(`build-charts: -> ${outdir}/chart-metrics.js, chart-dashboard.js\n`);
