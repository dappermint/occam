package proto

import "fmt"

// Audio commands, read straight off the commandDesc strings Synapse 4 logs
// next to every transfer. This is the whole surface the Windows app uses; ids
// not listed here were never observed and are not guessed at.
const (
	GetSerialNumber       byte = 0x00
	GetEditionID          byte = 0x01
	GetFirmwareVersion    byte = 0x02
	GetANCStatusAndLevel  byte = 0x12
	GetCustomerEQBand     byte = 0x15
	GetMicPresetEQIndex   byte = 0x16
	GetMicCustomerEQBand  byte = 0x17
	GetSidetoneVolume     byte = 0x19
	GetWirelessConnection byte = 0x20
	GetBatteryStatus      byte = 0x21
	GetChargingStatus     byte = 0x2A
	GetAutoPowerOffStatus byte = 0x2C
	GetMicStatus          byte = 0x55
	GetGameChatBalance    byte = 0x5C
	GetBTIncomingCall     byte = 0x5D
	GetHyperSpeedMode     byte = 0x5F
	GetEQOrderInfo        byte = 0x60
	GetVoicePromptStatus  byte = 0x65
	GetDongleLEDStatus    byte = 0x66
	GetRollerFunction     byte = 0x6A

	// Derived, not captured. Every set/get pair in the capture satisfies
	// set = get | 0x80 (0x15/0x95, 0x16/0x96, 0x19/0x99, 0x60/0xE0,
	// 0x61/0xE1, 0x66/0xE6), and these four follow the same rule. Verified by
	// writing each one its own current value and checking the device replies
	// SUCCESS rather than NOT_SUPPORTED.
	SetANCStatusAndLevel  byte = 0x92
	SetAutoPowerOffStatus byte = 0xAC
	SetMicStatus          byte = 0xD5
	SetHyperSpeedMode     byte = 0xDF

	// Confirmed by a later capture: Synapse sent 0xDC itself once the
	// game/chat slider was touched, matching what the rule predicted.
	SetGameChatBalance byte = 0xDC

	SetCustomerEQBand    byte = 0x95
	SetMicPresetEQIndex  byte = 0x96
	SetMicCustomerEQBand byte = 0x97
	SetSidetoneStatus    byte = 0x98
	SetSidetoneVolume    byte = 0x99
	SetSpeakerPresetEQ   byte = 0x9E
	SetEQOrderInfo       byte = 0xE0
	SetEQOrderUpdate     byte = 0xE1
	SetDongleLEDStatus   byte = 0xE6
	SetEQFootstepScaling byte = 0xEB
)

// Device mode does not travel in the audio message. It rides the outer command
// id field instead, 0x84 to read and 0x04 to write, with the audio command at
// zero. Kept separate so nobody sends it down the normal path by accident.
const (
	OuterGetDeviceMode byte = 0x84
	OuterSetDeviceMode byte = 0x04
)

var commandNames = map[byte]string{
	GetSerialNumber:       "getSerialNumber",
	GetEditionID:          "getEditionID",
	GetFirmwareVersion:    "getFirmwareVersion",
	GetANCStatusAndLevel:  "getANCStatusAndLevel",
	GetCustomerEQBand:     "getCustomerEQBand",
	GetMicPresetEQIndex:   "getMicPresetEQIndex",
	GetMicCustomerEQBand:  "getMicCustomerEQBand",
	GetSidetoneVolume:     "getSidetoneVolume",
	GetWirelessConnection: "getWirelessConnectionStatus",
	GetBatteryStatus:      "getBatteryStatus",
	GetChargingStatus:     "getChargingStatus",
	GetAutoPowerOffStatus: "getAutoPowerOffStatus",
	GetMicStatus:          "getMicStatus",
	GetGameChatBalance:    "getGameChatBalance",
	GetBTIncomingCall:     "getBTIncomingCallAction",
	GetHyperSpeedMode:     "getHyperSpeedMode",
	GetEQOrderInfo:        "getEQOrderInfo",
	GetVoicePromptStatus:  "getVoicePromptStatus",
	GetDongleLEDStatus:    "getDongleLEDStatus",
	GetRollerFunction:     "getRollerFunction",

	SetANCStatusAndLevel:  "setANCStatusAndLevel",
	SetAutoPowerOffStatus: "setAutoPowerOffStatus",
	SetMicStatus:          "setMicStatus",
	SetHyperSpeedMode:     "setHyperSpeedMode",
	SetGameChatBalance:    "setGameChatBalance",

	SetCustomerEQBand:    "setCustomerEQBand",
	SetMicPresetEQIndex:  "setMicPresetEQIndex",
	SetMicCustomerEQBand: "setMicCustomerEQBand",
	SetSidetoneStatus:    "setSidetoneStatus",
	SetSidetoneVolume:    "setSidetoneVolume",
	SetSpeakerPresetEQ:   "setSpeakerPresetEQStatus",
	SetEQOrderInfo:       "setEQOrderInfo",
	SetEQOrderUpdate:     "setEQOrderUpdateStartStop",
	SetDongleLEDStatus:   "setDongleLEDStatus",
	SetEQFootstepScaling: "setEQFootstepScalingBands",
}

// CommandName names a command for logs and console output.
func CommandName(c byte) string {
	if n, ok := commandNames[c]; ok {
		return n
	}
	return fmt.Sprintf("unknown(0x%02X)", c)
}

// Commands returns every known command id, for the console to enumerate.
func Commands() map[byte]string { return commandNames }
