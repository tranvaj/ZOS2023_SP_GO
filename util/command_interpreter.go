package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// metody s (i *Interpreter) jsou metody, ktere jsou pristupne jen z Interpreteru (neco jako metoda tridy v jave)
// tohle je v podstate neco jako OOP ale v Go

type Interpreter struct {
	fs              *os.File
	superBlock      Superblock
	dataBitmap      []uint8
	inodeBitmap     []uint8
	currentDirInode PseudoInode
	currentDir      []DirectoryItem
	currentPath     string
}

// NewInterpreter creates a new instance of the Interpreter struct.
// It takes a pointer to an os.File as a parameter and returns a pointer to the Interpreter.
// The fs parameter represents the file system that the interpreter will operate on.
// The currentPath field of the Interpreter is initialized to "/" or "\" depending on the system OS.
func NewInterpreter(fs *os.File) *Interpreter {
	return &Interpreter{
		fs:          fs,
		currentPath: string(os.PathSeparator),
	}
}

func (i *Interpreter) LoadInterpreter() {
	var err error
	i.superBlock = LoadSuperBlock(i.fs)
	currentDirInodeId := i.currentDirInode.NodeId
	if currentDirInodeId == 0 {
		currentDirInodeId = 1
	}
	i.currentDirInode, err = LoadInode(i.fs, currentDirInodeId, int64(i.superBlock.InodeStartAddress))
	if err != nil {
		log.Fatal(err)
		return
	}
	i.dataBitmap, err = LoadBitmap(i.fs, i.superBlock.BitmapStartAddress, i.superBlock.BitmapSize)
	if err != nil {
		log.Fatal(err)
		return
	}
	i.inodeBitmap, err = LoadBitmap(i.fs, i.superBlock.BitmapiStartAddress, i.superBlock.BitmapiSize)
	if err != nil {
		log.Fatal(err)
		return
	}
	i.currentDir, err = LoadDirectory(i.fs, i.currentDirInode, i.superBlock)
	if err != nil {
		fmt.Println("could not load directory: " + err.Error())
		return
	}
}

// getPathDir returns the directory component of the given path.
// It cleans the path and then extracts the directory using the filepath.Dir function.
func getPathDir(path string) string {
	return filepath.Dir(filepath.Clean(path))
}

