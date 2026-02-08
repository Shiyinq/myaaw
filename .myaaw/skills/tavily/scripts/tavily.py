import json
import os
import sys
import urllib.request
import urllib.error

# Simple .env loader to avoid dependencies
def load_dotenv():
    env_path = os.path.join(os.getcwd(), '.env')
    if os.path.exists(env_path):
        try:
            with open(env_path, 'r') as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith('#'):
                        continue
                    if '=' in line:
                        key, value = line.split('=', 1)
                        # Remove quotes if present
                        value = value.strip()
                        if (value.startswith('"') and value.endswith('"')) or \
                           (value.startswith("'") and value.endswith("'")):
                            value = value[1:-1]
                        os.environ[key.strip()] = value
        except Exception:
            pass

TAVILY_API_URL = "https://api.tavily.com"

def call_tavily(endpoint, payload, api_key):
    url = f"{TAVILY_API_URL}{endpoint}"
    data = json.dumps(payload).encode('utf-8')
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}"
    }
    
    try:
        req = urllib.request.Request(url, data=data, headers=headers, method='POST')
        with urllib.request.urlopen(req) as response:
            if response.status != 200:
                return f"Error: Tavily API request failed with status {response.status}"
            return response.read().decode('utf-8')
    except urllib.error.HTTPError as e:
        return f"Error: Tavily API request failed with status {e.code}: {e.read().decode('utf-8')}"
    except Exception as e:
        return f"Error making request to Tavily API: {e}"

def main():
    load_dotenv()
    
    if len(sys.argv) < 2:
        print("Error: No arguments provided")
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print("Error: Invalid JSON argument")
        sys.exit(1)

    api_key = os.environ.get("TAVILY_API_KEY")
    if not api_key:
        print("Error: TAVILY_API_KEY environment variable not set in .env file or system environment.")
        sys.exit(1)  # Or just return the error string? Go version returned string.

    action = params.get("action")
    if not action:
        print("Error: Action not found")
        sys.exit(1)

    result = ""
    if action == "search":
        search_args = params.get("search_args")
        if not search_args:
            print("Error: search_args are required for search action.")
            sys.exit(1)
        result = call_tavily("/search", search_args, api_key)
        
    elif action == "extract":
        extract_args = params.get("extract_args")
        if not extract_args:
            print("Error: extract_args are required for extract action.")
            sys.exit(1)
        result = call_tavily("/extract", extract_args, api_key)
        
    else:
        print(f"Error: Invalid action '{action}'. Must be 'search' or 'extract'.")
        sys.exit(1)

    print(result)

if __name__ == "__main__":
    main()
