package main

import (
	"encoding/base64"
	"fmt"
	"net"

	naluHelper "github.com/LacavaDev/mitra-rtp-healer/helper"
	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
)

/*
I created the Mitra RTP Healer library to assist developers—especially those new to WebRTC—who often face issues rendering video from RTSP sources onto web pages via WebRTC transmission.

One common challenge is that video streams coming from IP cameras or RTSP servers aren't always immediately compatible with WebRTC players. Issues such as missing SPS/PPS headers or improper NALU fragmentation often lead to a black screen or corrupted video in the browser. Mitra RTP Healer simplifies the process by transforming and fixing H.264 RTP packets on-the-fly, making them ready for WebRTC transmission.

Below is an example of how to use this library to pull a stream from an RTSP server, parse and repair the incoming H.264 packets, and stream them via UDP. This can be easily adapted to integrate with WebRTC solutions like Pion WebRTC:
*/

/*
	you can test the stream with the command: ffplay -protocol_whitelist "file,udp,rtp" -loglevel debug -i stream.sdp
*/

const maxNaluSize = 650                                  //EXAMPLE
const simulatedRTSPSample = "rtsp://0.0.0.0:8554/sample" //SIMULATED RTSP SERVER SOURCE

func main() {

	c := gortsplib.Client{}
	u, err := base.ParseURL(simulatedRTSPSample)

	if err != nil {
		fmt.Printf("[ERROR]: error at RTSP url setup %s \n", err)
	}
	c.Scheme = u.Scheme
	c.Host = u.Host

	err = c.Start2()

	if err != nil {
		fmt.Printf("[ERROR]: error at RTSP client start %s \n", err)
	}
	defer c.Close()

	desc, _, err := c.Describe(u)
	if err != nil {
		fmt.Printf("[ERROR]: error at RTSP client describe %s \n", err)
	}

	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		fmt.Printf("[ERROR]: error at RTSP media not found (is the device using H264 codec?) %s \n", err)
	}

	err = c.SetupAll(desc.BaseURL, desc.Medias)
	if err != nil {
		fmt.Printf("[ERROR]: error at RTSP media setup %s \n", err)
	}

	fmt.Printf("sprop-parameter-sets=%s,%s \n", base64.StdEncoding.EncodeToString(forma.SPS), base64.StdEncoding.EncodeToString(forma.PPS))

	var lastSequence uint16

	udpAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", "0.0.0.0", 5004))

	conn, _ := net.DialUDP("udp", nil, udpAddr)

	defer conn.Close()

	var collecting = false
	var naluQeue []*rtp.Packet = []*rtp.Packet{}
	naluChan := make(chan *rtp.Packet)

	var pps, sps []byte
	pps = forma.PPS
	sps = forma.SPS

	c.OnPacketRTPAny(func(medi *description.Media, f format.Format, pkt *rtp.Packet) {

		if medi.Type != description.MediaTypeVideo && medi.Type != description.MediaTypeAudio {
			return
		}

		payload := pkt.Payload
		if len(payload) < 1 {
			return
		}

		nalType := payload[0] & 0x1F

		//NOT LISTED IN RFC 6184, JUST DISCARDS
		if nalType > 29 {
			return
		}

		/*sprop-parameter-set setup logics*/
		if nalType == 7 {
			sps = pkt.Payload
			return
		}
		if nalType == 8 {
			pps = pkt.Payload
			return
		}

		//SETUP SDP FILE TO FFPLAY DEBUG
		naluHelper.SaveSPSPPSIfNotExists(pps, sps, "sps_pps.bin")

		bytesHeader, _ := pkt.Header.Marshal()
		_, exceeds := naluHelper.NaluExceedsMTU(bytesHeader, pkt.Payload, maxNaluSize)
		allNaluInfo := naluHelper.RetrieveNaluInfo(pkt, sps, pps, &lastSequence, nil /* <- this is an TrackLocalStaticRTP object for webrtc streaming purposes*/)

		if nalType == 28 {
			naluHelper.MakeFUAStreamApproach(exceeds, naluChan, &allNaluInfo, &naluQeue, &collecting, maxNaluSize)
			return
		} else if nalType > 0 {
			naluHelper.MakeSingleNaluStreamApproach(exceeds, naluChan, &allNaluInfo, &naluQeue, &collecting, maxNaluSize)
			return
		}
	})
	fmt.Println("RTP STREAM INITIATED, PLEASE RUN ffplay -protocol_whitelist \"file,udp,rtp\" -loglevel debug -i stream.sdp")
	go func() {
		for a := range naluChan {
			//Goroutine that is listening new RTP packets to streaming purposes
			//You can put your WebRTC logics here, example with pion webrtc: track.WriteRTP(a)
			a.SequenceNumber = lastSequence
			lastSequence++
			b, _ := a.Marshal()
			conn.Write(b)

		}
	}()

	_, err = c.Play(nil)
	if err != nil {
		fmt.Printf("[ERROR]: error at RTSP flow %s \n", err)
	}
	panic(c.Wait())
}
