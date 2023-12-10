package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
)

func CreateBitmap(bytes int) []uint8 {
	bitmap := make([]uint8, bytes)
	return bitmap
}

// Creates a superblock and calculates required addresses
func createSuperBlock(filename string, diskSize int, clusterSize int) Superblock {
	var superBlock Superblock
	pseudoInode := PseudoInode{}
	copy(superBlock.Signature[:], "user")
	copy(superBlock.VolumeDescriptor[:], "description")
	superBlock.DiskSize = int64(diskSize)
	superBlock.ClusterSize = int32(clusterSize)
	superBlock.ClusterCount = int32(diskSize / clusterSize)
	superBlock.InodeCount = int32(diskSize / BytesPerInode)

	//divided by 8 because for example: 1000 blocks = 1000 bits and i need to calculate how many bytes i need for 1000bits
	superBlock.BitmapSize = int32(math.Ceil(float64(superBlock.ClusterCount) / 8.0))
	superBlock.BitmapiSize = int32(math.Ceil(float64(superBlock.InodeCount) / 8.0))

	superBlock.BitmapStartAddress = int32(binary.Size(superBlock))
	superBlock.BitmapiStartAddress = superBlock.BitmapStartAddress + int32(superBlock.BitmapSize)
	superBlock.InodeStartAddress = superBlock.BitmapiStartAddress + int32(superBlock.BitmapiSize)
	superBlock.DataStartAddress = superBlock.InodeStartAddress + superBlock.InodeCount*int32(binary.Size(pseudoInode))
	return superBlock
}

// Format formats a filesystem with the specified diskSize and fsName.
// It creates a superblock, data bitmap, inode bitmap, and root directory and saves it into the filesystem.
// The function returns the created superblock, data bitmap, inode bitmap, and any error encountered.
func Format(diskSize int, fsName string) (Superblock, []uint8, []uint8, error) {
	totalSize := diskSize
	superBlock := createSuperBlock(fsName, diskSize, DefaultClusterSize)
	dataBitmap := CreateBitmap(int(superBlock.BitmapSize))
	inodeBitmap := CreateBitmap(int(superBlock.BitmapiSize))

	fp, err := os.Create(fsName)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to create file: %v", err)
	}
	defer fp.Close()

	err = binary.Write(fp, binary.LittleEndian, &superBlock)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to write superblock: %v", err)
	}

	err = saveBitmap(fp, int64(superBlock.BitmapStartAddress), dataBitmap)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to create data bitmap: %v", err)
	}

	err = saveBitmap(fp, int64(superBlock.BitmapiStartAddress), inodeBitmap)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to create inode bitmap: %v", err)
	}

	_, err = fp.Seek(int64(totalSize-1), 0)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to seek to end of file: %v", err)
	}

	_, err = fp.Write([]byte{0})
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to write to end of file: %v", err)
	}

	_, _, err = CreateDirectory(fp, superBlock, inodeBitmap, dataBitmap, 1)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to create root directory: %v", err)
	}

	dataBitmap, err = LoadBitmap(fp, superBlock.BitmapStartAddress, superBlock.BitmapSize)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to load data bitmap: %v", err)
	}

	inodeBitmap, err = LoadBitmap(fp, superBlock.BitmapiStartAddress, superBlock.BitmapiSize)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to load inode bitmap: %v", err)
	}

	return superBlock, dataBitmap, inodeBitmap, nil
}

// Creates a new inode
// Returns the inode and new inode bitmap
func CreateInode(inodeBitmap []uint8, superBlock Superblock, isDirectory bool, filesize int32) (PseudoInode, []uint8, error) {
	//inodeBitmap = append([]uint8(nil), inodeBitmap...)
	inode := PseudoInode{}
	availableInode, inodeBitmapNew, err := GetAvailableInodeAddress(inodeBitmap, superBlock.InodeStartAddress, int32(binary.Size(PseudoInode{})))
	if err != nil {
		return PseudoInode{}, nil, err
	}
	inode.NodeId = 1 + ((availableInode - superBlock.InodeStartAddress) / int32(binary.Size(PseudoInode{}))) //plus 1 because 0 is reserved for free inodes
	inode.FileSize = filesize
	inode.IsDirectory = isDirectory
	//inodeBitmap[(inode.NodeId-1)/8] = setBit(inodeBitmap[(inode.NodeId-1)/8], uint8((inode.NodeId-1)%8), true)
	return inode, inodeBitmapNew, nil
}

