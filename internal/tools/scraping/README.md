# Web Scraping Tool

A tool for extracting content from web pages using the Jina AI Reader API.

## Overview

The Web Scraping Tool allows you to scrape and extract content from any web page by providing a URL. It uses the Jina AI Reader service to handle the web scraping process and return the extracted content.

## Features

- **Web Content Extraction**: Extract text content from any web page
- **Simple URL-based Interface**: Just provide a URL to get content
- **Automatic Content Processing**: Handles various web page formats
- **Error Handling**: Robust error handling for failed requests

## API Endpoint

Uses the Jina AI Reader API service at `https://r.jina.ai/` for web content extraction.

## Usage

### Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `url` | string | Yes | The full URL of the web page to scrape | "<https://example.com/article>" |

### Example Usage

```json
{
  "url": "https://news.ycombinator.com/item?id=123456"
}
```

### Response Format

The tool returns the raw extracted content from the web page as a string.

## Implementation Details

### Dependencies

- `github.com/go-resty/resty/v2` - HTTP client for API requests

### Key Functions

- `NewScrapingTool()` - Creates a new scraping tool instance
- `CallTool(arguments string)` - Main function that processes scraping requests

### Data Processing

1. Parses JSON arguments for the target URL
2. Constructs API URL by prepending `https://r.jina.ai/` to the provided URL
3. Makes HTTP GET request to the Jina AI Reader API
4. Returns the raw response content from the web page

## Error Handling

- Invalid argument parsing
- Network request failures
- API response errors (non-200 status codes)
- Malformed URLs

## Security Considerations

- No API key required (uses public Jina AI Reader service)
- Input validation for URL parameter
- Error handling for malformed responses
- Sanitizes URL input before processing

## Limitations

- Dependent on Jina AI Reader service availability
- Rate limiting may apply
- Some websites may block automated scraping
- Content extraction quality depends on the target website structure
- No support for JavaScript-rendered content (depends on Jina AI Reader capabilities)

## Use Cases

- News article extraction
- Blog post content retrieval
- Documentation scraping
- Research data collection
- Content aggregation

## Best Practices

- Ensure the target URL is accessible and public
- Be respectful of website terms of service
- Consider rate limiting for multiple requests
- Validate extracted content for accuracy
