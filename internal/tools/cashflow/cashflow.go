package cashflow

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type TransactionType string

const (
	Income  TransactionType = "income"
	Expense TransactionType = "expense"
)

type CurrencyType string

const (
	IDR CurrencyType = "IDR"
	USD CurrencyType = "USD"
	EUR CurrencyType = "EUR"
	JPY CurrencyType = "JPY"
	GBP CurrencyType = "GBP"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Transaction struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	Type        TransactionType `json:"type"`
	Amount      float64         `json:"amount"`
	Currency    CurrencyType    `json:"currency"`
	Category    Category        `json:"category"`
	Description string          `json:"description"`
	Date        time.Time       `json:"date"`
}

type CashFlowTool struct {
	dataPath string
	mu       sync.RWMutex
}

type CashFlowData struct {
	Transactions []Transaction `json:"transactions"`
	Categories   []Category    `json:"categories"`
}

func NewCashFlowTool() *CashFlowTool {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting working directory: %v\n", err)
		return nil
	}

	dataDir := filepath.Join(workingDir, "data", "cashflow")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Error creating cashflow directory: %v\n", err)
		return nil
	}

	dataPath := filepath.Join(dataDir, "cashflow.json")
	return &CashFlowTool{
		dataPath: dataPath,
	}
}

func (ct *CashFlowTool) loadData() (*CashFlowData, error) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	data := &CashFlowData{
		Transactions: make([]Transaction, 0),
		Categories:   make([]Category, 0),
	}

	if _, err := os.Stat(ct.dataPath); err == nil {
		file, err := os.ReadFile(ct.dataPath)
		if err != nil {
			return nil, fmt.Errorf("error reading data file: %v", err)
		}

		if len(file) == 0 {
			return data, nil
		}

		if err := json.Unmarshal(file, data); err != nil {
			log.Printf("Warning: error parsing data file: %v, returning empty data", err)
			return data, nil
		}
	}

	return data, nil
}

func (ct *CashFlowTool) saveData(data *CashFlowData) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if data == nil {
		data = &CashFlowData{
			Transactions: make([]Transaction, 0),
			Categories:   make([]Category, 0),
		}
	}

	if data.Transactions == nil {
		data.Transactions = make([]Transaction, 0)
	}
	if data.Categories == nil {
		data.Categories = make([]Category, 0)
	}

	file, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling data: %v", err)
	}

	dir := filepath.Dir(ct.dataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	if err := os.WriteFile(ct.dataPath, file, 0644); err != nil {
		return fmt.Errorf("error writing data file: %v", err)
	}

	return nil
}

func toLowerCase(s string) string {
	return strings.ToLower(s)
}

func parseDate(dateStr string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
		return t, nil
	}

	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
	}

	return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
}

func (ct *CashFlowTool) CallTool(arguments string) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	action, ok := params["action"].(string)
	if !ok {
		return "Error: action not found"
	}

	switch action {
	case "add_transaction":
		return ct.handleAddTransaction(params)
	case "get_transactions":
		return ct.handleGetTransactions(params)
	case "update_transaction":
		return ct.handleUpdateTransaction(params)
	case "delete_transaction":
		return ct.handleDeleteTransaction(params)
	case "get_analytics":
		return ct.handleGetAnalytics(params)
	case "add_category":
		return ct.handleAddCategory(params)
	case "get_categories":
		return ct.handleGetCategories()
	default:
		return fmt.Sprintf("Error: invalid action: %s", action)
	}
}

