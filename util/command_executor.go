package util

import (
	"os"
)

func execFormat(sizeStr string, fsname string) (*os.File, error) {
	size, err := ParseFormatString(sizeStr)
	if err != nil {
		return nil, err
	}
	_, _, _, err = Format(int(size), fsname)
	if err != nil {
		return nil, err
	}
	fs, _ := os.OpenFile(fsname, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	return fs, nil
}
