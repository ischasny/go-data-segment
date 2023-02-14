package datasegment

import (
	"github.com/filecoin-project/go-data-segment/fr32"
	"github.com/filecoin-project/go-data-segment/merkletree"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

// InclusionVerifierData is the information required for verification of the proof and is sourced
// from the client.
type InclusionVerifierData struct {
	// Piece Commitment to client's data
	CommPc cid.Cid
	// SizePc is size of client's data
	SizePc abi.PaddedPieceSize
}

// InclusionAuxData is required for verification of the proof and needs to be cross-checked with the chain state
type InclusionAuxData struct {
	// Piece Commitment to aggregator's deal
	CommPa cid.Cid
	// SizePa is padded size of aggregator's deal
	SizePa abi.PaddedPieceSize
}

// InclusionPoof is produced by the aggregator (or possibly by the SP)
type InclusionProof struct {
	// ProofSubtree is proof of inclusion of the client's data segment in the data aggregator's Merkle tree (includes position information)
	// I.e. a proof that the root node of the subtree containing all the nodes (leafs) of a data segment is contained in CommDA
	ProofSubtree merkletree.ProofData
	// ProofDS is a proof that an entry for the user's data is contained in the index of the aggregator's deal.
	// I.e. a proof that the data segment index constructed from the root of the user's data segment subtree is contained in the index of the deal tree.
	ProofIndex merkletree.ProofData
}

func (ip InclusionProof) ComputeExpectedAuxData(veriferData InclusionVerifierData) (*InclusionAuxData, error) {
	// Verification flow:
	//	1. Decode Client's Piece commitment
	//	2. Compute assumed aggregator's commitment based on the subtree inclusion proof
	//	3. Compute size of aggregator's deal and offset of Client's deal within the Aggreggator's deal.
	//	4. Create the DataSegmentIndexEntry based on Client's data and offset from 3
	//  5. Check if DataSegmentIndexEntry falls into the correct area. TODO
	//	5. Compute second assumed aggregator's commitment based on the data segment index entry inclusion proof.
	//	6. Compute second assumed aggregator's deal size.
	//	7. Compare deal sizes and commitments from steps 2+3 against steps 5+6. Fail if not equal.
	//	8. Return the computed values of aggregator's Commitment and Size as AuxData.

	// we can do this simpler than the library code
	commPc, err := commcid.CIDToPieceCommitmentV1(veriferData.CommPc)
	if err != nil {
		return nil, xerrors.Errorf("invalid piece commitment: %w", err)
	}
	nodeCommPc := (*merkletree.Node)(commPc)

	// Compute the Commitment to aggregator's data and assume it is correct
	// we will cross validate it against the other proof and then return it for futher validation
	assumedCommPa, err := ip.ProofSubtree.ComputeRoot(nodeCommPc)
	if err != nil {
		return nil, xerrors.Errorf("could not validate the subtree proof: %w", err)
	}

	// TODO: check overflow
	assumedSizePa := abi.PaddedPieceSize((1 << ip.ProofSubtree.Depth()) * veriferData.SizePc)
	// TODO: check overflow
	// inclusion proof verification checks that index is less than the 1<<(path length)
	dataOffset := ip.ProofSubtree.Index() * uint64(veriferData.SizePc)

	en, err := MakeDataSegmentIndexEntry((*fr32.Fr32)(nodeCommPc), dataOffset, uint64(veriferData.SizePc))
	if err != nil {
		return nil, xerrors.Errorf("createding data segment index entry: %w", err)
	}

	enNode := merkletree.TruncatedHash(en.SerializeFr32())

	assumedCommPa2, err := ip.ProofIndex.ComputeRoot(enNode)
	if *assumedCommPa2 != *assumedCommPa {
		return nil, xerrors.Errorf("aggregator's data commiements don't match: %x != %x", assumedCommPa, assumedCommPa2)
	}
	//TODO: check if the index entry falls into index area

	const BytesInDataSegmentIndexEntry = 2 * BytesInNode
	// check overflow
	assumedSizePa2 := abi.PaddedPieceSize((1 << ip.ProofIndex.Depth()) * BytesInDataSegmentIndexEntry)
	if assumedSizePa2 != assumedSizePa {
		return nil, xerrors.Errorf("aggregator's data size doesn't match")
	}

	cidPa, err := commcid.PieceCommitmentV1ToCID(assumedCommPa[:])
	if err != nil {
		return nil, xerrors.Errorf("converting raw commiement to CID: %w", err)
	}

	return &InclusionAuxData{
		CommPa: cidPa,
		SizePa: assumedSizePa,
	}, nil
}
