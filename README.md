# mitra-rtp-healer

<p align="center">
  <img src="assets/pet.png" alt="Mitra RTP Healer" width="500"/>
</p>

<div style="text-align: justify;">

A Go library designed to correct and adapt RTP video streams from RTSP sources, ensuring compatibility with WebRTC pipelines and other real-time streaming applications.  
It currently focuses on **H.264** video and provides tools to handle RTP packet fragmentation, NALU reconstruction, and compatibility fixes for real-time playback.

</div>

---

## ⛑️ H264 Codec "Healing" Features 

<div style="text-align: justify;">

- 🔧 **Custom MTU adjustment:**  
  Automatically <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-5.6">reconstructs NALUs</a> and <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-5.8">re-fragments them into **FU-A**</a>  RTP packets <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-6.1">based on a configurable MTU size</a>.

- 🎯 **SPS and PPS injection:**  
  Periodically or <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-8.4">on-demand</a> injects **SPS** and **PPS** using <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-5.7.1">**STAP-A**</a>, ensuring fast decoding and rendering when new clients join an ongoing session.

- 🧰 **Modular utilities for NALU handling:**  
  Provides isolated and reusable functions to:
  - <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-5.6">Reconstruct fragmented NALUs</a> (FU-A → Full NALU)
  - <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-5.8">Fragment large NALUs</a> into FU-A for <a href="https://datatracker.ietf.org/doc/html/rfc6184#section-12.5">safe RTP transmission</a>

- 🐞 **Built-in debugging tools:**  
  Functions for logging, inspecting, and analyzing RTP and NALU structures for easier troubleshooting.

</div>

---

## 🎥 Supported H.264 NALU Types

- **FU-A** (Fragmentation Units - Type 28)  
- **Single NALU Packets** (Types 1–23)  
- **STAP-A** (Single-Time Aggregation Packet - Type 24), used for injecting SPS and PPS

---

## 📦 Codec Support

**Currently Supported:**

- ✅ H.264

**Planned Support:**

- ⏳ VP8  
- ⏳ VP9

---

## 📚 Use Cases

- Fixing compatibility issues between RTSP cameras and WebRTC playback
- Injecting critical decoder metadata (SPS/PPS) to ensure immediate video rendering
- Custom RTP pipeline development and testing
- Real-time NALU analysis and debugging
- Specialized Entire FU-A Stream Validation Methods 

---

## ✅ H264 Approach Example (with GoRTSPLib)

```bash
goRtspLibClient.OnPacketRTPAny(func(medi *description.Media, f format.Format, pkt *rtp.Packet) {

		if medi.Type != description.MediaTypeVideo && medi.Type != description.MediaTypeAudio {
			return
		}

		payload := pkt.Payload
		if len(payload) < 1 {
			return
		}

		nalType := payload[0] & 0x1F

		//NOT LISTED IN RFC 6184
		if nalType > 29 {
			return
		}

		if nalType == 7 {
			sps = pkt.Payload
			return
		}
		if nalType == 8 {
			pps = pkt.Payload
			return
		}
		helper.SaveSPSPPSIfNotExists(pps, sps, "sps_pps.bin")

		bytesHeader, _ := pkt.Header.Marshal()
		_, exceeds := helper.NaluExceedsMTU(bytesHeader, pkt.Payload, maxNaluSize)
		allNaluInfo := helper.RetrieveNaluInfo(pkt, sps, pps, &lastSequence, track)

		if nalType == 28 {
			helper.MakeFUAStreamApproach(exceeds, naluChan, &allNaluInfo, &naluQeue, &collecting, maxNaluSize)
			return
		} else if nalType > 0 {
			helper.MakeSingleNaluStreamApproach(exceeds, naluChan, &allNaluInfo, &naluQeue, &collecting, maxNaluSize)
			return
		}
	})

	go func() {
		for a := range naluChan {
			a.SequenceNumber = lastSequence
			track.WriteRTP(a)
			lastSequence++
			b, _ := a.Marshal()
			conn.Write(b)
		}
	}()

	_, err = c.Play(nil)
	if err != nil {
        fmt.PrintLn("[ERROR]: error at RTSP flow %s", err)
	} 
}
```
## 🛠️ Installation

```bash
go get github.com/LacavaDev/mitra-rtp-healer
