---
name: Scraping
description: Scrape data from a specified URL using the scraping tool.
---

# Scraping Skill

Scrape data from a specified URL using the scraping tool.

## Usage

The skill is executed via a Python script: `.teo/skills/scraping/scripts/scraping.py`.
It accepts a JSON string as the first argument calling the tool.

### Arguments

The input JSON should contain the following fields:

| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `url` | string | The full URL of the web page to scrape, e.g. `https://r.jina.ai/example`. | Yes |

### Example

```bash
.venv/bin/python .teo/skills/scraping/scripts/scraping.py '{"url": "https://example.com"}'
```
