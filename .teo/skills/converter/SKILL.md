---
name: Converter
description: A tool to convert values between various units (excluding currency).
---

# Converter Skill

A tool to convert values between various units (excluding currency). Supported categories include:
*   **Temperature**: Celsius, Fahrenheit, Kelvin
*   **Distance**: meter, kilometer, centimeter, inch, foot
*   **Mass**: gram, kilogram, ounce, pound
*   **Volume**: liter, milliliter, gallon, quart
*   **Time**: second, minute, hour
*   **Speed**: meter per second, kilometer per hour, mile per hour

## Usage

The skill is executed via a Python script: `.teo/skills/converter/scripts/converter.py`.
It accepts a JSON string as the first argument calling the tool.

### Arguments

The input JSON should contain the following fields:

| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `value` | number | The value to convert. | Yes |
| `from_unit` | string | The source unit (e.g., meter, ounce, celsius). | Yes |
| `to_unit` | string | The target unit (e.g., kilometer, gram, fahrenheit). | Yes |

#### Supported Units

*   `celsius`, `fahrenheit`, `kelvin`
*   `meter`, `kilometer`, `centimeter`, `inch`, `foot`
*   `gram`, `kilogram`, `ounce`, `pound`
*   `liter`, `milliliter`, `gallon`, `quart`
*   `second`, `minute`, `hour`
*   `meter per second`, `kilometer per hour`, `mile per hour`

### Example

```bash
.venv/bin/python .teo/skills/converter/scripts/converter.py '{"value": 100, "from_unit": "celsius", "to_unit": "fahrenheit"}'
```