// mapDataToInode maps data blocks to the given inode, considering the available data blocks, superblock information,
// and the size of the file. It calculates the number of singly indirect and doubly indirect blocks needed to store
// the data blocks. It returns the singly indirect block, doubly indirect block, updated data bitmap, and any error encountered.
func mapDataToInode(superBlock Superblock, inode *PseudoInode, availableDataBlocks []int32, dataBitmap []uint8) (SinglyIndirectBlock, DoublyIndirectBlock, []uint8, error) {
	addrInOneBlock := superBlock.ClusterSize / AddressByteLen
	directAddrLen := len(inode.Direct)
	IndirectOne := SinglyIndirectBlock{}
	IndirectTwo := DoublyIndirectBlock{}

	doublyIndirectBlockNeeded := 0
	//one singly will belong in indirect 1 and other singlys will belong in indirect 2
	singlyIndirectBlockNeeded := int(math.Ceil(float64(len(availableDataBlocks)-directAddrLen) / float64(addrInOneBlock)))
	if singlyIndirectBlockNeeded > 1 {
		doublyIndirectBlockNeeded++
	}

	//addrInLastBlock := singlyIndirectBlockNeeded*int(addrInOneBlock) - (len(availableDataBlocks) - directAddrLen)
	//lastBlockDataLen := (int(inode.FileSize) - int(dataMaxBlocks-1)*int(blockSize))
	//pointingToDataBlocks := singlyIndirectBlockNeeded * int(addrInOneBlock)
	extraBlocksNeeded := doublyIndirectBlockNeeded + singlyIndirectBlockNeeded
	extraBlocksSize := extraBlocksNeeded * int(superBlock.ClusterSize)

	//number of addresses pointing to data blocks vs amount of data blocks for data
	if int(addrInOneBlock*addrInOneBlock+addrInOneBlock)+directAddrLen < len(availableDataBlocks) {
		return SinglyIndirectBlock{}, DoublyIndirectBlock{}, nil, fmt.Errorf("file is too big (not enough references available)")
	}

	extraDataBlocks, dataBitmapNew, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(extraBlocksSize), superBlock.ClusterSize)
	if err != nil {
		return SinglyIndirectBlock{}, DoublyIndirectBlock{}, nil, err
	}

	copy(inode.Direct[:], availableDataBlocks)

	if len(extraDataBlocks) != 0 {
		remainingDataBlocks := availableDataBlocks[directAddrLen:]

		singlyIndirectBlocks := make([]SinglyIndirectBlock, singlyIndirectBlockNeeded)
		for i := 0; i < singlyIndirectBlockNeeded; i++ {
			startIndex := i * int(addrInOneBlock)
			endIndex := (i + 1) * int(addrInOneBlock)
			if i == singlyIndirectBlockNeeded-1 {
				singlyIndirectBlocks[i].Pointers = make([]int32, len(remainingDataBlocks[startIndex:]))
				copy(singlyIndirectBlocks[i].Pointers, remainingDataBlocks[startIndex:])
			} else {
				singlyIndirectBlocks[i].Pointers = make([]int32, len(remainingDataBlocks[startIndex:endIndex]))
				copy(singlyIndirectBlocks[i].Pointers, remainingDataBlocks[startIndex:endIndex])
			}
			singlyIndirectBlocks[i].Address = extraDataBlocks[i]
		}

		doublyIndirectBlock := DoublyIndirectBlock{}
		if doublyIndirectBlockNeeded > 0 {
			doublyIndirectBlock.Pointers = make([]SinglyIndirectBlock, singlyIndirectBlockNeeded-1)
			copy(doublyIndirectBlock.Pointers, singlyIndirectBlocks[1:])
			doublyIndirectBlock.Address = extraDataBlocks[len(extraDataBlocks)-1]
		}

		IndirectOne = singlyIndirectBlocks[0]
		inode.Indirect[0] = IndirectOne.Address

		IndirectTwo = doublyIndirectBlock
		inode.Indirect[1] = IndirectTwo.Address
	}

	return IndirectOne, IndirectTwo, dataBitmapNew, nil
}

// Writes data into the file system given available data blocks for the data.
// If the data is larger than the available data blocks, data will be truncated.
//
// Use GetAvailableDataBlocks() method to get correct amount of data blocks needed for the data.
// Returns the number of bytes written and an error if any.
func saveDataBlocks(src []byte, destPtr *os.File, superBlock Superblock, availableDataBlocks []int32) (int, error) {
	data := src
	bytesWritten := 0
	for i, v := range availableDataBlocks {
		var writeData []byte
		if i == 0 {
			writeData = data[:]
		} else if i == len(availableDataBlocks)-1 {
			writeData = data[i*int(superBlock.ClusterSize):]
		} else {
			writeData = data[i*int(superBlock.ClusterSize) : i*int(superBlock.ClusterSize)+int(superBlock.ClusterSize)]
		}

		_, err2 := destPtr.Seek(int64(v), 0)
		err := binary.Write(destPtr, binary.LittleEndian, writeData)
		bytesWritten += int(superBlock.ClusterSize)

		if err2 != nil || err != nil {
			return 0, fmt.Errorf("could not write into datablock: %v, %v", err, err2)
		}
	}

	return bytesWritten, nil
}