func ExecFormat(sizeStr string, fsname string) (*os.File, error) {
	size, err := ParseFormatString(sizeStr)
	if err != nil {
		return nil, err
	}
	_, _, _, err = Format(int(size), fsname)
	if err != nil {
		return nil, err
	}
	fs, err := os.OpenFile(fsname, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	return fs, nil
}

// ExecCommand executes the specified command based on the input array. The arr parameter is an array of strings representing the command and its arguments.
//
// It returns an error if the command is unknown or if there is no filesystem loaded.
//
// The supported commands are: format, incp, cat, ls, mkdir, cd, rmdir, rm, pwd, info, cp, mv, outcp, load, xcp, and short.
// Example usage: interpreter.ExecCommand([]string{"ls"})
func (i *Interpreter) ExecCommand(arr []string) error {
	if i.fs == nil {
		return fmt.Errorf("no filesystem loaded")
	}
	switch command := strings.ToLower(arr[0]); command {
	case "format":
		var err error
		i.fs, err = ExecFormat(arr[1], i.fs.Name())
		//fmt.Println(i.fs.Name())
		if err != nil {
			//return err
			return fmt.Errorf("CANNOT CREATE FILE")
		} else {
			fmt.Println("OK")
		}

	case "incp":
		err := i.Incp(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "cat":
		err := i.Cat(arr)
		if err != nil {
			return err
		}
	case "ls":
		err := i.Ls(arr)
		if err != nil {
			return err
		}
	case "mkdir":
		err := i.Mkdir(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "cd":
		err := i.Cd(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "rmdir":
		err := i.Rmdir(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "rm":
		err := i.Rm(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "pwd":
		err := i.Pwd()
		if err != nil {
			return err
		}
	case "info":
		err := i.Info(arr)
		if err != nil {
			return err
		}
	case "cp":
		err := i.Cp(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "mv":
		err := i.Mv(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "outcp":
		err := i.Outcp(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "load":
		err := i.Load(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "xcp":
		err := i.Xcp(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	case "short":
		err := i.Short(arr)
		if err != nil {
			return err
		} else {
			fmt.Println("OK")
		}
	default:
		return fmt.Errorf("unknown command")
	}
	return nil
}

func (i *Interpreter) Incp(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the path to the file and the name of the file in the filesystem.")
	}

	data, err := os.ReadFile(arr[1])
	if err != nil {
		//return fmt.Errorf(err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}

	_, fileInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
	}

	destInode, _, err := PathToInode(i.fs, getPathDir(arr[2]), i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistuje cílová cesta)")
	}

	err = AddDirItem(destInode.NodeId, int32(fileInodeId), filepath.Base(arr[2]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}
	return nil
}

func (i *Interpreter) Cat(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file in the filesystem.")
	}

	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}

	if destInode.IsDirectory {
		return fmt.Errorf("cannot cat a directory")
	}

	data, err := ReadFileData(i.fs, destInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}

	fmt.Println(string(data))
	return nil
}

func (i *Interpreter) Ls(arr []string) error {
	if len(arr) > 2 {
		return fmt.Errorf("Wrong amount of arguments.")
	}

	destDir := i.currentDir
	if len(arr) == 2 {
		destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
		if err != nil {
			//return fmt.Errorf("could not find destination: " + err.Error())
			return fmt.Errorf("PATH NOT FOUND (neexistující adresář)")
		}
		if !destInode.IsDirectory {
			return fmt.Errorf("not a directory")
		}
		destDir, err = LoadDirectory(i.fs, destInode, i.superBlock)
		if err != nil {
			return fmt.Errorf("could not load directory: " + err.Error())
		}
	}

	//fmt.Printf("%-20s %-20s %-20s %-20s\n", "Name", "Inode", "Size", "References")
	for _, v := range destDir {
		if v.Inode == 0 {
			continue
		}
		dirItemInode, err := LoadInode(i.fs, v.Inode, int64(i.superBlock.InodeStartAddress))
		if err != nil {
			return fmt.Errorf("could not load inode: " + err.Error())
		}
		if !dirItemInode.IsDirectory {
			fmt.Printf("-%s\n", v.ItemName)
		} else {
			fmt.Printf("+%s\n", v.ItemName)
		}
		//fmt.Printf("%-20s %-20d %-20d %-20d\n", v.ItemName, v.Inode, dirItemInode.FileSize, dirItemInode.References)
	}

	return nil
}

func (i *Interpreter) Mkdir(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the directory.")
	}

	destInode, _, err := PathToInode(i.fs, getPathDir(arr[1]), i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistuje zadaná cesta)")
	}

	_, newDirNodeId, err := CreateDirectory(i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, destInode.NodeId)
	if err != nil {
		return fmt.Errorf("could not create directory: " + err.Error())
	}

	err = AddDirItem(destInode.NodeId, int32(newDirNodeId), filepath.Base(arr[1]), i.fs, i.superBlock)
	if err != nil {
		//return fmt.Errorf("could not add directory item: " + err.Error())
		return fmt.Errorf("EXIST (nelze založit, již existuje)")
	}

	return nil
}

func (i *Interpreter) Cd(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the directory.")
	}

	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistující cesta)")
	}

	isDir, err := IsInodeDirectory(i.fs, destInode.NodeId, int64(i.superBlock.InodeStartAddress))
	if err != nil {
		return fmt.Errorf("could not load inode: " + err.Error())
	}
	if !isDir {
		return fmt.Errorf("not a directory")
	}

	//update current directory path string
	if filepath.IsAbs(arr[1]) || strings.TrimSpace(arr[1]) == string(os.PathSeparator) || strings.TrimSpace(arr[1]) == "/" {
		i.currentPath = filepath.Clean(arr[1])
	} else {
		// If the path is relative, join it with the current path
		i.currentPath = filepath.Join(i.currentPath, arr[1])
		// Clean the path to handle "." and ".."
		i.currentPath = filepath.Clean(i.currentPath)
	}

	i.currentDirInode = destInode

	return nil
}

func (i *Interpreter) Rmdir(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file or directory.")
	}

	destInode, parentInode, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (neexistující adresář)")
	}

	if destInode.IsDirectory {
		inodeDir, err := LoadDirectory(i.fs, destInode, i.superBlock)
		if err != nil {
			return fmt.Errorf("could not load directory: " + err.Error())
		}
		inodeDirLen := 0
		for _, v := range inodeDir {
			if v.Inode != 0 {
				inodeDirLen++
			}
		}
		if inodeDirLen > 2 {
			//return fmt.Errorf("directory not empty")
			return fmt.Errorf("NOT EMPTY (adresář obsahuje podadresáře, nebo soubory)")
		}
	} else {
		return fmt.Errorf("not a directory")
	}
	err = RemoveDirItem(parentInode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock, true)
	if err != nil {
		return fmt.Errorf("could not remove directory: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Rm(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file or directory.")
	}

	destInode, parentInode, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND")
	}

	if destInode.IsDirectory {
		return fmt.Errorf("cannot remove a directory with rm")
	}
	err = RemoveDirItem(parentInode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock, true)
	if err != nil {
		return fmt.Errorf("could not remove file: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Pwd() error {
	//prints the current directory path
	fmt.Println(strings.ReplaceAll(i.currentPath, string(os.PathSeparator), "/"))
	return nil
}

func (i *Interpreter) Info(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file or directory.")
	}
	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}
	fmt.Printf("%s - %d - %d - ", arr[1], destInode.FileSize, destInode.NodeId)
	for _, v := range destInode.Direct {
		fmt.Printf("%d ", v)
	}
	for _, v := range destInode.Indirect {
		fmt.Printf("%d ", v)
	}
	fmt.Println()
	/*
		clusterAddrs, indirectPtrAddrs, err := GetFileClusters(i.fs, destInode, i.superBlock)
		if err != nil {
			return fmt.Errorf("could not get file clusters: " + err.Error())
		}
		fmt.Printf("inode: %d\n", destInode.NodeId)
		fmt.Printf("size: %d\n", destInode.FileSize)
		fmt.Printf("references: %d\n", destInode.References)
		fmt.Printf("cluster addresses: %v\n", clusterAddrs)
		fmt.Printf("extra clusters for indirect pointers: %v\n", indirectPtrAddrs)
		fmt.Printf("is directory: %v\n", destInode.IsDirectory)*/
	return nil
}

