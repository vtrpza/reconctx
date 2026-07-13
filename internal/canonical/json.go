package canonical

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"
)

const maxCanonicalJSONDepth = 100

// Marshal emits compact JSON with lexicographically ordered object keys and
// rejects values that JSON cannot represent.
func Marshal(value any) ([]byte, error) {
	if !validUTF8(reflect.ValueOf(value), 0) {
		return nil, errors.New("canonical JSON contains invalid UTF-8")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON: %w", err)
	}
	return Canonicalize(raw)
}

// Canonicalize validates one JSON value, rejects duplicate object keys, and
// emits its compact deterministic representation.
func Canonicalize(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("canonical JSON contains invalid UTF-8")
	}
	if err := validateJSONEscapes(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("canonical JSON contains multiple values")
		}
		return nil, fmt.Errorf("canonical JSON trailing data: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode canonical JSON: %w", err)
	}
	return encoded, nil
}

func validUTF8(value reflect.Value, depth int) bool {
	if !value.IsValid() {
		return true
	}
	if depth > 1000 {
		return false
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return true
		}
		return validUTF8(value.Elem(), depth+1)
	case reflect.Pointer:
		if value.IsNil() {
			return true
		}
		return validUTF8(value.Elem(), depth+1)
	case reflect.String:
		return utf8.ValidString(value.String())
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.PkgPath == "" && field.Tag.Get("json") != "-" && !validUTF8(value.Field(index), depth+1) {
				return false
			}
		}
	case reflect.Map:
		if value.IsNil() {
			return true
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if !validUTF8(iterator.Key(), depth+1) || !validUTF8(iterator.Value(), depth+1) {
				return false
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return true
		}
		fallthrough
	case reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if !validUTF8(value.Index(index), depth+1) {
				return false
			}
		}
	}
	return true
}

func validateJSONEscapes(raw []byte) error {
	inString := false
	for index := 0; index < len(raw); index++ {
		switch raw[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(raw) {
				continue
			}
			index++
			if raw[index] != 'u' || index+4 >= len(raw) {
				continue
			}
			first, err := strconv.ParseUint(string(raw[index+1:index+5]), 16, 16)
			if err != nil {
				continue
			}
			index += 4
			if first >= 0xdc00 && first <= 0xdfff {
				return errors.New("canonical JSON contains an isolated surrogate")
			}
			if first < 0xd800 || first > 0xdbff {
				continue
			}
			if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
				return errors.New("canonical JSON contains an isolated surrogate")
			}
			second, err := strconv.ParseUint(string(raw[index+3:index+7]), 16, 16)
			if err != nil || second < 0xdc00 || second > 0xdfff {
				return errors.New("canonical JSON contains an isolated surrogate")
			}
			index += 6
		}
	}
	return nil
}

func decodeValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxCanonicalJSONDepth {
		return nil, errors.New("canonical JSON exceeds maximum depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode canonical JSON: %w", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode object key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON object key %q", key)
			}
			value, err := decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
			return nil, errors.New("unterminated JSON object")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			return nil, errors.New("unterminated JSON array")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
