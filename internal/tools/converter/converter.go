package converter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ConverterTool struct{}

func (t *ConverterTool) Name() string {
	return "converter"
}

func (t *ConverterTool) Description() string {
	return "Converts values from one unit to another (excluding currency)."
}

func (t *ConverterTool) Args() map[string]any {
	return map[string]any{
		"value": map[string]any{
			"type":        "number",
			"description": "The value to convert",
		},
		"from_unit": map[string]any{
			"type":        "string",
			"description": "The source unit (e.g., meter, ounce, celsius)",
		},
		"to_unit": map[string]any{
			"type":        "string",
			"description": "The target unit (e.g., kilometer, gram, fahrenheit)",
		},
	}
}

func (t *ConverterTool) Run(args map[string]any) (string, error) {
	value, ok := args["value"].(float64)
	if !ok {
		return "", errors.New("argument \"value\" must be a number")
	}
	fromUnit, ok := args["from_unit"].(string)
	if !ok || fromUnit == "" {
		return "", errors.New("invalid \"from_unit\" argument")
	}
	toUnit, ok := args["to_unit"].(string)
	if !ok || toUnit == "" {
		return "", errors.New("invalid \"to_unit\" argument")
	}

	convertedValue, err := t.convert(value, fromUnit, toUnit)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%.2f %s is equal to %.2f %s", value, fromUnit, convertedValue, toUnit), nil
}

