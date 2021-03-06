package eth

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/glog"
	ethTypes "github.com/livepeer/go-livepeer/eth/types"
	"github.com/livepeer/go-livepeer/ipfs"
	ffmpeg "github.com/livepeer/lpms/ffmpeg"
)

func TestShouldVerify(t *testing.T) {
	client := &StubClient{}
	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, client, &ipfs.StubIpfsApi{})

	blkHash := common.Hash([32]byte{0, 2, 4, 42, 2, 3, 4, 4, 4, 2, 21, 1, 1, 24, 134, 0, 02, 43})
	//Just make sure the results are different
	same := true
	var result bool
	for i := int64(0); i < 10; i++ {
		if tResult := cm.shouldVerifySegment(i, 0, 10, 100, blkHash, 5); result != tResult {
			same = false
			break
		} else {
			result = tResult
		}
	}
	if same {
		t.Errorf("Should give different results")
	}
}

func TestProfileOrder(t *testing.T) {
	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, &StubClient{}, &ipfs.StubIpfsApi{})

	if cm.profiles[0] != ffmpeg.P720p30fps4x3 || cm.profiles[1] != ffmpeg.P360p30fps4x3 || cm.profiles[2] != ffmpeg.P240p30fps16x9 {
		t.Errorf("wrong ordering: %v", cm.profiles)
	}
}

func TestAddReceipt(t *testing.T) {
	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, &StubClient{}, &ipfs.StubIpfsApi{})

	//Should get error for adding to a non-existing profile
	if err := cm.AddReceipt(0, []byte("data"), []byte("tdatahash"), []byte("sig"), ffmpeg.P144p30fps16x9); err == nil {
		t.Errorf("Expecting an error for adding to a non-existing profile.")
	}

	if err := cm.AddReceipt(0, []byte("data"), []byte("tdatahash"), []byte("sig"), ffmpeg.P240p30fps16x9); err != nil {
		t.Errorf("Error: %v", err)
	}

	if string(cm.segClaimMap[0].segData) != "data" {
		t.Errorf("Expecting %v, got %v", "data", string(cm.segClaimMap[0].segData))
	}

	if string(cm.segClaimMap[0].dataHash) != string(crypto.Keccak256([]byte("data"))) { //appended by ipfs.StubIpfsApi
		t.Errorf("Expecting %v, got %v", string(crypto.Keccak256([]byte("data"))), string(cm.segClaimMap[0].dataHash))
	}

	if string(cm.segClaimMap[0].tDataHashes[ffmpeg.P240p30fps16x9]) != "tdatahash" {
		t.Errorf("Expecting %v, got %v", "datahash", cm.segClaimMap[0].tDataHashes[ffmpeg.P240p30fps16x9])
	}

	if string(cm.segClaimMap[0].bSig) != "sig" {
		t.Errorf("Expecting %v, got %v", "sig", string(cm.segClaimMap[0].bSig))
	}
}

//We add 6 ranges (0, 3-13, 15-18, 20-25, 27, 29)
func setupRanges(t *testing.T) *BasicClaimManager {
	ethClient := &StubClient{ClaimStart: make([]*big.Int, 0), ClaimEnd: make([]*big.Int, 0), ClaimJid: make([]*big.Int, 0), ClaimRoot: make(map[[32]byte]bool)}
	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, ethClient, &ipfs.StubIpfsApi{})

	for _, segRange := range [][2]int64{[2]int64{0, 0}, [2]int64{3, 13}, [2]int64{15, 18}, [2]int64{21, 25}, [2]int64{27, 27}, [2]int64{29, 29}} {
		for i := segRange[0]; i <= segRange[1]; i++ {
			for _, p := range ps {
				data := []byte(fmt.Sprintf("data%v%v", p.Name, i))
				hash := []byte(fmt.Sprintf("hash%v%v", p.Name, i))
				sig := []byte(fmt.Sprintf("sig%v%v", p.Name, i))
				if err := cm.AddReceipt(int64(i), []byte(data), hash, []byte(sig), p); err != nil {
					t.Errorf("Error: %v", err)
				}
				if i == 16 {
					glog.Infof("data: %v", data)
				}
			}
		}
	}

	//Add invalid 19 out of order (invalid because it's only a single video profile)
	i := 19
	p := ffmpeg.P360p30fps4x3
	data := []byte(fmt.Sprintf("data%v%v", p.Name, i))
	hash := []byte(fmt.Sprintf("hash%v%v", p.Name, i))
	sig := []byte(fmt.Sprintf("sig%v%v", p.Name, i))
	if err := cm.AddReceipt(int64(i), []byte(data), hash, []byte(sig), p); err != nil {
		t.Errorf("Error: %v", err)
	}

	return cm
}

