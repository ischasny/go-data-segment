package merkleTree

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"reflect"
)

const digestBits = 254
const digestBytes = 32

type MerkleTree interface {
	// Depth returns the Depth of the tree. A single-node tree has Depth 1
	Depth() int
	// Leafs returns the amount of leafs in the Merkle tree
	Leafs() int
	// GetRoot returns the root node of the tree
	GetRoot() *Node
	// ConstructProof constructs a Merkle proof of the subtree (or leaf) at level lvl with index idx.
	// level 0 is the root and index 0 is the left-most node in a level.
	ConstructProof(lvl int, idx int) (MerkleProof, error)
	// ConstructBatchedProof constructs a batched Merkle proof of the nodes from and including leftLvl, leftIdx, to and including rightLvl, rightIdx.
	// That is, if leftLvl, or rightLvl, is not the leaf-level, then the proof is of the entire subtree from leftLvl at leftIdx to rightLvl at rightIdx
	// Level 0 is the root and index 0 is the left-most node in a level.
	ConstructBatchedProof(leftLvl int, leftIdx int, rightLvl int, rightIdx int) (BatchedMerkleProof, error)
	// ValidateFromLeafs checks that the Merkle tree is correctly constructed based on all the leaf data
	ValidateFromLeafs(leafData [][]byte) bool
	// Validate checks that the Merkle tree is correctly constructed, based on the internal nodes
	Validate() bool
}

type TreeData struct {
	// nodes start from root and go down left-to-right
	// thus len(nodes[0]) = 1, len(nodes[1]) = 2, etc.
	nodes [][]Node
}

type Node struct {
	data [digestBytes]byte
}

type MerkleProof interface {
	// ValidateLeaf ensures the correctness of the proof of a leaf against the root of a Merkle tree
	ValidateLeaf(leafs []byte, root *Node) bool
	// ValidateSubtree ensures the correctness of the proof of a subtree against the root of a Merkle tree
	ValidateSubtree(subtree *Node, root *Node) bool
}

type ProofData struct {
	path []Node
	// lvl indicates the level in the Merkle tree where root has level 0
	lvl int
	// idx indicates the index within the level where the element whose membership to prove is located
	// Leftmost node is index 0
	idx int
}

// Depth returns the amount of levels in the tree, including the root level and leafs.
// I.e. a tree with 3 leafs will have one leaf level, a middle level and a root, and hence Depth 3.
func (d TreeData) Depth() int {
	return len(d.nodes)
}

func (d TreeData) Leafs() int {
	return len(d.nodes[len(d.nodes)-1])
}

func (d TreeData) GetRoot() *Node {
	return &d.nodes[0][0]
}

func (d TreeData) ValidateFromLeafs(leafs [][]byte) bool {
	tree, err := GrowTree(leafs)
	if err != nil {
		log.Println("could not grow tree")
		return false
	}
	return reflect.DeepEqual(d.nodes, tree.nodes)
}

func (d TreeData) Validate() bool {
	tree := growTreeHashedLeafs(d.nodes[d.Depth()-1])
	return reflect.DeepEqual(d.nodes, tree.nodes)
}

func NewBareTree(elements int) TreeData {
	var tree TreeData
	tree.nodes = make([][]Node, 1+log2Ceil(elements))
	for i := 0; i <= log2Ceil(elements); i++ {
		tree.nodes[i] = make([]Node, 1<<i)
	}
	return tree
}

func GrowTree(leafData [][]byte) (TreeData, error) {
	var tree TreeData
	if leafData == nil || len(leafData) == 0 {
		return tree, errors.New("empty input")
	}
	leafLevel := hashList(leafData)
	return growTreeHashedLeafs(leafLevel), nil
}

