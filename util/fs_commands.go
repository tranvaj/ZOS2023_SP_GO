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

func createSuperBlock(filename string, diskSize int, clusterSize int) Superblock {
	var superBlock Superblock
	pseudoInode := PseudoInode{}
	copy(superBlock.Signature[:], "user")
	copy(superBlock.VolumeDescriptor[:], "description")
	superBlock.DiskSize = int32(diskSize)
	superBlock.ClusterSize = int32(clusterSize)
	superBlock.ClusterCount = int32(diskSize) / int32(clusterSize)
	superBlock.InodeCount = int32(diskSize) / BytesPerInode

	//divided by 8 because for example: 1000 blocks = 1000 bits and i need to calculate how many bytes i need for 1000bits
	superBlock.BitmapSize = int32(math.Ceil(float64(superBlock.ClusterCount) / 8.0))
	superBlock.BitmapiSize = int32(math.Ceil(float64(superBlock.InodeCount) / 8.0))

	superBlock.BitmapStartAddress = int32(binary.Size(superBlock))
	superBlock.BitmapiStartAddress = superBlock.BitmapStartAddress + int32(superBlock.BitmapSize)
	superBlock.InodeStartAddress = superBlock.BitmapiStartAddress + int32(superBlock.BitmapiSize)
	superBlock.DataStartAddress = superBlock.InodeStartAddress + superBlock.InodeCount*int32(binary.Size(pseudoInode))
	return superBlock
}

