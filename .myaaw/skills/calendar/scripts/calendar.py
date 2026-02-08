import sys
import json
import os
import datetime
import time

# Constants
DATA_DIR = os.path.join("data", "calendar")
DATA_FILE = os.path.join(DATA_DIR, "calendar.json")

def ensure_data_dir():
    if not os.path.exists(DATA_DIR):
        os.makedirs(DATA_DIR)

def load_schedules():
    ensure_data_dir()
    if os.path.exists(DATA_FILE):
        try:
            with open(DATA_FILE, 'r') as f:
                data = json.load(f)
                if isinstance(data, list):
                    return data
        except Exception:
            return []
    return []

def save_schedules(schedules):
    ensure_data_dir()
    with open(DATA_FILE, 'w') as f:
        json.dump(schedules, f, indent=2)

def add_schedule(user_id, schedule_data):
    schedules = load_schedules()
    
    # Validation
    required_fields = ["title", "description", "start_time", "end_time", "tags"]
    for field in required_fields:
        if field not in schedule_data:
            return json.dumps({"error": f"Error: {field} is required"})
            
    # Create Schedule object
    new_schedule = {
        "id": str(time.time_ns()),
        "user_id": user_id,
        "title": schedule_data["title"],
        "description": schedule_data["description"],
        "start_time": schedule_data["start_time"],
        "end_time": schedule_data["end_time"],
        "tags": schedule_data.get("tags", [])
    }
    
    schedules.append(new_schedule)
    save_schedules(schedules)
    return json.dumps({"message": "Schedule added successfully"})

def update_schedule(user_id, schedule_data):
    if "id" not in schedule_data:
         return json.dumps({"error": "Error: id is required"})
         
    schedules = load_schedules()
    target_id = schedule_data["id"]
    
    for i, s in enumerate(schedules):
        if s["id"] == target_id and s["user_id"] == user_id:
            # Update fields
            schedules[i]["title"] = schedule_data.get("title", s["title"])
            schedules[i]["description"] = schedule_data.get("description", s["description"])
            schedules[i]["start_time"] = schedule_data.get("start_time", s["start_time"])
            schedules[i]["end_time"] = schedule_data.get("end_time", s["end_time"])
            schedules[i]["tags"] = schedule_data.get("tags", s["tags"])
            
            save_schedules(schedules)
            return json.dumps({"message": "Schedule updated successfully"})
            
    return json.dumps({"error": f"schedule with ID {target_id} not found"})

def delete_schedule(user_id, schedule_id):
    schedules = load_schedules()
    original_len = len(schedules)
    
    schedules = [s for s in schedules if not (s["id"] == schedule_id and s["user_id"] == user_id)]
    
    if len(schedules) == original_len:
         return json.dumps({"error": f"schedule with ID {schedule_id} not found"})
         
    save_schedules(schedules)
    return json.dumps({"message": "Schedule deleted successfully"})

def search_by_date(user_id, start_str, end_str):
    schedules = load_schedules()
    results = []
    
    try:
        # Using string comparison for ISO format dates (RFC3339) is usually safe if consistent
        # But let's try to be a bit more robust if needed, though simple string compare works for ISO8601
        pass
    except Exception as e:
        return json.dumps({"error": f"Date parsing error: {e}"})

    for s in schedules:
        if s["user_id"] == user_id:
            # Check overlap or containment. 
            # Logic from Go: (s.StartTime >= start) && (s.EndTime <= end) ? 
            # Go logic: (s.StartTime.Equal(start) || s.StartTime.After(start)) && (s.EndTime.Equal(end) || s.EndTime.Before(end))
            # Meaning schedule must be entirely within the range? Or start/end within range?
            # Actually Go logic was: s.StartTime >= start AND s.EndTime <= end. So "contained within".
            
            if s["start_time"] >= start_str and s["end_time"] <= end_str:
                results.append(s)
                
    return json.dumps(results, indent=2)

def search_by_title(user_id, title_query):
    schedules = load_schedules()
    results = []
    title_query = title_query.lower()
    
    for s in schedules:
        if s["user_id"] == user_id and title_query in s["title"].lower():
            results.append(s)
            
    return json.dumps(results, indent=2)

def search_by_tags(user_id, tags):
    schedules = load_schedules()
    results = []
    
    for s in schedules:
        if s["user_id"] == user_id:
            for tag in tags:
                if tag in s.get("tags", []):
                    results.append(s)
                    break
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

    action = args.get("action", "")
    user_id = args.get("user_id", "")
    
    if not user_id:
        print(json.dumps({"error": "Error: user_id is required"}))
        return

    if action == "add_schedule":
        print(add_schedule(user_id, args.get("schedule", {})))
    elif action == "update_schedule":
        print(update_schedule(user_id, args.get("schedule", {})))
    elif action == "delete_schedule":
        print(delete_schedule(user_id, args.get("schedule_id", "")))
    elif action == "search_by_date":
        date_range = args.get("date_range", {})
        print(search_by_date(user_id, date_range.get("start", ""), date_range.get("end", "")))
    elif action == "search_by_title":
        print(search_by_title(user_id, args.get("title", "")))
    elif action == "search_by_tags":
        print(search_by_tags(user_id, args.get("tags", [])))
    else:
        print(json.dumps({"error": f"Error: invalid action: {action}"}))

if __name__ == "__main__":
    main()
