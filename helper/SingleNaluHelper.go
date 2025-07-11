package helper

import (
	"fmt"

	healerTypes "github.com/LacavaDev/mitra-rtp-healer/apptypes"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

/*

				Single NALU SYNTAX - RFC 6184 5.6
   0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |F|NRI|  Type   |                                               |
    +-+-+-+-+-+-+-+-+                                               |
    |                                                               |
    |               Bytes 2..n of a single NAL unit                 |
    |                                                               |
    |                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                               :...OPTIONAL RTP padding        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


*/

/*
		FU-A NALU SYNTAX - RFC 6184 5.8
     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    | FU indicator  |   FU header   |                               |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
    |                                                               |
    |                         FU payload                            |
    |                                                               |
    |                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                               :...OPTIONAL RTP padding        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

	FU INDICATOR
	+---------------+
	|0|1|2|3|4|5|6|7|
	+-+-+-+-+-+-+-+-+
	|F|NRI|  FU-A Type (28)|
	+---------------+

	FU HEADER
    +---------------+
	|0|1|2|3|4|5|6|7|
	+-+-+-+-+-+-+-+-+
	|S|E|R| ORIGINAL Type|
	+---------------+

*/

func BuildSingleNaluFromFUAPackets(naluQeue []*healerTypes.NaluInfo, lastSequence *uint16) (*rtp.Packet, error) {
	firstNal := naluQeue[0]

	sampleHeaderNaluHeader := firstNal.Pkt.Payload[0]
	naluType := firstNal.OriginalNalType

	sampleHeaderNaluHeader = (sampleHeaderNaluHeader & 0b11100000) | (naluType & 0b00011111)

	concatenatePayload := []byte{}
	for _, nalu := range naluQeue {
		concatenatePayload = append(concatenatePayload, nalu.Pkt.Payload[2:]...)
	}

	singleNaluPayload := append([]byte{sampleHeaderNaluHeader}, concatenatePayload...)

	packet := &rtp.Packet{
		Header:  firstNal.Pkt.Header.Clone(),
		Payload: singleNaluPayload,
	}
	packet.SequenceNumber = *lastSequence

	return packet, nil
}

func RetrieveSingleNaluType(pkt *rtp.Packet, lastSequence *uint16, track *webrtc.TrackLocalStaticRTP) byte {
	naluHeader := pkt.Payload[0]
	originalNalType := naluHeader & 0x1F
	return originalNalType
}

func MakeSingleNaluStreamApproach(exceeds bool, naluChan chan *rtp.Packet, allNaluInfo *healerTypes.NaluInfo, naluQeue *[]*rtp.Packet, collecting *bool, maxNaluSize int) {
	if exceeds {

		pkts, _ := FragmentSingleNaluToFUAPackets(*allNaluInfo.Pkt, maxNaluSize, allNaluInfo.LastSequence)

		var infos2 []*healerTypes.NaluInfo
		for _, nalu := range pkts {
			info := RetrieveNaluInfo(nalu, allNaluInfo.Sps, allNaluInfo.Pps, &nalu.SequenceNumber, allNaluInfo.Track)
			infos2 = append(infos2, &info)
		}

		err := ValidateFUASequence(infos2)
		if err != nil {
			fmt.Println(err)
		}

		StapAVerification(*infos2[0], naluChan)

		for _, d := range pkts {
			naluChan <- d
		}
	} else {
		naluChan <- allNaluInfo.Pkt
	}
}
