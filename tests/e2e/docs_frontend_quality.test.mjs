import { describe, it, after } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import http from "node:http";
import net from "node:net";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const REPO_ROOT = path.resolve(__dirname, "../..");
const FRONTEND_DIR = path.join(REPO_ROOT, "web", "opspulse-frontend");
const FRONTEND_SRC_DIR = path.join(FRONTEND_DIR, "src");
const SPECS_DIR = path.join(REPO_ROOT, "specs");
const DOCS_DIR = path.join(REPO_ROOT, "docs");
const INTERNAL_API_DIR = path.join(REPO_ROOT, "internal", "api");
const INTERNAL_CONFIG_DIR = path.join(REPO_ROOT, "internal", "config");

const SENTINEL_API_URL = "https://e2e-sentinel-api.invalid";
const SENTINEL_PREFIX_URL = "https://e2e-sentinel-api.invalid/opspulse/";

const createdTempDirs = new Set();

after(() => {
  for (const dir of createdTempDirs) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
  createdTempDirs.clear();
});

function createTempDir(prefix = "opspulse-e2e-") {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  createdTempDirs.add(dir);
  return dir;
}

function removeTempDir(dir) {
  if (!dir) return;
  fs.rmSync(dir, { recursive: true, force: true });
  createdTempDirs.delete(dir);
}

function goEnv(extra = {}) {
  const pathSep = path.delimiter;
  const currentPath = process.env.PATH || "";
  const updatedPath = currentPath.includes("/usr/local/go/bin")
    ? currentPath
    : `/usr/local/go/bin${pathSep}${currentPath}`;
  return {
    ...process.env,
    PATH: updatedPath,
    ...extra,
  };
}

function runCommand(cmd, args, options = {}) {
  const res = spawnSync(cmd, args, {
    cwd: options.cwd || REPO_ROOT,
    env: options.env || process.env,
    encoding: "utf8",
    maxBuffer: 10 * 1024 * 1024,
  });
  return {
    status: res.status ?? (res.error ? 1 : 0),
    stdout: res.stdout || "",
    stderr: res.stderr || "",
    error: res.error,
  };
}

function listFilesRecursive(dir, predicate = () => true) {
  if (!fs.existsSync(dir)) return [];
  const results = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...listFilesRecursive(full, predicate));
    } else if (predicate(full, entry.name)) {
      results.push(full);
    }
  }
  return results;
}

function readFrontendPackageJson() {
  const pkgPath = path.join(FRONTEND_DIR, "package.json");
  return JSON.parse(fs.readFileSync(pkgPath, "utf8"));
}

function assertFrontendToolchainInstalled() {
  const binDir = path.join(FRONTEND_DIR, "node_modules", ".bin");
  const requiredBins = ["oxlint", "tsc", "vite"];
  const missing = requiredBins.filter(
    (bin) => !fs.existsSync(path.join(binDir, bin)),
  );
  if (missing.length > 0) {
    assert.fail(
      `Frontend toolchain not installed (missing ${missing.join(", ")} in ${binDir}). ` +
        `Run 'npm --prefix web/opspulse-frontend install' first.`,
    );
  }
}

function discoverViteEnvVars() {
  const vars = new Set(["VITE_API_BASE_URL"]);
  for (const envFile of [".env.example", ".env"]) {
    const full = path.join(FRONTEND_DIR, envFile);
    if (!fs.existsSync(full)) continue;
    const content = fs.readFileSync(full, "utf8");
    for (const match of content.matchAll(/^(VITE_[A-Z0-9_]+)\s*=/gm)) {
      vars.add(match[1]);
    }
  }
  const srcFiles = listFilesRecursive(FRONTEND_SRC_DIR, (f) =>
    /\.(ts|tsx|js|jsx|mts|mjs)$/.test(f),
  );
  for (const file of srcFiles) {
    const content = fs.readFileSync(file, "utf8");
    for (const match of content.matchAll(/import\.meta\.env\.(VITE_[A-Z0-9_]+)/g)) {
      vars.add(match[1]);
    }
  }
  return [...vars];
}

function buildViteEnvWithSentinel(sentinelUrl) {
  const env = { ...process.env };
  for (const key of discoverViteEnvVars()) {
    if (key.includes("API") || key.includes("URL") || key.includes("BASE")) {
      env[key] = sentinelUrl;
    }
  }
  env.VITE_API_BASE_URL = sentinelUrl;
  return env;
}

function runFrontendBuildToTempDir(sentinelUrl = SENTINEL_API_URL) {
  assertFrontendToolchainInstalled();
  const outDir = createTempDir("opspulse-vite-dist-");
  const env = buildViteEnvWithSentinel(sentinelUrl);

  const tscBin = path.join(FRONTEND_DIR, "node_modules", ".bin", "tsc");
  const tscRes = runCommand(tscBin, ["-b"], { cwd: FRONTEND_DIR, env });
  if (tscRes.status !== 0) {
    removeTempDir(outDir);
    assert.fail(
      `tsc -b failed (exit ${tscRes.status}):\n${tscRes.stdout}\n${tscRes.stderr}`,
    );
  }

  const viteBin = path.join(FRONTEND_DIR, "node_modules", ".bin", "vite");
  const viteRes = runCommand(
    viteBin,
    ["build", "--outDir", outDir, "--emptyOutDir"],
    { cwd: FRONTEND_DIR, env },
  );
  if (viteRes.status !== 0) {
    removeTempDir(outDir);
    assert.fail(
      `vite build failed (exit ${viteRes.status}):\n${viteRes.stdout}\n${viteRes.stderr}`,
    );
  }

  return outDir;
}

function parseOpenApiWithPython() {
  const openApiPath = path.join(DOCS_DIR, "openapi.yaml");
  assert.ok(
    fs.existsSync(openApiPath),
    `Expected OpenAPI spec at ${openApiPath}`,
  );
  const pyScript = `
import json, sys, yaml
with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = yaml.safe_load(f)
assert isinstance(data, dict), "OpenAPI root must be a YAML mapping"
print(json.dumps(data))
`;
  const res = runCommand("python3", ["-c", pyScript, openApiPath]);
  assert.equal(
    res.status,
    0,
    `Failed to parse docs/openapi.yaml as valid YAML:\n${res.stderr}\n${res.stdout}`,
  );
  return JSON.parse(res.stdout);
}

function getFreePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.unref();
    srv.on("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      srv.close((err) => (err ? reject(err) : resolve(port)));
    });
  });
}

function httpGetText(url) {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => {
        resolve({
          statusCode: res.statusCode || 0,
          headers: res.headers,
          body: Buffer.concat(chunks).toString("utf8"),
        });
      });
    });
    req.on("error", reject);
    req.setTimeout(5000, () => {
      req.destroy(new Error(`Timeout fetching ${url}`));
    });
  });
}

async function waitForHttpReady(url, child, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  let lastErr = null;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(
        `Preview server exited early with code ${child.exitCode}`,
      );
    }
    try {
      const res = await httpGetText(url);
      if (res.statusCode >= 200 && res.statusCode < 500) {
        return res;
      }
    } catch (err) {
      lastErr = err;
    }
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error(
    `Timed out waiting for ${url}: ${lastErr ? lastErr.message : "unknown"}`,
  );
}

async function startVitePreview(outDir, port) {
  const viteBin = path.join(FRONTEND_DIR, "node_modules", ".bin", "vite");
  const child = spawn(
    viteBin,
    [
      "preview",
      "--outDir",
      outDir,
      "--host",
      "127.0.0.1",
      "--port",
      String(port),
      "--strictPort",
    ],
    {
      cwd: FRONTEND_DIR,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  let logs = "";
  child.stdout.on("data", (d) => {
    logs += d.toString();
  });
  child.stderr.on("data", (d) => {
    logs += d.toString();
  });

  try {
    const baseUrl = `http://127.0.0.1:${port}`;
    await waitForHttpReady(`${baseUrl}/`, child, 15000);
    return { child, baseUrl, getLogs: () => logs };
  } catch (err) {
    if (child.exitCode === null) {
      child.kill("SIGTERM");
    }
    throw new Error(`${err.message}\nPreview logs:\n${logs}`);
  }
}

async function stopChildProcess(child) {
  if (!child || child.exitCode !== null) return;
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      if (child.exitCode === null) child.kill("SIGKILL");
      resolve();
    }, 3000);
    child.once("exit", () => {
      clearTimeout(timer);
      resolve();
    });
    child.kill("SIGTERM");
  });
}

function getAllFrontendSourceTexts() {
  const files = listFilesRecursive(FRONTEND_SRC_DIR, (f) =>
    /\.(ts|tsx|css)$/.test(f),
  );
  return files.map((file) => ({
    file,
    rel: path.relative(REPO_ROOT, file),
    content: fs.readFileSync(file, "utf8"),
  }));
}

function stripGoComments(source) {
  let out = "";
  let i = 0;
  const len = source.length;
  while (i < len) {
    const ch = source[i];
    const next = source[i + 1];
    if (ch === '"') {
      out += ch;
      i++;
      while (i < len) {
        const c = source[i];
        out += c;
        i++;
        if (c === "\\") {
          if (i < len) {
            out += source[i];
            i++;
          }
        } else if (c === '"') {
          break;
        }
      }
    } else if (ch === "`") {
      out += ch;
      i++;
      while (i < len) {
        const c = source[i];
        out += c;
        i++;
        if (c === "`") break;
      }
    } else if (ch === "'" ) {
      out += ch;
      i++;
      while (i < len) {
        const c = source[i];
        out += c;
        i++;
        if (c === "\\") {
          if (i < len) {
            out += source[i];
            i++;
          }
        } else if (c === "'") {
          break;
        }
      }
    } else if (ch === "/" && next === "/") {
      i += 2;
      while (i < len && source[i] !== "\n") {
        i++;
      }
    } else if (ch === "/" && next === "*") {
      i += 2;
      while (i < len && !(source[i] === "*" && source[i + 1] === "/")) {
        if (source[i] === "\n") out += "\n";
        i++;
      }
      if (i < len) i += 2;
    } else {
      out += ch;
      i++;
    }
  }
  return out;
}

function extractHtmlTagAttribute(tag, attrName) {
  const re = new RegExp(`(?:^|\\s)${attrName}=["']([^"']+)["']`, "i");
  const match = tag.match(re);
  return match ? match[1] : null;
}

function extractServedHtmlAssets(html) {
  const jsUrls = new Set();
  const cssUrls = new Set();

  for (const m of html.matchAll(/<script\b[^>]*>/gi)) {
    const src = extractHtmlTagAttribute(m[0], "src");
    if (src) jsUrls.add(src);
  }

  for (const m of html.matchAll(/<link\b[^>]*>/gi)) {
    const tag = m[0];
    const rel = (extractHtmlTagAttribute(tag, "rel") || "").toLowerCase();
    const href = extractHtmlTagAttribute(tag, "href");
    if (!href) continue;
    const relTokens = rel.split(/\s+/);
    if (
      relTokens.includes("modulepreload") ||
      (relTokens.includes("preload") && /\.m?js(\?|$)/i.test(href))
    ) {
      jsUrls.add(href);
    } else if (relTokens.includes("stylesheet")) {
      cssUrls.add(href);
    }
  }

  return { jsPaths: [...jsUrls], cssPaths: [...cssUrls] };
}

function getGoNonTestFiles(dir) {
  return listFilesRecursive(
    dir,
    (f, name) => name.endsWith(".go") && !name.endsWith("_test.go"),
  ).map((file) => ({
    file,
    rel: path.relative(REPO_ROOT, file),
    content: fs.readFileSync(file, "utf8"),
  }));
}