// saveIndirectData handles writing required indirect pointers into the file system.
// In other words, handles writing SinglyIndirectBlock and DoublyIndirectBlock into file system.
// Returns an error if there is any issue writing the data to the file system.
func saveIndirectData(fs *os.File, inode PseudoInode, singlyIndirectBlock SinglyIndirectBlock, doublyIndirectBlock DoublyIndirectBlock) error {
	//write indirect one
	if singlyIndirectBlock.Address != 0 {
		fs.Seek(int64(singlyIndirectBlock.Address), 0)
		err := binary.Write(fs, binary.LittleEndian, singlyIndirectBlock.Pointers)
		if err != nil {
			return fmt.Errorf("could not write into datablock: %v", err)
		}
	}

	//write indirect two
	if doublyIndirectBlock.Address != 0 {
		doublyIndirectBlockPointers := make([]int32, len(doublyIndirectBlock.Pointers))
		for _, singlyIndirectBlock := range doublyIndirectBlock.Pointers {
			doublyIndirectBlockPointers = append(doublyIndirectBlockPointers, singlyIndirectBlock.Address)
			fs.Seek(int64(singlyIndirectBlock.Address), 0)
			err := binary.Write(fs, binary.LittleEndian, singlyIndirectBlock.Pointers)
			if err != nil {
				return fmt.Errorf("could not write into datablock: %v", err)
			}
		}
		fs.Seek(int64(doublyIndirectBlock.Address), 0)
		err := binary.Write(fs, binary.LittleEndian, doublyIndirectBlockPointers)
		if err != nil {
			return fmt.Errorf("could not write into datablock: %v", err)
		}
	}

	return nil
}

// WriteAndSaveData writes and saves data to the file system.
// It returns the number of bytes written, the inode ID of the new file, and an error if any.
func WriteAndSaveData(src []byte, destPtr *os.File, superBlock Superblock, inodeBitmap []uint8, dataBitmap []uint8, isDirectory bool) (int, int, error) {
	bytesWritten := 0
	data := src
	//Create inode, get new inodebitmap
	inode, inodeBitmap, err := CreateInode(inodeBitmap, superBlock, isDirectory, int32(len(data)))
	if err != nil {
		return 0, 0, err
	}
	//get datablocks needed
	availableDataBlocks, dataBitmap, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(len(data)), superBlock.ClusterSize)
	if err != nil {
		return 0, 0, err
	}

	//save data, get new databitmap
	bytesWritten, err = saveDataBlocks(data, destPtr, superBlock, availableDataBlocks)
	if err != nil {
		return 0, 0, err
	}

	/* 	//dataBitmap gets modified
	   	indirectBlockOne, indirectBlockTwo, dataBitmap, err := mapDataInode(superBlock, &inode, availableDataBlocks, dataBitmap)
	   	if err != nil {
	   		return 0, 0, err
	   	}

	   	err = saveIndirectData(destPtr, inode, indirectBlockOne, indirectBlockTwo)
	   	if err != nil {
	   		return 0, 0, err
	   	} */

	singlyIndirectBlock, doublyIndirectBlock, dataBitmap, err := mapDataToInode(superBlock, &inode, availableDataBlocks, dataBitmap)
	if err != nil {
		return 0, 0, err
	}

	err = saveIndirectData(destPtr, inode, singlyIndirectBlock, doublyIndirectBlock)
	if err != nil {
		return 0, 0, err
	}

	err = saveInode(destPtr, int64(superBlock.InodeStartAddress), inode)
	if err != nil {
		return 0, 0, err
	}

	err = saveBitmap(destPtr, int64(superBlock.BitmapStartAddress), dataBitmap)
	if err != nil {
		return 0, 0, err
	}

	err = saveBitmap(destPtr, int64(superBlock.BitmapiStartAddress), inodeBitmap)
	if err != nil {
		return 0, 0, err
	}
	return bytesWritten, int(inode.NodeId), nil
}

