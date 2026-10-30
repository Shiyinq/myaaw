#!/usr/bin/env python3
"""Validate a Myaaw Agent Skill against the Agent Skills rules.

Checks the directory name, required frontmatter fields, field limits, and that
the body is not empty. Uses only the Python standard library.

Usage:
    python3 validate_skill.py <path-to-skill-dir>
"""

from __future__ import annotations

import json
import os
import re
import sys

NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
MAX_BODY_LINES = 500


def parse_frontmatter(lines: list[str]) -> tuple[dict[str, str], list[str], list[str]]:
    """Return top-level scalar fields, body lines, and structural errors."""
    if not lines or lines[0].strip() != "---":
        return {}, [], ["SKILL.md must start with a YAML frontmatter block (---)"]

    end = None
    for index in range(1, len(lines)):
        if lines[index].strip() == "---":
            end = index
            break
    if end is None:
        return {}, [], ["frontmatter is missing the closing --- line"]

    fields: dict[str, str] = {}
    for raw in lines[1:end]:
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        if raw[:1].isspace():
            continue  # nested value such as metadata.*
        if ":" not in raw:
            return {}, [], ["invalid frontmatter line: %r" % raw]
        key, value = raw.split(":", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        fields[key.strip()] = value

    body = lines[end + 1 :]
    return fields, body, []


def main() -> None:
    if len(sys.argv) != 2:
        print(json.dumps({"ok": False, "errors": ["usage: validate_skill.py <skill-dir>"]}))
        sys.exit(1)

    skill_dir = os.path.abspath(os.path.expanduser(sys.argv[1]))
    errors: list[str] = []
    warnings: list[str] = []

    if not os.path.isdir(skill_dir):
        print(json.dumps({"ok": False, "errors": ["not a directory: %s" % skill_dir]}))
        sys.exit(1)

    skill_md = os.path.join(skill_dir, "SKILL.md")
    if not os.path.isfile(skill_md):
        print(json.dumps({"ok": False, "errors": ["SKILL.md not found in %s" % skill_dir]}))
        sys.exit(1)

    with open(skill_md, "r", encoding="utf-8") as handle:
        lines = handle.read().splitlines()

    fields, body, structure_errors = parse_frontmatter(lines)
    errors.extend(structure_errors)

    name = fields.get("name", "")
    description = fields.get("description", "")
    compatibility = fields.get("compatibility", "")
    dir_name = os.path.basename(os.path.normpath(skill_dir))

    if not name:
        errors.append("frontmatter is missing the required 'name' field")
    else:
        if not NAME_RE.fullmatch(name):
            errors.append(
                "name %r must use lowercase letters, digits, and single hyphens" % name
            )
        if len(name) > 64:
            errors.append("name is %d characters; the maximum is 64" % len(name))
        if name != dir_name:
            errors.append("name %r does not match the folder name %r" % (name, dir_name))

    if not description:
        errors.append("frontmatter is missing the required 'description' field")
    elif len(description) > 1024:
        errors.append("description is %d characters; the maximum is 1024" % len(description))
    elif len(description) < 20:
        warnings.append("description is very short; explain what the skill does and when to use it")
    elif "use when" not in description.lower():
        warnings.append("description has no 'use when' clause; triggers may be unclear")

    if compatibility and len(compatibility) > 500:
        errors.append("compatibility is %d characters; the maximum is 500" % len(compatibility))

    if not "".join(body).strip():
        errors.append("SKILL.md has no instructions after the frontmatter")
    elif len(body) > MAX_BODY_LINES:
        warnings.append(
            "body has %d lines (recommended maximum %d); move detail into references/"
            % (len(body), MAX_BODY_LINES)
        )

    result = {"ok": not errors, "path": skill_dir, "errors": errors, "warnings": warnings}
    print(json.dumps(result, indent=2))
    sys.exit(0 if not errors else 1)


if __name__ == "__main__":
    main()
