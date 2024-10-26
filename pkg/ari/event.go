package ari

import "time"

const AsteriskLayout = "2006-01-02T15:04:05.000-0700"

type AsteriskTime time.Time

func (t *AsteriskTime) UnmarshalJSON(b []byte) (err error) {
	parsed, err := time.Parse(AsteriskLayout, string(b[1:len(b)-1]))
	*t = AsteriskTime(parsed)

	return err
}

type Event struct {
	Timestamp  AsteriskTime `json:"timestamp"`
	AsteriskID string       `json:"asterisk_id"`
	Type       string       `json:"type"`
}

type Member struct {
	Name   string `json:"name"`
	Number string `json:"number"`
}

type Channel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Caller      Member   `json:"caller"`
	Connected   Member   `json:"connected"`
	AccountCode string   `json:"accountcode"`
	Plan        Dialplan `json:"dialplan"`
}

type Dialplan struct {
	Context   string `json:"context"`
	Extension string `json:"exten"`
}

type StasisStart struct {
	Args []string `json:"args"`
	Chan Channel  `json:"channel"`
}

type StasisEnd struct {
	Chan Channel `json:"channel"`
}

type ChannelStateChange struct {
	Chan Channel `json:"channel"`
}

type Bridge struct {
	ID       string   `json:"id"`
	Channels []string `json:"channels"`
}

type BridgeBlindTransfer struct {
	Channel        Channel `json:"channel"`
	Extension      string  `json:"exten"`
	Result         string  `json:"result"`
	Transferee     Channel `json:"transferee"`
	ReplaceChannel Channel `json:"replace_channel"`
}

type ChannelMemberBridge struct {
	Bridge  Bridge  `json:"bridge"`
	Channel Channel `json:"channel"`
}

type BridgeDestroyed struct {
	Bridge Bridge `json:"bridge"`
}

type Playback struct {
	ID        string `json:"id"`
	MediaURI  string `json:"media_uri"`
	TargetURI string `json:"target_uri"`
	Language  string `json:"language"`
	State     string `json:"state"`
}

type PlaybackFinished struct {
	Playback Playback `json:"playback"`
}