// ============================================================================
// Tier 1 — Feature Coverage (F1, F2, F7, F8)
// ============================================================================
describe("Tier 1 — Feature Coverage", () => {
  describe("F1: SDD Documentation Reorganization", () => {
    it("T1-F1-01: specs/ directory exists", () => {
      assert.ok(
        fs.existsSync(SPECS_DIR) && fs.statSync(SPECS_DIR).isDirectory(),
        "Expected specs/ directory to exist at repo root",
      );
    });

    it("T1-F1-02: specs/ contains constitution.md, spec.md, plan.md, and tasks.md with markdown headings", () => {
      for (const name of ["constitution.md", "spec.md", "plan.md", "tasks.md"]) {
        const full = path.join(SPECS_DIR, name);
        assert.ok(fs.existsSync(full), `Missing SDD document: ${full}`);
        const content = fs.readFileSync(full, "utf8");
        assert.match(
          content,
          /^#{1,6}\s+\S+/m,
          `SDD document ${name} must contain substantive markdown headings`,
        );
      }
    });

    it("T1-F1-03: docs/steps/ directory has been removed", () => {
      const stepsDir = path.join(DOCS_DIR, "steps");
      assert.equal(
        fs.existsSync(stepsDir),
        false,
        "Expected legacy docs/steps/ directory to be removed after SDD consolidation",
      );
    });

    it("T1-F1-04: docs/ retains DEVELOPMENT_WORKFLOW.md, SOURCES_DEVELOPEMENT.md, and openapi.yaml", () => {
      for (const name of [
        "DEVELOPMENT_WORKFLOW.md",
        "SOURCES_DEVELOPEMENT.md",
        "openapi.yaml",
      ]) {
        const full = path.join(DOCS_DIR, name);
        assert.ok(fs.existsSync(full), `Expected ${full} to be retained`);
        assert.ok(
          fs.statSync(full).size > 0,
          `Expected ${full} to be non-empty`,
        );
      }
    });

    it("T1-F1-05: docs/openapi.yaml parses as valid YAML and documents /api/v1/targets and /api/v1/check-logs", () => {
      const spec = parseOpenApiWithPython();
      assert.ok(
        spec.paths && typeof spec.paths === "object",
        "OpenAPI spec must define a paths object",
      );
      const pathKeys = Object.keys(spec.paths);
      assert.ok(
        pathKeys.includes("/api/v1/targets"),
        `OpenAPI paths missing /api/v1/targets (found: ${pathKeys.join(", ")})`,
      );
      assert.ok(
        pathKeys.includes("/api/v1/check-logs"),
        `OpenAPI paths missing /api/v1/check-logs (found: ${pathKeys.join(", ")})`,
      );
    });
  });

  describe("F2: Go Unit Tests & Binary Builds", () => {
    it("T1-F2-01: go test -race ./... passes cleanly without a live MongoDB daemon", () => {
      const res = runCommand("go", ["test", "-race", "./..."], {
        env: goEnv({
          DB_URI: "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=200",
        }),
      });
      assert.equal(
        res.status,
        0,
        `go test -race ./... failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
      );
    });

    it("T1-F2-02: go build succeeds for ./cmd/api and ./cmd/opspulse", () => {
      for (const targetPkg of ["./cmd/api", "./cmd/opspulse"]) {
        const res = runCommand(
          "go",
          ["build", "-o", "/dev/null", targetPkg],
          { env: goEnv() },
        );
        assert.equal(
          res.status,
          0,
          `go build ${targetPkg} failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
        );
      }
    });
  });

  describe("F7: Frontend R3 Requirements", () => {
    it("T1-F7-01: axios is declared in web/opspulse-frontend/package.json dependencies", () => {
      const pkg = readFrontendPackageJson();
      assert.ok(
        pkg.dependencies && typeof pkg.dependencies.axios === "string",
        "Expected 'axios' in web/opspulse-frontend/package.json dependencies",
      );
    });

    it("T1-F7-02: web/opspulse-frontend/.env.example (or .env) documents API base URL env var (VITE_*)", () => {
      const candidates = [
        path.join(FRONTEND_DIR, ".env.example"),
        path.join(FRONTEND_DIR, ".env"),
      ];
      const existing = candidates.filter((p) => fs.existsSync(p));
      assert.ok(
        existing.length > 0,
        "Expected web/opspulse-frontend/.env.example (or .env) to exist",
      );
      const combined = existing
        .map((p) => fs.readFileSync(p, "utf8"))
        .join("\n");
      assert.match(
        combined,
        /^VITE_[A-Z0-9_]*(API|URL|BASE)[A-Z0-9_]*=/m,
        "Expected VITE_* API base URL variable documented in frontend .env.example/.env",
      );
    });

    it("T1-F7-03: npm --prefix web/opspulse-frontend run lint (oxlint) exits 0", () => {
      assertFrontendToolchainInstalled();
      const res = runCommand("npm", [
        "--prefix",
        "web/opspulse-frontend",
        "run",
        "lint",
      ]);
      assert.equal(
        res.status,
        0,
        `Frontend lint failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
      );
    });

    it("T1-F7-04: tsc -b and vite build --outDir <tmp> succeed with sentinel VITE_API_BASE_URL", () => {
      const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
      try {
        assert.ok(
          fs.existsSync(path.join(outDir, "index.html")),
          "Expected built index.html inside temporary --outDir",
        );
      } finally {
        removeTempDir(outDir);
      }
    });

    it("T1-F7-05: built JS bundle contains injected sentinel API URL and axios", () => {
      const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
      try {
        const assetsDir = path.join(outDir, "assets");
        const jsFiles = listFilesRecursive(assetsDir, (f) => f.endsWith(".js"));
        assert.ok(
          jsFiles.length > 0,
          `Expected compiled JS assets in ${assetsDir}`,
        );
        const allJs = jsFiles
          .map((f) => fs.readFileSync(f, "utf8"))
          .join("\n");
        assert.ok(
          allJs.includes(SENTINEL_API_URL),
          `Expected compiled JS bundle to include sentinel URL ${SENTINEL_API_URL}`,
        );
        assert.match(
          allJs,
          /axios/i,
          "Expected compiled JS bundle to include axios client code",
        );
      } finally {
        removeTempDir(outDir);
      }
    });

    it("T1-F7-06: frontend source implements Target create/update modal, Card layout, and Checklogs page with filters & insights", () => {
      const files = getAllFrontendSourceTexts();
      const tsFiles = files.filter((f) => /\.(ts|tsx)$/.test(f.file));

      // 1. Target create/update modal (UI and HTTP service module may be decoupled)
      const hasModalUi = tsFiles.some(
        (f) =>
          (/modal|dialog/i.test(f.rel) ||
            /role=["']dialog["']|fixed inset-0/.test(f.content)) &&
          /\bname\b/i.test(f.content) &&
          /\burl\b/i.test(f.content) &&
          /\bmethod\b/i.test(f.content),
      );
      assert.ok(
        hasModalUi,
        "Expected Target create/update modal UI component with target form fields (name, url, method)",
      );

      const hasTargetCrudApi = tsFiles.some(
        (f) =>
          /\b(post|put|patch)\s*(?:<[^>()]+>)?\s*\(/i.test(f.content) &&
          /\/targets/.test(f.content),
      );
      assert.ok(
        hasTargetCrudApi,
        "Expected Target CRUD API client calls (post/put/patch on /targets) in modal or service module",
      );

      // 2. Card-based main target layout (excluding StatCards, CheckLog, Insight, and Modal files)
      const hasCardLayout = tsFiles.some(
        (f) =>
          !/(StatCards|Check-?Log|Insight|Modal)/i.test(f.rel) &&
          (/card/i.test(f.rel) || /grid-cols/i.test(f.content)) &&
          /\/targets|Target|CheckResult/.test(f.content) &&
          !/<table[\s>]/i.test(f.content),
      );
      assert.ok(
        hasCardLayout,
        "Expected Card-based responsive grid layout for monitored targets (excluding StatCards, CheckLog, Insight, and Modal files)",
      );

      // 3. Checklogs page with filters and insights (insights may live in a dedicated insights component/helper)
      const checklogFiles = tsFiles.filter(
        (f) =>
          /check-?logs?/i.test(f.rel) ||
          /check-logs/i.test(f.content),
      );
      assert.ok(
        checklogFiles.length > 0,
        "Expected Checklogs page/service consuming /check-logs",
      );
      const checklogCombined = checklogFiles.map((f) => f.content).join("\n");
      const checklogUiCombined = checklogFiles
        .filter((f) => !/Modal/i.test(f.rel))
        .map((f) => f.content)
        .join("\n");
      const hasFilters =
        /(is_up|status_code|target_id|search|from|to)/i.test(checklogCombined) &&
        /(<select|<input)/i.test(checklogUiCombined);
      assert.ok(
        hasFilters,
        "Expected Checklogs page to expose interactive filters (status, target, status_code, search, or date range)",
      );

      const insightsFiles = tsFiles.filter(
        (f) =>
          !/StatCards\.tsx$/i.test(f.rel) &&
          (/check-?logs?/i.test(f.rel) ||
            /insight/i.test(f.rel) ||
            /check-logs/i.test(f.content)),
      );
      const insightsCombined = insightsFiles.map((f) => f.content).join("\n");
      const hasInsights =
        /(uptime|p50|p95|p99|percentil|avg|average|m[eé]dia|success_?rate|failure_?rate|error_?rate|insight|disponibilidade|taxa\s+de\s+(?:sucesso|erro|falha)|\.reduce\s*\()/i.test(
          insightsCombined,
        );
      assert.ok(
        hasInsights,
        "Expected Checklogs/Insights component or helper to compute insights (uptime, latency percentiles/averages, failure/error rates) from check_logs",
      );
    });
  });

  describe("F8: Code Quality & Dependency Discipline (R4)", () => {
    it("T1-F8-01: go vet ./... passes with zero issues", () => {
      const res = runCommand("go", ["vet", "./..."], { env: goEnv() });
      assert.equal(
        res.status,
        0,
        `go vet ./... failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
      );
    });

    it("T1-F8-02: web/opspulse-frontend/package.json contains no bloat or redundant packages", () => {
      const pkg = readFrontendPackageJson();
      const allDeps = {
        ...(pkg.dependencies || {}),
        ...(pkg.devDependencies || {}),
      };
      const forbidden = [
        "lodash",
        "lodash-es",
        "moment",
        "jquery",
        "underscore",
        "ramda",
        "ky",
        "got",
        "superagent",
        "node-fetch",
        "@mui/material",
        "antd",
        "@chakra-ui/react",
        "bootstrap",
        "react-bootstrap",
        "semantic-ui-react",
        "primereact",
      ];
      const found = forbidden.filter((name) => Object.hasOwn(allDeps, name));
      assert.deepEqual(
        found,
        [],
        `Found forbidden bloat/redundant packages in frontend package.json: ${found.join(", ")}`,
      );
    });

    it("T1-F8-03: go.mod direct requires contain no unnecessary bloat libraries", () => {
      const goMod = fs.readFileSync(path.join(REPO_ROOT, "go.mod"), "utf8");
      const directLines = goMod
        .split(/\r?\n/)
        .filter((line) => /^\s+\S+\s+v/.test(line) && !line.includes("// indirect"));
      const directModules = directLines.map((l) => l.trim().split(/\s+/)[0]);

      const bloatPatterns = [
        /^github\.com\/gin-gonic\/gin$/,
        /^github\.com\/labstack\/echo/,
        /^github\.com\/gofiber\/fiber/,
        /^gorm\.io\//,
        /^github\.com\/sirupsen\/logrus$/,
        /^go\.uber\.org\/zap$/,
        /^github\.com\/go-resty\/resty/,
        /^github\.com\/samber\/lo$/,
        /^go\.mongodb\.org\/mongo-driver$/,
      ];
      const flagged = directModules.filter((mod) =>
        bloatPatterns.some((re) => re.test(mod)),
      );
      assert.deepEqual(
        flagged,
        [],
        `Found unnecessary/redundant direct Go dependencies in go.mod: ${flagged.join(", ")}`,
      );
    });

    it("T1-F8-04: no hardcoded Access-Control-Allow-Origin: * in internal/api/", () => {
      const apiFiles = getGoNonTestFiles(INTERNAL_API_DIR);
      assert.ok(apiFiles.length > 0, "Expected Go files in internal/api/");
      const offenders = [];
      for (const { rel, content } of apiFiles) {
        if (/"Access-Control-Allow-Origin"\s*,\s*"\*"/.test(content)) {
          offenders.push(rel);
        }
      }
      assert.deepEqual(
        offenders,
        [],
        `Found hardcoded wildcard Access-Control-Allow-Origin "*" in: ${offenders.join(", ")}`,
      );
    });

    it("T1-F8-05: root .env.example documents CORS_ALLOWED_ORIGINS", () => {
      const envExample = fs.readFileSync(
        path.join(REPO_ROOT, ".env.example"),
        "utf8",
      );
      assert.match(
        envExample,
        /^CORS_ALLOWED_ORIGINS\s*=/m,
        "Expected CORS_ALLOWED_ORIGINS documented in root .env.example",
      );
    });
  });
});

