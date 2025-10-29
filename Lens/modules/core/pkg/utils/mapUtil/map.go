package mapUtil

import (
	"encoding/json"
	"errors"
	"fmt"
)

func ConvertInterfaceToExt(data interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ConvertToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		result[k] = fmt.Sprintf("%+v", v)
	}
	return result
}

func ConvertToInterfaceMap(data map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}
	return result
}

func DecodeKeyFromMap(data map[string]interface{}, key string, targetObject interface{}) error {
	input, ok := data[key]
	if !ok {
		return errors.New("key not exist")
	}
	return DecodeFromMap(input, targetObject)
}

func EncodeMap(data interface{}) (map[string]interface{}, error) {
	jsonByte, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	err = json.Unmarshal(jsonByte, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func DecodeFromMap(data interface{}, targetObject interface{}) error {
	jsonByte, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonByte, targetObject)
}

func DecodeKeyFromMapIfExists(data map[string]interface{}, key string, targetObject interface{}) error {
	if _, ok := data[key]; ok {
		return DecodeKeyFromMap(data, key, targetObject)
	}
	return nil
}

// ParseJSONMap parses s, which must contain JSON map of {"k1":"v1",...,"kN":"vN"}
func ParseJSONMap(s string) (map[string]string, error) {
	if s == "" {
		// Special case
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}
