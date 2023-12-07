package main

import (
	"bytes"
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
	fmt.Println(currentDir)
	//bmp := util.CreateBitmap(8)
	//println(binary.Size(bmp))
	//fmt.Println(bmp)
}