func (ct *CashFlowTool) handleAddTransaction(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	transactionData, ok := params["transaction"].(map[string]interface{})
	if !ok {
		return "Error: invalid transaction data"
	}

	transactionType, ok := transactionData["type"].(string)
	if !ok || (transactionType != "income" && transactionType != "expense") {
		return "Error: invalid transaction type"
	}

	amount, ok := transactionData["amount"].(float64)
	if !ok || amount <= 0 {
		return "Error: transaction amount must be greater than 0"
	}

	currency, ok := transactionData["currency"].(string)
	if !ok {
		currency = string(IDR)
	}
	currencyType := CurrencyType(currency)
	if currencyType != IDR && currencyType != USD && currencyType != EUR && currencyType != JPY && currencyType != GBP {
		return "Error: invalid currency"
	}

	dateStr, ok := transactionData["date"].(string)
	if !ok {
		return "Error: invalid date format"
	}
	date, err := parseDate(dateStr)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	categoryData, ok := transactionData["category"].(map[string]interface{})
	if !ok {
		return "Error: invalid category data"
	}

	categoryName, ok := categoryData["name"].(string)
	if !ok || categoryName == "" {
		return "Error: category name cannot be empty"
	}

	categoryName = toLowerCase(categoryName)

	transaction := Transaction{
		ID:          fmt.Sprintf("trx_%d", time.Now().UnixNano()),
		UserID:      userID,
		Type:        TransactionType(transactionType),
		Amount:      amount,
		Currency:    currencyType,
		Description: transactionData["description"].(string),
		Date:        date,
		Category: Category{
			ID:   fmt.Sprintf("cat_%d", time.Now().UnixNano()),
			Name: categoryName,
		},
	}

	categoryExists := false
	for _, c := range data.Categories {
		if c.Name == transaction.Category.Name {
			transaction.Category.ID = c.ID
			categoryExists = true
			break
		}
	}
	if !categoryExists {
		data.Categories = append(data.Categories, transaction.Category)
	}

	data.Transactions = append(data.Transactions, transaction)

	if err := ct.saveData(data); err != nil {
		return fmt.Sprintf("Error saving data: %v", err)
	}

	return fmt.Sprintf("Transaction added successfully with ID: %s", transaction.ID)
}

func (ct *CashFlowTool) handleGetTransactions(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	dateRange, ok := params["date_range"].(map[string]interface{})
	if !ok {
		return "Error: invalid date range"
	}

	start, err := parseDate(dateRange["start"].(string))
	if err != nil {
		return fmt.Sprintf("Error: invalid start date format: %v", err)
	}

	end, err := parseDate(dateRange["end"].(string))
	if err != nil {
		return fmt.Sprintf("Error: invalid end date format: %v", err)
	}

	var filteredTransactions []Transaction
	for _, t := range data.Transactions {
		if t.UserID == userID && (t.Date.Equal(start) || t.Date.After(start)) && (t.Date.Equal(end) || t.Date.Before(end)) {
			filteredTransactions = append(filteredTransactions, t)
		}
	}

	result, err := json.Marshal(filteredTransactions)
	if err != nil {
		return fmt.Sprintf("Error: failed to convert data: %v", err)
	}

	if err := ct.saveData(data); err != nil {
		return fmt.Sprintf("Error saving data: %v", err)
	}

	return string(result)
}

func (ct *CashFlowTool) handleUpdateTransaction(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	transactionID, ok := params["transaction_id"].(string)
	if !ok {
		return "Error: transaction ID not found"
	}

	transactionData, ok := params["transaction"].(map[string]interface{})
	if !ok {
		return "Error: invalid transaction data"
	}

	transactionType, ok := transactionData["type"].(string)
	if !ok || (transactionType != "income" && transactionType != "expense") {
		return "Error: invalid transaction type"
	}

	amount, ok := transactionData["amount"].(float64)
	if !ok || amount <= 0 {
		return "Error: transaction amount must be greater than 0"
	}

	currency, ok := transactionData["currency"].(string)
	if !ok {
		currency = string(IDR)
	}
	currencyType := CurrencyType(currency)
	if currencyType != IDR && currencyType != USD && currencyType != EUR && currencyType != JPY && currencyType != GBP {
		return "Error: invalid currency"
	}

	dateStr, ok := transactionData["date"].(string)
	if !ok {
		return "Error: invalid date format"
	}
	date, err := parseDate(dateStr)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	categoryData, ok := transactionData["category"].(map[string]interface{})
	if !ok {
		return "Error: invalid category data"
	}

	categoryName, ok := categoryData["name"].(string)
	if !ok || categoryName == "" {
		return "Error: category name cannot be empty"
	}

	categoryName = toLowerCase(categoryName)

	found := false
	for i, t := range data.Transactions {
		if t.ID == transactionID && t.UserID == userID {
			categoryExists := false
			for _, c := range data.Categories {
				if c.Name == categoryName {
					data.Transactions[i] = Transaction{
						ID:          transactionID,
						UserID:      userID,
						Type:        TransactionType(transactionType),
						Amount:      amount,
						Currency:    currencyType,
						Description: transactionData["description"].(string),
						Date:        date,
						Category:    c,
					}
					categoryExists = true
					break
				}
			}

			if !categoryExists {
				newCategory := Category{
					ID:   fmt.Sprintf("cat_%d", time.Now().UnixNano()),
					Name: categoryName,
				}
				data.Categories = append(data.Categories, newCategory)
				data.Transactions[i] = Transaction{
					ID:          transactionID,
					UserID:      userID,
					Type:        TransactionType(transactionType),
					Amount:      amount,
					Currency:    currencyType,
					Description: transactionData["description"].(string),
					Date:        date,
					Category:    newCategory,
				}
			}

			found = true
			break
		}
	}

	if !found {
		return "Error: transaction not found"
	}

	if err := ct.saveData(data); err != nil {
		return fmt.Sprintf("Error saving data: %v", err)
	}

	return fmt.Sprintf("Transaction updated successfully with ID: %s", transactionID)
}

