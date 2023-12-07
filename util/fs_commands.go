package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
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

	dataBitmap, err = loadBitmap(fp, superBlock.BitmapStartAddress, superBlock.BitmapSize)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to load data bitmap: %v", err)
	}

	inodeBitmap, err = loadBitmap(fp, superBlock.BitmapiStartAddress, superBlock.BitmapiSize)
	if err != nil {
		return Superblock{}, nil, nil, fmt.Errorf("failed to load inode bitmap: %v", err)
	}

	return superBlock, dataBitmap, inodeBitmap, nil
}

// creates a new inode
// returns the inode and modified inode bitmap
func CreateInode(inodeBitmap []uint8, superBlock Superblock, isDirectory bool, filesize int32) (PseudoInode, []uint8, error) {
	inodeBitmap = append([]uint8(nil), inodeBitmap...)
	inode := PseudoInode{}
	availableInode, err := GetAvailableInodeAddress(inodeBitmap, superBlock.InodeStartAddress, int32(binary.Size(PseudoInode{})))
	if err != nil {
		return PseudoInode{}, nil, err
	}
	inode.NodeId = 1 + ((availableInode - superBlock.InodeStartAddress) / int32(binary.Size(PseudoInode{}))) //plus 1 because 0 is reserved for free inodes
	inode.FileSize = filesize
	inode.IsDirectory = isDirectory
	inodeBitmap[(inode.NodeId-1)/8] = setBit(inodeBitmap[(inode.NodeId-1)/8], uint8((inode.NodeId-1)%8), true)
	return inode, inodeBitmap, nil
}

// maps given data blocks to direct and indirect pointers, if indirect pointers are not required, returns empty maps
// if data requries indirect pointers, returns maps that contain mapping address -> list of addresses (indirect one)
// and mapping address -> (address -> list of addresses)
func mapDataInode(superBlock Superblock, inode *PseudoInode, availableDataBlocks []int32, dataBitmap []uint8) (map[int32][]int32, map[int32]map[int32][]int32, error) {
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
		return nil, nil, fmt.Errorf("file is too big (not enough references available)")
	}

	copy(inode.Direct[:], availableDataBlocks)

	//empty slice means no indirect blocks needed
	availableIndirectDataBlocks, dataBitmapNew, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(indirectBlocksSize), superBlock.ClusterSize)
	copy(dataBitmap, dataBitmapNew)
	indirectOneBlock := make(map[int32][]int32)
	indirectTwoBlock := make(map[int32]map[int32][]int32)
	if err != nil {
		return nil, nil, err
	}

	if len(availableIndirectDataBlocks) != 0 {
		indirectOneBlock[availableIndirectDataBlocks[0]] = make([]int32, addrInOneBlock)
		end := int(math.Min(float64(directAddrLen+int(addrInOneBlock)), float64(len(availableDataBlocks))))
		copy(indirectOneBlock[availableIndirectDataBlocks[0]], availableDataBlocks[directAddrLen:end])
		inode.Indirect[0] = availableIndirectDataBlocks[0]

		if len(availableIndirectDataBlocks) > 1 {
			if start := end; start <= len(availableDataBlocks) {
				indirectBlocksNeededTwo := int(math.Ceil(float64(len(availableDataBlocks[start:])) / float64(addrInOneBlock)))
				indirectTwoBlock[availableIndirectDataBlocks[1]] = make(map[int32][]int32)
				for i := 0; i < indirectBlocksNeededTwo; i++ {
					//i+1 because 0 is reserved for indirect level one
					if i == indirectBlocksNeededTwo-1 {
						end = len(availableDataBlocks)
					} else {
						end = start + int(addrInOneBlock)
					}
					index := i + 1
					indirectTwoBlock[availableIndirectDataBlocks[1]][availableIndirectDataBlocks[index+1]] = make([]int32, addrInOneBlock)
					copy(indirectTwoBlock[availableIndirectDataBlocks[1]][availableIndirectDataBlocks[index+1]], availableDataBlocks[start:end])
					start = end
				}
			}
			inode.Indirect[1] = availableIndirectDataBlocks[1]
		}
	}
	return indirectOneBlock, indirectTwoBlock, nil
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

	//inode gets modified
	indirectBlockOne, indirectBlockTwo, err := mapDataInode(superBlock, &inode, availableDataBlocks, dataBitmap)
	if err != nil {
		return 0, 0, err
	}

	err = saveIndirectData(destPtr, inode, indirectBlockOne, indirectBlockTwo)
	if err != nil {
		return 0, 0, err
	}

	err = saveInode(destPtr, int64(superBlock.InodeStartAddress+int32(binary.Size(inode))*(inode.NodeId-1)), inode)
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

