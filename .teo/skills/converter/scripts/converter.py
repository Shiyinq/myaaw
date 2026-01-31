import json
import sys

def convert_temperature(value, from_unit, to_unit):
    # Convert to Celsius first
    celsius = 0
    if from_unit == "celsius":
        celsius = value
    elif from_unit == "fahrenheit":
        celsius = (value - 32) * 5/9
    elif from_unit == "kelvin":
        celsius = value - 273.15
        
    # Convert from Celsius to target
    if to_unit == "celsius":
        return celsius
    elif to_unit == "fahrenheit":
        return (celsius * 9/5) + 32
    elif to_unit == "kelvin":
        return celsius + 273.15
    return None

def convert_length(value, from_unit, to_unit):
    # Convert to meters first
    meters = 0
    factors = {
        "meter": 1,
        "kilometer": 1000,
        "centimeter": 0.01,
        "inch": 0.0254,
        "foot": 0.3048
    }
    
    if from_unit in factors:
        meters = value * factors[from_unit]
    else:
        return None
        
    if to_unit in factors:
        return meters / factors[to_unit]
    return None

def convert_mass(value, from_unit, to_unit):
    # Convert to grams first
    grams = 0
    factors = {
        "gram": 1,
        "kilogram": 1000,
        "ounce": 28.3495,
        "pound": 453.592
    }
    
    if from_unit in factors:
        grams = value * factors[from_unit]
    else:
        return None
        
    if to_unit in factors:
        return grams / factors[to_unit]
    return None
    
def convert_volume(value, from_unit, to_unit):
    # Convert to liters first
    liters = 0
    factors = {
        "liter": 1,
        "milliliter": 0.001,
        "gallon": 3.78541,
        "quart": 0.946353
    }

    if from_unit in factors:
        liters = value * factors[from_unit]
    else:
        return None
        
    if to_unit in factors:
        return liters / factors[to_unit]
    return None

def convert_time(value, from_unit, to_unit):
    # Convert to seconds first
    seconds = 0
    factors = {
        "second": 1,
        "minute": 60,
        "hour": 3600
    }
    
    if from_unit in factors:
        seconds = value * factors[from_unit]
    else:
        return None
        
    if to_unit in factors:
        return seconds / factors[to_unit]
    return None

def convert_speed(value, from_unit, to_unit):
    # Convert to m/s first
    ms = 0
    factors = {
        "meter per second": 1,
        "kilometer per hour": 0.277778,
        "mile per hour": 0.44704
    }
    
    if from_unit in factors:
        ms = value * factors[from_unit]
    else:
        return None
        
    if to_unit in factors:
        return ms / factors[to_unit]
    return None

def main():
    if len(sys.argv) < 2:
        print("Error: No arguments provided")
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print("Error: Invalid JSON argument")
        sys.exit(1)
        
    value = params.get("value")
    from_unit = params.get("from_unit")
    to_unit = params.get("to_unit")
    
    if value is None or not from_unit or not to_unit:
        print("Error: value, from_unit, and to_unit are required")
        sys.exit(1)
        
    try:
        value = float(value)
    except ValueError:
        print("Error: value must be a number")
        sys.exit(1)

    categories = [
        (["celsius", "fahrenheit", "kelvin"], convert_temperature),
        (["meter", "kilometer", "centimeter", "inch", "foot"], convert_length),
        (["gram", "kilogram", "ounce", "pound"], convert_mass),
        (["liter", "milliliter", "gallon", "quart"], convert_volume),
        (["second", "minute", "hour"], convert_time),
        (["meter per second", "kilometer per hour", "mile per hour"], convert_speed)
    ]
    
    result = None
    
    for units, converter in categories:
        if from_unit in units and to_unit in units:
            result = converter(value, from_unit, to_unit)
            break
            
    if result is not None:
        print(f"{value} {from_unit} is equal to {result:.4f} {to_unit}")
    else:
        print(f"Error: Conversion from {from_unit} to {to_unit} not supported or units mismatched")

if __name__ == "__main__":
    main()
