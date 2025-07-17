package helper

import (
	"errors"
	"fmt"
	"sort"

	healerTypes "github.com/LacavaDev/mitra-rtp-healer/apptypes"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func SortRTPPacketsBySequence(pkts []*rtp.Packet) []*rtp.Packet {
	sort.SliceStable(pkts, func(i, j int) bool {
		a := pkts[i].SequenceNumber
		b := pkts[j].SequenceNumber
		return isSeqLess(a, b)
	})
	return pkts
}
func isSeqLess(x, y uint16) bool {
	return (int16(x - y)) < 0
}

func NaluExceedsMTU(headerBytes []byte, payload []byte, expectedNaluSize int) (int, bool) {
	totalNaluLength := len(headerBytes) + len(payload)
	return totalNaluLength, totalNaluLength > expectedNaluSize
}

func ValidateFUASequence(nalus []*healerTypes.NaluInfo) error {
	if len(nalus) == 0 {
		return errors.New("empty FU-A sequence")
	}

	var (
		startCount      int
		endCount        int
		expectedNalType = nalus[0].OriginalNalType
		expectedTS      = nalus[0].Pkt.Timestamp
		expectedSSRC    = nalus[0].Pkt.SSRC
	)

	for i, n := range nalus {
		if n.OriginalNalType != expectedNalType {
			return fmt.Errorf("inconsistency: packet %d has NAL type %d (expected %d)", i, n.OriginalNalType, expectedNalType)
		}

		if n.Pkt.SSRC != expectedSSRC {
			return fmt.Errorf("inconsistency: packet %d has SSRC %d (expected %d)", i, n.Pkt.SSRC, expectedSSRC)
		}

		if n.Pkt.Timestamp != expectedTS {
			return fmt.Errorf("inconsistency: packet %d has timestamp %d (expected %d)", i, n.Pkt.Timestamp, expectedTS)
		}

		if n.StartBit {
			startCount++
		}
		if n.EndBit {
			endCount++
		}

		if !n.StartBit && !n.EndBit && len(nalus) > 2 && (i != 0 && i != len(nalus)-1) {
			continue
		}
	}

	if startCount != 1 {
		return fmt.Errorf("expected 1 packet with StartBit=true, but found %d", startCount)
	}

	if endCount != 1 {
		return fmt.Errorf("expected 1 packet with EndBit=true, but found %d", endCount)
	}

	return nil
}

func HasDifferentNalTypes(pkts []*rtp.Packet) (bool, map[uint8]int) {
	nalTypes := make(map[uint8]int)

	for _, pkt := range pkts {
		payload := pkt.Payload
		if len(payload) < 1 {
			continue
		}

		nalType := payload[0] & 0x1F

		if nalType == 28 && len(payload) >= 2 {
			nalType = payload[1] & 0x1F
		}

		nalTypes[nalType]++
	}

	if len(nalTypes) > 1 {
		return true, nalTypes
	}

	return false, nalTypes
}

func GetNALType(pkt *rtp.Packet) int {
	if len(pkt.Payload) == 0 {
		return -1
	}
	return int(pkt.Payload[0] & 0x1F)
}

func DebugLastPacketInfo(naluQueue []*rtp.Packet) string {
	if len(naluQueue) == 0 {
		return "Empty Buffer"
	}
	lastPkt := naluQueue[len(naluQueue)-1]
	nalType := GetNALType(lastPkt)
	ret := fmt.Sprintf("Last buffer rtp packet: Timestamp=%d, SequenceNumber=%d, NALType=%d\n",
		lastPkt.Timestamp, lastPkt.SequenceNumber, nalType)
	return ret
}

func RetrieveNaluInfo(pkt *rtp.Packet, sps []byte, pps []byte, lastSequence *uint16, track *webrtc.TrackLocalStaticRTP) healerTypes.NaluInfo {

	if isFUA := (pkt.Payload[0] & 0x1F) == 28; isFUA {
		fuHeader := pkt.Payload[1]
		startBit := (fuHeader & 0x80) != 0
		endBit := (fuHeader & 0x40) != 0
		originalNalType := fuHeader & 0x1F
		isIDR := originalNalType == 5

		allNaluInfo := healerTypes.NaluInfo{
			Pkt:             pkt,
			Track:           track,
			Pps:             pps,
			Sps:             sps,
			LastSequence:    lastSequence,
			StartBit:        startBit,
			EndBit:          endBit,
			IsIDR:           isIDR,
			FuHeader:        fuHeader,
			OriginalNalType: originalNalType,
		}

		return allNaluInfo
	} else {
		nalType := pkt.Payload[0] & 0x1F
		isIDR := nalType == 5

		allNaluInfo := healerTypes.NaluInfo{
			Pkt:             pkt,
			Track:           track,
			Pps:             pps,
			Sps:             sps,
			LastSequence:    lastSequence,
			StartBit:        false,
			EndBit:          false,
			IsIDR:           isIDR,
			FuHeader:        0,
			OriginalNalType: nalType,
		}
		return allNaluInfo
	}
}

func PrintRTPHeader(pkt *rtp.Packet) {
	h := pkt.Header

	fmt.Println("🔎 RTP HEADER DEBUG")
	fmt.Printf("Version:           %d\n", h.Version)
	fmt.Printf("Padding:           %v\n", h.Padding)
	fmt.Printf("Extension:         %v\n", h.Extension)
	fmt.Printf("Marker:            %v\n", h.Marker)
	fmt.Printf("PayloadType:       %d\n", h.PayloadType)
	fmt.Printf("SequenceNumber:    %d\n", h.SequenceNumber)
	fmt.Printf("Timestamp:         %d\n", h.Timestamp)
	fmt.Printf("SSRC:              %d\n", h.SSRC)

	if len(h.CSRC) > 0 {
		fmt.Printf("CSRCs:             %v\n", h.CSRC)
	} else {
		fmt.Println("CSRCs:             (nenhum)")
	}

	fmt.Println("====================================")
}

// DecoderReset clears RTP header fields that might confuse the decoder,
// specifically the Marker and Padding bits.
//
// !!! Use this function only when using the FU-A or Single NALU approaches !!!
// from the Mitra RTP Healer library, as these techniques require certain
// bits to be reset for compatibility with video decoders.
func DecoderReset(pkt *rtp.Packet) {
	pkt.Header.Marker = false
	pkt.Header.Padding = false
}