// ============================================================================
// Tier 2 — Boundary & Corner Cases
// ============================================================================
describe("Tier 2 — Boundary & Corner Cases", () => {
  describe("F1: SDD Documentation Integrity", () => {
    it("T2-F1-01: none of specs/{constitution,spec,plan,tasks}.md are placeholder stubs", () => {
      for (const name of ["constitution.md", "spec.md", "plan.md", "tasks.md"]) {
        const full = path.join(SPECS_DIR, name);
        assert.ok(fs.existsSync(full), `Missing ${full}`);
        const content = fs.readFileSync(full, "utf8");
        assert.ok(
          Buffer.byteLength(content, "utf8") >= 200,
          `${name} is too small (<200 bytes) and appears to be a stub`,
        );
        assert.equal(
          /^\s*\[?(TBD|TODO)\]?\s*$/m.test(content),
          false,
          `${name} contains standalone TBD/TODO placeholder lines`,
        );
      }
    });

    it("T2-F1-02: no leftover markdown files remain inside docs/steps", () => {
      const stepsDir = path.join(DOCS_DIR, "steps");
      const leftover = listFilesRecursive(stepsDir, (f) => f.endsWith(".md"));
      assert.deepEqual(
        leftover,
        [],
        `Leftover markdown files found in docs/steps: ${leftover.join(", ")}`,
      );
    });

    it("T2-F1-03: docs/openapi.yaml defines POST, PUT, DELETE on targets and error responses", () => {
      const spec = parseOpenApiWithPython();
      const targetsRoot = spec.paths?.["/api/v1/targets"] || {};
      const targetsById = spec.paths?.["/api/v1/targets/{id}"] || {};
      assert.ok(targetsRoot.get && targetsRoot.post, "Expected GET and POST on /api/v1/targets");
      assert.ok(
        targetsById.get && targetsById.put && targetsById.delete,
        "Expected GET, PUT, and DELETE on /api/v1/targets/{id}",
      );

      const allResponses = new Set();
      for (const pathObj of [targetsRoot, targetsById]) {
        for (const op of Object.values(pathObj)) {
          if (op && typeof op === "object" && op.responses) {
            for (const code of Object.keys(op.responses)) {
              allResponses.add(String(code));
            }
          }
        }
      }
      assert.ok(
        [...allResponses].some((c) => /^4\d\d$/.test(c)),
        `Expected 4xx error responses documented for target endpoints (found: ${[...allResponses].join(", ")})`,
      );
    });

    it("T2-F1-04: specs/ files reference key domain concepts (targets, check_logs, MongoDB, jitter, CORS)", () => {
      const mdFiles = listFilesRecursive(SPECS_DIR, (f) => f.endsWith(".md"));
      assert.ok(mdFiles.length >= 4, "Expected at least 4 markdown files in specs/");
      const combined = mdFiles
        .map((f) => fs.readFileSync(f, "utf8"))
        .join("\n");
      for (const term of ["targets", "check_logs", "MongoDB", "jitter", "CORS"]) {
        assert.match(
          combined,
          new RegExp(term, "i"),
          `Expected specs/ documentation to cover domain concept '${term}'`,
        );
      }
    });

    it("T2-F1-05: docs/DEVELOPMENT_WORKFLOW.md remains intact and readable", () => {
      const wfPath = path.join(DOCS_DIR, "DEVELOPMENT_WORKFLOW.md");
      const content = fs.readFileSync(wfPath, "utf8");
      assert.ok(content.length > 500, "DEVELOPMENT_WORKFLOW.md looks truncated");
      assert.match(content, /go test/i);
      assert.match(content, /go build/i);
    });
  });

  describe("F7: Frontend URL Injection, Preview & Layout Boundaries", () => {
    it("T2-F7-01: zero hardcoded backend URLs in web/opspulse-frontend/src/**/*.ts*", () => {
      const tsFiles = getAllFrontendSourceTexts().filter((f) =>
        /\.(ts|tsx)$/.test(f.file),
      );
      const offenders = [];
      const forbiddenPattern =
        /https?:\/\/(localhost|127\.0\.0\.1|api):3333|:3333\b/;
      for (const { rel, content } of tsFiles) {
        if (forbiddenPattern.test(content)) {
          offenders.push(rel);
        }
      }
      assert.deepEqual(
        offenders,
        [],
        `Found hardcoded backend URLs/:3333 in frontend source: ${offenders.join(", ")}`,
      );
    });

    it("T2-F7-02: zero hardcoded :3333 or unintended fallback URLs in compiled JS bundle", () => {
      const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
      try {
        const jsFiles = listFilesRecursive(
          path.join(outDir, "assets"),
          (f) => f.endsWith(".js"),
        );
        const allJs = jsFiles
          .map((f) => fs.readFileSync(f, "utf8"))
          .join("\n");
        assert.equal(
          /(localhost:3333|127\.0\.0\.1:3333|http:\/\/api:3333|:3333\b)/.test(
            allJs,
          ),
          false,
          "Compiled JS bundle must not contain hardcoded :3333 backend URLs",
        );
      } finally {
        removeTempDir(outDir);
      }
    });

    it("T2-F7-03: build succeeds when VITE_API_BASE_URL has a trailing slash and custom path prefix", () => {
      const outDir = runFrontendBuildToTempDir(SENTINEL_PREFIX_URL);
      try {
        const jsFiles = listFilesRecursive(
          path.join(outDir, "assets"),
          (f) => f.endsWith(".js"),
        );
        const allJs = jsFiles
          .map((f) => fs.readFileSync(f, "utf8"))
          .join("\n");
        assert.ok(
          allJs.includes("e2e-sentinel-api.invalid/opspulse"),
          "Expected custom path prefix sentinel URL to be injected into bundle",
        );
      } finally {
        removeTempDir(outDir);
      }
    });

    it("T2-F7-04: vite preview serves / with HTTP 200, <div id=\"root\">, and valid JS/CSS assets", async () => {
      const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
      const port = await getFreePort();
      let preview = null;
      try {
        preview = await startVitePreview(outDir, port);
        const indexRes = await httpGetText(`${preview.baseUrl}/`);
        assert.equal(indexRes.statusCode, 200);
        assert.match(indexRes.body, /<div\s+id=["']root["']\s*>/i);

        const assetMatches = [
          ...indexRes.body.matchAll(/(?:src|href)=["'](\/assets\/[^"']+)["']/g),
        ].map((m) => m[1]);
        assert.ok(
          assetMatches.length > 0,
          "Expected at least one /assets/ script or stylesheet reference in served HTML",
        );
        for (const assetPath of assetMatches) {
          const assetRes = await httpGetText(`${preview.baseUrl}${assetPath}`);
          assert.equal(
            assetRes.statusCode,
            200,
            `Expected HTTP 200 for asset ${assetPath}, got ${assetRes.statusCode}`,
          );
          assert.ok(assetRes.body.length > 0, `Empty asset body for ${assetPath}`);
        }
      } finally {
        await stopChildProcess(preview?.child);
        removeTempDir(outDir);
      }
    });

    it("T2-F7-05: main Dashboard target list does not regress to an HTML <table> layout", () => {
      const dashboardPath = path.join(FRONTEND_SRC_DIR, "pages", "Dashboard.tsx");
      assert.ok(fs.existsSync(dashboardPath), "Expected src/pages/Dashboard.tsx");
      const dashboard = fs.readFileSync(dashboardPath, "utf8");
      assert.equal(
        /TargetTable/.test(dashboard),
        false,
        "Dashboard.tsx must not reference legacy TargetTable component",
      );
      assert.equal(
        /<table[\s>]/i.test(dashboard),
        false,
        "Dashboard.tsx must not render targets using an HTML <table>",
      );
      const targetComponents = listFilesRecursive(
        path.join(FRONTEND_SRC_DIR, "components"),
        (f) => /target/i.test(path.basename(f)) && /\.(ts|tsx)$/.test(f),
      );
      for (const comp of targetComponents) {
        const content = fs.readFileSync(comp, "utf8");
        assert.equal(
          /<table[\s>]/i.test(content),
          false,
          `Target component ${path.basename(comp)} must use cards instead of <table>`,
        );
      }
    });
  });

  describe("F8: Security, Hygiene & Artifact Boundaries", () => {
    it("T2-F8-01: frontend src/ has no console.log spam and passes strict oxlint (--deny-warnings)", () => {
      assertFrontendToolchainInstalled();
      const tsFiles = getAllFrontendSourceTexts().filter((f) =>
        /\.(ts|tsx)$/.test(f.file),
      );
      const consoleLogs = tsFiles
        .filter((f) => /\bconsole\.log\s*\(/.test(f.content))
        .map((f) => f.rel);
      assert.deepEqual(
        consoleLogs,
        [],
        `Found console.log statements in frontend src/: ${consoleLogs.join(", ")}`,
      );

      const oxlintBin = path.join(FRONTEND_DIR, "node_modules", ".bin", "oxlint");
      const res = runCommand(oxlintBin, ["--deny-warnings"], {
        cwd: FRONTEND_DIR,
      });
      assert.equal(
        res.status,
        0,
        `oxlint --deny-warnings failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
      );
    });

    it("T2-F8-02: no hardcoded MongoDB production credentials (mongodb+srv:// or user:pass@) in cmd/, internal/, or web/", () => {
      const scanDirs = [
        path.join(REPO_ROOT, "cmd"),
        path.join(REPO_ROOT, "internal"),
        FRONTEND_SRC_DIR,
      ];
      const offenders = [];
      const credPattern = /mongodb\+srv:\/\/|mongodb:\/\/[^/\s"']+:[^/\s"']+@/;
      for (const dir of scanDirs) {
        for (const file of listFilesRecursive(dir, (f) =>
          /\.(go|ts|tsx|js|jsx)$/.test(f) && !f.endsWith("_test.go"),
        )) {
          const content = fs.readFileSync(file, "utf8");
          if (credPattern.test(content)) {
            offenders.push(path.relative(REPO_ROOT, file));
          }
        }
      }
      assert.deepEqual(
        offenders,
        [],
        `Found hardcoded MongoDB credentials in source files: ${offenders.join(", ")}`,
      );
    });

    it("T2-F8-03: internal/config/ does not hardcode monitoring target URLs in Go source", () => {
      const cfgFiles = getGoNonTestFiles(INTERNAL_CONFIG_DIR);
      assert.ok(cfgFiles.length > 0, "Expected Go files in internal/config/");
      const allowedCorsOriginRe =
        /^http:\/\/(localhost|127\.0\.0\.1):(5173|5174)\/?$/i;
      const corsFieldRe =
        /CORS_ALLOWED_ORIGINS|AllowedOrigins|CORSOrigins/i;
      const targetContextRe = /\b(TargetURLs|domain\.TargetResult|domain\.Target)\b/;

      const offenders = [];
      for (const { rel, content } of cfgFiles) {
        const codeOnly = stripGoComments(content);
        const lines = codeOnly.split(/\r?\n/);
        const isMonitorFile = rel.endsWith(`${path.sep}monitor.go`) || rel === "monitor.go" || rel.endsWith("/monitor.go");

        for (let idx = 0; idx < lines.length; idx++) {
          const line = lines[idx];
          const urlMatches = [...line.matchAll(/https?:\/\/[^\s"',`)]+/g)];
          if (urlMatches.length === 0) continue;

          let isCorsFallbackLine = false;
          if (!isMonitorFile && !targetContextRe.test(line)) {
            for (let back = idx; back >= Math.max(0, idx - 5); back--) {
              if (targetContextRe.test(lines[back])) break;
              if (corsFieldRe.test(lines[back])) {
                isCorsFallbackLine = true;
                break;
              }
            }
          }

          for (const m of urlMatches) {
            const url = m[0];
            if (isCorsFallbackLine && allowedCorsOriginRe.test(url)) {
              continue;
            }
            offenders.push(`${rel}:${idx + 1} (${url})`);
          }
        }
      }
      assert.deepEqual(
        offenders,
        [],
        `Found hardcoded monitoring/non-CORS-dev URLs in internal/config/: ${offenders.join(", ")}`,
      );
    });

    it("T2-F8-04: web/opspulse-frontend/package.json runtime dependencies stay lean (<= 15)", () => {
      const pkg = readFrontendPackageJson();
      const count = Object.keys(pkg.dependencies || {}).length;
      assert.ok(
        count <= 15,
        `Expected <= 15 runtime dependencies in frontend package.json, found ${count}`,
      );
    });

    it("T2-F8-05: .gitignore ignores build/env artifacts and no dist/ or binaries are tracked in git", () => {
      const gitignore = fs.readFileSync(
        path.join(REPO_ROOT, ".gitignore"),
        "utf8",
      );
      for (const entry of ["dist", "node_modules/", ".env"]) {
        assert.ok(
          gitignore.includes(entry),
          `Expected .gitignore to contain '${entry}'`,
        );
      }

      const trackedRes = runCommand("git", ["ls-files"]);
      assert.equal(trackedRes.status, 0, "git ls-files failed");
      const tracked = trackedRes.stdout.split(/\r?\n/).filter(Boolean);
      const forbiddenTracked = tracked.filter(
        (f) =>
          f.includes("/dist/") ||
          f.startsWith("dist/") ||
          f.includes("node_modules/") ||
          /(^|\/)\.env$/.test(f),
      );
      assert.deepEqual(
        forbiddenTracked,
        [],
        `Found tracked build/env artifacts in git: ${forbiddenTracked.join(", ")}`,
      );
    });
  });
});

