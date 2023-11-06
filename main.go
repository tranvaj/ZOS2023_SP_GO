package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"tranvaj/ZOS2023_SP_GO/util"
)

func main() {
	arr, err := util.LoadCommand()
	if err != nil {
		log.Fatal(err)
		return
	} 
	fmt.Println(arr)

	size, err := util.ParseFormatString(arr[1])
	fmt.Println(size, err)

	superBlock, dataBitmap, inodeBitmap, _ := util.Format(int(size), "fat32")


	//test
	fp, err := os.OpenFile("fat32", os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(err)
		return
	}
	bytesWritten, err := util.WriteData("testfile.txt", fp, superBlock, inodeBitmap, dataBitmap, false)
	fmt.Println(bytesWritten, err)

	defer fp.Close()
	
	fp.Seek(0,0)
	superBlock = util.Superblock{}
	binary.Read(fp, binary.LittleEndian, &superBlock)
	fmt.Printf("%+v\n",superBlock)


	dataBitmap = make([]uint8, superBlock.BitmapSize)
	inodeBitmap = make([]uint8, superBlock.BitmapiSize)
	inode := util.PseudoInode{}

	fp.Seek(int64(superBlock.BitmapStartAddress),0)
	err = binary.Read(fp, binary.LittleEndian, &dataBitmap)	
	if err == io.EOF {
		fmt.Println(err)
	}
	fmt.Printf("%v\n", dataBitmap)

	fp.Seek(int64(superBlock.BitmapiStartAddress),0)
	err = binary.Read(fp, binary.LittleEndian, &inodeBitmap)	
	if err == io.EOF {
		fmt.Println(err)
	}
	fmt.Printf("%v\n", inodeBitmap)

	fp.Seek(int64(superBlock.InodeStartAddress),0)
	err = binary.Read(fp, binary.LittleEndian, &inode)	
	if err == io.EOF {
		fmt.Println(err)
	}

	fmt.Printf("%+v\n",inode)

	datablocks, _ := util.GetAvailableDataBlocks(dataBitmap, superBlock.DataStartAddress, 0, superBlock.ClusterSize)
	fmt.Println(datablocks)

	inodeFree, _ := util.GetAvailableInodeAddress(inodeBitmap, superBlock.InodeStartAddress, int32(binary.Size(util.PseudoInode{})))
	fmt.Println(inodeFree, binary.Size(util.PseudoInode{}))


	//bmp := util.CreateBitmap(8)
	//println(binary.Size(bmp))
	//fmt.Println(bmp)
}
