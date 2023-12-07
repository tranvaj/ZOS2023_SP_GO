package util

import (
	"fmt"
	"strings"
)

func execFormat(command string) ([]string, error) {
	args := strings.Fields(command)
	if len(args) == 0 {
		return nil, fmt.Errorf("could not parse arguments")
	}
	return args, nil
}