// ============================================================================
// Tier 3 — Cross-Feature Combinations
// ============================================================================
describe("Tier 3 — Cross-Feature Combinations", () => {
  it("T3-01 (F1+F2+F3): docs/openapi.yaml paths align with internal/api routes and frontend API calls", () => {
    const spec = parseOpenApiWithPython();
    const specPaths = Object.keys(spec.paths || {});
    assert.ok(
      specPaths.includes("/api/v1/targets") &&
        specPaths.includes("/api/v1/check-logs"),
      "OpenAPI spec must define /api/v1/targets and /api/v1/check-logs",
    );

    const apiGo = getGoNonTestFiles(INTERNAL_API_DIR)
      .map((f) => f.content)
      .join("\n");
    assert.match(apiGo, /\/api\/v1/);
    assert.match(apiGo, /\/targets/);
    assert.match(apiGo, /\/check-logs/);

    const frontendSrc = getAllFrontendSourceTexts()
      .filter((f) => /\.(ts|tsx)$/.test(f.file))
      .map((f) => f.content)
      .join("\n");
    assert.match(frontendSrc, /\/targets/);
    assert.match(frontendSrc, /\/check-logs/);
  });

  it("T3-02 (F3+F7+F8): vite preview + sentinel VITE_API_BASE_URL + CORS_ALLOWED_ORIGINS end-to-end alignment", async () => {
    const envExample = fs.readFileSync(
      path.join(REPO_ROOT, ".env.example"),
      "utf8",
    );
    const corsEnvMatch = envExample.match(/^CORS_ALLOWED_ORIGINS=(.*)$/m);
    assert.ok(
      corsEnvMatch && corsEnvMatch[1].trim().length > 0,
      "Expected non-empty CORS_ALLOWED_ORIGINS documented in root .env.example",
    );
    assert.notEqual(
      corsEnvMatch[1].trim(),
      "*",
      "Root .env.example must not set CORS_ALLOWED_ORIGINS=*",
    );

    const serverCfg = fs.readFileSync(
      path.join(INTERNAL_CONFIG_DIR, "server.go"),
      "utf8",
    );
    assert.match(
      serverCfg,
      /CORS_ALLOWED_ORIGINS/,
      "internal/config/server.go must load CORS_ALLOWED_ORIGINS",
    );
    assert.match(
      serverCfg,
      /AllowedOrigins/,
      "internal/config/server.go must expose AllowedOrigins",
    );

    const middleware = fs.readFileSync(
      path.join(INTERNAL_API_DIR, "middleware.go"),
      "utf8",
    );
    assert.match(
      middleware,
      /AllowedOrigins|allowedOrigins/i,
      "internal/api/middleware.go must enforce configured AllowedOrigins",
    );
    assert.equal(
      /"Access-Control-Allow-Origin"\s*,\s*"\*"/.test(middleware),
      false,
      "internal/api/middleware.go must not hardcode wildcard Access-Control-Allow-Origin *",
    );

    const distinctApiOrigin = "https://api-cors-e2e.opspulse.invalid:8443";
    const outDir = runFrontendBuildToTempDir(distinctApiOrigin);
    const port = await getFreePort();
    let preview = null;
    try {
      preview = await startVitePreview(outDir, port);
      const previewOrigin = preview.baseUrl;
      assert.notEqual(
        previewOrigin,
        distinctApiOrigin,
        "Preview origin must be distinct from injected API origin",
      );

      const htmlRes = await httpGetText(`${previewOrigin}/`);
      assert.equal(htmlRes.statusCode, 200);
      assert.match(htmlRes.body, /<div\s+id=["']root["']\s*>/i);

      const { jsPaths } = extractServedHtmlAssets(htmlRes.body);
      assert.ok(
        jsPaths.length > 0,
        "Expected <script src=...> or <link rel=\"modulepreload\" href=...> JS assets in served index.html",
      );

      let combinedServedJs = "";
      for (const jsPath of jsPaths) {
        const assetUrl = jsPath.startsWith("http")
          ? jsPath
          : `${previewOrigin}${jsPath.startsWith("/") ? "" : "/"}${jsPath}`;
        const assetRes = await httpGetText(assetUrl);
        assert.equal(
          assetRes.statusCode,
          200,
          `Expected HTTP 200 for served JS ${assetUrl}`,
        );
        assert.ok(assetRes.body.length > 0, `Empty served JS asset ${assetUrl}`);
        combinedServedJs += "\n" + assetRes.body;
      }

      assert.ok(
        combinedServedJs.includes(distinctApiOrigin),
        `Served JS bundle must include injected distinct API origin ${distinctApiOrigin}`,
      );
      assert.match(
        combinedServedJs,
        /axios/i,
        "Served JS bundle must include axios client code",
      );
      assert.equal(
        /(localhost:3333|127\.0\.0\.1:3333|http:\/\/api:3333|:3333\b)/.test(
          combinedServedJs,
        ),
        false,
        "Served JS bundle must not hardcode :3333 backend URLs",
      );
    } finally {
      await stopChildProcess(preview?.child);
      removeTempDir(outDir);
    }
  });
});

// ============================================================================
// Tier 4 — Real-World Application Scenarios
// ============================================================================
describe("Tier 4 — Real-World Application Scenarios", () => {
  it("T4-Scenario-3: Cross-stack build with sentinel VITE_API_BASE_URL, live vite preview HTTP serving, and asset verification", async () => {
    const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
    const port = await getFreePort();
    let preview = null;
    try {
      preview = await startVitePreview(outDir, port);

      const htmlRes = await httpGetText(`${preview.baseUrl}/`);
      assert.equal(htmlRes.statusCode, 200);
      assert.match(htmlRes.body, /<div\s+id=["']root["']\s*>/i);

      const { jsPaths, cssPaths } = extractServedHtmlAssets(htmlRes.body);
      assert.ok(
        jsPaths.length > 0,
        "Expected <script src=...> or <link rel=\"modulepreload\" href=...> JS entries in served index.html",
      );

      let combinedServedJs = "";
      for (const jsPath of jsPaths) {
        const assetUrl = jsPath.startsWith("http")
          ? jsPath
          : `${preview.baseUrl}${jsPath.startsWith("/") ? "" : "/"}${jsPath}`;
        const assetRes = await httpGetText(assetUrl);
        assert.equal(
          assetRes.statusCode,
          200,
          `Expected HTTP 200 for served JS ${assetUrl}`,
        );
        assert.ok(assetRes.body.length > 0, `Empty served JS ${assetUrl}`);
        combinedServedJs += "\n" + assetRes.body;
      }

      for (const cssPath of cssPaths) {
        const cssUrl = cssPath.startsWith("http")
          ? cssPath
          : `${preview.baseUrl}${cssPath.startsWith("/") ? "" : "/"}${cssPath}`;
        const cssRes = await httpGetText(cssUrl);
        assert.equal(
          cssRes.statusCode,
          200,
          `Expected HTTP 200 for served stylesheet ${cssUrl}`,
        );
        assert.ok(cssRes.body.length > 0, `Empty served stylesheet ${cssUrl}`);
      }

      assert.ok(
        combinedServedJs.includes(SENTINEL_API_URL),
        `Served JS bundle must include injected ${SENTINEL_API_URL}`,
      );
      assert.match(
        combinedServedJs,
        /axios/i,
        "Served JS bundle must contain axios client code",
      );
    } finally {
      await stopChildProcess(preview?.child);
      removeTempDir(outDir);
    }
  });

  it("T4-Scenario-5: Full workspace production readiness gate (SDD docs + Go vet/test/build + Frontend lint/build)", () => {
    // 1. SDD Docs
    for (const name of ["constitution.md", "spec.md", "plan.md", "tasks.md"]) {
      assert.ok(
        fs.existsSync(path.join(SPECS_DIR, name)),
        `Missing specs/${name}`,
      );
    }
    assert.equal(
      fs.existsSync(path.join(DOCS_DIR, "steps")),
      false,
      "docs/steps/ must be removed",
    );

    // 2. Go vet, test -race, and builds
    for (const [label, args] of [
      ["go vet ./...", ["vet", "./..."]],
      ["go test -race ./...", ["test", "-race", "./..."]],
      ["go build ./cmd/api", ["build", "-o", "/dev/null", "./cmd/api"]],
      [
        "go build ./cmd/opspulse",
        ["build", "-o", "/dev/null", "./cmd/opspulse"],
      ],
    ]) {
      const res = runCommand("go", args, {
        env: goEnv({
          DB_URI: "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=200",
        }),
      });
      assert.equal(
        res.status,
        0,
        `${label} failed (exit ${res.status}):\n${res.stdout}\n${res.stderr}`,
      );
    }

    // 3. Frontend lint & build
    assertFrontendToolchainInstalled();
    const lintRes = runCommand("npm", [
      "--prefix",
      "web/opspulse-frontend",
      "run",
      "lint",
    ]);
    assert.equal(
      lintRes.status,
      0,
      `Frontend lint failed (exit ${lintRes.status}):\n${lintRes.stdout}\n${lintRes.stderr}`,
    );

    const outDir = runFrontendBuildToTempDir(SENTINEL_API_URL);
    try {
      assert.ok(fs.existsSync(path.join(outDir, "index.html")));
    } finally {
      removeTempDir(outDir);
    }
  });
});