func getInodeDataAddresses(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]int32, error) {
	addrs := make([]int32, 0)
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
		addrs = append(addrs, blockAddr)
		blocksRead++
	}

	//indirect level one
	if inode.Indirect[0] != 0 {
		indirectOneBlockData, err := readBlockInt32(destPtr, int64(inode.Indirect[0]), blockSize)
		if err != nil {
			return nil, err
		}

		for _, indirectOneBlockAddr := range indirectOneBlockData {
			if indirectOneBlockAddr == 0 {
				continue
			}

			if blocksRead == dataMaxBlocks {
				break
			}
			addrs = append(addrs, indirectOneBlockAddr)
			blocksRead++
		}
	}

	//indirect level two
	if inode.Indirect[1] != 0 {
		indirectTwoBlockDataFirst, err := readBlockInt32(destPtr, int64(inode.Indirect[1]), blockSize)
		if err != nil {
			return nil, err
		}
		SortInt32(indirectTwoBlockDataFirst) //sort because map was used and it doesnt guarantee sortion

		for _, addr := range indirectTwoBlockDataFirst {
			if addr == 0 {
				continue
			}
			indirectTwoBlockDataSecond, err := readBlockInt32(destPtr, int64(addr), blockSize)
			if err != nil {
				return nil, err
			}
			SortInt32(indirectTwoBlockDataSecond) //sort because map was used and it doesnt guarantee sortion
			for _, addr2 := range indirectTwoBlockDataSecond {
				if addr2 == 0 {
					continue
				}

				if blocksRead == dataMaxBlocks {
					break
				}
				addrs = append(addrs, addr2)
				blocksRead++
			}
		}
	}
	return addrs, nil
}

func ReadFileData(destPtr *os.File, inode PseudoInode, superblock Superblock) ([]byte, error) {
	addresses, err := getInodeDataAddresses(destPtr, inode, superblock)
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

func LoadInode(destPtr *os.File, inodeId int32, address int64) (PseudoInode, error) {
	inode := PseudoInode{}
	_, err2 := destPtr.Seek(address+int64(binary.Size(inode)*int(inodeId-1)), 0)

	err := binary.Read(destPtr, binary.LittleEndian, &inode)
	if err2 != nil || err != nil || inodeId == 0 {
		return PseudoInode{}, fmt.Errorf("could not read inode: %v", err)
	}
	return inode, nil
}

func saveInode(destPtr *os.File, address int64, inode PseudoInode) error {
	_, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &inode)
	if err2 != nil || err != nil {
		return fmt.Errorf("could not write inode: %v", err)
	}
	return nil
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

func AddDirItem(currentDirInodeId int32, dirItemNodeId int32, dirItemName string, destPtr *os.File, superBlock Superblock) {
	oldDataBuf := new(bytes.Buffer)
	newDataBuf := new(bytes.Buffer)
	currentDir := make([]DirectoryItem, superBlock.ClusterSize/int32(binary.Size(DirectoryItem{})))

	dirItem := DirectoryItem{}
	dirItem.Inode = dirItemNodeId
	copy(dirItem.ItemName[:], []byte(dirItemName))
	currentDirInode, err := LoadInode(destPtr, currentDirInodeId, int64(superBlock.InodeStartAddress))

	usedDataBlocks, err := getInodeDataAddresses(destPtr, currentDirInode, superBlock)
	data, err := ReadFileData(destPtr, currentDirInode, superBlock)

	oldDataBuf.Write(data)
	binary.Read(oldDataBuf, binary.LittleEndian, currentDir)
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
	saveDataBlocks(newDataBuf.Bytes(), destPtr, superBlock, usedDataBlocks)
	if err != nil {
		return
	}
}

func saveBitmap(destPtr *os.File, address int64, bitmap []uint8) error {
	_, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return fmt.Errorf("could not write bitmap: %v", err)
	}
	return nil
}

func loadBitmap(destPtr *os.File, startAddress int32, bitmapSize int32) ([]uint8, error) {
	bitmap := make([]uint8, bitmapSize)
	_, err2 := destPtr.Seek(int64(startAddress), 0)
	err := binary.Read(destPtr, binary.LittleEndian, &bitmap)
	if err2 != nil || err != nil {
		return nil, fmt.Errorf("could not write bitmap: %v", err)
	}
	return bitmap, nil
}

func GetAvailableDataBlocks(bitmap []uint8, startAddress int32, dataSize int32, blockSize int32) ([]int32, []uint8, error) {
	bitmap = append([]uint8(nil), bitmap...)
	blockAddressList := make([]int32, 0)
	allocatedSize := 0
	for i := 0; i < len(bitmap); i++ {
		for j := 0; j < 8; j++ {
			if allocatedSize >= int(dataSize) {
				for _, v := range blockAddressList {
					dataBit := (v - startAddress) / blockSize
					bitmap[dataBit/8] = setBit(bitmap[dataBit/8], uint8(dataBit%8), true)
				}
				return blockAddressList, bitmap, nil
			}
			blockAddress := startAddress + int32(i*8*int(blockSize)) + int32(j)*blockSize
			if getBit(bitmap[i], int32(j)) == ClusterIsFree {
				blockAddressList = append(blockAddressList, blockAddress)
				allocatedSize += int(blockSize)
			}
		}
	}
	return nil, nil, fmt.Errorf("not enough available data blocks")
}

func GetAvailableInodeAddress(bitmap []uint8, startAddress int32, inodeSize int32) (int32, error) {
	for i := 0; i < len(bitmap); i++ {
		for j := 0; j < 8; j++ {
			blockAddress := startAddress + int32(i*8*int(inodeSize)) + int32(j)*inodeSize
			if getBit(bitmap[i], int32(j)) == InodeIsFree {
				return blockAddress, nil
			}
		}
	}
	return -1, fmt.Errorf("no free inodes")
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
