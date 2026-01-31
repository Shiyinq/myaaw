import sys
import json
import os
import datetime
import time

# Constants
DATA_DIR = os.path.join("data", "cashflow")
DATA_FILE = os.path.join(DATA_DIR, "cashflow.json")

def ensure_data_dir():
    if not os.path.exists(DATA_DIR):
        os.makedirs(DATA_DIR)

def load_data():
    ensure_data_dir()
    if os.path.exists(DATA_FILE):
        try:
            with open(DATA_FILE, 'r') as f:
                data = json.load(f)
                # Ensure structure
                if "transactions" not in data: data["transactions"] = []
                if "categories" not in data: data["categories"] = []
                return data
        except Exception:
            pass
    return {"transactions": [], "categories": []}

def save_data(data):
    ensure_data_dir()
    with open(DATA_FILE, 'w') as f:
        json.dump(data, f, indent=2)

def handle_add_transaction(user_id, params):
    data = load_data()
    t_data = params.get("transaction", {})
    
    # Validation
    if t_data.get("amount", 0) <= 0:
        return json.dumps({"error": "Error: transaction amount must be greater than 0"})
    if t_data.get("type") not in ["income", "expense"]:
        return json.dumps({"error": "Error: invalid transaction type"})
    
    cat_name = t_data.get("category", {}).get("name", "").lower()
    if not cat_name:
         return json.dumps({"error": "Error: category name cannot be empty"})

    # Check/Create Category
    cat_id = f"cat_{time.time_ns()}"
    cat_obj = {"id": cat_id, "name": cat_name}
    
    found_cat = False
    for c in data["categories"]:
        if c["name"] == cat_name:
            cat_obj = c
            found_cat = True
            break
    
    if not found_cat:
        data["categories"].append(cat_obj)

    # Create Transaction
    new_t = {
        "id": f"trx_{time.time_ns()}",
        "user_id": user_id,
        "type": t_data["type"],
        "amount": t_data["amount"],
        "currency": t_data.get("currency", "IDR"),
        "category": cat_obj,
        "description": t_data.get("description", ""),
        "date": t_data.get("date", datetime.datetime.now().isoformat())
    }
    
    data["transactions"].append(new_t)
    save_data(data)
    return json.dumps({"message": f"Transaction added successfully with ID: {new_t['id']}"})

def handle_get_transactions(user_id, params):
    data = load_data()
    date_range = params.get("date_range", {})
    start = date_range.get("start")
    end = date_range.get("end")
    
    if not start or not end:
        return json.dumps({"error": "Error: invalid date range"})

    results = []
    # Simple string comparison for ISO dates
    for t in data["transactions"]:
        if t["user_id"] == user_id:
            if t["date"] >= start and t["date"] <= end:
                results.append(t)
                
    return json.dumps(results, indent=2)

def handle_update_transaction(user_id, params):
    data = load_data()
    trx_id = params.get("transaction_id")
    t_data = params.get("transaction", {})
    
    if not trx_id: return json.dumps({"error": "Error: transaction ID not found"})

    # Find transaction
    trx_idx = -1
    for i, t in enumerate(data["transactions"]):
        if t["id"] == trx_id and t["user_id"] == user_id:
            trx_idx = i
            break
            
    if trx_idx == -1:
        return json.dumps({"error": "Error: transaction not found"})

    # Update logic (similar to add, handle category check)
    cat_name = t_data.get("category", {}).get("name", "").lower()
    if cat_name:
        cat_obj = {"id": f"cat_{time.time_ns()}", "name": cat_name}
        found_cat = False
        for c in data["categories"]:
            if c["name"] == cat_name:
                cat_obj = c
                found_cat = True
                break
        if not found_cat:
            data["categories"].append(cat_obj)
        data["transactions"][trx_idx]["category"] = cat_obj

    if "amount" in t_data: data["transactions"][trx_idx]["amount"] = t_data["amount"]
    if "type" in t_data: data["transactions"][trx_idx]["type"] = t_data["type"]
    if "description" in t_data: data["transactions"][trx_idx]["description"] = t_data["description"]
    if "date" in t_data: data["transactions"][trx_idx]["date"] = t_data["date"]
    
    save_data(data)
    return json.dumps({"message": f"Transaction updated successfully with ID: {trx_id}"})

def handle_delete_transaction(user_id, params):
    data = load_data()
    trx_id = params.get("transaction_id")
    
    initial_len = len(data["transactions"])
    data["transactions"] = [t for t in data["transactions"] if not (t["id"] == trx_id and t["user_id"] == user_id)]
    
    if len(data["transactions"]) == initial_len:
        return json.dumps({"error": "Error: transaction not found"})
        
    save_data(data)
    return json.dumps({"message": "Transaction deleted successfully"})

def handle_get_analytics(user_id, params):
    data = load_data()
    date_range = params.get("date_range", {})
    start = date_range.get("start")
    end = date_range.get("end")
    
    total_income = 0
    total_expense = 0
    income_by_category = {}
    expense_by_category = {}
    count = 0
    
    for t in data["transactions"]:
        if t["user_id"] == user_id:
             if t["date"] >= start and t["date"] <= end:
                 count += 1
                 amt = t["amount"]
                 cat = t["category"]["name"]
                 
                 if t["type"] == "income":
                     total_income += amt
                     income_by_category[cat] = income_by_category.get(cat, 0) + amt
                 else:
                     total_expense += amt
                     expense_by_category[cat] = expense_by_category.get(cat, 0) + amt
                     
    analytics = {
        "total_income": total_income,
        "total_expense": total_expense,
        "balance": total_income - total_expense,
        "income_by_category": income_by_category,
        "expense_by_category": expense_by_category,
        "transaction_count": count
    }
    return json.dumps(analytics, indent=2)

def handle_add_category(params):
    data = load_data()
    cat_data = params.get("category", {})
    name = cat_data.get("name", "").lower()
    
    if not name: return json.dumps({"error": "Error: category name cannot be empty"})
    
    for c in data["categories"]:
        if c["name"] == name:
            return json.dumps({"error": "Error: category with this name already exists"})
            
    new_cat = {
        "id": cat_data.get("id") or f"cat_{time.time_ns()}",
        "name": name
    }
    data["categories"].append(new_cat)
    save_data(data)
    return json.dumps({"message": "Category added successfully"})

def handle_get_categories():
    data = load_data()
    return json.dumps(data["categories"], indent=2)

def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "No arguments provided"}))
        return

    try:
        args = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print(json.dumps({"error": "Invalid JSON arguments"}))
        return

    action = args.get("action", "")
    user_id = args.get("user_id", "")
    
    # Validation for user_id on user-specific actions
    if action in ["add_transaction", "get_transactions", "update_transaction", "delete_transaction", "get_analytics"] and not user_id:
         print(json.dumps({"error": "Error: user_id is required"}))
         return

    if action == "add_transaction":
        print(handle_add_transaction(user_id, args))
    elif action == "get_transactions":
        print(handle_get_transactions(user_id, args))
    elif action == "update_transaction":
        print(handle_update_transaction(user_id, args))
    elif action == "delete_transaction":
        print(handle_delete_transaction(user_id, args))
    elif action == "get_analytics":
        print(handle_get_analytics(user_id, args))
    elif action == "add_category":
        print(handle_add_category(args))
    elif action == "get_categories":
        print(handle_get_categories())
    else:
        print(json.dumps({"error": f"Error: invalid action: {action}"}))

if __name__ == "__main__":
    main()
