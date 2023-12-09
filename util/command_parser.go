package util

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func parseCommand(command string) ([]string, error) {
	args := strings.Fields(command)
	if len(args) == 0 {
		return nil, fmt.Errorf("could not parse arguments")
	}
	return args, nil
}

func LoadCommand(input *os.File) ([]string, error) {
	reader := bufio.NewReader(os.Stdin)
	command, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	command = command[:len(command)-1] // remove the newline
	return parseCommand(command)
}

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
