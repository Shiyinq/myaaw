# Frontmatter Reference

Condensed from the Agent Skills specification:
<https://agentskills.io/specification>.

`SKILL.md` must begin with a YAML frontmatter block delimited by `---` lines,
followed by Markdown instructions.

## Fields

| Field | Required | Constraints |
| :--- | :--- | :--- |
| `name` | Yes | 1–64 chars, only `a-z`, `0-9`, and `-`; no leading or trailing hyphen; no `--`; must equal the folder name |
| `description` | Yes | 1–1024 chars, non-empty, explains **what** the skill does and **when** to use it |
| `license` | No | License name or reference to a bundled license file |
| `compatibility` | No | 1–500 chars, environment requirements (packages, binaries, network) |
| `metadata` | No | Map of string keys to string values (`author`, `version`, ...) |
| `allowed-tools` | No | Experimental, space-separated pre-approved tools |
| `enabled` | No | **Myaaw extension**: `false` hides the skill without deleting it |

Myaaw reads `name`, `description`, and the optional `enabled` flag. Other keys
are accepted and ignored by Myaaw, but keep them so the skill stays valid for
other Agent Skills clients.

## Name rules

```text
valid:    pdf-processing, unit-converter, release-notes-2
invalid:  PDF-Processing   (uppercase)
invalid:  -pdf             (leading hyphen)
invalid:  pdf--processing  (double hyphen)
invalid:  pdf_processing   (underscore)
invalid:  pdf processing   (space)
```

The folder name must match:

```text
pdf-processing/
└── SKILL.md      # name: pdf-processing
```

## Description

The description is loaded for every skill at startup and is the only signal the
agent has before deciding to activate the skill. Write it as a compact trigger
sentence.

```yaml
# Good
description: Extract text and tables from PDF files, fill PDF forms, and merge PDFs. Use when the user mentions PDFs, forms, or document extraction.

# Poor
description: Helps with PDFs.
```

Guidelines:

- Start with an action verb: "Extract", "Convert", "Create", "Audit".
- Name the object: PDFs, spreadsheets, release notes, invoices.
- Add the trigger words a user would actually type.
- Keep it under 1024 characters; 1–3 sentences is usually enough.

## Optional fields

```yaml
---
name: pdf-processing
description: Extract PDF text and fill forms. Use when handling PDF documents.
license: Apache-2.0
compatibility: Requires Python 3.11+, pypdf, and network access.
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(python3:*) Read
---
```

Most skills only need `name` and `description`. Add `compatibility` when the
skill depends on something outside the standard environment, and `metadata` only
when another tool consumes it.

## Progressive disclosure

| Layer | Loaded | Budget |
| :--- | :--- | :--- |
| `name` + `description` | Always, for every skill | ~100 tokens |
| `SKILL.md` body | When the skill activates | Keep under ~500 lines |
| `scripts/`, `references/`, `assets/` | Only when the instructions ask for them | As small as practical |

Keep file references one level deep from `SKILL.md`:

```markdown
See [the reference guide](references/REFERENCE.md).
Run `scripts/extract.py`.
```

Avoid chains such as `SKILL.md` → `references/a.md` → `references/b/c.md`; the
agent has to traverse every hop.

## Validation

```bash
skills-ref validate ./pdf-processing
```

or, without extra dependencies:

```bash
python3 scripts/validate_skill.py ./pdf-processing
```
