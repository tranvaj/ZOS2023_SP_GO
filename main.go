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

	util.Format(int(size), "fat32")
	fp, _ := os.Open("fat32")
	defer fp.Close()
	superBlock := util.Superblock{}
	binary.Read(fp, binary.LittleEndian, &superBlock)
	fmt.Printf("%+v\n",superBlock)

	dataBitmap := make([]uint8, superBlock.BitmapSize)
	inodeBitmap := make([]uint8, superBlock.BitmapiSize)

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

	//bmp := util.CreateBitmap(8)
	//println(binary.Size(bmp))
	//fmt.Println(bmp)
}