// PathToInode takes a file system, a path, a superblock, and a current inode as input.
// It converts a relative or absolute path into an inode, representing the file or directory specified by the path.
// The function returns the current inode, the parent inode, and an error (if any).
func PathToInode(fs *os.File, path string, superBlock Superblock, currentInode PseudoInode) (PseudoInode, PseudoInode, error) {
	// Split the path into individual directories and file name
	directories := strings.Split(path, "/")
	fileName := directories[len(directories)-1]

	var err error
	i := 0

	if directories[0] == "" {
		currentInode, err = LoadInode(fs, 1, int64(superBlock.InodeStartAddress))
		if err != nil {
			return PseudoInode{}, PseudoInode{}, err
		}
		i = 1
	}

	parentInode := currentInode
	for ; i < len(directories); i++ {
		if currentInode.IsDirectory == false && i != len(directories)-1 {
			return PseudoInode{}, PseudoInode{}, fmt.Errorf("path is not a directory")
		}

		if directories[i] == "" {
			return currentInode, parentInode, nil
		}

		directory, err := LoadDirectory(fs, currentInode, superBlock)
		if err != nil {
			return PseudoInode{}, PseudoInode{}, err
		}

		if i == len(directories)-1 {
			dirItemIndex := GetDirItemIndex(directory, fileName)
			if len(strings.TrimSpace(fileName)) == 0 {
				return PseudoInode{}, PseudoInode{}, fmt.Errorf("filename is empty")
			}
			if dirItemIndex == -1 {
				return PseudoInode{}, PseudoInode{}, fmt.Errorf("file does not exist")
			}
			parentInode = currentInode
			currentInode, err := LoadInode(fs, directory[dirItemIndex].Inode, int64(superBlock.InodeStartAddress))
			if err != nil {
				return PseudoInode{}, PseudoInode{}, err
			}
			return currentInode, parentInode, nil
		} else {
			dirItemIndex := GetDirItemIndex(directory, directories[i])
			if dirItemIndex == -1 {
				return PseudoInode{}, PseudoInode{}, fmt.Errorf("directory does not exist")
			}

			parentInode = currentInode
			currentInode, err = LoadInode(fs, directory[dirItemIndex].Inode, int64(superBlock.InodeStartAddress))
			if err != nil {
				return PseudoInode{}, PseudoInode{}, err
			}
		}

	}
	return PseudoInode{}, PseudoInode{}, fmt.Errorf("could not find file")
}

// GetFileClusters retrieves the clusters of a file given its inode and superblock.
// It returns two slices: dataAddrs containing the addresses of the data clusters,
// and indirectPtrAddrs containing the addresses of extra blocks allocated for singly and doubly indirect pointer blocks.
// The destPtr parameter is a pointer to the os.File object representing the file.
// The inode parameter is the PseudoInode struct representing the file's inode.
// The superblock parameter is the Superblock struct representing the file system's superblock.
// The function returns an error if there was an issue reading the clusters.
func GetFileClusters(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]int32, []int32, error) {
	dataAddrs := make([]int32, 0)
	indirectPtrAddrs := make([]int32, 0)
	blocksRead := 0
	blockSize := superblock.ClusterSize
	dataMaxBlocks := int(math.Ceil(float64(inode.FileSize) / float64(blockSize)))

	for _, blockAddr := range inode.Direct {
		if blockAddr == 0 {
			continue
		}

		if blocksRead == dataMaxBlocks {
			break
		}
		dataAddrs = append(dataAddrs, blockAddr)
		blocksRead++
	}

	//indirect level one
	if inode.Indirect[0] != 0 {
		indirectPtrAddrs = append(indirectPtrAddrs, inode.Indirect[0])
		indirectOneBlockData, err := readBlockInt32(destPtr, int64(inode.Indirect[0]), blockSize)
		if err != nil {
			return nil, nil, err
		}

		for _, indirectOneBlockAddr := range indirectOneBlockData {
			if indirectOneBlockAddr == 0 {
				continue
			}

			if blocksRead == dataMaxBlocks {
				break
			}
			dataAddrs = append(dataAddrs, indirectOneBlockAddr)
			blocksRead++
		}
	}

	//indirect level two
	if inode.Indirect[1] != 0 {
		indirectPtrAddrs = append(indirectPtrAddrs, inode.Indirect[1])
		indirectTwoBlockDataFirst, err := readBlockInt32(destPtr, int64(inode.Indirect[1]), blockSize)
		if err != nil {
			return nil, nil, err
		}

		for _, addr := range indirectTwoBlockDataFirst {
			if addr == 0 {
				continue
			}
			indirectPtrAddrs = append(indirectPtrAddrs, addr)
			indirectTwoBlockDataSecond, err := readBlockInt32(destPtr, int64(addr), blockSize)
			if err != nil {
				return nil, nil, err
			}
			for _, addr2 := range indirectTwoBlockDataSecond {
				if addr2 == 0 {
					continue
				}

				if blocksRead == dataMaxBlocks {
					break
				}
				dataAddrs = append(dataAddrs, addr2)
				blocksRead++
			}
		}
	}
	return dataAddrs, indirectPtrAddrs, nil
}

