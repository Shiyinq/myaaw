import json
import sys
import urllib.request
import urllib.parse
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

    location = params.get("location")
    unit = params.get("unit", "celsius")

    if not location:
        print("Error: location is required")
        sys.exit(1)

    # Encode location for URL
    encoded_location = urllib.parse.quote(location)
    url = f"http://wttr.in/{encoded_location}?format=j1"

    try:
        with urllib.request.urlopen(url) as response:
            if response.status != 200:
                print(f"Error response from API: {response.status}")
                return
            
            data = json.loads(response.read().decode('utf-8'))
            
            current_condition = data.get("current_condition", [])
            if not current_condition:
                print("No current weather data available.")
                return

            current = current_condition[0]
            
            temp = ""
            temp_feels_like = ""
            
            if unit == "fahrenheit":
                temp = current.get("temp_F")
                temp_feels_like = current.get("FeelsLikeF")
            else:
                temp = current.get("temp_C")
                temp_feels_like = current.get("FeelsLikeC")
                
            weather_desc_list = current.get("weatherDesc", [])
            weather_desc = ""
            for item in weather_desc_list:
                weather_desc += item.get("value", "") + "\n"
            weather_desc = weather_desc.strip()

            print(f"The current weather in {location} is {weather_desc} and temperature is {temp}°{unit} (feels like {temp_feels_like}°{unit}).")

    except urllib.error.HTTPError as e:
        print(f"Error making request: {e}")
    except Exception as e:
        print(f"Error making request: {e}")

if __name__ == "__main__":
    main()
