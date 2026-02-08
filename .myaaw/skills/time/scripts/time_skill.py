import sys
import json
import datetime
import pytz

def get_time(timezone_str=None):
    try:
        if timezone_str:
            tz = pytz.timezone(timezone_str)
            now = datetime.datetime.now(tz)
        else:
            # Default to local system time (which might be UTC in some envs)
            # or use 'localtime' if available
            now = datetime.datetime.now().astimezone()
            tz = now.tzinfo

        response = {
            "current_time": now.isoformat(),
            "timezone": str(tz),
            "year": now.year,
            "month": now.month,
            "day": now.day,
            "hour": now.hour,
            "minute": now.minute,
            "second": now.second,
            "weekday": now.strftime("%A")
        }
        return json.dumps(response, indent=2)
    except pytz.UnknownTimeZoneError:
        return json.dumps({"error": f"Invalid timezone: {timezone_str}"})
    except Exception as e:
        return json.dumps({"error": f"An error occurred: {str(e)}"})

if __name__ == "__main__":
    if len(sys.argv) > 1:
        timezone_arg = sys.argv[1]
    else:
        timezone_arg = None
    
    print(get_time(timezone_arg))
