---
name: Tavily
description: Performs a search or extracts content using the Tavily API.
---

# Tavily Search Skill

Performs a search or extracts content using the Tavily API. Specify 'search' or 'extract' in the 'action' parameter.

## Usage

The skill is executed via a Python script: `.teo/skills/tavily/scripts/tavily.py`.
It accepts a JSON string as the first argument calling the tool.

### Arguments

The input JSON should contain the following fields:

| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `action` | string | The action to perform: `search` (Search API) or `extract` (Extract API). | Yes |
| `search_args` | object | Arguments for the Tavily search API. Required if action is `search`. | Conditional |
| `extract_args` | object | Arguments for the Tavily extract API. Required if action is `extract`. | Conditional |

#### Search Arguments (`search_args`)

| Field | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `query` | string | The search query. (Required) | - |
| `topic` | string | Category of the search: `general` or `news`. | `general` |
| `search_depth` | string | Depth of the search: `basic` or `advanced`. | `basic` |
| `chunks_per_source` | int | Number of content chunks per source (advanced search only). | 3 |
| `max_results` | int | Maximum number of search results. | 5 |
| `time_range` | string | Time range (`day`, `week`, `month`, `year`) or abbrevs (`d`, etc). | - |
| `days` | int | Number of days back to include (news topic only). | 7 |
| `include_answer` | bool | Include LLM-generated answer. | `false` |
| `include_raw_content` | bool | Include cleaned HTML content of search results. | `false` |
| `include_images` | bool | Include image search results. | `false` |
| `include_image_descriptions` | bool | Include descriptive text for images. | `false` |
| `include_domains` | array | List of domains to include. | - |
| `exclude_domains` | array | List of domains to exclude. | - |

#### Extract Arguments (`extract_args`)

| Field | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| `urls` | string | URL(s) to extract content from. Separated by commas/newlines for multiple. | (Required) |
| `include_images` | bool | Include images from extracted content. | `false` |
| `extract_depth` | string | Depth of the extraction process: `basic` or `advanced`. | `basic` |

### Environment

Requires `TAVILY_API_KEY` to be set in the environment or in a `.env` file in the project root.

### Example

```bash
.venv/bin/python .teo/skills/tavily/scripts/tavily.py '{"action": "search", "search_args": {"query": "AI Agents"}}'
```
