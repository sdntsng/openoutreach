import fs from "node:fs";

const dest = "src/container-env.ts";
const example = "src/container-env.example.ts";
const revision =
  process.env.WORKERS_CI_COMMIT_SHA ||
  process.env.GITHUB_SHA ||
  process.env.CF_PAGES_COMMIT_SHA ||
  "";

if (!fs.existsSync(dest)) {
  fs.copyFileSync(example, dest);
}

if (!revision) {
  process.exit(0);
}

const src = fs.readFileSync(dest, "utf8");
const line = `  CONTAINER_BOOT_REVISION: ${JSON.stringify(revision)},\n`;

if (src.includes("CONTAINER_BOOT_REVISION")) {
  fs.writeFileSync(
    dest,
    src.replace(
      /CONTAINER_BOOT_REVISION:\s*(["'`])(?:\\.|[^\\])*?\1,?/,
      `CONTAINER_BOOT_REVISION: ${JSON.stringify(revision)},`,
    ),
  );
  process.exit(0);
}

const replaced = src.replace(
  /export const containerEnv: Record<string, string> = \{([^}]*)\};/,
  (_m, body) =>
    `export const containerEnv: Record<string, string> = {${body}${line}};`,
);

if (replaced === src) {
  fs.writeFileSync(
    dest,
    `/** Generated for Workers Builds / CI. */\nexport const containerEnv: Record<string, string> = {\n${line}};\n`,
  );
  process.exit(0);
}

fs.writeFileSync(dest, replaced);
