# Extra Messages Tool

The Extra Messages tool allows an agent to present rich display and interactive UI blocks alongside its reply: multi-file media galleries (images, videos, audio, PDFs, files from local agent paths or online URLs), carousels, clickable action/link buttons, quick-reply chips, dynamic forms, checklists, summary cards, callouts, and link previews.

For full architecture, persistence, and channel rendering details, see [Extra Messages Guide](../../../../docs/agents/extra-messages.md).

## Function Name

`extra_messages`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `blocks` | `array` | **Yes** | Ordered list of 1–10 UI blocks to render with the reply. |
| `blocks[].id` | `string` | No | Optional stable block ID (auto-generated if omitted). |
| `blocks[].type` | `string` | **Yes** | Block kind: `media`, `carousel`, `buttons`, `quick_replies`, `form`, `checklist`, `callout`, `summary`, or `link_preview`. |
| `blocks[].single_use` | `boolean` | No | For interactive blocks: lock after the first click/submission (default `true`). |
| `blocks[].data` | `object` | **Yes** | Block-specific payload (see schemas and examples below). |

> **Routing safety:** Never pass `user_id` or `conversation_id`. Delivery is automatically bound to the active turn's session.

## Display-Only vs. Interactive Blocks

- **Display-only (`interactive: false`)**: `media`, `callout`, `summary`, `link_preview`, and `carousel` with link-only buttons.
- **Interactive (`interactive: true`)**: `buttons` (with `type: "action"`), `quick_replies`, `form`, `checklist`, and `carousel` with action buttons. User interactions lock single-use blocks atomically in SQLite and send a structured `<interaction kind="...">` turn back to the agent.

## Usage Examples

### 1. Multi-File Media Gallery (`media`)

Send 1–10 images, videos, audio clips, PDFs, or files using either local paths (inside allowed agent roots) or online `http`/`https` URLs.

```json
{
  "blocks": [
    {
      "type": "media",
      "data": {
        "layout": "grid",
        "caption": "Generated charts and report",
        "items": [
          {
            "path": "/home/reports/revenue.png",
            "title": "Revenue Q4"
          },
          {
            "url": "https://example.com/assets/comparison.png",
            "kind": "image",
            "caption": "Benchmark"
          },
          {
            "path": "/home/reports/summary.pdf"
          }
        ]
      }
    }
  ]
}
```

### 2. Action & Link Buttons (`buttons`)

```json
{
  "blocks": [
    {
      "type": "buttons",
      "single_use": true,
      "data": {
        "prompt": "Deploy build v2.4.0 to production?",
        "layout": "row",
        "buttons": [
          {
            "id": "deploy_prod",
            "label": "Deploy Now",
            "type": "action",
            "value": "deploy_prod",
            "style": "primary",
            "confirm": "This will roll out v2.4.0 to production servers."
          },
          {
            "id": "cancel",
            "label": "Cancel",
            "type": "action",
            "value": "cancel",
            "style": "danger"
          },
          {
            "id": "changelog",
            "label": "View Changelog",
            "type": "link",
            "url": "https://example.com/releases/v2.4.0"
          }
        ]
      }
    }
  ]
}
```

### 3. Quick Replies (`quick_replies`)

```json
{
  "blocks": [
    {
      "type": "quick_replies",
      "data": {
        "prompt": "Which environment should I inspect?",
        "options": [
          { "id": "dev", "label": "Development", "value": "dev" },
          { "id": "staging", "label": "Staging", "value": "staging" },
          { "id": "prod", "label": "Production", "value": "prod" }
        ]
      }
    }
  ]
}
```

### 4. Carousel (`carousel`)

