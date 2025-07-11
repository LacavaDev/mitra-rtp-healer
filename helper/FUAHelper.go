package helper

import (
	"encoding/base64"
	"fmt"
	"math"
	"net"
	"os"

	healerTypes "github.com/LacavaDev/mitra-rtp-healer/apptypes"
	"github.com/pion/rtp"
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

func SaveFuHeadersToFile(packets [][]byte, filename string, le int) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo: %w", err)
	}
	defer file.Close()
	totalLen := 0
	for _, b := range packets {
		totalLen += len(b)
	}
	for i, pkt := range packets {
		if len(pkt) < 2 {
			continue // pula pacotes inválidos
		}

		fuIndicator := pkt[0]
		fuHeader := pkt[1]

		_, err := fmt.Fprintf(
			file,
			"Pacote %d: FU Indicator = 0x%02X (%08b), FU Header = 0x%02X (%08b), Tamanho %d, Tamanho Original %d, tamanho nalu %d\n",
			i, fuIndicator, fuIndicator, fuHeader, fuHeader, len(pkt), totalLen, le,
		)
		if err != nil {
			return fmt.Errorf("erro ao escrever no arquivo: %w", err)
		}
	}

	return nil
}

func FragmentSingleNaluToFUAPackets(nalu rtp.Packet, expectedNaluSize int, lastSequence *uint16) ([]*rtp.Packet, error) {
	headerBytes, _ := nalu.Header.Marshal()

	totalNaluLength, _ := NaluExceedsMTU(headerBytes, nalu.Payload, expectedNaluSize)

	var calculateDecimalPart = func() int {
		newPacketsDecimalQtd := float64(totalNaluLength) / float64(expectedNaluSize)

		_, decimalPart := math.Modf(newPacketsDecimalQtd)

		if decimalPart != 0 {
			newPacketsDecimalQtd++
		}
		newPacketsQtd := int(newPacketsDecimalQtd)
		return newPacketsQtd
	}

	var buildFuAIndicatorFromSingleNaluHeader = func() byte {
		rawSingleNaluHeader := nalu.Payload[0]
		const FUANaluType = 28
		sampleHeaderNaluHeader := (rawSingleNaluHeader & 0b11100000) | (FUANaluType & 0b00011111)
		return sampleHeaderNaluHeader
	}

	var buildFuAHeader = func(packetType string) byte {
		result := byte(0)
		originalNalType := nalu.Payload[0] & 0x1F
		switch packetType {
		case "start":
			result |= 0x80
		case "end":
			result |= 0x40
		default:
			result &^= 0xC0
		}

		result = (result & 0b11100000) | (originalNalType & 0b00011111)

		return result
	}

	newPacketsQtd := calculateDecimalPart()

	var result []*rtp.Packet = []*rtp.Packet{}
	originalPayload := nalu.Payload[1:]

	if len(originalPayload) < expectedNaluSize {
		newPacketsQtd = 1
	}

	var lastBytePos int = 0
	offset := 0
	originalLength := len(originalPayload)
	expectedMaxLength := originalLength

	//precisaremos add 2 bytes de FUAI e FUAH para cada pacote
	expectedMaxLength += int(newPacketsQtd) * 2
	fuIndicator := buildFuAIndicatorFromSingleNaluHeader()

	for offset < int(newPacketsQtd) {
		var lastOffset = offset == int(newPacketsQtd)-1
		var firstOffset = offset == 0
		if lastBytePos > originalLength {
			lastOffset = true
			lastBytePos = originalLength - (lastBytePos - originalLength)
		}

		packetOrderType := ""

		if firstOffset {
			packetOrderType = "start"
		} else if lastOffset {
			packetOrderType = "end"
		} else {
			packetOrderType = "middle"
		}

		fuHeader := buildFuAHeader(packetOrderType)

		limit := expectedNaluSize * (offset + 1)
		if limit > len(originalPayload) {
			limit = len(originalPayload)
		}

		sub := []byte{fuIndicator, fuHeader}
		if lastOffset {
			sub = append(sub, originalPayload[lastBytePos:]...)
		} else {
			sub = append(sub, originalPayload[lastBytePos:limit]...)
		}

		newHeader := nalu.Header.Clone()
		if lastOffset {
			newHeader.Marker = true
		} else {
			newHeader.Marker = false
		}
		packet := rtp.Packet{
			Payload: sub,
			Header:  newHeader,
		}

		result = append(result, &packet)
		lastBytePos += expectedNaluSize
		offset++
	}

	return result, nil
}

func DebugNaluFUAInfo(nalu healerTypes.NaluInfo) {
	fmt.Println("╔══════════════════════╤════════════════════════════════╗")
	fmt.Println("║ Field                │ Value                          ║")
	fmt.Println("╟──────────────────────┼────────────────────────────────╢")

	fmt.Printf("║ Timestamp            │ %d    ║\n", nalu.Pkt.SSRC)
	fmt.Printf("║ Is IDR               │ %-30v ║\n", nalu.IsIDR)
	fmt.Printf("║ Start Bit            │ %-30v ║\n", nalu.StartBit)
	fmt.Printf("║ End Bit              │ %-30v ║\n", nalu.EndBit)
	fmt.Printf("║ Original NAL Type    │ %-30d ║\n", nalu.OriginalNalType)
	fmt.Printf("║ FU Header Byte       │ %-30d ║\n", nalu.FuHeader)
	fmt.Printf("║ Last Sequence Ptr    │ %-30d ║\n", nalu.LastSequence)

	fmt.Printf("╟──────────────────────┼────────────────────────────────╢\n")
	fmt.Printf("║ SPS (base64)         │ %-30s ║\n", base64.StdEncoding.EncodeToString(nalu.Sps))
	fmt.Printf("║ PPS (base64)         │ %-30s ║\n", base64.StdEncoding.EncodeToString(nalu.Pps))

	fmt.Println("╚══════════════════════╧════════════════════════════════╝")
}

func sendRTPPackets(addr string, port int, packets [][]byte) error {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, pkt := range packets {
		_, err := conn.Write(pkt)
		if err != nil {
			return err
		}
	}

	return nil
}

func MakeFUAStreamApproach(exceeds bool, naluChan chan *rtp.Packet, allNaluInfo *healerTypes.NaluInfo, naluQeue *[]*rtp.Packet, collecting *bool, maxNaluSize int) {

	if !exceeds && !*collecting {
		StapAVerification(*allNaluInfo, naluChan)
		naluChan <- allNaluInfo.Pkt
		return
	}

	if exceeds && allNaluInfo.StartBit {
		*collecting = true
		*naluQeue = append(*naluQeue, allNaluInfo.Pkt)
		return
	}

	if *collecting {
		*naluQeue = append(*naluQeue, allNaluInfo.Pkt)

		if allNaluInfo.EndBit {
			*collecting = false
			var infos []*healerTypes.NaluInfo

			for _, nalu := range *naluQeue {
				info := RetrieveNaluInfo(nalu, allNaluInfo.Sps, allNaluInfo.Pps, &nalu.SequenceNumber, allNaluInfo.Track)
				infos = append(infos, &info)
			}

			newPkt, _ := BuildSingleNaluFromFUAPackets(infos, allNaluInfo.LastSequence)

			*naluQeue = (*naluQeue)[:0]

			pkts, _ := FragmentSingleNaluToFUAPackets(*newPkt, maxNaluSize, allNaluInfo.LastSequence)

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
		}
	}

}
