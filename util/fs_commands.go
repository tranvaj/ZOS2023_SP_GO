package util

import (
    "fmt"
    "os"
    "encoding/binary"
	"math"
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

func Format(diskSize int, filename string) error {
    totalSize := diskSize
    superBlock := createSuperBlock(filename, diskSize, DefaultClusterSize)
	dataBitmap := CreateBitmap(int(superBlock.BitmapSize))
	dataBitmap[superBlock.BitmapSize-1] = bitSetTo(dataBitmap[superBlock.BitmapSize-1],0,true)
	dataBitmap[0] = bitSetTo(dataBitmap[0],0,true)
	inodeBitmap := CreateBitmap(int(superBlock.BitmapiSize))

	inodeBitmap[superBlock.BitmapiSize-1] = bitSetTo(inodeBitmap[superBlock.BitmapiSize-1],1,true)
	inodeBitmap[0] = bitSetTo(inodeBitmap[0],7,true)

    fp, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create file: %v", err)
    }
    defer fp.Close()

    err = binary.Write(fp, binary.LittleEndian, &superBlock)
    if err != nil {
        return fmt.Errorf("failed to write superblock: %v", err)
    }

	_, err2 := fp.Seek(int64(superBlock.BitmapStartAddress), 0)
	err = binary.Write(fp, binary.LittleEndian, &dataBitmap)
	
	if (err2 != nil || err != nil){
		return fmt.Errorf("could not create data bitmap: %v", err)
	}

	_, err2 = fp.Seek(int64(superBlock.BitmapiStartAddress), 0)
	err = binary.Write(fp, binary.LittleEndian, &inodeBitmap)
	
	if (err2 != nil || err != nil){
		return fmt.Errorf("could not create inode bitmap: %v", err)
	}

    _, err = fp.Seek(int64(totalSize-1), 0)
    if err != nil {
        return fmt.Errorf("failed to seek to end of file: %v", err)
    }

    _, err = fp.Write([]byte{0})
    if err != nil {
        return fmt.Errorf("failed to write to end of file: %v", err)
    }

    return nil
}

// bitSetTo sets the nth bit of the number to x.
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
// bitSetTo(8, 0, true) returns 9, because 8 is 1000 in binary and setting the 0th bit to 1 results in 1001, which is 9 in decimal.
func bitSetTo(number uint8, n uint8, x bool) uint8 {
    if x {
        return (number & ^(1 << n)) | (1 << n)
    }
    return (number & ^(1 << n)) | (0 << n)
}