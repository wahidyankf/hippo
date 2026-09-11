#!/usr/bin/env node
// ==============================================================================
// generate-adapters.mjs — write every repo-local harness adapter from the canon
// ==============================================================================
// The canonical files under `.agents/` are what a human edits. An adapter is
// generated from them and never edited in place, because an adapter that can be
// edited is a second place for the rule to live and the two will disagree.
//
// This writes; it checks nothing. `./rhino harness parity validate` decides
// whether what is on disk matches what `repo-config.yml` declares, and keeping
// the writer and the judge apart is what makes the judge worth running.
//
// Usage: node scripts/generate-adapters.mjs
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";

const ROOT = execFileSync("git", ["rev-parse", "--show-toplevel"], {
  encoding: "utf8",
}).trim();

const SKILL_ROUTE = (p) =>
  `Read ${p} completely, resolve every relative resource from that skill directory, and follow it as authoritative before acting.`;
const AGENT_ROUTE = (p) =>
  `Before acting, read the complete canonical agent definition at the repository-root path ${p} and follow it as authoritative. If it cannot be read, stop and report the missing path.`;

// The canonical capability vocabulary in each harness's own words. Every table
// mirrors `harness-parity.harnesses[].agent-adapter.translations`; the `""` key
// is the `always` row. Denials are projected too, because this repository's
// declaration names them explicitly rather than inferring them from silence.
const CLAUDE_TOOLS = {
  requires: {
    "": ["Read"],
    "repository-read": ["Glob", "Grep"],
    "repository-write": ["Write", "Edit"],
    shell: ["Bash"],
  },
  denies: {},
};
const OPENCODE_PERMISSIONS = {
  requires: {
    "": [["read", "allow"]],
    "repository-read": [
      ["glob", "allow"],
      ["grep", "allow"],
    ],
    "repository-write": [["edit", "allow"]],
    shell: [["bash", "allow"]],
  },
  denies: {
    "repository-write": [["edit", "deny"]],
    shell: [["bash", "deny"]],
    "nested-agent": [["task", "deny"]],
  },
};
// Codex states the whole boundary in one scalar, so it is projected from the
// one capability that decides it rather than pinned for every agent.
const CODEX_SANDBOX = {
  requires: { "repository-write": "workspace-write" },
  denies: { "repository-write": "read-only" },
};

/// Front matter as written, not as a decoder would rather have it: the key
/// order and the folded scalars are what the adapter has to carry forward.
function frontMatter(file) {
  const text = readFileSync(file, "utf8");
  if (!text.startsWith("---\n")) return {};
  const end = text.indexOf("\n---\n", 3);
  const values = {};
  let key = null;
  for (const line of text.slice(4, end + 1).split("\n")) {
    if (!line.trim()) continue;
    if (line.startsWith("  - ")) {
      if (!Array.isArray(values[key])) values[key] = [];
      values[key].push(line.slice(4).trim());
    } else if (line.startsWith("  ")) {
      values[key] = `${values[key] ?? ""} ${line.trim()}`.trim();
    } else {
      const [name, ...rest] = line.split(":");
      const value = rest.join(":").trim();
      key = name;
      values[key] = [">-", ">", "|", "|-"].includes(value) ? "" : value;
    }
  }
  return values;
}

function write(rel, text) {
  const file = path.join(ROOT, rel);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, text);
  process.stdout.write(`[adapters] wrote ${rel}\n`);
}

const list = (value) => (Array.isArray(value) ? value : []);

const skills = readdirSync(path.join(ROOT, ".agents/skills"), {
  withFileTypes: true,
})
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name)
  .sort();

for (const name of skills) {
  const canon = `.agents/skills/${name}/SKILL.md`;
  const fm = frontMatter(path.join(ROOT, canon));
  write(
    `.claude/skills/${name}/SKILL.md`,
    `---\nname: ${fm.name}\ndescription: ${fm.description}\n---\n\n${SKILL_ROUTE(canon)}\n`,
  );
}

const agents = readdirSync(path.join(ROOT, ".agents/agents"))
  .filter((file) => file.endsWith(".md") && file !== "README.md")
  .map((file) => file.slice(0, -3))
  .sort();

for (const name of agents) {
  const canon = `.agents/agents/${name}.md`;
  const fm = frontMatter(path.join(ROOT, canon));
  const requires = ["", ...list(fm.requires)];
  const denies = list(fm.denies);
  const route = AGENT_ROUTE(canon);
  const project = (table) => [
    ...requires.flatMap((c) => table.requires[c] ?? []),
    ...denies.flatMap((c) => table.denies[c] ?? []),
  ];

  write(
    `.claude/agents/${name}.md`,
    [
      "---",
      `name: ${fm.name}`,
      `description: ${fm.description}`,
      `tools: ${project(CLAUDE_TOOLS).join(", ")}`,
      "---",
      "",
      route,
      "",
    ].join("\n"),
  );

  const sandbox =
    requires.map((c) => CODEX_SANDBOX.requires[c]).find(Boolean) ??
    denies.map((c) => CODEX_SANDBOX.denies[c]).find(Boolean);
  write(
    `.codex/agents/${name}.toml`,
    [
      `name = "${fm.name}"`,
      `description = "${fm.description.replaceAll('"', '\\"')}"`,
      ...(sandbox ? [`sandbox_mode = "${sandbox}"`] : []),
      `developer_instructions = """\n${route}\n"""`,
      "",
    ].join("\n"),
  );

  const permissions = new Map(project(OPENCODE_PERMISSIONS));
  write(
    `.opencode/agents/${name}.md`,
    [
      "---",
      `description: ${fm.description}`,
      "mode: subagent",
      "permission:",
      ...[...permissions].map(([key, value]) => `  ${key}: ${value}`),
      "---",
      "",
      route,
      "",
    ].join("\n"),
  );
}
