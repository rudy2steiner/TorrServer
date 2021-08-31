package dlna

import (
	"fmt"
	"strings"
)

type ContentFeatures struct {
	ProfileName     string
	SupportTimeSeek bool
	SupportRange    bool
	// Play speeds, DLNA.ORG_PS would go here if supported.
	Transcoded bool
}

func BinaryInt(b bool) uint {
	if b {
		return 1
	} else {
		return 0
	}
}

// flags are in hex. trailing 24 zeroes, 26 are after the space
// "DLNA.ORG_OP=" time-seek-range-supp bytes-range-header-supp
func (cf ContentFeatures) String() (ret string) {
	//DLNA.ORG_PN=[a-zA-Z0-9_]*
	params := make([]string, 0, 2)
	if cf.ProfileName != "" {
		params = append(params, "DLNA.ORG_PN="+cf.ProfileName)
	}
	params = append(params, fmt.Sprintf(
		"DLNA.ORG_OP=%b%b;DLNA.ORG_CI=%b",
		BinaryInt(cf.SupportTimeSeek),
		BinaryInt(cf.SupportRange),
		BinaryInt(cf.Transcoded)))
	return strings.Join(params, ";")
}