func Format(diskSize int, fsName string) (Superblock, []uint8, []uint8, error) {
	totalSize := diskSize
	superBlock := createSuperBlock(fsName, diskSize, DefaultClusterSize)
	dataBitmap := CreateBitmap(int(superBlock.BitmapSize))
	//dataBitmap[superBlock.BitmapSize-1] = setBit(dataBitmap[superBlock.BitmapSize-1], 0, true)
	//dataBitmap[0] = setBit(dataBitmap[0], 0, true)
	inodeBitmap := CreateBitmap(int(superBlock.BitmapiSize))
	//inodeBitmap[superBlock.BitmapiSize-1] = setBit(inodeBitmap[superBlock.BitmapiSize-1], 0, true)
	//inodeBitmap[0] = setBit(inodeBitmap[0], 0, true)

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

// creates a new inode
// returns the inode and modified inode bitmap
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

func assignDataToInode(superBlock Superblock, inode *PseudoInode, availableDataBlocks []int32, dataBitmap []uint8) (SinglyIndirectBlock, DoublyIndirectBlock, []uint8, error) {
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
	pointingToDataBlocks := singlyIndirectBlockNeeded * int(addrInOneBlock)
	extraBlocksNeeded := doublyIndirectBlockNeeded + pointingToDataBlocks
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
				singlyIndirectBlocks[i].Pointers = make([]int32, len(remainingDataBlocks[startIndex:]))
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

// maps given data blocks to direct and indirect pointers, if indirect pointers are not required, returns empty maps
// if data requries indirect pointers, returns maps that contain mapping address -> list of addresses (indirect one)
// and mapping address -> (address -> list of addresses)
func mapDataInode(superBlock Superblock, inode *PseudoInode, availableDataBlocks []int32, dataBitmap []uint8) (map[int32][]int32, map[int32]map[int32][]int32, []uint8, error) {
	addrInOneBlock := superBlock.ClusterSize / AddressByteLen
	directAddrLen := len(inode.Direct)

	indirectBlocksNeeded := int(math.Ceil(float64(len(availableDataBlocks)-directAddrLen) / float64(addrInOneBlock)))
	indirectBlocksSize := 0

	if indirectBlocksNeeded > 1 {
		//+1 because need to reserve extra block that points to block full of pointers to blocks
		//need indirect level two and one
		indirectBlocksSize = (indirectBlocksNeeded + 1) * int(superBlock.ClusterSize)
	} else {
		//need only indirect level one
		indirectBlocksSize = indirectBlocksNeeded * int(superBlock.ClusterSize)
	}
	//number of addresses pointing to data blocks vs amount of data blocks for data
	if int(addrInOneBlock*addrInOneBlock+addrInOneBlock)+directAddrLen < len(availableDataBlocks) {
		return nil, nil, nil, fmt.Errorf("file is too big (not enough references available)")
	}

	copy(inode.Direct[:], availableDataBlocks)

	//empty slice means no indirect blocks needed
	extraDataBlocks, dataBitmapNew, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(indirectBlocksSize), superBlock.ClusterSize)
	//copy(dataBitmap, dataBitmapNew) //modify slice in argument
	indirectOneBlock := make(map[int32][]int32)
	indirectTwoBlock := make(map[int32]map[int32][]int32)
	if err != nil {
		return nil, nil, nil, err
	}

	if len(extraDataBlocks) != 0 {
		indirectOneBlock[extraDataBlocks[0]] = make([]int32, addrInOneBlock)
		end := int(math.Min(float64(directAddrLen+int(addrInOneBlock)), float64(len(availableDataBlocks))))
		copy(indirectOneBlock[extraDataBlocks[0]], availableDataBlocks[directAddrLen:end])
		inode.Indirect[0] = extraDataBlocks[0]

		if len(extraDataBlocks) > 1 {
			if start := end; start <= len(availableDataBlocks) {
				indirectBlocksNeededTwo := int(math.Ceil(float64(len(availableDataBlocks[start:])) / float64(addrInOneBlock)))
				indirectTwoBlock[extraDataBlocks[1]] = make(map[int32][]int32)
				for i := 0; i < indirectBlocksNeededTwo; i++ {
					//i+1 because 0 is reserved for indirect level one
					if i == indirectBlocksNeededTwo-1 {
						end = len(availableDataBlocks)
					} else {
						end = start + int(addrInOneBlock)
					}
					index := i + 1
					indirectTwoBlock[extraDataBlocks[1]][extraDataBlocks[index+1]] = make([]int32, addrInOneBlock)
					copy(indirectTwoBlock[extraDataBlocks[1]][extraDataBlocks[index+1]], availableDataBlocks[start:end])
					start = end
				}
			}
			inode.Indirect[1] = extraDataBlocks[1]
		}
	}
	return indirectOneBlock, indirectTwoBlock, dataBitmapNew, nil
}

// writes given data into given available data blocks
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

// save data from indirect one and two
func saveIndirectData2(fs *os.File, inode PseudoInode, singlyIndirectBlock SinglyIndirectBlock, doublyIndirectBlock DoublyIndirectBlock) error {
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

// save data from indirect one and two
func saveIndirectData(destPtr *os.File, inode PseudoInode, indirectOneBlock map[int32][]int32, indirectTwoBlock map[int32]map[int32][]int32) error {
	//write indirect one
	for i, v := range indirectOneBlock[inode.Indirect[0]] {
		_, err2 := destPtr.Seek(int64(inode.Indirect[0])+int64(i)*int64(binary.Size(v)), 0)
		err := binary.Write(destPtr, binary.LittleEndian, v)
		if err2 != nil || err != nil {
			return fmt.Errorf("could not write into datablock: %v, %v", err, err2)
		}
	}

	//write indirect two
	i := 0
	for k, v := range indirectTwoBlock[inode.Indirect[1]] {
		_, err2 := destPtr.Seek(int64(inode.Indirect[1])+int64(i)*int64(binary.Size(k)), 0)
		err := binary.Write(destPtr, binary.LittleEndian, k)
		if err2 != nil || err != nil {
			return fmt.Errorf("could not write into datablock: %v, %v", err, err2)
		}

		for y, x := range v {
			_, err2 := destPtr.Seek(int64(k)+int64(y)*int64(binary.Size(x)), 0)
			err = binary.Write(destPtr, binary.LittleEndian, x)
			if err2 != nil || err != nil {
				return fmt.Errorf("could not write into datablock: %v, %v", err, err2)
			}
		}
		i++
	}
	return nil
}

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

	singlyIndirectBlock, doublyIndirectBlock, dataBitmap, err := assignDataToInode(superBlock, &inode, availableDataBlocks, dataBitmap)
	if err != nil {
		return 0, 0, err
	}

	err = saveIndirectData2(destPtr, inode, singlyIndirectBlock, doublyIndirectBlock)
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

func getInodeDataAddresses(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]int32, []int32, error) {
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
		//SortInt32(indirectTwoBlockDataFirst) //sort because map was used and it doesnt guarantee sortion

		for _, addr := range indirectTwoBlockDataFirst {
			if addr == 0 {
				continue
			}
			indirectPtrAddrs = append(indirectPtrAddrs, addr)
			indirectTwoBlockDataSecond, err := readBlockInt32(destPtr, int64(addr), blockSize)
			if err != nil {
				return nil, nil, err
			}
			//SortInt32(indirectTwoBlockDataSecond) //sort because map was used and it doesnt guarantee sortion
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

func ReadFileData(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]byte, error) {
	addresses, _, err := getInodeDataAddresses(destPtr, inode, superblock)
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
	//	err = saveInode(destPtr, int64(superBlock.InodeStartAddress+int32(binary.Size(inode))*(inode.NodeId-1)), inode)
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

func CreateDirectory(destPtr *os.File, superBlock Superblock, inodeBitmap []uint8, dataBitmap []uint8, parentNodeId int32) (int, int, error) {
	//TODO check if directory with same name already exists
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

// todo create a function that will modify inodes and remove data (set inode id = 0 and only need to change databitmap + inode bitmap)
func GetDirItemIndex(currentDir []DirectoryItem, dirItemName string) int {
	for i, v := range currentDir {
		if removeNullCharsFromString(string(v.ItemName[:])) == dirItemName {
			return i
		}
	}
	return -1
}

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

func AddDirItem(currentDirInodeId int32, dirItemNodeId int32, dirItemName string, fs *os.File, superBlock Superblock) error {
	newDataBuf := new(bytes.Buffer)
	dirItem := DirectoryItem{}
	dirItem.Inode = dirItemNodeId
	copy(dirItem.ItemName[:], []byte(dirItemName))

	currentDirInode, err := LoadInode(fs, currentDirInodeId, int64(superBlock.InodeStartAddress))
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

	usedDataBlocks, _, err := getInodeDataAddresses(fs, currentDirInode, superBlock)
	if err != nil {
		return err
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

func RemoveDirItem(currentDirInodeId int32, dirItemName string, destPtr *os.File, superBlock Superblock) error {
	oldDataBuf := new(bytes.Buffer)
	newDataBuf := new(bytes.Buffer)
	currentDir := make([]DirectoryItem, superBlock.ClusterSize/int32(binary.Size(DirectoryItem{})))
	dirItemInode := PseudoInode{}

	currentDirInode, err := LoadInode(destPtr, currentDirInodeId, int64(superBlock.InodeStartAddress))
	if err != nil {
		return err
	}

	//currentDirInode.References--

	usedDataBlocks, _, err := getInodeDataAddresses(destPtr, currentDirInode, superBlock)
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

	if dirItemInode.References <= 0 {
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

func removeNullCharsFromString(s string) string {
	return strings.Replace(s, "\x00", "", -1)
}

func DeleteFile(fs *os.File, inode PseudoInode, superBlock Superblock) error {
	inodeBitmap, err := LoadBitmap(fs, superBlock.BitmapiStartAddress, superBlock.BitmapiSize)
	dataBitmap, err := LoadBitmap(fs, superBlock.BitmapStartAddress, superBlock.BitmapSize)
	dataAddresses, indirectPtrAddresess, err := getInodeDataAddresses(fs, inode, superBlock)

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

func SetValueInInodeBitmap(inodeBitmap []uint8, inode PseudoInode, value bool) []uint8 {
	bitmap := append([]uint8(nil), inodeBitmap...)
	bitmap[(inode.NodeId-1)/8] = setBit(bitmap[(inode.NodeId-1)/8], uint8((inode.NodeId-1)%8), false)
	return bitmap
}

func saveBitmap(destPtr *os.File, address int64, bitmap []uint8) error {
	_, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return fmt.Errorf("could not write bitmap: %v", err)
	}
	return nil
}

func LoadBitmap(destPtr *os.File, bitmapStartAddress int32, bitmapSize int32) ([]uint8, error) {
	bitmap := make([]uint8, bitmapSize)
	_, err2 := destPtr.Seek(int64(bitmapStartAddress), 0)
	err := binary.Read(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return nil, fmt.Errorf("could not write bitmap: %v", err)
	}
	return bitmap, nil
}

func SetValuesInDataBitmap(dataBitmap []uint8, dataBlockAddresses []int32, dataStartAddress int32, blockSize int32, value bool) []uint8 {
	bitmap := append([]uint8(nil), dataBitmap...)
	for _, v := range dataBlockAddresses {
		dataBit := (v - dataStartAddress) / blockSize
		bitmap[dataBit/8] = setBit(bitmap[dataBit/8], uint8(dataBit%8), value)
	}
	return bitmap
}

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

func getBit(num uint8, position int32) uint8 {
	return (num >> position) & 1
}

// setBit sets the nth bit of the number to x.
//
// The function takes three arguments:
// - number: the original number whose bit is to be set.
// - n: the position of the bit to set, starting from 0 for the least significant bit (rightmost).
// - x: a boolean indicating whether the bit should be set to 1 (true) or 0 (false).
//
// The function returns a new number that is the same as the original number, but with the nth bit set to x.
//
// If x is true, the nth bit is set to 1.
// If x is false, the nth bit is set to 0.
//
// Example:
// setBit(8, 0, true) returns 9, because 8 is 1000 in binary and setting the 0th bit to 1 results in 1001, which is 9 in decimal.
func setBit(number uint8, n uint8, x bool) uint8 {
	if x {
		return (number & ^(1 << n)) | (1 << n)
	}
	return (number & ^(1 << n)) | (0 << n)
}
