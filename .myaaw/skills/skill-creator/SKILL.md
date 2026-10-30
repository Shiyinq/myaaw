---
name: skill-creator
description: Create, update, or validate a Myaaw Agent Skill (a folder with SKILL.md plus optional scripts, references, and assets). Use when the user asks to add a new skill, write or review a SKILL.md, package a workflow as a reusable skill, or scaffold skill scripts.
compatibility: Requires the filesystem and bash tools. The helper scripts need Python 3.9+ with the standard library only.
metadata:
  standard: https://agentskills.io/specification
  version: "1.0"
---

# Skill Creator

Use this skill to build a new Agent Skill for the current Myaaw agent, or to
review and fix an existing one. It follows the Agent Skills open standard:
<https://agentskills.io/home> and <https://agentskills.io/specification>.

## What a skill is

A skill is a folder with a `SKILL.md` file. The file has YAML frontmatter
(`name` and `description` at minimum) followed by Markdown instructions. A
skill can bundle executable code and reference material:

```text
<skill-name>/
├── SKILL.md          # Required: metadata + instructions
├── scripts/          # Optional: executable code
├── references/       # Optional: documentation loaded on demand
├── assets/           # Optional: templates, data files, images
└── ...               # Any extra files
```

Myaaw loads skills with progressive disclosure:

1. **Discovery** — only `name` and `description` are injected into the system
   prompt, under `# Agent Skills`.
2. **Activation** — when a task matches the description, the agent reads the
   full `SKILL.md` with the `filesystem` tool.
3. **Execution** — the agent runs bundled scripts with `bash` or
   `execute_python`, and reads `references/` only when needed.

## Where skills live in Myaaw

Skills are **per agent**:

```text
~/.myaaw/<agent>/skills/<skill-name>/
```

The exact absolute path is already in your prompt, in the `# Agent Skills`
section (for example `/home/you/.myaaw/agent-b/skills`). Use that path in
commands instead of hardcoding `~/.myaaw/skills`, because that shorthand is
only correct for the legacy `default` agent.

**Boundaries:** a new skill must live in the current agent's own skills
directory. Never create it under another agent (`~/.myaaw/other-agent/skills`)
and never fall back to `~/.myaaw/skills`. The helper scripts enforce this: they
resolve their own location and refuse a different directory.

A skill is available when its folder exists. Two ways to turn one off without
deleting it:

- `skills.json` in the same skills directory: `{"<skill-name>": false}`
- `enabled: false` in the skill's own frontmatter

Onboarding also installs bundled skills (`myaaw onboard` → **Skills** step;
headless: `--skills name1,name2`), and it never deletes a skill folder.

## Workflow

### 1. Clarify the job

Before writing files, know:

- What task the skill performs, and the concrete triggers that should activate
  it (user phrases, file types, tools).
- Whether it needs a script, or instructions and reference material are enough.
- Any external dependency (network, CLI, API key) and where it comes from.

### 2. Choose the name

The folder name and the frontmatter `name` must match:

- 1–64 characters, lowercase letters, digits, and single hyphens.
- Must not start or end with a hyphen, and must not contain `--`.
- Examples: `pdf-processing`, `unit-converter`, `release-notes`.

### 3. Scaffold the folder

Use the bundled scaffolder so the layout and frontmatter are correct:

```bash
python3 <skill-dir>/scripts/new_skill.py \
  --name pdf-processing \
  --description "Extract text and tables from PDFs. Use when the user mentions PDFs, forms, or document extraction."
```

`--dir` is optional and defaults to the skills directory that contains this
skill. Passing any other directory is rejected, so a new skill cannot land in
another agent's folder. Pass `--force` only when you intend to overwrite an
existing `SKILL.md`.

It creates `<skills-dir>/pdf-processing/SKILL.md` plus `scripts/`,
`references/`, and `assets/`. Writing the files by hand is fine too; keep the
same layout.

### 4. Write the frontmatter

`SKILL.md` must start with YAML frontmatter.

| Field | Required | Constraints |
| :--- | :--- | :--- |
| `name` | Yes | 1–64 chars, lowercase `a-z0-9-`, no leading/trailing hyphen, no `--`, must equal the folder name |
| `description` | Yes | 1–1024 chars; say **what it does and when to use it**, with keywords the user would say |
| `license` | No | Short license name or a bundled license file |
| `compatibility` | No | 1–500 chars; environment requirements (tools, packages, network) |
| `metadata` | No | Map of string keys to string values (for example `author`, `version`) |
| `allowed-tools` | No | Experimental, space-separated pre-approved tools |
| `enabled` | No | Myaaw extension; `false` hides the skill without deleting it |

