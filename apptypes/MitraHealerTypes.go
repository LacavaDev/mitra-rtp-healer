package healertypes

import (
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type NaluInfo struct {
	Track           *webrtc.TrackLocalStaticRTP
	Pkt             *rtp.Packet
	Sps             []byte
	Pps             []byte
	LastSequence    *uint16
	IsIDR           bool
	StartBit        bool
	OriginalNalType byte
	FuHeader        byte
	EndBit          bool
}
