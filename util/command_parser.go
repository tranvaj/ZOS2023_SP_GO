package util

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// parseCommand parses the given command string and returns a slice of arguments.
// It splits the command string into individual arguments using white space as the delimiter.
// If the command string is empty or contains only white space, it returns an error.
func parseCommand(command string) ([]string, error) {
	args := strings.Fields(command)
	if len(args) == 0 {
		return nil, fmt.Errorf("could not parse arguments")
	}
	return args, nil
}

// LoadCommand reads a command from the given input file and returns a slice of strings representing the parsed command.
// It takes an *os.File as input and returns the parsed command as a []string.
// If an error occurs while reading the command, it returns nil and the error.
func LoadCommand(input *os.File) ([]string, error) {
	reader := bufio.NewReader(input)
	command, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	command = command[:len(command)-1] // remove the newline
	return parseCommand(command)
}

// ParseFormatString parses a string in format for example: "2B" or "2KB" or "2GB" and returns the corresponding target size in bytes.
// The inputString parameter is the formatted string to be parsed.
// The function returns the target size in bytes and an error if any occurred during parsing.
func ParseFormatString(inputString string) (uint64, error) {
	inputString = strings.ToUpper((inputString))
	sizeSuffixes := "KMGT"
	var shiftAmount uint64 = 0
	var parsedValue uint64
	var err error

	index := strings.IndexFunc(inputString, unicode.IsLetter)
	if index == -1 {
		return 0, fmt.Errorf("unspecified suffix (K, M, G, T)")
	}

	parsedValue, err = strconv.ParseUint(inputString[:index], 10, 64)
	if err != nil {
		return 0, err
	}

	suffix := inputString[index:][0]
	suffixMatch := strings.Index(sizeSuffixes, string(suffix))

	if suffixMatch == -1 {
		return 0, fmt.Errorf("invalid size suffix")
	} else {
		shiftAmount = uint64((suffixMatch + 1) * 10)
	}
	targetSize := parsedValue * (1 << shiftAmount)

	return targetSize, nil
}
