# Agent Workspace

This directory is where the agent stores everything it creates. Keeping generated
output here keeps `~/.myaaw/home/` reserved for identity, memory, and configuration
files.

## Layout

- `projects/` — coding projects, cloned repositories, and build artifacts.
  Use one subdirectory per project: `projects/<project-name>/`.
- `downloads/` — downloaded files, media, datasets, and archives.

## Rules

- Create all new projects and downloads under this directory.
- Do not place project folders or downloads directly in `~/.myaaw/home/`.
- Use absolute paths or `cd ~/.myaaw/home/workspace` before creating files with
  Bash or Python, because those tools start in `~/.myaaw/home/`.
