package util

const (
	DefaultClusterSize = 512
	IdItemFree         = 0
	BytesPerInode      = 2048
	ClusterIsFree      = 0
	InodeIsFree        = 0
	AddressByteLen     = 4
)

type Superblock struct {
	//byte represents char in GO
	Signature           [9]byte   // author's FS login
	VolumeDescriptor    [251]byte // description of the generated FS
	DiskSize            int64     // total VFS size
	ClusterSize         int32     // cluster size
	ClusterCount        int32     // number of clusters
	InodeCount          int32     // inode size is the size of struct pseudo_inode
	BitmapiStartAddress int32     // start address of the inode bitmap
	BitmapiSize         int32     // size of bitmap for inodes in bytes
	BitmapSize          int32     // size of bitmap for data in bytes
	BitmapStartAddress  int32     // start address of the data block bitmap
	InodeStartAddress   int32     // start address of the inodes
	DataStartAddress    int32     // start address of the data blocks
}

type PseudoInode struct {
	NodeId      int32     // ID of the inode, if ID = IdItemFree, the item is free
	IsDirectory bool      // file or directory
	References  int8      // number of references to the inode, used for hard links
	FileSize    int32     // file size in bytes
	Direct      [12]int32 // direct links to data blocks
	//Example: with a 512-byte block size, and 4-byte block pointers, each indirect block can consist of 128 (512 / 4) pointers.
	//as many pointers as opssible within 1 block (cluster)
	Indirect [3]int32 // indirect links (link - data blocks)
}

// SinglyIndirectBlock is a block containing pointers to data blocks
type SinglyIndirectBlock struct {
	Address  int32   //address of the block
	Pointers []int32 //array of pointers to data blocks
}

// DoublyIndirectBlock represents a block in the file system that contains an array of singly indirect blocks.
type DoublyIndirectBlock struct {
	Address  int32                 //address of the block
	Pointers []SinglyIndirectBlock //array of singly indirect blocks
}

type DirectoryItem struct {
	// Inode is the inode id corresponding to the file.
	Inode int32
	// ItemName is the name of the directory item.
	ItemName [12]byte
}