```json
{
  "blocks": [
    {
      "type": "carousel",
      "data": {
        "cards": [
          {
            "id": "plan_starter",
            "title": "Starter Plan",
            "subtitle": "$9 / month",
            "description": "Includes 10 GB storage and email support.",
            "image_url": "https://example.com/plans/starter.png",
            "badges": ["Popular"],
            "buttons": [
              {
                "id": "pick_starter",
                "label": "Choose Starter",
                "type": "action",
                "value": "starter",
                "style": "primary"
              }
            ]
          },
          {
            "id": "plan_pro",
            "title": "Pro Plan",
            "subtitle": "$29 / month",
            "description": "Unlimited storage and priority support.",
            "image_path": "/home/assets/pro.png",
            "buttons": [
              {
                "id": "pick_pro",
                "label": "Choose Pro",
                "type": "action",
                "value": "pro"
              },
              {
                "id": "compare",
                "label": "Compare",
                "type": "link",
                "url": "https://example.com/pricing"
              }
            ]
          }
        ]
      }
    }
  ]
}
```

### 5. Dynamic Form (`form`)

Supports 10 field types: `text`, `textarea`, `number`, `select`, `multiselect`, `checkbox`, `switch`, `date`, `time`, and `radio`.

```json
{
  "blocks": [
    {
      "type": "form",
      "data": {
        "id": "incident_report",
        "title": "Create Incident Report",
        "description": "Fill in the details below to open a ticket.",
        "submit_label": "Create Ticket",
        "fields": [
          {
            "name": "title",
            "label": "Incident Title",
            "type": "text",
            "required": true,
            "placeholder": "e.g. API latency spike"
          },
          {
            "name": "severity",
            "label": "Severity",
            "type": "select",
            "required": true,
            "options": [
              { "label": "Low", "value": "low" },
              { "label": "High", "value": "high" },
              { "label": "Critical", "value": "critical" }
            ]
          },
          {
            "name": "notify_team",
            "label": "Notify on-call channel",
            "type": "switch",
            "default": true
          }
        ]
      }
    }
  ]
}
```

### 6. Checklist (`checklist`)

```json
{
  "blocks": [
    {
      "type": "checklist",
      "data": {
        "title": "Release Readiness",
        "submit_label": "Confirm Items",
        "items": [
          { "id": "tests", "label": "All unit tests pass", "checked": true },
          { "id": "migrations", "label": "DB migrations verified" },
          { "id": "docs", "label": "Release notes updated" }
        ]
      }
    }
  ]
}
```

### 7. Callout, Summary & Link Preview (`callout`, `summary`, `link_preview`)

```json
{
  "blocks": [
    {
      "type": "callout",
      "data": {
        "variant": "success",
        "title": "Backup Completed",
        "text": "Snapshot saved without errors."
      }
    },
    {
      "type": "summary",
      "data": {
        "title": "Build Summary",
        "items": [
          { "label": "Status", "value": "Passed", "tone": "positive" },
          { "label": "Duration", "value": "42s" },
          { "label": "Warnings", "value": "0" }
        ]
      }
    },
    {
      "type": "link_preview",
      "data": {
        "url": "https://example.com/docs/v2",
        "title": "Myaaw v2 Documentation",
        "description": "Setup, tools, and channel guides."
      }
    }
  ]
}
```

## Interaction Round-Trip Format

When a user clicks an action button, quick reply, checklist, or submits a form, the agent receives a follow-up user turn containing a structured `<interaction>` envelope:

```xml
<interaction kind="button">
{"action_id":"deploy_prod","block_id":"blk_123456","label":"Deploy Now","value":"deploy_prod"}
</interaction>
Deploy Now
```

## Limits & Security

- **Bounds**: max 10 blocks/call, 10 media items/block, 10 carousel cards, 20 buttons/block, 12 quick replies, 20 form fields, 25 checklist items, 20 summary items.
- **Local path sandbox**: local `path` and `image_path` values are resolved with `attachment.Resolve(...)` against the agent's allowed roots only (`../` traversal, escaping symlinks, and host dotfiles like `.env` are rejected).
- **URL validation**: only `http://` and `https://` URLs are accepted.
- **Sub-agents**: excluded from background sub-agents automatically.