// ReadFileData reads the data of a file from the given destination file pointer, inode, and superblock.
// It returns the file data as a byte slice and an error if any.
func ReadFileData(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]byte, error) {
	addresses, _, err := GetFileClusters(destPtr, inode, superblock)
	blockSize := superblock.ClusterSize
	dataMaxBlocks := int(math.Ceil(float64(inode.FileSize) / float64(blockSize)))
	lastBlockDataLen := (int(inode.FileSize) - int(dataMaxBlocks-1)*int(blockSize))

	var data []byte
	if err != nil {
		return nil, err
	}
	for i, addr := range addresses {
		if i == len(addresses)-1 {
			blockSize = int32(lastBlockDataLen)
		}
		blockData, err := readBlock(destPtr, int64(addr), blockSize)
		if err != nil {
			return nil, err
		}
		data = append(data, blockData...)
	}
	return data, nil
}

// readBlockInt32 reads a block of data from the specified file at the given block address.
// It returns a slice of int32 values representing the data read from the block.
// The destPtr parameter is a pointer to the file to read from.
// The blockAddr parameter specifies the starting address of the block.
// The blockSize parameter specifies the size of the block in bytes.
// If an error occurs during the read operation, it is returned along with a nil slice.
func readBlockInt32(destPtr *os.File, blockAddr int64, blockSize int32) ([]int32, error) {
	blockData := make([]int32, blockSize/AddressByteLen)
	_, err := destPtr.Seek(blockAddr, 0)
	if err != nil {
		return nil, err
	}

	err = binary.Read(destPtr, binary.LittleEndian, &blockData)
	if err != nil {
		return nil, err
	}

	return blockData, nil
}

// readBlock reads a block of data from the specified file at the given block address.
// It returns the block data as a byte slice and an error if any.
func readBlock(destPtr *os.File, blockAddr int64, blockSize int32) ([]byte, error) {
	blockData := make([]byte, blockSize)
	_, err := destPtr.Seek(blockAddr, 0)
	if err != nil {
		return nil, err
	}

	err = binary.Read(destPtr, binary.LittleEndian, &blockData)
	if err != nil {
		return nil, err
	}

	return blockData, nil
}

func LoadSuperBlock(fs *os.File) Superblock {
	fs.Seek(0, 0)
	superBlock := Superblock{}
	binary.Read(fs, binary.LittleEndian, &superBlock)
	return superBlock
}

func LoadInode(destPtr *os.File, inodeId int32, inodeStartAddress int64) (PseudoInode, error) {
	inode := PseudoInode{}
	_, err2 := destPtr.Seek(inodeStartAddress+int64(binary.Size(inode)*int(inodeId-1)), 0)

	err := binary.Read(destPtr, binary.LittleEndian, &inode)
	if err2 != nil || err != nil || inodeId == 0 {
		return PseudoInode{}, fmt.Errorf("could not read inode: %v", err)
	}
	return inode, nil
}

func saveInode(destPtr *os.File, inodeStartAddress int64, inode PseudoInode) error {
	_, err2 := destPtr.Seek(int64(inodeStartAddress+int64(binary.Size(inode))*int64(inode.NodeId-1)), 0)
	err := binary.Write(destPtr, binary.LittleEndian, &inode)
	if err2 != nil || err != nil {
		return fmt.Errorf("could not write inode: %v", err)
	}
	return nil
}

func IsInodeDirectory(destPtr *os.File, inodeId int32, inodeStartAddress int64) (bool, error) {
	inode, err := LoadInode(destPtr, inodeId, inodeStartAddress)
	if err != nil {
		return false, err
	}
	return inode.IsDirectory, nil
}

// CreateDirectory creates a new directory in the file system.
// It returns the bytes written to the file system, the inode ID of the new directory, and an error if any.
func CreateDirectory(destPtr *os.File, superBlock Superblock, inodeBitmap []uint8, dataBitmap []uint8, parentNodeId int32) (int, int, error) {
	buf := new(bytes.Buffer)

	//create inode (but dont save it into FS) so i can get free inode id
	inode, _, err := CreateInode(inodeBitmap, superBlock, true, int32(binary.Size(buf.Bytes())))
	dir := make([]DirectoryItem, superBlock.ClusterSize/int32(binary.Size(DirectoryItem{})))
	copy(dir[1].ItemName[:], []byte("."))
	copy(dir[0].ItemName[:], []byte(".."))
	dir[1].Inode = inode.NodeId
	dir[0].Inode = parentNodeId

	err = binary.Write(buf, binary.LittleEndian, dir)
	if err != nil {
		fmt.Println(err)
		return 0, 0, err
	}

	return WriteAndSaveData(buf.Bytes(), destPtr, superBlock, inodeBitmap, dataBitmap, true)
}

