package pluginauthor

import (
	"encoding/json"
	"regexp"
)

var nameOK = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func jsonMarshalString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
