package helper

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	healerTypes "github.com/LacavaDev/mitra-rtp-healer/apptypes"
	"github.com/pion/rtp"
)

// generates an SDP file for tests purposes like playing incoming stream with ffplay and analysis H264 requirements
func SaveSPSPPSIfNotExists(sps, pps []byte, filename string) error {

	if _, err := os.Stat(filename); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return fmt.Errorf("error under sps pps file creation: %v", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo SPS/PPS: %v", err)
	}
	defer file.Close()

	if err := binary.Write(file, binary.BigEndian, uint16(len(sps))); err != nil {
		return err
	}
	if _, err := file.Write(sps); err != nil {
		return err
	}

	if err := binary.Write(file, binary.BigEndian, uint16(len(pps))); err != nil {
		return err
	}
	if _, err := file.Write(pps); err != nil {
		return err
	}

	spsBase64 := base64.StdEncoding.EncodeToString(sps)
	ppsBase64 := base64.StdEncoding.EncodeToString(pps)
	spropParam := fmt.Sprintf("packetization-mode=1; sprop-parameter-sets=%s,%s", spsBase64, ppsBase64)

	sdpContent := fmt.Sprintf(
		"v=0\n"+
			"o=- 0 0 IN IP4 127.0.0.1\n"+
			"s=No Name\n"+
			"c=IN IP4 127.0.0.1\n"+
			"t=0 0\n"+
			"m=video 5004 RTP/AVP 96\n"+
			"a=rtpmap:96 H264/90000\n"+
			"a=fmtp:96 %s\n",
		spropParam,
	)

	sdpFilename := filepath.Join(filepath.Dir(filename), "stream.sdp")
	err = os.WriteFile(sdpFilename, []byte(sdpContent), 0644)
	if err != nil {
		return fmt.Errorf("error under SDP file setup: %v", err)
	}

	return nil
}

// ps: its mandatory that the stream NALU packet mode must be 1
func GenSTAPPacket(sps []byte, pps []byte, header rtp.Header) (rtp.Packet, error) {

	if len(sps) == 0 || len(pps) == 0 {
		return rtp.Packet{}, fmt.Errorf("rtsp without sprop parameter set")
	}

	var buf bytes.Buffer

	buf.WriteByte(24)

	if err := binary.Write(&buf, binary.BigEndian, uint16(len(sps))); err != nil {
		panic("erro ao escrever tamanho do SPS: " + err.Error())
	}
	buf.Write(sps)

	if err := binary.Write(&buf, binary.BigEndian, uint16(len(pps))); err != nil {
		panic("erro ao escrever tamanho do PPS: " + err.Error())
	}
	buf.Write(pps)
	err := SaveSPSPPSIfNotExists(sps, pps, "sps_pps.bin")
	if err != nil {
		fmt.Println(err)
	}
	return rtp.Packet{
		Header:  header,
		Payload: buf.Bytes(),
	}, nil
}
func StapAVerification(params healerTypes.NaluInfo, nc chan *rtp.Packet) error {

	if params.StartBit && params.IsIDR {

		stapAData, err := GenSTAPPacket(params.Sps, params.Pps, rtp.Header{
			PayloadType:    96,
			Version:        2,
			SequenceNumber: *params.LastSequence,
			Timestamp:      params.Pkt.Timestamp,
			Marker:         false,
			SSRC:           params.Pkt.SSRC,
		})

		if err != nil {
			return fmt.Errorf("stream without sps/pps props")
		}

		nc <- &stapAData
		var arr [][]byte = [][]byte{}
		b, _ := stapAData.Marshal()
		arr = append(arr, b)
		sendRTPPackets("0.0.0.0", 5004, arr)

	}

	return nil
}
