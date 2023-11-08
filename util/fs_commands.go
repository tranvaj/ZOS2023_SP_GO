package util

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

func CreateBitmap(bytes int) []uint8{
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
	
    superBlock.BitmapSize = int32(math.Ceil(float64(superBlock.ClusterCount) / 8.0))
    superBlock.BitmapiSize = int32(math.Ceil(float64(superBlock.InodeCount)/8.0))

    superBlock.BitmapStartAddress = int32(binary.Size(superBlock))
    superBlock.BitmapiStartAddress = superBlock.BitmapStartAddress + int32(superBlock.BitmapSize)
    superBlock.InodeStartAddress = superBlock.BitmapiStartAddress + int32(superBlock.BitmapiSize)
    superBlock.DataStartAddress = superBlock.InodeStartAddress + superBlock.InodeCount * int32(binary.Size(pseudoInode))
    return superBlock
}

func Format(diskSize int, fsName string) (Superblock, []uint8, []uint8, error) {
    totalSize := diskSize
    superBlock := createSuperBlock(fsName, diskSize, DefaultClusterSize)
	dataBitmap := CreateBitmap(int(superBlock.BitmapSize))
	dataBitmap[superBlock.BitmapSize-1] = setBit(dataBitmap[superBlock.BitmapSize-1],0,true)
	dataBitmap[0] = setBit(dataBitmap[0],0,true)
	inodeBitmap := CreateBitmap(int(superBlock.BitmapiSize))

	inodeBitmap[superBlock.BitmapiSize-1] = setBit(inodeBitmap[superBlock.BitmapiSize-1],0,true)
	inodeBitmap[0] = setBit(inodeBitmap[0],0,true)

    fp, err := os.Create(fsName)
    if err != nil {
        return Superblock{}, nil, nil, fmt.Errorf("failed to create file: %v", err)
    }
    defer fp.Close()

    err = binary.Write(fp, binary.LittleEndian, &superBlock)
    if err != nil {
        return Superblock{}, nil, nil, fmt.Errorf("failed to write superblock: %v", err)
    }

    err = writeBitmap(fp, int64(superBlock.BitmapStartAddress), dataBitmap)
    if (err != nil) {
        return Superblock{}, nil, nil, fmt.Errorf("failed to create data bitmap: %v", err)
    }

    err = writeBitmap(fp, int64(superBlock.BitmapiStartAddress), inodeBitmap)
    if (err != nil) {
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

    return superBlock, dataBitmap, inodeBitmap, nil
}



func WriteData(src string, destPtr *os.File, superBlock Superblock, inodeBitmap []uint8, dataBitmap []uint8, isDirectory bool) (int,error){
    bytesWritten := 0
    inode := PseudoInode{}
    data, err := os.ReadFile(src)
    if err != nil {
		return 0, err
	}

    availableDataBlocks, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(len(data)), superBlock.ClusterSize)
    if err != nil {
		return 0, err
	}

    availableInode, err := GetAvailableInodeAddress(inodeBitmap, superBlock.InodeStartAddress, int32(binary.Size(PseudoInode{})))
    if err != nil {
        return 0, err
    }

    for _, v := range availableDataBlocks{
        dataBit := (v - superBlock.DataStartAddress) / superBlock.ClusterSize 
        dataBitmap[dataBit / 8] = setBit(dataBitmap[dataBit / 8], uint8(dataBit % 8), true)
    }

    addrInOneBlock := superBlock.ClusterSize / AddressByteLen
    //indirectTwoAddrLen := addrInOneBlock * addrInOneBlock + addrInOneBlock

    directAddrLen := len(inode.Direct)
    //indirectOneBlocksLen := int(addrInOneBlock / addrInOneBlock)
    //indirectTwoBlocksLen := int(indirectTwoAddrLen / addrInOneBlock)

    indirectBlocksNeeded := int(math.Ceil(float64(len(availableDataBlocks) - directAddrLen) / float64(addrInOneBlock)))
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
    if (int(addrInOneBlock*addrInOneBlock + addrInOneBlock) + directAddrLen < len(availableDataBlocks)){
        return 0, fmt.Errorf("file is too big (not enough references available)")
    }

    //empty slice means no indirect blocks needed
    availableIndirectDataBlocks, err := GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, int32(indirectBlocksSize), superBlock.ClusterSize) 
    indirectOneBlock := make(map[int32][]int32) 
    indirectTwoBlock := make(map[int32]map[int32][]int32)
    if err != nil {
        return 0, err
    }
    if len(availableIndirectDataBlocks) != 0 {
        indirectOneBlock[availableIndirectDataBlocks[0]] = make([]int32, addrInOneBlock)
        end := int(math.Min(float64(directAddrLen + int(addrInOneBlock)), float64(len(availableDataBlocks))))
        copy(indirectOneBlock[availableIndirectDataBlocks[0]], availableDataBlocks[directAddrLen:end])
        inode.Indirect[0] = availableIndirectDataBlocks[0]

        //write indirect one
        for i, v := range indirectOneBlock[inode.Indirect[0]]{
            _, err2 := destPtr.Seek(int64(availableIndirectDataBlocks[0]) + int64(i) * int64(binary.Size(v)), 0)
            err = binary.Write(destPtr, binary.LittleEndian, v)
            if (err2 != nil || err != nil){
                return 0, fmt.Errorf("could not write into datablock: %v, %v", err, err2)
            }
        }

        if len(availableIndirectDataBlocks) > 1 {
            if start := end; start <= len(availableDataBlocks) {
                indirectBlocksNeededTwo := int(math.Ceil(float64(len(availableDataBlocks[start:])) / float64(addrInOneBlock))) 
                indirectTwoBlock[availableIndirectDataBlocks[1]] = make(map[int32][]int32)
                for i := 0; i < indirectBlocksNeededTwo; i++ {
                    //i+1 because 0 is reserved for indirect level one
                    if i == indirectBlocksNeededTwo - 1 {
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
    
            //write indirect two
            i := 0
            for k,v := range indirectTwoBlock[inode.Indirect[1]]{
                _, err2 := destPtr.Seek(int64(inode.Indirect[1]) + int64(i) * int64(binary.Size(k)), 0)
                err = binary.Write(destPtr, binary.LittleEndian, k)
                if (err2 != nil || err != nil){
                    return 0, fmt.Errorf("could not write into datablock: %v, %v", err, err2)
                }
    
                for y,x := range v{
                    _, err2 := destPtr.Seek(int64(k) + int64(y) * int64(binary.Size(x)), 0)
                    err = binary.Write(destPtr, binary.LittleEndian, x)
                    if (err2 != nil || err != nil){
                        return 0, fmt.Errorf("could not write into datablock: %v, %v", err, err2)
                    }
                } 
                i++
            }
        }
    } 

    for i, v := range availableDataBlocks {
        var writeData []byte
        if i == 0 {
            writeData = data[:]
        } else if i == len(availableDataBlocks)-1 {
            writeData = data[i*int(superBlock.ClusterSize):]
        } else {
            writeData = data[i*int(superBlock.ClusterSize):i*int(superBlock.ClusterSize) + int(superBlock.ClusterSize)]
        }

        _, err2 := destPtr.Seek(int64(v), 0)
        err = binary.Write(destPtr, binary.LittleEndian, writeData)
        bytesWritten += int(superBlock.ClusterSize)     

        if (err2 != nil || err != nil){
            return 0, fmt.Errorf("could not write into datablock: %v, %v", err, err2)
        }
    }

    inode.NodeId = 1 + ((availableInode - superBlock.InodeStartAddress) / int32(binary.Size(PseudoInode{}))) //plus 1 because 0 is reserved for free inodes
    inode.FileSize = int32(len(data))
    copy(inode.Direct[:], availableDataBlocks) //TODO indirect
    inode.IsDirectory = isDirectory

    err = writeInode(destPtr, int64(superBlock.InodeStartAddress), inode)
    if (err != nil){
        return -1, err
    }

    inodeBitmap[(inode.NodeId - 1) / 8] = setBit(inodeBitmap[(inode.NodeId - 1) / 8], uint8((inode.NodeId - 1) % 8), true)

    err = writeBitmap(destPtr, int64(superBlock.BitmapStartAddress), dataBitmap)
    if (err != nil) {
        return -1, err
    }

    err = writeBitmap(destPtr, int64(superBlock.BitmapiStartAddress), inodeBitmap)
    if (err != nil) {
        return -1, err
    }

    return bytesWritten, nil
}

func ReadFileData(destPtr *os.File, inode PseudoInode, blockSize int32) ([]byte, error) {
    var data []byte
    blocksRead := 0
    dataMaxBlocks := int(math.Ceil(float64(inode.FileSize) / float64(blockSize)))
    lastBlockDataLen := (int(inode.FileSize) - int(dataMaxBlocks-1) * int(blockSize))

    for _, blockAddr := range inode.Direct {
        if blockAddr == 0 {
            continue
        }

        if blocksRead == dataMaxBlocks - 1 {
            blockSize = int32(lastBlockDataLen)
        }
        blockData, err := readBlock(destPtr, int64(blockAddr), blockSize)
        if err != nil {
            return nil, err
        }
        blocksRead++
        data = append(data, blockData...)
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
    
            if blocksRead == dataMaxBlocks - 1 {
                blockSize = int32(lastBlockDataLen)
            }
            blockData, err := readBlock(destPtr, int64(indirectOneBlockAddr), blockSize)
            if err != nil {
                return nil, err
            }
            blocksRead++
            data = append(data, blockData...)
        }
    }
    
    //indirect level two
    if inode.Indirect[1] != 0 {
        indirectTwoBlockDataFirst, err := readBlockInt32(destPtr, int64(inode.Indirect[1]), blockSize)
        if err != nil {
            return nil, err
        }
        SortInt32(indirectTwoBlockDataFirst) //sort because map was used and it doesnt guarantee sortion

        for _, addr := range indirectTwoBlockDataFirst{
            if addr == 0 {
                continue
            }
            indirectTwoBlockDataSecond, err := readBlockInt32(destPtr, int64(addr), blockSize)
            if err != nil {
                return nil, err
            }
            SortInt32(indirectTwoBlockDataSecond) //sort because map was used and it doesnt guarantee sortion
            for _, addr2 := range indirectTwoBlockDataSecond{
                if addr2 == 0 {
                    continue
                }

                if blocksRead == dataMaxBlocks - 1 {
                    blockSize = int32(lastBlockDataLen)
                }
                blockData, err := readBlock(destPtr, int64(addr2), blockSize)
                if err != nil {
                    return nil, err
                }
                blocksRead++
                data = append(data, blockData...)
            }
        }
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

func writeInode(destPtr *os.File, address int64, inode PseudoInode) error{
    _, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &inode)
    if (err2 != nil || err != nil){
		return fmt.Errorf("could not write inode: %v", err)
	}
    return nil
}

func writeBitmap(destPtr *os.File, address int64, bitmap []uint8) error{
    _, err2 := destPtr.Seek(address, 0)
	err := binary.Write(destPtr, binary.LittleEndian, &bitmap)
    if (err2 != nil || err != nil){
		return fmt.Errorf("could not write bitmap: %v", err)
	}
    return nil
}

func GetAvailableDataBlocks(bitmap []uint8, startAddress int32, dataSize int32, blockSize int32) ([]int32, error){
    blockAddressList := make([]int32, 0)
    allocatedSize := 0
    for i := 0; i < len(bitmap); i++ {
        for j := 0; j < 8; j++ {
            if allocatedSize >= int(dataSize) {
                return blockAddressList, nil
            }
            blockAddress := startAddress + int32(i * 8 * int(blockSize)) + int32(j) * blockSize
            if getBit(bitmap[i], int32(j)) == ClusterIsFree {
                blockAddressList = append(blockAddressList, blockAddress)
                allocatedSize += int(blockSize)
            }
        }
    }
    return nil, fmt.Errorf("not enough available data blocks")
}

func GetAvailableInodeAddress (bitmap []uint8, startAddress int32, inodeSize int32) (int32, error){
    for i := 0; i < len(bitmap); i++ {
        for j := 0; j < 8; j++ {
            blockAddress := startAddress + int32(i * 8 * int(inodeSize)) + int32(j) * inodeSize
            if getBit(bitmap[i], int32(j)) == InodeIsFree {
                return blockAddress, nil
            }
        }
    }
    return -1, fmt.Errorf("no free inodes")
}

func getBit (num uint8, position int32) uint8 {
   return (num >> position) & 1;
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