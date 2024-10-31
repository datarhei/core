// Package json implements
package json

import (
	"encoding/json"
	"fmt"
	"io"
)

type UnmarshalTypeError = json.UnmarshalTypeError
type SyntaxError = json.SyntaxError
type Number = json.Number

// Unmarshal is a wrapper for json.Unmarshal
func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// Marshal is a wrapper for json.Marshal
func Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// MarshalIndent is a wrapper for json.MarshalIndent
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

func NewDecoder(r io.Reader) *json.Decoder {
	return json.NewDecoder(r)
}

func NewEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}

// FormatError takes the marshalled data and the error from Unmarshal and returns a detailed
// error message where the error was and what the error is.
func FormatError(input []byte, err error) error {
	if jsonError, ok := err.(*json.SyntaxError); ok {
		line, character, offsetError := lineAndCharacter(input, int(jsonError.Offset))
		if offsetError != nil {
			return err
		}

		return fmt.Errorf("syntax error at line %d, character %d: %w", line, character, err)
	}

	if jsonError, ok := err.(*json.UnmarshalTypeError); ok {
		line, character, offsetError := lineAndCharacter(input, int(jsonError.Offset))
		if offsetError != nil {
			return err
		}

		return fmt.Errorf("expect type '%s' for '%s' at line %d, character %d: %w", jsonError.Type.String(), jsonError.Field, line, character, err)
	}

	return err
}

func lineAndCharacter(input []byte, offset int) (line int, character int, err error) {
	lf := byte(0x0A)

	if offset > len(input) || offset < 0 {
		return 0, 0, fmt.Errorf("couldn't find offset %d within the input", offset)
	}

	// Humans tend to count from 1.
	line = 1

	for i, b := range input {
		if b == lf {
			line++
			character = 0
		}
		character++
		if i == offset {
			break
		}
	}

	return line, character, nil
}

func ToNumber(f float64) json.Number {
	var s string

	if f == float64(int64(f)) {
		s = fmt.Sprintf("%.0f", f) // 0 decimal if integer
	} else {
		s = fmt.Sprintf("%.3f", f) // max. 3 decimal if float
	}

	return json.Number(s)
}
