package resolve

import (
	"fmt"
	"strconv"

	"github.com/evmac/go-bake/internal/config"
)

// ParseArgs parses args after the target name: declared (--name value, -short value), live (undeclared --k v), and passthrough (after --).
func ParseArgs(tgt *config.Target, args []string) (declared map[string]string, live map[string]string, passthrough []string, err error) {
	declared = make(map[string]string)
	live = make(map[string]string)
	for i, a := range args {
		if a == "--" {
			passthrough = args[i+1:]
			args = args[:i]
			break
		}
	}
	// Set defaults for declared args
	for _, arg := range tgt.Args {
		declared[arg.Name] = arg.Default
	}
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			break
		}
		var key string
		var value string
		if len(a) >= 2 && a[:2] == "--" {
			key = a[2:]
			if eq := indexByte(key, '='); eq >= 0 {
				value = key[eq+1:]
				key = key[:eq]
				i++
			} else if i+1 < len(args) {
				value = args[i+1]
				i += 2
			} else {
				// Bare --flag => bool true
				value = "true"
				i++
			}
			normalizeKey(&key)
		} else if len(a) >= 1 && a[0] == '-' && len(a) == 2 {
			short := a[1:]
			key = findDeclaredByShort(tgt, short)
			if key == "" {
				key = short
			}
			if i+1 < len(args) {
				value = args[i+1]
				i += 2
			} else {
				value = "true"
				i++
			}
		} else {
			i++
			continue
		}
		if isDeclared(tgt, key) {
			declared[key] = value
		} else {
			live[key] = value
		}
	}
	for _, a := range tgt.Args {
		if a.Required && (declared[a.Name] == "" && a.Default == "") {
			return nil, nil, nil, fmt.Errorf("required arg %q is missing", a.Name)
		}
	}
	return declared, live, passthrough, nil
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func normalizeKey(k *string) {
	// Keep as-is for template; optional: convert to snake_case for env
}

func findDeclaredByShort(tgt *config.Target, short string) string {
	for _, a := range tgt.Args {
		if a.Short == short {
			return a.Name
		}
	}
	return ""
}

func isDeclared(tgt *config.Target, key string) bool {
	for _, a := range tgt.Args {
		if a.Name == key {
			return true
		}
	}
	return false
}

// TemplateData builds the map passed to template expansion: .argName and .live.
func TemplateData(declared, live map[string]string) map[string]interface{} {
	data := make(map[string]interface{})
	for k, v := range declared {
		data[k] = v
	}
	if len(live) > 0 {
		data["live"] = live
	}
	return data
}

// FormatBool returns "true" or "false" for template bool formatting.
func FormatBool(s string) string {
	b, _ := strconv.ParseBool(s)
	return strconv.FormatBool(b)
}
