# Tavily Search Tool

A tool for performing web searches and content extraction using the Tavily AI API.

## Overview

The Tavily Search Tool provides access to Tavily's AI-powered search and content extraction capabilities. It supports both search operations and content extraction from URLs, making it useful for research, content analysis, and information gathering.

## Features

- **Web Search**: AI-powered search with multiple parameters
- **Content Extraction**: Extract content from specific URLs
- **Configurable Search Depth**: Basic and advanced search options
- **Topic Filtering**: General and news search categories
- **Time Range Filtering**: Search within specific time periods
- **Domain Filtering**: Include or exclude specific domains
- **Content Chunking**: Configurable content chunks per source

## API Configuration

### Environment Variable

The tool requires a Tavily API key set in the environment:

```bash
TAVILY_API_KEY=your_api_key_here
```

The tool automatically loads the `.env` file if present.

## Usage

### Search Operation

#### Parameters

| Parameter | Type | Required | Description | Default |
|-----------|------|----------|-------------|---------|
| `query` | string | Yes | Search query | - |
| `topic` | string | No | Search category (general/news) | "general" |
| `search_depth` | string | No | Search depth (basic/advanced) | "basic" |
| `chunks_per_source` | integer | No | Content chunks per source | 3 |
| `max_results` | integer | No | Maximum results to return | - |
| `time_range` | string | No | Time range filter | - |
| `days` | integer | No | Number of days to search back | - |
| `include_answer` | boolean | No | Include AI-generated answer | false |
| `include_raw_content` | boolean | No | Include raw content | false |
| `include_images` | boolean | No | Include images | false |
| `include_image_descriptions` | boolean | No | Include image descriptions | false |
| `include_domains` | array | No | Domains to include | - |
| `exclude_domains` | array | No | Domains to exclude | - |

#### Example Search Request

```json
{
  "action": "search",
  "search_args": {
    "query": "artificial intelligence trends 2024",
    "topic": "general",
    "search_depth": "advanced",
    "max_results": 10,
    "include_answer": true
  }
}
```

### Extract Operation

#### Parameters

| Parameter | Type | Required | Description | Default |
|-----------|------|----------|-------------|---------|
| `urls` | string | Yes | URL(s) to extract content from | - |
| `include_images` | boolean | No | Include images in extraction | false |
| `extract_depth` | string | No | Extraction depth | - |

#### Example Extract Request

```json
{
  "action": "extract",
  "extract_args": {
    "urls": "https://example.com/article",
    "include_images": true
  }
}
```

## Implementation Details

### Dependencies

- `github.com/joho/godotenv` - Environment variable loading
- Standard `net/http` - HTTP client for API requests

### Key Functions

- `NewTavilyTool()` - Creates new Tavily tool instance
- `CallTool(arguments string)` - Main function that processes requests
- `validateInput(input TavilyToolInput)` - Validates input parameters

### API Endpoints

- **Search**: `https://api.tavily.com/search`
- **Extract**: `https://api.tavily.com/extract`

### Request Processing

1. **Input Validation**: Validates action and required parameters
2. **API Key Check**: Ensures TAVILY_API_KEY is set
3. **Request Building**: Constructs appropriate API request
4. **HTTP Request**: Makes POST request to Tavily API
5. **Response Handling**: Returns raw API response

## Error Handling

- Missing API key
- Invalid action parameter
- Missing required arguments
- Network request failures
- API response errors
- JSON parsing errors

## Search Features

### Search Depth Options

- **Basic**: Faster, less comprehensive search
- **Advanced**: More thorough search with additional sources

### Topic Categories

- **General**: General web search
- **News**: News-specific search

### Time Filtering

- **Time Range**: Predefined time periods
- **Days**: Custom number of days to search back

### Content Options

- **Include Answer**: AI-generated summary answer
- **Raw Content**: Full raw content from sources
- **Images**: Image URLs and metadata
- **Image Descriptions**: AI-generated image descriptions

## Content Extraction Features

- **URL Support**: Single or multiple URLs
- **Image Extraction**: Optional image content
- **Depth Control**: Configurable extraction depth
- **Structured Output**: Clean, structured content

## Response Format

The tool returns the raw JSON response from the Tavily API, which includes:

### Search Response
```json
{
  "query": "search query",
  "results": [...],
  "answer": "AI-generated answer",
  "images": [...],
  "follow_up_questions": [...]
}
```

### Extract Response
```json
{
  "content": "extracted content",
  "images": [...],
  "metadata": {...}
}
```

## Security Considerations

- API key stored in environment variables
- No sensitive data in request logs
- Input validation for all parameters
- Error message sanitization

## Rate Limiting

- Subject to Tavily API rate limits
- Consider implementing request throttling
- Monitor API usage and quotas

## Best Practices

- Use specific, targeted search queries
- Leverage topic and depth parameters appropriately
- Include/exclude domains for focused results
- Use time filtering for recent information
- Handle API errors gracefully
- Cache results when appropriate

## Limitations

- Dependent on Tavily API availability
- Subject to API rate limits and quotas
- Search quality depends on Tavily's indexing
- Content extraction limited to accessible URLs
- No offline search capabilities 