func TestRanges(t *testing.T) {
	//We added 6 ranges (0, 3-13, 15-18, 20-25, 27, 29)
	cm := setupRanges(t)
	ranges := cm.makeRanges()
	glog.Infof("ranges: %v", ranges)
	if len(ranges) != 6 {
		t.Errorf("Expecting 6 ranges, got %v", ranges)
	}
}

func TestClaimVerifyAndDistributeFees(t *testing.T) {
	ethClient := &StubClient{ClaimStart: make([]*big.Int, 0), ClaimEnd: make([]*big.Int, 0), ClaimJid: make([]*big.Int, 0), ClaimRoot: make(map[[32]byte]bool)}
	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, ethClient, &ipfs.StubIpfsApi{})

	//Add some receipts(0-9)
	receiptHashes1 := make([]common.Hash, 10)
	for i := 0; i < 10; i++ {
		segTDataHashes := make([][]byte, len(ps))
		data := []byte(fmt.Sprintf("data%v", i))
		sig := []byte(fmt.Sprintf("sig%v", i))
		for pi, p := range ps {
			tHash := []byte(fmt.Sprintf("tHash%v%v", ffmpeg.P240p30fps16x9.Name, i))
			if err := cm.AddReceipt(int64(i), data, tHash, []byte(sig), p); err != nil {
				t.Errorf("Error: %v", err)
			}
			segTDataHashes[pi] = tHash
		}
		receipt := &ethTypes.TranscodeReceipt{
			StreamID:                 "strmID",
			SegmentSequenceNumber:    big.NewInt(int64(i)),
			DataHash:                 crypto.Keccak256(data),
			ConcatTranscodedDataHash: crypto.Keccak256(segTDataHashes...),
			BroadcasterSig:           []byte(sig),
		}
		receiptHashes1[i] = receipt.Hash()
	}

	//Add some receipts(15-24)
	receiptHashes2 := make([]common.Hash, 10)
	for i := 15; i < 25; i++ {
		segTDataHashes := make([][]byte, len(ps))
		data := []byte(fmt.Sprintf("data%v", i))
		sig := []byte(fmt.Sprintf("sig%v", i))
		for pi, p := range ps {
			tHash := []byte(fmt.Sprintf("tHash%v%v", ffmpeg.P240p30fps16x9.Name, i))
			if err := cm.AddReceipt(int64(i), []byte(data), tHash, []byte(sig), p); err != nil {
				t.Errorf("Error: %v", err)
			}

			segTDataHashes[pi] = tHash
		}
		receipt := &ethTypes.TranscodeReceipt{
			StreamID:                 "strmID",
			SegmentSequenceNumber:    big.NewInt(int64(i)),
			DataHash:                 crypto.Keccak256(data),
			ConcatTranscodedDataHash: crypto.Keccak256(segTDataHashes...),
			BroadcasterSig:           []byte(sig),
		}
		receiptHashes2[i-15] = receipt.Hash()
	}

	err := cm.ClaimVerifyAndDistributeFees()
	if err != nil {
		t.Fatal(err)
	}

	//Make sure the roots are used for calling Claim
	root1, _, err := ethTypes.NewMerkleTree(receiptHashes1)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if _, ok := ethClient.ClaimRoot[[32]byte(root1.Hash)]; !ok {
		t.Errorf("Expecting claim to have root %v, but got %v", [32]byte(root1.Hash), ethClient.ClaimRoot)
	}

	root2, _, err := ethTypes.NewMerkleTree(receiptHashes2)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if _, ok := ethClient.ClaimRoot[[32]byte(root2.Hash)]; !ok {
		t.Errorf("Expecting claim to have root %v, but got %v", [32]byte(root2.Hash), ethClient.ClaimRoot)
	}
}

