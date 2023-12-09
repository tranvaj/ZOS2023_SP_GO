package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"tranvaj/ZOS2023_SP_GO/util"
)

func main() {
	var fs *os.File
	var currentDirInode util.PseudoInode
	fsExists := true
	var superBlock util.Superblock
	var dataBitmap []uint8
	var inodeBitmap []uint8
	var currentDir []util.DirectoryItem
	currentDirInodeId := int32(1)
	initialized := false
	currentPath := "/"

	if len(os.Args[1:]) != 1 {
		fmt.Println("Wrong amount of arguments. The argument should be the name of the filesystem.")
		return
	}
	FSNAME := os.Args[1:][0]

	//check if filesystem exists
	if _, err := os.Stat(FSNAME); err != nil {
		fsExists = false
	} else {
		initialized = true
		fs, _ = os.OpenFile(FSNAME, os.O_RDWR, 0666)
		if err != nil {
			log.Fatal(err)
			return
		}
		defer fs.Close()
	}

	for {
		arr, err := util.LoadCommand()
		if err != nil {
			log.Fatal(err)
			return
		}

		if !fsExists && strings.ToLower(arr[0]) != "format" {
			fmt.Println("Filesystem does not exist. Please format it first.")
			continue
		}

		if initialized {
			superBlock = util.LoadSuperBlock(fs)
			currentDirInode, err = util.LoadInode(fs, currentDirInodeId, int64(superBlock.InodeStartAddress))
			if err != nil {
				log.Fatal(err)
				return
			}
			dataBitmap, err = util.LoadBitmap(fs, superBlock.BitmapStartAddress, superBlock.BitmapSize)
			if err != nil {
				log.Fatal(err)
				return
			}
			inodeBitmap, err = util.LoadBitmap(fs, superBlock.BitmapiStartAddress, superBlock.BitmapiSize)
			if err != nil {
				log.Fatal(err)
				return
			}
			currentDir, err = util.LoadDirectory(fs, currentDirInode, superBlock)
			if err != nil {
				fmt.Println("could not load directory: " + err.Error())
				break
			}
		}

		switch command := strings.ToLower(arr[0]); command {
		case "format":
			size, err := util.ParseFormatString(arr[1])
			if err != nil {
				log.Fatal(err)
				return
			}
			_, _, _, err = util.Format(int(size), FSNAME)
			if err != nil {
				log.Fatal(err)
				return
			}
			fs, _ = os.OpenFile(FSNAME, os.O_RDWR, 0666)
			if err != nil {
				log.Fatal(err)
				return
			}
			defer fs.Close()
			initialized = true
			fsExists = true
		case "incp":
			//loads file from host OS to the filesystem
			if len(arr) != 3 {
				fmt.Println("Wrong amount of arguments. The arguments should be the name of the path to the file and the name of the file in the filesystem.")
				break
			}
			data, err := os.ReadFile(arr[1])
			if err != nil {
				return
			}
			_, fileInodeId, err := util.WriteAndSaveData(data, fs, superBlock, inodeBitmap, dataBitmap, false)
			if err != nil {
				fmt.Println("could not write data to the filesystem: " + err.Error())
				break
			}

			destInode, _, err := util.PathToInode(fs, filepath.Dir(arr[2]), superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			err = util.AddDirItem(destInode.NodeId, int32(fileInodeId), filepath.Base(arr[2]), fs, superBlock)
			if err != nil {
				fmt.Println("could not add directory item: " + err.Error())
				break
			}
		case "cat":
			//prints the content of the file
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the file in the filesystem.")
				break
			}
			destInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			if destInode.IsDirectory {
				fmt.Println("cannot cat a directory")
				break
			}
			data, err := util.ReadFileData(fs, destInode, superBlock)
			if err != nil {
				fmt.Println("could not read data: " + err.Error())
				break
			}
			fmt.Println(string(data))
		case "ls":
			//prints the content of the directory and inode id
			destDir := currentDir
			if len(arr) > 2 {
				fmt.Println("Wrong amount of arguments.")
				break
			}
			if len(arr) == 2 {
				destInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
				if err != nil {
					fmt.Println("could not find destination: " + err.Error())
					break
				}
				destDir, err = util.LoadDirectory(fs, destInode, superBlock)
				if err != nil {
					fmt.Println("could not load directory: " + err.Error())
					break
				}
			}
			//fmt.Println("Current directory: " + currentPath)
			//print table header
			fmt.Printf("%-20s %-20s %-20s %-20s\n", "Name", "Inode", "Size", "References")
			for _, v := range destDir {
				if v.Inode == 0 {
					continue
				}
				dirItemInode, err := util.LoadInode(fs, v.Inode, int64(superBlock.InodeStartAddress))
				if err != nil {
					fmt.Println("could not load inode: " + err.Error())
					break
				}
				fmt.Printf("%-20s %-20d %-20d %-20d\n", v.ItemName, v.Inode, dirItemInode.FileSize, dirItemInode.References)
			}

		case "mkdir":
			//creates a directory
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the directory.")
				break
			}
			destInode, _, err := util.PathToInode(fs, filepath.Dir(arr[1]), superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			_, newDirNodeId, err := util.CreateDirectory(fs, superBlock, inodeBitmap, dataBitmap, destInode.NodeId)
			if err != nil {
				fmt.Println("could not create directory: " + err.Error())
				break
			}
			err = util.AddDirItem(destInode.NodeId, int32(newDirNodeId), filepath.Base(arr[1]), fs, superBlock)
			if err != nil {
				fmt.Println("could not add directory item: " + err.Error())
				break
			}

		case "cd":
			//changes the current directory
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the directory.")
				break
			}

			destInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			isDir, err := util.IsInodeDirectory(fs, destInode.NodeId, int64(superBlock.InodeStartAddress))
			if err != nil {
				fmt.Println("could not load inode: " + err.Error())
				break
			}
			if !isDir {
				fmt.Println("not a directory")
				break
			}

			//update current directory path string
			if filepath.IsAbs(arr[1]) {
				currentPath = filepath.Clean(arr[1])
			} else {
				// If the path is relative, join it with the current path
				currentPath = filepath.Join(currentPath, arr[1])
				// Clean the path to handle "." and ".."
				currentPath = filepath.Clean(currentPath)
			}

			currentDirInodeId = destInode.NodeId

		case "rmdir":
			//removes a file or directory
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the file or directory.")
				break
			}

			destInode, parentInode, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			if destInode.IsDirectory {
				inodeDir, err := util.LoadDirectory(fs, destInode, superBlock)
				if err != nil {
					fmt.Println("could not load directory: " + err.Error())
					break
				}
				inodeDirLen := 0
				for _, v := range inodeDir {
					if v.Inode != 0 {
						inodeDirLen++
					}
				}
				if inodeDirLen > 2 {
					fmt.Println("directory not empty")
					break
				}
			}
			err = util.RemoveDirItem(parentInode.NodeId, filepath.Base(arr[1]), fs, superBlock, true)
			if err != nil {
				fmt.Println("could not remove directory: " + err.Error())
				break
			}
		case "rm":
			//removes a file or directory
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the file or directory.")
				break
			}

			destInode, parentInode, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			if destInode.IsDirectory {
				fmt.Println("cannot remove a directory with rm")
				break
			}
			err = util.RemoveDirItem(parentInode.NodeId, filepath.Base(arr[1]), fs, superBlock, true)
			if err != nil {
				fmt.Println("could not remove file: " + err.Error())
				break
			}
		case "pwd":
			//prints the current directory path
			fmt.Println(currentPath)
		case "info":
			if len(arr) != 2 {
				fmt.Println("Wrong amount of arguments. The argument should be the name of the file or directory.")
				break
			}
			destInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}
			clusterAddrs, indirectPtrAddrs, err := util.GetFileClusters(fs, destInode, superBlock)
			if err != nil {
				fmt.Println("could not get file clusters: " + err.Error())
				break
			}
			fmt.Printf("inode: %d\n", destInode.NodeId)
			fmt.Printf("size: %d\n", destInode.FileSize)
			fmt.Printf("references: %d\n", destInode.References)
			fmt.Printf("cluster addresses: %v\n", clusterAddrs)
			fmt.Printf("extra clusters for indirect pointers: %v\n", indirectPtrAddrs)
			fmt.Printf("is directory: %v\n", destInode.IsDirectory)
		case "cp":
			if len(arr) != 3 {
				fmt.Println("Wrong amount of arguments. The arguments should be the name of the source and destination.")
				break
			}
			srcInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find source: " + err.Error())
				break
			}
			destInode, _, err := util.PathToInode(fs, arr[2], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}
			if srcInode.IsDirectory {
				fmt.Println("cannot copy a directory")
				break
			}
			data, err := util.ReadFileData(fs, srcInode, superBlock)
			if err != nil {
				fmt.Println("could not read data: " + err.Error())
				break
			}

			_, copyInodeId, err := util.WriteAndSaveData(data, fs, superBlock, inodeBitmap, dataBitmap, false)
			if err != nil {
				fmt.Println("could not write data to the filesystem: " + err.Error())
				break
			}

			err = util.AddDirItem(destInode.NodeId, int32(copyInodeId), filepath.Base(arr[2]), fs, superBlock)
			if err != nil {
				fmt.Println("could not add directory item: " + err.Error())
				break
			}
		case "mv":
			if len(arr) != 3 {
				fmt.Println("Wrong amount of arguments. The arguments should be the name of the source and destination.")
				break
			}
			srcInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find source: " + err.Error())
				break
			}
			destInode, _, err := util.PathToInode(fs, arr[2], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find destination: " + err.Error())
				break
			}

			err = util.RemoveDirItem(currentDirInode.NodeId, filepath.Base(arr[1]), fs, superBlock, false)
			if err != nil {
				fmt.Println("could not remove directory item: " + err.Error())
				break
			}

			err = util.AddDirItem(destInode.NodeId, srcInode.NodeId, filepath.Base(arr[2]), fs, superBlock)
			if err != nil {
				fmt.Println("could not add directory item: " + err.Error())
				break
			}
		case "outcp":
			if len(arr) != 3 {
				fmt.Println("Wrong amount of arguments. The arguments should be the name of the source and destination.")
				break
			}
			srcInode, _, err := util.PathToInode(fs, arr[1], superBlock, currentDirInode)
			if err != nil {
				fmt.Println("could not find source: " + err.Error())
				break
			}
			data, err := util.ReadFileData(fs, srcInode, superBlock)
			if err != nil {
				fmt.Println("could not read data: " + err.Error())
				break
			}
			//write data slice to specific absolute or relative path in OS
			err = os.WriteFile(arr[2], data, 0666)
			if err != nil {
				fmt.Println("could not write data to file: " + err.Error())
				break
			}
		case "load":

		default:
			fmt.Println("Unknown command.")
		}

	}

	/* superBlock, dataBitmap, inodeBitmap, _ := util.Format(int(size), "fat32")

	//test
	fp, err := os.OpenFile("fat32", os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(err)
		return
	}

	data, err := os.ReadFile("testfile.txt")
	if err != nil {
		return
	}

	bytesWritten, _, err := util.WriteAndSaveData(data, fp, superBlock, inodeBitmap, dataBitmap, false)
	fmt.Println(bytesWritten, err)

	defer fp.Close()

	fp.Seek(0, 0)
	superBlock = util.Superblock{}
	binary.Read(fp, binary.LittleEndian, &superBlock)
	fmt.Printf("%+v\n", superBlock)

	dataBitmap = make([]uint8, superBlock.BitmapSize)
	inodeBitmap = make([]uint8, superBlock.BitmapiSize)
	inode := util.PseudoInode{}

	fp.Seek(int64(superBlock.BitmapStartAddress), 0)
	err = binary.Read(fp, binary.LittleEndian, &dataBitmap)
	if err == io.EOF {
		fmt.Println(err)
	}
	fmt.Printf("databitmap: %v\n", dataBitmap)

	fp.Seek(int64(superBlock.BitmapiStartAddress), 0)
	err = binary.Read(fp, binary.LittleEndian, &inodeBitmap)
	if err == io.EOF {
		fmt.Println(err)
	}
	fmt.Printf("%v\n", inodeBitmap)

	inode, err = util.LoadInode(fp, 2, int64(superBlock.InodeStartAddress))
	if inode.NodeId == 0 || err != nil {
		fmt.Printf("%d", inode.NodeId)
		log.Fatal(err)
		return
	}
	fmt.Printf("%+v\n", inode)

	datablocks, _, _ := util.GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, 0, superBlock.ClusterSize)
	fmt.Println(datablocks)

	inodeFree, _ := util.GetAvailableInodeAddress(inodeBitmap, superBlock.InodeStartAddress, int32(binary.Size(util.PseudoInode{})))
	fmt.Println(inodeFree, binary.Size(util.PseudoInode{}))

	var xdd []int32 = make([]int32, superBlock.ClusterSize/4)
	fp.Seek(int64(inode.Indirect[1]), 0)
	_ = binary.Read(fp, binary.LittleEndian, &xdd)
	//data := util.BytesToInt32(xdd)
	util.SortInt32(xdd)

	fmt.Printf("YAAA: %v | \n", xdd)

	//byte_text, _ := util.ReadFileData(fp, inode, superBlock)
	//fmt.Print(string(byte_text))

	util.AddDirItem(1, 2, "AHAHHA", fp, superBlock)
	util.AddDirItem(1, 3, "DIS", fp, superBlock)
	currentDir := make([]util.DirectoryItem, superBlock.ClusterSize/int32(binary.Size(util.DirectoryItem{})))

	inoderootdir, _ := util.LoadInode(fp, 1, int64(superBlock.InodeStartAddress))
	rootdir, _ := util.ReadFileData(fp, inoderootdir, superBlock)
	buf := new(bytes.Buffer)
	buf.Write(rootdir)
	binary.Read(buf, binary.LittleEndian, currentDir)
	fmt.Println(currentDir) */
	//bmp := util.CreateBitmap(8)
	//println(binary.Size(bmp))
	//fmt.Println(bmp)
}
