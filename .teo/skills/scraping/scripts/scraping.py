import json
import sys
import urllib.request
import urllib.error

def main():
    if len(sys.argv) < 2:
        print("Error: No arguments provided")
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print("Error: Invalid JSON argument")
        sys.exit(1)

    url_arg = params.get("url")
    if not url_arg:
        print("Error: url is required")
        sys.exit(1)

    # Jina AI Reader API
    api_url = f"https://r.jina.ai/{url_arg}"
    
    try:
        req = urllib.request.Request(api_url)
        # Jina AI might require headers sometimes, but usually works without for basic usage
        # Adding a User-Agent just in case
        req.add_header('User-Agent', 'Mozilla/5.0 (compatible; TeoSkill/1.0)')

        with urllib.request.urlopen(req) as response:
            if response.status != 200:
                print(f"Error response from API: {response.status}")
                return
            
            print(response.read().decode('utf-8'))

    except urllib.error.HTTPError as e:
         print(f"Error making request: {e}")
    except Exception as e:
        print(f"Error making request: {e}")

if __name__ == "__main__":
    main()
