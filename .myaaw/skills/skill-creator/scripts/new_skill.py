#!/usr/bin/env python3
"""Scaffold a new Myaaw Agent Skill.

Creates SKILL.md plus scripts/, references/, and assets/ inside this agent's
skills directory (the parent of this skill's own folder). Writing to another
agent's folder or to ~/.myaaw/skills is refused.
Follows https://agentskills.io/specification for naming and frontmatter.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from typing import NoReturn

NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")

# A skill lives at <skills-dir>/<name>/scripts/new_skill.py, so the parent of
# the skill folder is the only directory this scaffolder may write into.
SELF_SKILLS_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

TEMPLATE = """---
name: __NAME__
description: __DESCRIPTION__
---

# __TITLE__

One short paragraph: what this skill does and when the agent should use it.

## Usage

Replace `<skill-dir>` with the absolute skill path shown in the `# Agent Skills`
section of your prompt.

```bash
python3 <skill-dir>/scripts/main.py '{"action":"example"}'
```

## Arguments

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `action` | string | Yes | Operation to perform |
| `payload` | object | No | Extra data for the action |

## Examples

```bash
python3 <skill-dir>/scripts/main.py '{"action":"example","payload":{"id":"42"}}'
```

Expected output:

```json
{"ok": true, "result": "..."}
```

## Errors

| Output | Meaning | What to do |
| :--- | :--- | :--- |
| `missing argument` | Required field not supplied | Ask the user for the value |
| `not found` | Target does not exist | Confirm the identifier with the user |

## Safety

Describe any action that changes data, spends money, or contacts an external
service, and when to ask for confirmation first.
"""


def fail(message: str) -> NoReturn:
    print(json.dumps({"ok": False, "error": message}))
    sys.exit(1)


def main() -> None:
    parser = argparse.ArgumentParser(description="Scaffold a Myaaw Agent Skill.")
    parser.add_argument(
        "--dir",
        default="",
        help="skills directory of THIS agent (defaults to the agent that owns this skill)",
    )
    parser.add_argument("--name", required=True, help="skill name and folder name (lowercase-hyphen)")
    parser.add_argument("--description", default="", help="what the skill does and when to use it")
    parser.add_argument("--force", action="store_true", help="overwrite an existing SKILL.md")
    parser.add_argument("--empty", action="store_true", help="do not create scripts/, references/, or assets/")
    args = parser.parse_args()

    target_dir = os.path.abspath(os.path.expanduser(args.dir)) if args.dir.strip() else SELF_SKILLS_DIR
    if os.path.realpath(target_dir) != os.path.realpath(SELF_SKILLS_DIR):
        fail(
            "refusing to write outside this agent's skills directory (%s); "
            "a skill must live in the agent that owns it" % SELF_SKILLS_DIR
        )

    name = args.name.strip()
    if not NAME_RE.fullmatch(name) or len(name) > 64:
        fail(
            "invalid skill name %r: use 1-64 lowercase letters, digits, and single hyphens "
            "(for example pdf-processing)" % name
        )

    description = args.description.strip()
    warnings = []
    if not description:
        description = "Describe what this skill does and when to use it. Include trigger words a user would say."
        warnings.append("no --description provided, wrote a placeholder")
    if len(description) > 1024:
        fail("description is %d characters; the maximum is 1024" % len(description))

    skills_dir = target_dir
    skill_dir = os.path.join(skills_dir, name)
    skill_md = os.path.join(skill_dir, "SKILL.md")

    if os.path.exists(skill_md) and not args.force:
        fail("%s already exists; pass --force to overwrite" % skill_md)

    created = []
    os.makedirs(skill_dir, exist_ok=True)
    if not args.empty:
        for sub in ("scripts", "references", "assets"):
            path = os.path.join(skill_dir, sub)
            os.makedirs(path, exist_ok=True)
            created.append(sub + "/")

    title = " ".join(part.capitalize() for part in name.split("-"))
    content = (
        TEMPLATE.replace("__NAME__", name)
        .replace("__TITLE__", title)
        .replace("__DESCRIPTION__", description)
    )
    with open(skill_md, "w", encoding="utf-8") as handle:
        handle.write(content)
    created.insert(0, "SKILL.md")

    print(json.dumps({"ok": True, "path": skill_dir, "created": created, "warnings": warnings}))


if __name__ == "__main__":
    main()