// GetDirItemIndex returns the index of a directory item with the given name in the provided directory.
// If the item is not found, it returns -1.
func GetDirItemIndex(dir []DirectoryItem, dirItemName string) int {
	for i, v := range dir {
		if removeNullCharsFromString(string(v.ItemName[:])) == dirItemName {
			return i
		}
	}
	return -1
}

// LoadDirectory loads the directory items from the specified inode. It does not check if the inode is a directory.
// It returns a slice of DirectoryItem and an error if any.
func LoadDirectory(fs *os.File, dirInode PseudoInode, superBlock Superblock) ([]DirectoryItem, error) {
	buf := new(bytes.Buffer)

	dir := make([]DirectoryItem, superBlock.ClusterSize/int32(binary.Size(DirectoryItem{})))

	dirInBytes, err := ReadFileData(fs, dirInode, superBlock)
	if err != nil {
		return nil, err
	}

	_, err = buf.Write(dirInBytes)
	if err != nil {
		return nil, err
	}

	err = binary.Read(buf, binary.LittleEndian, dir)
	if err != nil {
		return nil, err
	}
	return dir, nil
}

func IsDirectoryFull(fs *os.File, dirInode PseudoInode, superBlock Superblock) (bool, error) {
	dir, err := LoadDirectory(fs, dirInode, superBlock)
	if err != nil {
		return false, err
	}
	for i, v := range dir {
		if i == 0 || i == 1 {
			continue
		}
		if v.Inode == 0 {
			return false, nil
		}
	}
	return true, nil
}

// AddDirItem adds a directory item to the specified directory.
// It takes the directory inode ID, ID of the item to be added to the directory and its name,
// the file system, and the superblock as parameters.
// It returns an error if any operation fails.
func AddDirItem(dirInodeId int32, dirItemNodeId int32, dirItemName string, fs *os.File, superBlock Superblock) error {
	newDataBuf := new(bytes.Buffer)
	dirItem := DirectoryItem{}
	dirItem.Inode = dirItemNodeId
	copy(dirItem.ItemName[:], []byte(dirItemName))

	currentDirInode, err := LoadInode(fs, dirInodeId, int64(superBlock.InodeStartAddress))
	if err != nil {
		return err
	}

	dirItemInode, err := LoadInode(fs, dirItemNodeId, int64(superBlock.InodeStartAddress))
	if err != nil {
		return err
	}
	dirItemInode.References++

	currentDir, err := LoadDirectory(fs, currentDirInode, superBlock)
	if err != nil {
		return err
	}

	usedDataBlocks, _, err := GetFileClusters(fs, currentDirInode, superBlock)
	if err != nil {
		return err
	}

	isFull, err := IsDirectoryFull(fs, currentDirInode, superBlock)
	if err != nil {
		return err
	}
	if isFull {
		return fmt.Errorf("directory is full")
	}

	if GetDirItemIndex(currentDir, dirItemName) > -1 {
		dirItemInode.References--
		if dirItemInode.References <= 0 {
			DeleteFile(fs, dirItemInode, superBlock)
		}
		return fmt.Errorf("file with same name already exists")
	}

	for i, v := range currentDir {
		if i == 0 || i == 1 {
			continue
		}
		if v.Inode == 0 {
			currentDir[i] = dirItem
			break
		}
	}
	err = binary.Write(newDataBuf, binary.LittleEndian, currentDir)
	if err != nil {
		return err
	}

	_, err = saveDataBlocks(newDataBuf.Bytes(), fs, superBlock, usedDataBlocks)
	if err != nil {
		return err
	}

	err = saveInode(fs, int64(superBlock.InodeStartAddress), currentDirInode)
	if err != nil {
		return err
	}

	err = saveInode(fs, int64(superBlock.InodeStartAddress), dirItemInode)
	if err != nil {
		return err
	}

	return nil
}

