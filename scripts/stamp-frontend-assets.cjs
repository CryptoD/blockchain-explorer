#!/usr/bin/env node
/**
 * Writes stamped copies of root *.html to build/stamped/ with ?v= on /dist, /static, and /images URLs.
 * Optional CDN prefix: CDN_BASE_URL (no trailing slash), e.g. https://d111111abcdef8.cloudfront.net
 *
 * Version: ASSET_VERSION, else GITHUB_SHA (first 7), else git rev-parse --short HEAD, else hash of dist/styles.css.
 * Writes dist/.asset-version for operators.
 *
 * Does not modify source HTML in the repo root (roadmap 49: clean dev tree; Docker copies build/stamped).
 */
"use strict";

const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const { createHash } = require("crypto");

const root = path.resolve(__dirname, "..");
const distDir = path.join(root, "dist");
const outDir = path.join(root, "build", "stamped");
const stylesPath = path.join(distDir, "styles.css");

function resolveVersion() {
  const fromEnv = process.env.ASSET_VERSION && String(process.env.ASSET_VERSION).trim();
  if (fromEnv) {
    return fromEnv.replace(/[^a-zA-Z0-9._-]/g, "").slice(0, 64) || "dev";
  }
  const gh = process.env.GITHUB_SHA && String(process.env.GITHUB_SHA).trim();
  if (gh && gh.length >= 7) {
    return gh.slice(0, 7);
  }
  try {
    const v = execSync("git rev-parse --short HEAD", {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (v) {
      return v;
    }
  } catch (_) {
    /* no git */
  }
  if (fs.existsSync(stylesPath)) {
    const h = createHash("sha256");
    h.update(fs.readFileSync(stylesPath));
    return h.digest("hex").slice(0, 12);
  }
  return "dev";
}

function resolveCDNBase() {
  const u = process.env.CDN_BASE_URL && String(process.env.CDN_BASE_URL).trim();
  if (!u) {
    return "";
  }
  return u.replace(/\/+$/, "");
}

function stampHtml(html, version, cdnBase) {
  return html.replace(
    /(\b(?:href|src))="(\/(?:dist|static|images)\/[^"?#]*)"/g,
    (match, attr, assetPath) => {
      let url = assetPath;
      if (cdnBase) {
        url = cdnBase + assetPath;
      }
      const joiner = url.includes("?") ? "&" : "?";
      return `${attr}="${url}${joiner}v=${encodeURIComponent(version)}"`;
    }
  );
}

function main() {
  const version = resolveVersion();
  const cdnBase = resolveCDNBase();

  if (!fs.existsSync(distDir)) {
    fs.mkdirSync(distDir, { recursive: true });
  }
  fs.writeFileSync(path.join(distDir, ".asset-version"), version + "\n", "utf8");

  fs.mkdirSync(outDir, { recursive: true });

  const htmlFiles = fs
    .readdirSync(root)
    .filter((f) => f.endsWith(".html"))
    .map((f) => path.join(root, f));

  if (htmlFiles.length === 0) {
    process.stderr.write("stamp-frontend-assets: no *.html in repo root\n");
    process.exit(1);
  }

  for (const file of htmlFiles) {
    const base = path.basename(file);
    const raw = fs.readFileSync(file, "utf8");
    const next = stampHtml(raw, version, cdnBase);
    fs.writeFileSync(path.join(outDir, base), next, "utf8");
  }

  process.stderr.write(
    `stamp-frontend-assets: version=${version} cdn=${cdnBase || "(same-origin)"} -> build/stamped/ (${htmlFiles.length} files)\n`
  );
}

main();