func (i *Interpreter) Cp(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	srcInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find source: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}
	destInode, _, err := PathToInode(i.fs, getPathDir(arr[2]), i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistuje cílová cesta)")
	}
	if srcInode.IsDirectory {
		return fmt.Errorf("cannot copy a directory")
	}
	data, err := ReadFileData(i.fs, srcInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}

	_, copyInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
	}

	err = AddDirItem(destInode.NodeId, int32(copyInodeId), filepath.Base(arr[2]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Mv(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	var filename string
	srcInode, srcParentNode, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find source: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}
	/* destInode, _, err := PathToInode(i.fs, arr[2], i.superBlock, i.currentDirInode)
	if err != nil {
		destInode, _, err = PathToInode(i.fs, filepath.Dir(arr[2]), i.superBlock, i.currentDirInode)
		if err != nil && destInode.IsDirectory {
			return fmt.Errorf("could not find destination: " + err.Error())
		}
		//dest file does not exist so rename it
		filename = filepath.Base(arr[2])
	} */

	destInode, _, err := PathToInode(i.fs, getPathDir(arr[2]), i.superBlock, i.currentDirInode)
	if err != nil && destInode.IsDirectory {
		//return fmt.Errorf("could not find destination: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistuje cílová cesta)")
	}
	destInodeDir, err := LoadDirectory(i.fs, destInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not load directory: " + err.Error())
	}
	var finalDestInodeId int32
	if itemIndex := GetDirItemIndex(destInodeDir, filepath.Base(arr[2])); itemIndex != -1 {
		//dest exists might be a directory or file
		filename = filepath.Base(arr[1])
		oldDestInodeId := destInode.NodeId
		destInode, _, err = PathToInode(i.fs, arr[2], i.superBlock, i.currentDirInode)
		finalDestInodeId = destInode.NodeId
		if err != nil {
			//return fmt.Errorf("could not load inode: " + err.Error())
			return fmt.Errorf("PATH NOT FOUND (neexistuje cílová cesta)")
		}
		if !destInode.IsDirectory {
			fmt.Printf("%s is a file, overwriting\n", destInodeDir[itemIndex].ItemName)
			err = RemoveDirItem(oldDestInodeId, string(removeNullCharsFromString(string(destInodeDir[itemIndex].ItemName[:]))), i.fs, i.superBlock, true)
			finalDestInodeId = oldDestInodeId //dest is a file so we want to overwrite it, add dir item takes parent node id where file resides
			if err != nil {
				return fmt.Errorf("could not remove directory item: " + err.Error())
			}
		}
	} else {
		//dest doesnt exist
		finalDestInodeId = destInode.NodeId
		filename = filepath.Base(arr[2])
	}

	//remove src
	err = RemoveDirItem(srcParentNode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock, false)
	if err != nil {
		fmt.Println("could not remove directory item: " + err.Error())
	}
	//add src to dest or rename
	err = AddDirItem(finalDestInodeId, srcInode.NodeId, filename, i.fs, i.superBlock)
	if err != nil {
		fmt.Println("could not add directory item: " + err.Error())
		err = AddDirItem(srcParentNode.NodeId, srcInode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock)
		if err != nil {
			return fmt.Errorf("could not add directory item: " + err.Error())
		}
	}

	return nil
}

func (i *Interpreter) Outcp(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	srcInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		//return fmt.Errorf("could not find source: " + err.Error())
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}
	data, err := ReadFileData(i.fs, srcInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}
	//write data slice to specific absolute or relative path in OS
	err = os.WriteFile(arr[2], data, 0666)
	if err != nil {
		//return fmt.Errorf("could not write data to file: " + err.Error())
		return fmt.Errorf("PATH NOT FOUND (neexistuje cílová cesta)")
	}

	return nil
}