func (t *ConverterTool) convert(value float64, fromUnit, toUnit string) (float64, error) {
	fromUnit = strings.ToLower(strings.TrimSpace(fromUnit))
	toUnit = strings.ToLower(strings.TrimSpace(toUnit))

	switch fromUnit {
	case "celsius", "fahrenheit", "kelvin":
		return t.convertTemperature(value, fromUnit, toUnit)
	case "meter", "kilometer", "centimeter", "inch", "foot":
		return t.convertDistance(value, fromUnit, toUnit)
	case "gram", "kilogram", "ounce", "pound":
		return t.convertMass(value, fromUnit, toUnit)
	case "liter", "milliliter", "gallon", "quart":
		return t.convertVolume(value, fromUnit, toUnit)
	case "second", "minute", "hour":
		return t.convertTime(value, fromUnit, toUnit)
	case "meter per second", "kilometer per hour", "mile per hour":
		return t.convertSpeed(value, fromUnit, toUnit)
	default:
		return 0, fmt.Errorf("unsupported source unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertTemperature(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "celsius":
		switch toUnit {
		case "fahrenheit":
			return (value * 9 / 5) + 32, nil
		case "kelvin":
			return value + 273.15, nil
		default:
			return 0, fmt.Errorf("temperature conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "fahrenheit":
		switch toUnit {
		case "celsius":
			return (value - 32) * 5 / 9, nil
		case "kelvin":
			return (value-32)*5/9 + 273.15, nil
		default:
			return 0, fmt.Errorf("temperature conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "kelvin":
		switch toUnit {
		case "celsius":
			return value - 273.15, nil
		case "fahrenheit":
			return (value-273.15)*9/5 + 32, nil
		default:
			return 0, fmt.Errorf("temperature conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source temperature unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertDistance(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "meter":
		switch toUnit {
		case "kilometer":
			return value / 1000, nil
		case "centimeter":
			return value * 100, nil
		case "inch":
			return value * 39.3701, nil
		case "foot":
			return value * 3.28084, nil
		default:
			return 0, fmt.Errorf("distance conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "kilometer":
		switch toUnit {
		case "meter":
			return value * 1000, nil
		default:
			return 0, fmt.Errorf("distance conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "centimeter":
		switch toUnit {
		case "meter":
			return value / 100, nil
		case "inch":
			return value / 2.54, nil
		case "foot":
			return value * 0.0328084, nil
		default:
			return 0, fmt.Errorf("distance conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "inch":
		switch toUnit {
		case "meter":
			return value / 39.3701, nil
		case "centimeter":
			return value * 2.54, nil
		case "foot":
			return value / 12, nil
		default:
			return 0, fmt.Errorf("distance conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "foot":
		switch toUnit {
		case "meter":
			return value / 3.28084, nil
		case "inch":
			return value * 12, nil
		default:
			return 0, fmt.Errorf("distance conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source distance unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertMass(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "gram":
		switch toUnit {
		case "kilogram":
			return value / 1000, nil
		case "ounce":
			return value / 28.3495, nil
		case "pound":
			return value / 453.592, nil
		default:
			return 0, fmt.Errorf("mass conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "kilogram":
		switch toUnit {
		case "gram":
			return value * 1000, nil
		case "ounce":
			return value * 35.274, nil
		case "pound":
			return value * 2.20462, nil
		default:
			return 0, fmt.Errorf("mass conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "ounce":
		switch toUnit {
		case "gram":
			return value * 28.3495, nil
		case "kilogram":
			return value * 0.0283495, nil
		case "pound":
			return value / 16, nil
		default:
			return 0, fmt.Errorf("mass conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "pound":
		switch toUnit {
		case "gram":
			return value * 453.592, nil
		case "kilogram":
			return value * 0.453592, nil
		case "ounce":
			return value * 16, nil
		default:
			return 0, fmt.Errorf("mass conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source mass unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertVolume(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "liter":
		switch toUnit {
		case "milliliter":
			return value * 1000, nil
		case "gallon":
			return value * 0.264172, nil
		case "quart":
			return value * 1.05669, nil
		default:
			return 0, fmt.Errorf("volume conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "milliliter":
		switch toUnit {
		case "liter":
			return value / 1000, nil
		default:
			return 0, fmt.Errorf("volume conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "gallon":
		switch toUnit {
		case "liter":
			return value * 3.78541, nil
		case "quart":
			return value * 4, nil
		default:
			return 0, fmt.Errorf("volume conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "quart":
		switch toUnit {
		case "liter":
			return value * 0.946353, nil
		case "gallon":
			return value / 4, nil
		default:
			return 0, fmt.Errorf("volume conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source volume unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertTime(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "second":
		switch toUnit {
		case "minute":
			return value / 60, nil
		case "hour":
			return value / 3600, nil
		default:
			return 0, fmt.Errorf("time conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "minute":
		switch toUnit {
		case "second":
			return value * 60, nil
		case "hour":
			return value / 60, nil
		default:
			return 0, fmt.Errorf("time conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "hour":
		switch toUnit {
		case "second":
			return value * 3600, nil
		case "minute":
			return value * 60, nil
		default:
			return 0, fmt.Errorf("time conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source time unit: '%s'", fromUnit)
	}
}

func (t *ConverterTool) convertSpeed(value float64, fromUnit, toUnit string) (float64, error) {
	switch fromUnit {
	case "meter per second":
		switch toUnit {
		case "kilometer per hour":
			return value * 3.6, nil
		case "mile per hour":
			return value * 2.23694, nil
		default:
			return 0, fmt.Errorf("speed conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "kilometer per hour":
		switch toUnit {
		case "meter per second":
			return value / 3.6, nil
		case "mile per hour":
			return value * 0.621371, nil
		default:
			return 0, fmt.Errorf("speed conversion from %s to %s not supported", fromUnit, toUnit)
		}
	case "mile per hour":
		switch toUnit {
		case "meter per second":
			return value / 2.23694, nil
		case "kilometer per hour":
			return value / 0.621371, nil
		default:
			return 0, fmt.Errorf("speed conversion from %s to %s not supported", fromUnit, toUnit)
		}
	default:
		return 0, fmt.Errorf("unsupported source speed unit: '%s'", fromUnit)
	}
}

type ConverterToolFactory struct {
	tool *ConverterTool
}

func (f *ConverterToolFactory) CallTool(arguments string) string {
	var args map[string]any
	err := json.Unmarshal([]byte(arguments), &args)
	if err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	res, err := f.tool.Run(args)
	if err != nil {
		return fmt.Sprintf("Error running converter tool: %v", err)
	}

	return res
}

func NewConverterTool() *ConverterToolFactory {
	return &ConverterToolFactory{&ConverterTool{}}
}
