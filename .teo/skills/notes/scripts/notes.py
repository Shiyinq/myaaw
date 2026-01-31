import sys
import json
import os
import datetime
import glob

# Constants
DATA_DIR = os.path.join("data", "notes")

def ensure_user_dir(user_id):
    user_path = os.path.join(DATA_DIR, user_id)
    if not os.path.exists(user_path):
        os.makedirs(user_path)
    return user_path

def get_note_path(user_id, title):
    return os.path.join(DATA_DIR, user_id, f"{title}.json")

def validate_input(args):
    action = args.get("action", "").upper()
    if not action:
        return "Error: action is required"
    if action in ["POST", "PUT"]:
        if not args.get("title"):
            return "Error: title is required"
        if not args.get("content"):
            return "Error: content is required"
    if not args.get("user_id"):
        return "Error: user_id is required"
    return None

def get_notes(user_id):
    user_path = ensure_user_dir(user_id)
    files = glob.glob(os.path.join(user_path, "*.json"))
    notes = []
    for file_path in files:
        try:
            with open(file_path, 'r') as f:
                note = json.load(f)
                notes.append(note)
        except Exception:
            continue
    return json.dumps(notes, indent=2)

def get_note_detail(user_id, title):
    file_path = get_note_path(user_id, title)
    if not os.path.exists(file_path):
        return json.dumps({"error": f"Note '{title}' not found"})
    try:
        with open(file_path, 'r') as f:
            note = json.load(f)
        return json.dumps(note, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})

def save_note(user_id, title, content):
    ensure_user_dir(user_id)
    file_path = get_note_path(user_id, title)
    if os.path.exists(file_path):
        return json.dumps({"error": f"Note '{title}' already exists. Use PUT to update it."})
    
    note = {
        "title": title,
        "content": content,
        "created_at": datetime.datetime.now().isoformat(),
        "updated_at": datetime.datetime.now().isoformat()
    }
    
    with open(file_path, 'w') as f:
        json.dump(note, f, indent=2)
    return json.dumps({"message": f"Note '{title}' has been saved successfully."})

def update_note(user_id, title, content):
    file_path = get_note_path(user_id, title)
    if not os.path.exists(file_path):
        return json.dumps({"error": f"Note '{title}' does not exist. Use POST to create it."})
    
    with open(file_path, 'r') as f:
        note = json.load(f)
    
    note["content"] = content
    note["updated_at"] = datetime.datetime.now().isoformat()
    
    with open(file_path, 'w') as f:
        json.dump(note, f, indent=2)
    return json.dumps({"message": f"Note '{title}' has been updated successfully."})

def delete_note(user_id, title):
    file_path = get_note_path(user_id, title)
    if not os.path.exists(file_path):
        return json.dumps({"error": f"Note '{title}' does not exist."})
    
    os.remove(file_path)
    return json.dumps({"message": f"Note '{title}' has been deleted successfully."})

def search_notes(user_id, query):
    user_path = ensure_user_dir(user_id)
    files = glob.glob(os.path.join(user_path, "*.json"))
    results = []
    query = query.lower()
    for file_path in files:
        try:
            with open(file_path, 'r') as f:
                note = json.load(f)
                if query in note["title"].lower() or query in note["content"].lower():
                    results.append(note)
        except Exception:
            continue
    return json.dumps(results, indent=2)

def get_notes_by_date(user_id, start_date, end_date):
    user_path = ensure_user_dir(user_id)
    try:
         # Flexible parsing (handling RFC3339 or YYYY-MM-DD)
        start_dt = datetime.datetime.fromisoformat(start_date) if "T" in start_date else datetime.datetime.strptime(start_date, "%Y-%m-%d")
        end_dt = datetime.datetime.fromisoformat(end_date) if "T" in end_date else datetime.datetime.strptime(end_date, "%Y-%m-%d")
        # Ensure end date includes the full day if it was just YYYY-MM-DD
        if "T" not in end_date:
             end_dt = end_dt + datetime.timedelta(days=1) - datetime.timedelta(microseconds=1)

    except ValueError as e:
        return json.dumps({"error": f"Invalid date format: {str(e)}"})
        
    files = glob.glob(os.path.join(user_path, "*.json"))
    results = []
    for file_path in files:
        try:
            with open(file_path, 'r') as f:
                note = json.load(f)
                created_at = datetime.datetime.fromisoformat(note["created_at"])
                # Compare naive or aware datetimes carefully, usually isoformat from python is aware if timezone provided
                # simplified comparison:
                if start_dt.replace(tzinfo=None) <= created_at.replace(tzinfo=None) <= end_dt.replace(tzinfo=None):
                    results.append(note)
        except Exception:
            continue
    return json.dumps(results, indent=2)

def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "No arguments provided"}))
        return

    try:
        args = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print(json.dumps({"error": "Invalid JSON arguments"}))
        return

    error = validate_input(args)
    if error:
        print(json.dumps({"error": error}))
        return

    action = args["action"].upper()
    user_id = args["user_id"]

    if action == "GET":
        print(get_notes(user_id))
    elif action == "GET_DETAIL":
        print(get_note_detail(user_id, args.get("title", "")))
    elif action == "POST":
        print(save_note(user_id, args["title"], args["content"]))
    elif action == "PUT":
        print(update_note(user_id, args["title"], args["content"]))
    elif action == "DELETE":
        print(delete_note(user_id, args.get("title", "")))
    elif action == "SEARCH":
        print(search_notes(user_id, args.get("search", "")))
    elif action == "GET_BY_DATE":
        print(get_notes_by_date(user_id, args.get("start_date", ""), args.get("end_date", "")))
    else:
        print(json.dumps({"error": f"Unknown action: {action}"}))

if __name__ == "__main__":
    main()