func (i *Interpreter) Load(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the filesystem.")
	}
	content, err := os.ReadFile(arr[1])
	if err != nil {
		//return fmt.Errorf("could not read file: %v", err)
		return fmt.Errorf("FILE NOT FOUND (není zdroj)")
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		i.LoadInterpreter()
		arg, err := parseCommand(line)
		if err != nil {
			return fmt.Errorf("error parsing command: %v", err)
		}
		if arg[0] == "format" {
			i.currentPath = string(os.PathSeparator)
			i.currentDirInode = PseudoInode{}
		}
		err = i.ExecCommand(arg)
		if err != nil {
			return fmt.Errorf("error executing command: %v", err)
		}
	}
	return nil
}

func (i *Interpreter) Xcp(arr []string) error {
	//combines 2 files into 1 and creates a new file
	if len(arr) != 4 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	srcInode1, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find source: " + err.Error())
	}
	srcInode2, _, err := PathToInode(i.fs, arr[2], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find source: " + err.Error())
	}
	data1, err := ReadFileData(i.fs, srcInode1, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}
	data2, err := ReadFileData(i.fs, srcInode2, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}
	//combine data
	data := append(data1, data2...)
	//get location of new file
	destInode, _, err := PathToInode(i.fs, getPathDir(arr[3]), i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}
	//write data to new file
	_, newFileInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
	}
	//remove old files
	err = RemoveDirItem(destInode.NodeId, filepath.Base(arr[3]), i.fs, i.superBlock, true)
	if err == nil {
		fmt.Println("overwriting file with same name...")
	}
	//add new file to directory
	err = AddDirItem(destInode.NodeId, int32(newFileInodeId), filepath.Base(arr[3]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}
	return nil
}

func (i *Interpreter) Short(arr []string) error {
	//if file is longer than 3000 bytes, it will be shortened to 3000 bytes
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file.")
	}
	destInode, parentInode, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}
	if destInode.IsDirectory {
		return fmt.Errorf("cannot shorten a directory")
	}
	data, err := ReadFileData(i.fs, destInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}
	if len(data) > 3000 {
		data = data[:3000]
	}
	_, newFileInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
	}
	err = RemoveDirItem(i.currentDirInode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock, true)
	if err != nil {
		return fmt.Errorf("could not remove directory item: " + err.Error())
	}
	err = AddDirItem(parentInode.NodeId, int32(newFileInodeId), filepath.Base(arr[1]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}
	return nil
}