// RemoveDirItem removes a directory item from a directory.
// It takes the directory inode ID, directory item name, destination pointer, superblock,
// and a delete flag as input parameters.
// If the delete flag is true and the directory item's inode references reach zero,
// the corresponding file is deleted from the file system.
// If the delete flag is false, the item is not deleted from the file system even if its inode references reach zero.
// Returns an error if any operation fails.
func RemoveDirItem(dirInodeId int32, dirItemName string, destPtr *os.File, superBlock Superblock, delete bool) error {
	oldDataBuf := new(bytes.Buffer)
	newDataBuf := new(bytes.Buffer)
	currentDir := make([]DirectoryItem, superBlock.ClusterSize/int32(binary.Size(DirectoryItem{})))
	dirItemInode := PseudoInode{}

	currentDirInode, err := LoadInode(destPtr, dirInodeId, int64(superBlock.InodeStartAddress))
	if err != nil {
		return err
	}

	//currentDirInode.References--

	usedDataBlocks, _, err := GetFileClusters(destPtr, currentDirInode, superBlock)
	if err != nil {
		return err
	}

	dirInBytes, err := ReadFileData(destPtr, currentDirInode, superBlock)
	if err != nil {
		return err
	}

	_, err = oldDataBuf.Write(dirInBytes)
	if err != nil {
		return err
	}
	err = binary.Read(oldDataBuf, binary.LittleEndian, currentDir)
	if err != nil {
		return err
	}

	dirItemIndex := GetDirItemIndex(currentDir, dirItemName)

	if dirItemIndex == -1 {
		return fmt.Errorf("file does not exist")
	} else {
		dirItemInode, err = LoadInode(destPtr, currentDir[dirItemIndex].Inode, int64(superBlock.InodeStartAddress))
		if err != nil {
			return err
		}
		dirItemInode.References--
		currentDir[dirItemIndex].Inode = 0
		currentDir[dirItemIndex] = DirectoryItem{}
	}

	//transform data back into bytes
	err = binary.Write(newDataBuf, binary.LittleEndian, currentDir)
	if err != nil {
		return err
	}

	//save data
	_, err = saveDataBlocks(newDataBuf.Bytes(), destPtr, superBlock, usedDataBlocks)
	if err != nil {
		return err
	}

	err = saveInode(destPtr, int64(superBlock.InodeStartAddress), currentDirInode)
	if err != nil {
		return err
	}

	if dirItemInode.References <= 0 && delete {
		err = DeleteFile(destPtr, dirItemInode, superBlock)
		if err != nil {
			return err
		}
	} else {
		err = saveInode(destPtr, int64(superBlock.InodeStartAddress), dirItemInode)
		if err != nil {
			return err
		}
	}

	return nil
}

// Removes null characters from a string.
func removeNullCharsFromString(s string) string {
	return strings.Replace(s, "\x00", "", -1)
}

// DeleteFile deletes a file from the file system.
// It takes a file system object (fs), a pseudo inode (inode), and a superblock (superBlock) as parameters.
// It first loads the inode bitmap and data bitmap from the file system.
// Then it retrieves the addresses of clusters and extra allocated blocks for singly and indirect pointer blocks of the file.
// It updates the inode bitmap and data bitmap to mark the clusters and indirect block addresses as free.
// It sets the NodeId of the inode to 0 to indicate that it is no longer in use.
// Finally, it saves the updated inode, inode bitmap, and data bitmap back to the file system.
// If any error occurs during the process, it returns the error.
func DeleteFile(fs *os.File, inode PseudoInode, superBlock Superblock) error {
	inodeBitmap, err := LoadBitmap(fs, superBlock.BitmapiStartAddress, superBlock.BitmapiSize)
	dataBitmap, err := LoadBitmap(fs, superBlock.BitmapStartAddress, superBlock.BitmapSize)
	dataAddresses, indirectPtrAddresess, err := GetFileClusters(fs, inode, superBlock)

	//inodeBitmap[(inode.NodeId-1)/8] = setBit(inodeBitmap[(inode.NodeId-1)/8], uint8((inode.NodeId-1)%8), true)
	inodeBitmap = SetValueInInodeBitmap(inodeBitmap, inode, false)
	dataBitmap = SetValuesInDataBitmap(dataBitmap, dataAddresses, superBlock.DataStartAddress, superBlock.ClusterSize, false)
	dataBitmap = SetValuesInDataBitmap(dataBitmap, indirectPtrAddresess, superBlock.DataStartAddress, superBlock.ClusterSize, false)

	inode.NodeId = 0
	err = saveInode(fs, int64(superBlock.InodeStartAddress), inode)
	if err != nil {
		return err
	}
	err = saveBitmap(fs, int64(superBlock.BitmapStartAddress), dataBitmap)
	if err != nil {
		return err
	}
	err = saveBitmap(fs, int64(superBlock.BitmapiStartAddress), inodeBitmap)
	if err != nil {
		return err
	}
	return nil
}

// SetValueInInodeBitmap sets the value of the bit corresponding to the given inode in the inode bitmap.
// It takes the inode bitmap, inode, and value as input parameters.
// It returns a copy of the inode bitmap with the value of the bit corresponding to the given inode set to the given value.
func SetValueInInodeBitmap(inodeBitmap []uint8, inode PseudoInode, value bool) []uint8 {
	bitmap := append([]uint8(nil), inodeBitmap...)
	bitmap[(inode.NodeId-1)/8] = setBit(bitmap[(inode.NodeId-1)/8], uint8((inode.NodeId-1)%8), false)
	return bitmap
}

