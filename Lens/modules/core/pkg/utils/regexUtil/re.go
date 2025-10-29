package regexUtil

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
)

func RegexToStruct(re *regexp.Regexp, input string, out interface{}) error {
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("out must be a pointer to a struct")
	}

	matches := re.FindStringSubmatch(input)
	if matches == nil {
		return fmt.Errorf("no match found")
	}

	names := re.SubexpNames()
	structVal := v.Elem()

	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}

		field := structVal.FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		raw := matches[i]
		switch field.Kind() {
		case reflect.String:
			field.SetString(raw)
		case reflect.Int:
			if num, err := strconv.Atoi(raw); err == nil {
				field.SetInt(int64(num))
			}
		case reflect.Float64:
			if num, err := strconv.ParseFloat(raw, 64); err == nil {
				field.SetFloat(num)
			}
		}
	}
	return nil
}
