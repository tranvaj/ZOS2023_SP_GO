package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Interpreter struct {
	fs              *os.File
	superBlock      Superblock
	dataBitmap      []uint8
	inodeBitmap     []uint8
	currentDirInode PseudoInode
	currentDir      []DirectoryItem
	currentPath     string
}

func NewInterpreter(fs *os.File) *Interpreter {
	return &Interpreter{
		fs: fs,
	}
}

func (i *Interpreter) Close() {
	i.fs.Close()
}

func (i *Interpreter) Init() {
	var err error
	i.superBlock = LoadSuperBlock(i.fs)
	i.currentDirInode, err = LoadInode(i.fs, i.currentDirInode.NodeId, int64(i.superBlock.InodeStartAddress))
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

func (i *Interpreter) ExecCommand(arr []string) {
	if i.fs == nil {
		fmt.Println("no filesystem loaded")
		return
	}
	switch command := strings.ToLower(arr[0]); command {
	case "incp":
		err := i.Incp(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "cat":
		err := i.Cat(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "ls":
		err := i.Ls(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "mkdir":
		err := i.Mkdir(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "cd":
		err := i.Cd(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "rmdir":
		err := i.Rmdir(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "rm":
		err := i.Rm(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "pwd":
		err := i.Pwd()
		if err != nil {
			fmt.Println(err.Error())
		}
	case "info":
		err := i.Info(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "cp":
		err := i.CP(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "mv":
		err := i.Mv(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "outcp":
		err := i.Outcp(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	case "load":
		err := i.Load(arr)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
}

func (i *Interpreter) Incp(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the path to the file and the name of the file in the filesystem.")
	}

	data, err := os.ReadFile(arr[1])
	if err != nil {
		return fmt.Errorf(err.Error())
	}

	_, fileInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
	}

	destInode, _, err := PathToInode(i.fs, filepath.Dir(arr[2]), i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
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
		return fmt.Errorf("could not find destination: " + err.Error())
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

func (i *Interpreter) execFormat(sizeStr string, fsname string) (*os.File, error) {
	size, err := ParseFormatString(sizeStr)
	if err != nil {
		return nil, err
	}
	_, _, _, err = Format(int(size), fsname)
	if err != nil {
		return nil, err
	}
	i.fs, err = os.OpenFile(fsname, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	return i.fs, nil
}

func (i *Interpreter) Ls(arr []string) error {
	if len(arr) > 2 {
		return fmt.Errorf("Wrong amount of arguments.")
	}

	destDir := i.currentDir
	if len(arr) == 2 {
		destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
		if err != nil {
			return fmt.Errorf("could not find destination: " + err.Error())
		}
		destDir, err = LoadDirectory(i.fs, destInode, i.superBlock)
		if err != nil {
			return fmt.Errorf("could not load directory: " + err.Error())
		}
	}

	fmt.Printf("%-20s %-20s %-20s %-20s\n", "Name", "Inode", "Size", "References")
	for _, v := range destDir {
		if v.Inode == 0 {
			continue
		}
		dirItemInode, err := LoadInode(i.fs, v.Inode, int64(i.superBlock.InodeStartAddress))
		if err != nil {
			return fmt.Errorf("could not load inode: " + err.Error())
		}
		fmt.Printf("%-20s %-20d %-20d %-20d\n", v.ItemName, v.Inode, dirItemInode.FileSize, dirItemInode.References)
	}

	return nil
}

func (i *Interpreter) Mkdir(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the directory.")
	}

	destInode, _, err := PathToInode(i.fs, filepath.Dir(arr[1]), i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}

	_, newDirNodeId, err := CreateDirectory(i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, destInode.NodeId)
	if err != nil {
		return fmt.Errorf("could not create directory: " + err.Error())
	}

	err = AddDirItem(destInode.NodeId, int32(newDirNodeId), filepath.Base(arr[1]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Cd(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the directory.")
	}

	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}

	isDir, err := IsInodeDirectory(i.fs, destInode.NodeId, int64(i.superBlock.InodeStartAddress))
	if err != nil {
		return fmt.Errorf("could not load inode: " + err.Error())
	}
	if !isDir {
		return fmt.Errorf("not a directory")
	}

	//update current directory path string
	if filepath.IsAbs(arr[1]) {
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
		return fmt.Errorf("could not find destination: " + err.Error())
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
			return fmt.Errorf("directory not empty")
		}
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
		return fmt.Errorf("could not find destination: " + err.Error())
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
	fmt.Println(i.currentPath)
	return nil
}

func (i *Interpreter) Info(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the file or directory.")
	}
	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}
	clusterAddrs, indirectPtrAddrs, err := GetFileClusters(i.fs, destInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not get file clusters: " + err.Error())
	}
	fmt.Printf("inode: %d\n", destInode.NodeId)
	fmt.Printf("size: %d\n", destInode.FileSize)
	fmt.Printf("references: %d\n", destInode.References)
	fmt.Printf("cluster addresses: %v\n", clusterAddrs)
	fmt.Printf("extra clusters for indirect pointers: %v\n", indirectPtrAddrs)
	fmt.Printf("is directory: %v\n", destInode.IsDirectory)
	return nil
}

func (i *Interpreter) CP(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	srcInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find source: " + err.Error())
	}
	destInode, _, err := PathToInode(i.fs, arr[2], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
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
	srcInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find source: " + err.Error())
	}
	destInode, _, err := PathToInode(i.fs, arr[2], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}

	err = RemoveDirItem(i.currentDirInode.NodeId, filepath.Base(arr[1]), i.fs, i.superBlock, false)
	if err != nil {
		return fmt.Errorf("could not remove directory item: " + err.Error())
	}

	err = AddDirItem(destInode.NodeId, srcInode.NodeId, filepath.Base(arr[2]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Outcp(arr []string) error {
	if len(arr) != 3 {
		return fmt.Errorf("Wrong amount of arguments. The arguments should be the name of the source and destination.")
	}
	srcInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find source: " + err.Error())
	}
	data, err := ReadFileData(i.fs, srcInode, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not read data: " + err.Error())
	}
	//write data slice to specific absolute or relative path in OS
	err = os.WriteFile(arr[2], data, 0666)
	if err != nil {
		return fmt.Errorf("could not write data to file: " + err.Error())
	}

	return nil
}

func (i *Interpreter) Load(arr []string) error {
	if len(arr) != 2 {
		return fmt.Errorf("Wrong amount of arguments. The argument should be the name of the filesystem.")
	}
	fileWithCommands, err := os.OpenFile(arr[1], os.O_RDWR, 0666)
	defer fileWithCommands.Close()
	if err != nil {
		return fmt.Errorf("could not open filesystem: " + err.Error())
	}

	for {
		arg, err := LoadCommand(fileWithCommands)
		if err != nil {
			return fmt.Errorf(err.Error())
		}
		i.ExecCommand(arg)
	}
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
	destInode, _, err := PathToInode(i.fs, filepath.Dir(arr[3]), i.superBlock, i.currentDirInode)
	if err != nil {
		return fmt.Errorf("could not find destination: " + err.Error())
	}
	//write data to new file
	_, newFileInodeId, err := WriteAndSaveData(data, i.fs, i.superBlock, i.inodeBitmap, i.dataBitmap, false)
	if err != nil {
		return fmt.Errorf("could not write data to the filesystem: " + err.Error())
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
	destInode, _, err := PathToInode(i.fs, arr[1], i.superBlock, i.currentDirInode)
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
	err = AddDirItem(destInode.NodeId, int32(newFileInodeId), filepath.Base(arr[1]), i.fs, i.superBlock)
	if err != nil {
		return fmt.Errorf("could not add directory item: " + err.Error())
	}
	return nil
}