func growTreeHashedLeafs(leafs []Node) TreeData {
	tree := NewBareTree(len(leafs))
	// Set the leaf nodes
	tree.nodes[log2Ceil(len(leafs))] = leafs
	preLevel := leafs
	// Construct the Merkle tree bottom-up, starting from the leafs
	// Note the -1 due to 0-indexing the root level
	for level := log2Ceil(len(leafs)) - 1; level >= 0; level-- {
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
	return tree
}

func (d TreeData) ConstructProof(lvl int, idx int) (ProofData, error) {
	if lvl < 1 || lvl >= d.Depth() {
		log.Println("level is either below 1 or bigger than the tree supports")
		return ProofData{}, errors.New("level is either below 1 or bigger than the tree supports")
	}
	if idx < 0 {
		log.Println(fmt.Sprintf("the requested index %d is negative", idx))
		return ProofData{}, errors.New(fmt.Sprintf("the requested index %d is negative", idx))
	}
	// The proof consists of appropriate siblings up to and including layer 1
	proof := make([]Node, lvl)
	currentIdx := idx
	// Compute the node we wish to prove membership of to the root
	for currentLvl := lvl; currentLvl >= 1; currentLvl-- {
		// For error handling check that no index impossibly large is requested
		if len(d.nodes[currentLvl]) <= currentIdx {
			log.Println(fmt.Sprintf("the requested index %d on level %d does not exist in the tree", currentIdx, currentLvl))
			return ProofData{}, errors.New(fmt.Sprintf("the requested index %d on level %d does not exist in the tree", currentIdx, currentLvl))
		}
		// Only try to store the sibling node when it exists,
		// if the tree is not complete this might not always be the case
		if len(d.nodes[currentLvl]) > getSiblingIdx(currentIdx) {
			proof[currentLvl-1] = d.nodes[currentLvl][getSiblingIdx(currentIdx)]
		}
		// Set next index to be the parent
		currentIdx = currentIdx / 2
	}
	return ProofData{path: proof, lvl: lvl, idx: idx}, nil
}

func (d TreeData) ConstructBatchedProof(leftLvl int, leftIdx int, rightLvl int, rightIdx int) (BatchedMerkleProof, error) {
	var factory BatchedProofFactory = CreateEmptyBatchedProof
	if leftLvl < 1 || leftLvl >= d.Depth() || rightLvl < 1 || rightLvl >= d.Depth() {
		log.Println("a level is either below 1 or bigger than the tree supports")
		return factory(), errors.New("a level is either below 1 or bigger than the tree supports")
	}
	if leftIdx < 0 || rightIdx < 0 {
		log.Println("a requested index is negative")
		return factory(), errors.New("a requested index is negative")
	}
	// Construct individual proofs
	leftProof, err := d.ConstructProof(leftLvl, leftIdx)
	if err != nil {
		return factory(), err
	}
	rightProof, err := d.ConstructProof(rightLvl, rightIdx)
	if err != nil {
		return factory(), err
	}
	// Find common index by starting from the top of the tree and see where the proof-path diverge
	maxLength := max(len(leftProof.path), len(rightProof.path))
	var ctr int
	for ctr = 0; ctr < maxLength; ctr++ {
		if leftProof.path[ctr] != rightProof.path[ctr] {
			break
		}
	}
	leftPath := leftProof.path[ctr:]
	rightPath := rightProof.path[ctr:]
	commonPath := rightProof.path[:ctr]
	return BatchedProofData{leftPath: leftPath, rightPath: rightPath, commonPath: commonPath, leftLvl: leftLvl, leftIdx: leftIdx, rightLvl: rightLvl, rightIdx: rightIdx}, nil
}

func (d ProofData) ValidateLeaf(data []byte, root *Node) bool {
	leaf := truncatedHash(data)
	return d.ValidateSubtree(leaf, root)
}

func (d ProofData) ValidateSubtree(subtree *Node, root *Node) bool {
	currentNode := subtree
	currentIdx := d.idx
	var parent *Node
	for currentLvl := d.lvl; currentLvl >= 1; currentLvl-- {
		sibIdx := getSiblingIdx(currentIdx)
		sibling := d.path[currentLvl-1]
		// If the node is all-0 then it means it does not exist
		// It is fine to assume this "magic" array since all nodes will be hash digests and so the all-0 string
		// will only happen with negligible probability
		if sibling.data == [digestBytes]byte{} {
			// In case the node does not exist, the only child will be hashed
			parent = truncatedHash(currentNode.data[:])
		} else {
			// If the sibling is "right" then we must hash currentNode first
			if sibIdx%2 == 1 {
				parent = computeNode(currentNode, &sibling)
			} else {
				parent = computeNode(&sibling, currentNode)
			}
		}
		currentNode = parent
		currentIdx = currentIdx / 2
	}
	// Validate the root against the tree
	if parent.data != root.data {
		return false
	}
	return true
}

// Returns the index of the sibling
func getSiblingIdx(idx int) int {
	if idx%2 == 0 {
		// If the index is even, then the node to the right should be returned
		return idx + 1
	} else {
		// Otherwise the node to the left should be returned
		return idx - 1
	}
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

func max(x int, y int) int {
	if x > y {
		return x
	}
	return y
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
