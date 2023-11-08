package util

import (
	"encoding/binary"
	"sort"
)

type Int32Slice []int32

func (p Int32Slice) Len() int           { return len(p) }
func (p Int32Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Int32Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func BytesToInt32(data []byte) []int32 {
    int32Data := make(Int32Slice, len(data)/4)
    for i := 0; i < len(data); i += 4 {
        int32Data[i/4] = int32(binary.LittleEndian.Uint32(data[i : i+4]))
    }
    sort.Sort(int32Data)
    return int32Data
}

func SortInt32(data []int32) {
    sort.Sort(Int32Slice(data))
}