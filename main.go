package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"tranvaj/ZOS2023_SP_GO/util"
)

func main() {
	var fs *os.File

	if len(os.Args[1:]) != 1 {
		fmt.Println("Wrong amount of arguments. The argument should be the name of the filesystem.")
		return
	}
	FSNAME := os.Args[1:][0]

	//check if filesystem exists
	if _, err := os.Stat(FSNAME); err != nil {
		arr, err := util.LoadCommand(os.Stdin)
		command := strings.ToLower(arr[0])
		if command != "format" {
			fmt.Println("Filesystem does not exist. Please format it first.")
			return
		}
		fs, err = util.ExecFormat(arr[1], FSNAME)
		if err != nil {
			fmt.Println(err)
			return
		} else {
			fmt.Println("OK")
		}
	} else {
		fs, _ = os.OpenFile(FSNAME, os.O_RDWR, 0666)
		if err != nil {
			log.Fatal(err)
			return
		}
	}
	defer fs.Close()

	commandInterpreter := util.NewInterpreter(fs)
	for {
		commandInterpreter.LoadInterpreter()
		arr, err := util.LoadCommand(os.Stdin)
		if err != nil {
			fmt.Println(err)
			continue
		}
		err = commandInterpreter.ExecCommand(arr)
		if err != nil {
			fmt.Println(err)
		}
	}
}