func (ct *CashFlowTool) handleDeleteTransaction(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	transactionID, ok := params["transaction_id"].(string)
	if !ok {
		return "Error: transaction ID not found"
	}

	found := false
	for i, t := range data.Transactions {
		if t.ID == transactionID && t.UserID == userID {
			data.Transactions = append(data.Transactions[:i], data.Transactions[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return "Error: transaction not found"
	}

	if err := ct.saveData(data); err != nil {
		return fmt.Sprintf("Error saving data: %v", err)
	}

	return "Transaction deleted successfully"
}

func (ct *CashFlowTool) handleGetAnalytics(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	dateRange, ok := params["date_range"].(map[string]interface{})
	if !ok {
		return "Error: invalid date range"
	}

	start, err := parseDate(dateRange["start"].(string))
	if err != nil {
		return fmt.Sprintf("Error: invalid start date format: %v", err)
	}

	end, err := parseDate(dateRange["end"].(string))
	if err != nil {
		return fmt.Sprintf("Error: invalid end date format: %v", err)
	}

	var totalIncome, totalExpense float64
	incomeByCategory := make(map[string]float64)
	expenseByCategory := make(map[string]float64)
	transactionCount := 0

	for _, t := range data.Transactions {
		if t.UserID == userID && (t.Date.Equal(start) || t.Date.After(start)) && (t.Date.Equal(end) || t.Date.Before(end)) {
			transactionCount++
			if t.Type == Income {
				totalIncome += t.Amount
				incomeByCategory[t.Category.Name] += t.Amount
			} else {
				totalExpense += t.Amount
				expenseByCategory[t.Category.Name] += t.Amount
			}
		}
	}

	analytics := map[string]interface{}{
		"total_income":        totalIncome,
		"total_expense":       totalExpense,
		"balance":             totalIncome - totalExpense,
		"income_by_category":  incomeByCategory,
		"expense_by_category": expenseByCategory,
		"transaction_count":   transactionCount,
	}

	result, err := json.Marshal(analytics)
	if err != nil {
		return fmt.Sprintf("Error: failed to convert data: %v", err)
	}

	return string(result)
}

func (ct *CashFlowTool) handleAddCategory(params map[string]interface{}) string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	categoryData, ok := params["category"].(map[string]interface{})
	if !ok {
		return "Error: invalid category data"
	}

	name, ok := categoryData["name"].(string)
	if !ok || name == "" {
		return "Error: category name cannot be empty"
	}

	name = toLowerCase(name)

	for _, c := range data.Categories {
		if c.Name == name {
			return "Error: category with this name already exists"
		}
	}

	id, _ := categoryData["id"].(string)
	if id == "" {
		id = fmt.Sprintf("cat_%d", time.Now().UnixNano())
	}

	category := Category{
		ID:   id,
		Name: name,
	}

	data.Categories = append(data.Categories, category)

	if err := ct.saveData(data); err != nil {
		return fmt.Sprintf("Error saving data: %v", err)
	}

	return "Category added successfully"
}

func (ct *CashFlowTool) handleGetCategories() string {
	data, err := ct.loadData()
	if err != nil {
		return fmt.Sprintf("Error loading data: %v", err)
	}

	result, err := json.Marshal(data.Categories)
	if err != nil {
		return fmt.Sprintf("Error: failed to convert data: %v", err)
	}

	return string(result)
}
