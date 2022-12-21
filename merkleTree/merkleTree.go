package merkleTree

import (
	"crypto/sha256"
	"errors"
)

const digestBits = 254
const digestBytes = 32

type MerkleTree interface {
	// Returns the depth of the tree. A single-node tree has depth 1
	depth() int
}

type MerkleTreeData struct {
	// nodes start from root and go down left-to-right
	// thus len(nodes[0]) = 1, len(nodes[1]) = 2, etc.
	nodes [][]Node
	leafs int
}

type Node struct {
	data [digestBytes]byte
}

// depth returns the amount of levels in the tree, including the root level and leafs.
// I.e. a tree with 3 leafs will have one leaf level, a middle level and a root, and hence depth 3.
func (d MerkleTreeData) depth() int {
	return len(d.nodes)
}

func NewBareTree(elements int) MerkleTreeData {
	var tree MerkleTreeData
	tree.nodes = make([][]Node, 1+log2Ceil(elements))
	for i := 0; i <= log2Ceil(elements); i++ {
		tree.nodes[i] = make([]Node, 1<<i)
	}
	return tree
}

func GrowTree(leafData [][]byte) (MerkleTreeData, error) {
	var tree MerkleTreeData
	if leafData == nil || len(leafData) == 0 {
		return tree, errors.New("empty input")
	}
	tree = NewBareTree(len(leafData))
	leafLevel := hashList(leafData)
	// Set the leaf nodes
	tree.nodes[log2Ceil(len(leafData))] = leafLevel
	preLevel := leafLevel
	// Construct the Merkle tree bottom-up, starting from the leafs
	// Note the -1 due to 0-indexing the root level
	for level := log2Ceil(len(leafLevel)) - 1; level >= 0; level-- {
		currentLevel := make([]Node, halfCeil(len(preLevel)))
		// Traverse the level left to right
		for i := 0; i+1 < len(preLevel); i = i + 2 {
			currentLevel[i/2] = *computeNode(&preLevel[i], &preLevel[i+1])
		}
		// Handle the edge case where the tree is not complete, i.e. there is an odd number of leafs
		// This is done by hashing the content of the node and letting it be its own parent
		if len(preLevel)%2 == 1 {
			currentLevel[halfCeil(len(preLevel))-1] = *truncatedHash(preLevel[len(preLevel)-1].data[:])
		}
		tree.nodes[level] = currentLevel
		preLevel = currentLevel
	}
	return tree, nil
}

func computeNode(left *Node, right *Node) *Node {
	toHash := make([]byte, 2*digestBytes)
	copy(toHash, (*left).data[:])
	copy(toHash[digestBytes:], (*right).data[:])
	return truncatedHash(toHash)
}

func hashList(input [][]byte) []Node {
	digests := make([]Node, len(input))
	for i := 0; i < len(input); i++ {
		digests[i] = *truncatedHash(input[i])
	}
	return digests
}

func truncatedHash(data []byte) *Node {
	digst := sha256.Sum256(data)
	digst[(256/8)-1] &= 0b00111111
	node := Node{digst}
	return &node
}

// Compute ceil(x/2)
func halfCeil(x int) int {
	if x%2 == 0 {
		return x / 2
	} else {
		// Since the amount of levels is odd, we compute ceil(1+x/2)
		return 1 + x/2
	}
}

var tab64 = [6]uint64{
	0xFFFFFFFF00000000,
	0x00000000FFFF0000,
	0x000000000000FF00,
	0x00000000000000F0,
	0x000000000000000C,
	0x0000000000000002}

// Computes the integer logarithm with ceiling for up to 64 bit ints
// Translated from https://www.appsloveworld.com/c/100/6/compute-fast-log-base-2-ceiling
func log2Ceil(value int) int {
	var y int
	if (value & (value - 1)) == 0 {
		y = 0
	} else {
		y = 1
	}
	j := 32
	for i := 0; i < 6; i++ {
		var k int
		if (uint64(value) & tab64[i]) == 0 {
			k = 0
		} else {
			k = j
		}
		y += k
		value >>= k
		j >>= 1
	}

	return y
}
