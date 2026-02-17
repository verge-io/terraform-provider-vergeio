package vergeos

import (
	"encoding/json"
	"strconv"
)

// FlexInt is an int that can be unmarshaled from either a JSON number or string.
// The VergeOS API sometimes returns IDs as strings when specific fields are requested.
type FlexInt int

// UnmarshalJSON implements json.Unmarshaler for FlexInt.
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*f = FlexInt(i)
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*f = FlexInt(i)
		return nil
	}

	// Default to 0
	*f = 0
	return nil
}

// MarshalJSON implements json.Marshaler for FlexInt.
func (f FlexInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(f))
}

// Int returns the FlexInt as an int.
func (f FlexInt) Int() int {
	return int(f)
}
