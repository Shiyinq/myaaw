# Unit Converter Tool

A comprehensive tool for converting values between different units of measurement across multiple categories.

## Overview

The Unit Converter Tool provides conversion capabilities for various measurement units including temperature, distance, mass, volume, time, and speed. It supports a wide range of common units and provides accurate conversion results.

## Features

- **Multi-category Conversions**: Temperature, distance, mass, volume, time, speed
- **Wide Unit Support**: Common units across different measurement systems
- **Precise Calculations**: Accurate conversion formulas
- **Error Handling**: Comprehensive validation and error reporting
- **Flexible Input**: Supports various unit formats and spellings

## Supported Conversion Categories

### Temperature
- **Celsius** (°C)
- **Fahrenheit** (°F)
- **Kelvin** (K)

### Distance/Length
- **Meter** (m)
- **Kilometer** (km)
- **Centimeter** (cm)
- **Inch** (in)
- **Foot** (ft)

### Mass/Weight
- **Gram** (g)
- **Kilogram** (kg)
- **Ounce** (oz)
- **Pound** (lb)

### Volume
- **Liter** (L)
- **Milliliter** (mL)
- **Gallon** (gal)
- **Quart** (qt)

### Time
- **Second** (s)
- **Minute** (min)
- **Hour** (h)

### Speed
- **Meter per second** (m/s)
- **Kilometer per hour** (km/h)
- **Mile per hour** (mph)

## Usage

### Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `value` | number | Yes | The value to convert | 100 |
| `from_unit` | string | Yes | Source unit | "celsius" |
| `to_unit` | string | Yes | Target unit | "fahrenheit" |

### Example Usage

#### Temperature Conversion
```json
{
  "value": 25,
  "from_unit": "celsius",
  "to_unit": "fahrenheit"
}
```

#### Distance Conversion
```json
{
  "value": 1000,
  "from_unit": "meter",
  "to_unit": "kilometer"
}
```

#### Mass Conversion
```json
{
  "value": 500,
  "from_unit": "gram",
  "to_unit": "pound"
}
```

### Response Format

The tool returns a human-readable string with the conversion result:

```
25 celsius is equal to 77.00 fahrenheit
```

## Implementation Details

### Dependencies

- Standard Go packages only - No external dependencies

### Key Functions

- `NewConverterTool()` - Creates new converter tool instance
- `CallTool(arguments string)` - Main function that processes conversions
- `convert(value float64, fromUnit, toUnit string)` - Main conversion logic
- `convertTemperature(value float64, fromUnit, toUnit string)` - Temperature conversions
- `convertDistance(value float64, fromUnit, toUnit string)` - Distance conversions
- `convertMass(value float64, fromUnit, toUnit string)` - Mass conversions
- `convertVolume(value float64, fromUnit, toUnit string)` - Volume conversions
- `convertTime(value float64, fromUnit, toUnit string)` - Time conversions
- `convertSpeed(value float64, fromUnit, toUnit string)` - Speed conversions

### Conversion Formulas

#### Temperature
- **Celsius to Fahrenheit**: °F = (°C × 9/5) + 32
- **Celsius to Kelvin**: K = °C + 273.15
- **Fahrenheit to Celsius**: °C = (°F - 32) × 5/9
- **Fahrenheit to Kelvin**: K = (°F - 32) × 5/9 + 273.15
- **Kelvin to Celsius**: °C = K - 273.15
- **Kelvin to Fahrenheit**: °F = (K - 273.15) × 9/5 + 32

#### Distance
- **Meter to Kilometer**: km = m ÷ 1000
- **Meter to Centimeter**: cm = m × 100
- **Meter to Inch**: in = m × 39.3701
- **Meter to Foot**: ft = m × 3.28084

#### Mass
- **Gram to Kilogram**: kg = g ÷ 1000
- **Gram to Ounce**: oz = g ÷ 28.3495
- **Gram to Pound**: lb = g ÷ 453.592
- **Kilogram to Gram**: g = kg × 1000
- **Kilogram to Ounce**: oz = kg × 35.274
- **Kilogram to Pound**: lb = kg × 2.20462

## Error Handling

### Invalid Units
```
Error: unsupported source unit: 'invalid_unit'
```

### Unsupported Conversions
```
Error: temperature conversion from celsius to kilometer not supported
```

### Invalid Arguments
```
Error: argument "value" must be a number
Error: invalid "from_unit" argument
Error: invalid "to_unit" argument
```

## Unit Recognition

### Case Insensitive
The tool accepts units in any case:
- "CELSIUS", "celsius", "Celsius" all work

### Common Variations
- "meter", "meters", "m" (for distance)
- "gram", "grams", "g" (for mass)
- "celsius", "c", "°c" (for temperature)

## Use Cases

- **Scientific Calculations**: Laboratory and research applications
- **Engineering**: Design and construction projects
- **Cooking**: Recipe conversions
- **Travel**: Distance and speed conversions
- **Education**: Teaching measurement concepts
- **International Trade**: Unit conversions for global commerce

## Best Practices

- Use standard unit names for best compatibility
- Validate input values before conversion
- Handle conversion errors gracefully
- Consider precision requirements for your use case
- Use appropriate units for your domain

## Limitations

- No currency conversion support
- Limited to supported unit categories
- No complex unit expressions (e.g., "m/s²")
- No historical unit support
- No custom unit definitions
- Precision limited to float64

## Performance

- Fast execution (no external API calls)
- Minimal memory usage
- Efficient conversion algorithms
- No network dependencies

## Examples

### Complete Conversion Examples

```json
// Temperature
{"value": 0, "from_unit": "celsius", "to_unit": "fahrenheit"}
// Result: 0 celsius is equal to 32.00 fahrenheit

// Distance
{"value": 1, "from_unit": "kilometer", "to_unit": "meter"}
// Result: 1 kilometer is equal to 1000.00 meter

// Mass
{"value": 1, "from_unit": "pound", "to_unit": "gram"}
// Result: 1 pound is equal to 453.59 gram

// Volume
{"value": 1, "from_unit": "gallon", "to_unit": "liter"}
// Result: 1 gallon is equal to 3.79 liter

// Time
{"value": 60, "from_unit": "minute", "to_unit": "hour"}
// Result: 60 minute is equal to 1.00 hour

// Speed
{"value": 100, "from_unit": "kilometer per hour", "to_unit": "meter per second"}
// Result: 100 kilometer per hour is equal to 27.78 meter per second
```  
