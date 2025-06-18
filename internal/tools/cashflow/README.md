# Cash Flow Management Tool

A comprehensive tool for managing personal or business cash flow with transaction tracking, categorization, and analytics.

## Overview

The Cash Flow Tool provides a complete financial management solution for tracking income and expenses, managing categories, and generating financial analytics. It supports multiple currencies and provides detailed transaction history.

## Features

- **Transaction Management**: Add, update, delete, and retrieve transactions
- **Category Management**: Create and manage transaction categories
- **Multi-currency Support**: Support for IDR, USD, EUR, JPY, GBP
- **Financial Analytics**: Income/expense analysis and reporting
- **Date-based Filtering**: Filter transactions by date ranges
- **JSON Storage**: Persistent data storage in JSON format
- **Thread-safe Operations**: Concurrent access support

## Data Structure

### Transaction Object

```json
{
  "id": "unique_transaction_id",
  "type": "income|expense",
  "amount": 1000.00,
  "currency": "IDR|USD|EUR|JPY|GBP",
  "category": {
    "id": "category_id",
    "name": "Category Name"
  },
  "description": "Transaction description",
  "date": "2024-01-01T10:00:00Z"
}
```

### Category Object

```json
{
  "id": "unique_category_id",
  "name": "Category Name"
}
```

## Usage

### Available Actions

| Action | Description | Required Parameters |
|--------|-------------|-------------------|
| `add_transaction` | Add new transaction | `transaction` object |
| `get_transactions` | Retrieve transactions | Optional filters |
| `update_transaction` | Update existing transaction | `id`, `transaction` object |
| `delete_transaction` | Delete transaction | `id` |
| `get_analytics` | Get financial analytics | Optional filters |
| `add_category` | Add new category | `category` object |
| `get_categories` | Retrieve all categories | None |

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | Action to perform |
| `transaction` | object | Conditional | Transaction data |
| `category` | object | Conditional | Category data |
| `id` | string | Conditional | Transaction ID |
| `start_date` | string | Conditional | Start date filter (YYYY-MM-DD) |
| `end_date` | string | Conditional | End date filter (YYYY-MM-DD) |
| `type` | string | Conditional | Transaction type filter |

### Transaction Types

- **income**: Money received
- **expense**: Money spent

### Supported Currencies

- **IDR**: Indonesian Rupiah
- **USD**: US Dollar
- **EUR**: Euro
- **JPY**: Japanese Yen
- **GBP**: British Pound

## Example Usage

### Add Transaction

```json
{
  "action": "add_transaction",
  "transaction": {
    "type": "expense",
    "amount": 50000,
    "currency": "IDR",
    "category": {
      "id": "food",
      "name": "Food & Dining"
    },
    "description": "Lunch at restaurant",
    "date": "2024-01-01T12:00:00Z"
  }
}
```

### Get Transactions with Filter

```json
{
  "action": "get_transactions",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31",
  "type": "expense"
}
```

### Add Category

```json
{
  "action": "add_category",
  "category": {
    "id": "transport",
    "name": "Transportation"
  }
}
```

### Get Analytics

```json
{
  "action": "get_analytics",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31"
}
```

## Implementation Details

### Storage Location

Data is stored in: `data/cashflow/cashflow.json`

### Key Functions

- `NewCashFlowTool()` - Creates new cash flow tool instance
- `CallTool(arguments string)` - Main function that processes operations
- `loadData()` - Loads data from JSON file
- `saveData(data *CashFlowData)` - Saves data to JSON file
- `handleAddTransaction(params map[string]interface{})` - Adds new transaction
- `handleGetAnalytics(params map[string]interface{})` - Generates analytics

### Data Processing

1. **Input Validation**: Validates required parameters and data types
2. **Data Persistence**: Thread-safe JSON file operations
3. **Date Parsing**: Converts date strings to time.Time objects
4. **Analytics Calculation**: Computes financial summaries
5. **Error Handling**: Comprehensive error handling for all operations

## Analytics Features

### Financial Summary

- **Total Income**: Sum of all income transactions
- **Total Expenses**: Sum of all expense transactions
- **Net Cash Flow**: Income minus expenses
- **Transaction Count**: Number of transactions

### Category Analysis

- **Category Breakdown**: Expenses by category
- **Top Categories**: Highest spending categories
- **Category Percentages**: Percentage of total expenses

### Time-based Analysis

- **Date Range Filtering**: Filter by specific date ranges
- **Monthly Trends**: Track spending patterns over time
- **Period Comparisons**: Compare different time periods

## Error Handling

- Missing required parameters
- Invalid transaction types
- Invalid currency codes
- Date format validation
- File system errors
- JSON parsing errors
- Duplicate transaction IDs

## Security Considerations

- Local file storage only
- Input validation for all parameters
- Thread-safe operations
- Error message sanitization
- No external API calls

## Limitations

- Local storage only (no cloud sync)
- No multi-user support
- No budget tracking
- No recurring transaction support
- No import/export functionality
- No backup/restore features

## Best Practices

- Use consistent currency for accurate analytics
- Create meaningful category names
- Regular data backups
- Use descriptive transaction descriptions
- Monitor cash flow regularly
- Validate transaction data before adding

## Use Cases

- **Personal Finance**: Track personal income and expenses
- **Small Business**: Monitor business cash flow
- **Budget Planning**: Analyze spending patterns
- **Financial Reporting**: Generate financial summaries
- **Expense Tracking**: Monitor category-wise expenses

## Performance Considerations

- Efficient JSON file operations
- Thread-safe concurrent access
- Minimal memory usage
- Fast transaction lookups
- Optimized analytics calculations  