A strong description is the most important part — it is the only thing the
agent sees before deciding to load the skill.

```yaml
---
name: pdf-processing
description: Extract text and tables from PDF files, fill PDF forms, and merge PDFs. Use when the user mentions PDFs, forms, or document extraction.
---
```

### 5. Write the body

Keep `SKILL.md` under ~500 lines. The agent reads the whole file when the skill
activates, so every line costs context. Recommended sections:

- **Usage** — the exact command to run, with a path that works for this agent.
- **Arguments** — table of fields, types, required/default, and meaning.
- **Examples** — realistic command lines and expected output.
- **Errors** — what the script reports and what the agent should do next.
- **Safety** — confirmation rules for destructive or paid actions.

When referencing files, use paths relative to the skill root and keep them one
level deep:

```markdown
See [the frontmatter reference](references/frontmatter.md).
Run `scripts/convert.py`.
```

For scripts, show how to invoke them with the skill's absolute path:

```bash
python3 <skill-dir>/scripts/convert.py '{"action":"convert","value":10,"from":"km","to":"m"}'
```

### 6. Write scripts carefully

- Read arguments from `sys.argv`/`process.argv`; prefer one JSON string or
  named flags, and validate before doing work.
- Print a single JSON object to stdout so the agent can reason about the result.
- Write errors to stderr and exit non-zero; never print a traceback as the only
  output.
- Keep scripts self-contained or document dependencies in `compatibility`, and
  list install commands in the body.
- Never embed secrets. Read them from the environment or the agent's `.env`.
- Make read-only operations safe to retry; require explicit confirmation for
  destructive ones.
- Support `--help` and give a clear message when arguments are missing.

Prefer `scripts/` for code, `references/` for long documentation, and `assets/`
for templates the skill copies or fills in.

### 7. Validate

Run the bundled validator:

```bash
python3 <skill-dir>/scripts/validate_skill.py <skills-dir>/pdf-processing
```

It checks the folder name, frontmatter constraints, required fields, and that
the body is not empty. If the [`skills-ref`](https://agentskills.io/specification)
CLI is available, `skills-ref validate ./pdf-processing` is the reference
implementation of the same checks.

Then test the skill end to end:

1. Add the folder (or re-run onboarding with this skill selected).
2. Start a new conversation so the skill index is rebuilt.
3. Ask something that should trigger the description.
4. Confirm the agent reads `SKILL.md` and runs the command successfully.
5. Try the error paths: missing argument, bad input, unavailable dependency.

### 8. Report back

When finished, tell the user:

- the skill path,
- what triggers it,
- the command(s) it runs,
- any dependency they must install,
- how to disable or remove it (`skills.json`, `enabled: false`, or delete the
  folder).

## Checklist

- [ ] Folder name matches frontmatter `name` and follows the naming rules.
- [ ] The skill folder is inside **this agent's** skills directory, not another agent's.
- [ ] `description` explains what and when, with trigger keywords, under 1024 chars.
- [ ] Body states the exact command, arguments, examples, and error behavior.
- [ ] Scripts are executable in this environment and print JSON to stdout.
- [ ] No secrets, no hardcoded home directory, no fake paths.
- [ ] `references/` and `assets/` are one level deep and only loaded when needed.
- [ ] Validator passes and a real conversation triggers the skill.

## Common mistakes

| Mistake | Fix |
| :--- | :--- |
| Name has uppercase, spaces, `--`, or differs from the folder | Use `lowercase-hyphen` and rename the folder |
| Description says only "Helps with X" | Say what it does, when to use it, and the words a user would say |
| Huge `SKILL.md` | Move detail into `references/` and keep the body short |
| Script assumes `~/.myaaw/skills` | Use the absolute skills path from the prompt |
| Script prints logs before JSON | Log to stderr; keep stdout a single JSON object |
| Skill assumes a package is installed | Document it in `compatibility` and give the install command |
| Wrote the skill but the agent never loads it | Rebuild the skill index by starting a new conversation |

## References

- [Frontmatter reference](references/frontmatter.md)
- [SKILL.md template](assets/SKILL.template.md)
- [Agent Skills specification](https://agentskills.io/specification)