// saveBitmap saves the given bitmap to the given address in the file system.
func saveBitmap(destPtr *os.File, address int64, bitmap []uint8) error {
	_, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return fmt.Errorf("could not write bitmap: %v", err)
	}
	return nil
}

// LoadBitmap loads the bitmap from the given address in the file system.
func LoadBitmap(destPtr *os.File, bitmapStartAddress int32, bitmapSize int32) ([]uint8, error) {
	bitmap := make([]uint8, bitmapSize)
	_, err2 := destPtr.Seek(int64(bitmapStartAddress), 0)
	err := binary.Read(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return nil, fmt.Errorf("could not write bitmap: %v", err)
	}
	return bitmap, nil
}

// SetValuesInDataBitmap sets the values of the bits corresponding to the given data blocks in the data bitmap.
// It takes the data bitmap, data block addresses, data start address, block size, and value as input parameters.
// It returns a copy of the data bitmap with the values of the bits corresponding to the given data blocks set to the given value.
func SetValuesInDataBitmap(dataBitmap []uint8, dataBlockAddresses []int32, dataStartAddress int32, blockSize int32, value bool) []uint8 {
	bitmap := append([]uint8(nil), dataBitmap...)
	for _, v := range dataBlockAddresses {
		dataBit := (v - dataStartAddress) / blockSize
		bitmap[dataBit/8] = setBit(bitmap[dataBit/8], uint8(dataBit%8), value)
	}
	return bitmap
}

// GetAvailableDataBlocks returns a list of available data blocks from the given bitmap and new data bitmap with updated values.
// It takes the bitmap, dataStartAddress, dataSize, and blockSize as input parameters.
// The function appends a copy of the bitmap to ensure immutability.
// It iterates through the bitmap to find available data blocks and adds their addresses to the blockAddressList.
// The allocatedSize keeps track of the total size of allocated data blocks.
// If the allocatedSize exceeds the dataSize, the function sets the values in the bitmap and returns the blockAddressList and updated bitmap.
// If there are not enough available data blocks, the function returns an error.
func GetAvailableDataBlocks(bitmap []uint8, dataStartAddress int32, dataSize int32, blockSize int32) ([]int32, []uint8, error) {
	bitmap = append([]uint8(nil), bitmap...)
	blockAddressList := make([]int32, 0)
	allocatedSize := 0
	for i := 0; i < len(bitmap); i++ {
		for j := 0; j < 8; j++ {
			if allocatedSize >= int(dataSize) {
				bitmap = SetValuesInDataBitmap(bitmap, blockAddressList, dataStartAddress, blockSize, true)
				return blockAddressList, bitmap, nil
			}
			blockAddress := dataStartAddress + int32(i*8*int(blockSize)) + int32(j)*blockSize
			if getBit(bitmap[i], int32(j)) == ClusterIsFree {
				blockAddressList = append(blockAddressList, blockAddress)
				allocatedSize += int(blockSize)
			}
		}
	}
	return nil, nil, fmt.Errorf("not enough available data blocks")
}

// GetAvailableInodeAddress returns the address of an available inode from the given bitmap and new inode bitmap with updated values.
// It takes the bitmap, startAddress, and inodeSize as input parameters.
// The function appends a copy of the bitmap to ensure immutability.
// It iterates through the bitmap to find an available inode and returns its address.
// If there are no available inodes, the function returns an error.
func GetAvailableInodeAddress(bitmap []uint8, startAddress int32, inodeSize int32) (int32, []uint8, error) {
	bitmap = append([]uint8(nil), bitmap...)
	for i := 0; i < len(bitmap); i++ {
		for j := 0; j < 8; j++ {
			blockAddress := startAddress + int32(i*8*int(inodeSize)) + int32(j)*inodeSize
			if getBit(bitmap[i], int32(j)) == InodeIsFree {
				bitmap[i] = setBit(bitmap[i], uint8(j), true)
				return blockAddress, bitmap, nil
			}
		}
	}
	return -1, nil, fmt.Errorf("no free inodes")
}

// getBit returns the value of the bit at the specified position in the given number.
// The number is treated as an 8-bit unsigned integer.
// Least significant bit is at position 0.
// The function returns 1 if the bit is set (1) and 0 if the bit is not set (0).
func getBit(num uint8, position int32) uint8 {
	return (num >> position) & 1
}

// setBit sets the nth bit of the number to x.
func setBit(number uint8, n uint8, x bool) uint8 {
	if x {
		return (number & ^(1 << n)) | (1 << n)
	}
	return (number & ^(1 << n)) | (0 << n)
}