// func TestVerify(t *testing.T) {
// 	ethClient := &StubClient{VeriRate: 10}
// 	ps := []ffmpeg.VideoProfile{ffmpeg.P240p30fps16x9, ffmpeg.P360p30fps4x3, ffmpeg.P720p30fps4x3}
// 	cm := NewBasicClaimManager("strmID", big.NewInt(5), common.Address{}, big.NewInt(1), ps, ethClient, &ipfs.StubIpfsApi{})
// 	start := int64(0)
// 	end := int64(100)
// 	blkNum := int64(100)
// 	seqNum := int64(10)
// 	//Find a blkHash that will trigger verification
// 	var blkHash common.Hash
// 	for i := 0; i < 100; i++ {
// 		blkHash = common.HexToHash(fmt.Sprintf("0xDEADBEEF%v", i))
// 		if cm.shouldVerifySegment(seqNum, start, end, blkNum, blkHash, 10) {
// 			break
// 		}
// 	}

// 	//Add a seg that shouldn't trigger
// 	cm.AddReceipt(seqNum, []byte("data240"), []byte("tDataHash240"), []byte("bSig"), ffmpeg.P240p30fps16x9)
// 	cm.AddReceipt(seqNum, []byte("data360"), []byte("tDataHash360"), []byte("bSig"), ffmpeg.P360p30fps4x3)
// 	cm.AddReceipt(seqNum, []byte("data720"), []byte("tDataHash720"), []byte("bSig"), ffmpeg.P720p30fps4x3)
// 	seg, _ := cm.segClaimMap[seqNum]
// 	seg.claimStart = start
// 	seg.claimEnd = end
// 	seg.claimBlkNum = big.NewInt(blkNum)
// 	ethClient.BlockHashToReturn = common.Hash{}

// 	if err := cm.Verify(); err != nil {
// 		t.Errorf("Error: %v", err)
// 	}
// 	if ethClient.VerifyCounter != 0 {
// 		t.Errorf("Expect verify to NOT be triggered")
// 	}

// 	// //Add a seg that SHOULD trigger
// 	cm.AddReceipt(seqNum, []byte("data240"), []byte("tDataHash240"), []byte("bSig"), ffmpeg.P240p30fps16x9)
// 	cm.AddReceipt(seqNum, []byte("data360"), []byte("tDataHash360"), []byte("bSig"), ffmpeg.P360p30fps4x3)
// 	cm.AddReceipt(seqNum, []byte("data720"), []byte("tDataHash720"), []byte("bSig"), ffmpeg.P720p30fps4x3)
// 	seg, _ = cm.segClaimMap[seqNum]
// 	seg.claimStart = start
// 	seg.claimEnd = end
// 	seg.claimBlkNum = big.NewInt(blkNum)
// 	seg.claimProof = []byte("proof")
// 	ethClient.BlockHashToReturn = blkHash

// 	if err := cm.Verify(); err != nil {
// 		t.Errorf("Error: %v", err)
// 	}
// 	lpCommon.WaitUntil(100*time.Millisecond, func() bool {
// 		return ethClient.VerifyCounter != 0
// 	})
// 	if ethClient.VerifyCounter != 1 {
// 		t.Errorf("Expect verify to be triggered")
// 	}
// 	if string(ethClient.Proof) != string(seg.claimProof) {
// 		t.Errorf("Expect proof to be %v, got %v", seg.claimProof, ethClient.Proof)
// 	}
// }
