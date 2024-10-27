package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"vouncer/pkg/ari"
)

type Config struct {
	AstHost     string `env:"AST_HOST"`
	ServiceHost string `env:"SERVICE_HOST"`
	AppName     string `env:"APP_NAME" envDefault:"vouncer"`
	Credentials string `env:"CREDENTIALS"`
	Debug       bool   `env:"DEBUG" envDefault:"false"`
}

type Call struct {
	Channels map[string]*Channel
	Bridge   string
}

type Channel struct {
	Ringing bool
	Joined  bool
}

var channelIndex = map[string]*Call{}
var bridgeIndex = map[string]*Call{}

var client *ari.Client
var serviceUrl string

func Start(ctx context.Context, cfg Config) int {
	u := url.URL{
		Scheme: "ws",
		Host:   cfg.AstHost,
		Path:   "/ari/events",
	}
	params := url.Values{}
	params.Add("api_key", cfg.Credentials)
	params.Add("app", cfg.AppName)
	u.RawQuery = params.Encode()

	slog.Info(
		"Connecting to websocket",
		slog.String("host", u.Host),
		slog.String("path", u.Path),
		slog.String("app_name", cfg.AppName),
	)
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		slog.Error("Websocket connection failed", slog.String("reason", err.Error()))
		return 1
	}
	slog.Info("Connected successfully")

	client = ari.New("http", cfg.AstHost, cfg.AppName, cfg.Credentials)

	serviceUrl, err = url.JoinPath(cfg.ServiceHost, "/bouncer")
	if err != nil {
		slog.Error("Failed to create service URL", slog.String("reason", err.Error()))
		return 2
	}

	return serve(ctx, c, cfg.Debug)
}

func serve(ctx context.Context, conn *websocket.Conn, debug bool) int {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			slog.Error("Websocket read failed", slog.String("reason", err.Error()))
			return 1
		}

		if debug {
			var pretty bytes.Buffer
			err = json.Indent(&pretty, payload, "", "  ")
			if err == nil {
				slog.Debug(pretty.String())
			}
		}

		evt := ari.Event{}
		if err := json.Unmarshal(payload, &evt); err != nil {
			slog.Error("Websocket message processing failed", slog.String("reason", err.Error()))
		}

		switch evt.Type {
		case "StasisStart":
			handleStart(payload)
		case "PlaybackFinished":
			handlePlaybackFinished(payload)
		case "ChannelEnteredBridge":
			handleChannelEnteredBridge(payload)
		case "ChannelLeftBridge":
			handleChannelLeftBridge(payload)
		case "BridgeBlindTransfer":
			handleBlindTransfer(payload)
		case "BridgeDestroyed":
			handleBridgeDestroyed(payload)
		case "ChannelDestroyed":
			handleChannelDestroyed(payload)
		case "ChannelHangupRequest":
			fallthrough
		case "StasisEnd":
			handleEnd(payload)
		default:
			continue
		}
	}
}

type BouncerRequest struct {
	Endpoint  string `json:"endpoint"`
	Extension string `json:"extension"`
}

type BouncerResponse struct {
	Allow    bool   `json:"allow"`
	Endpoint string `json:"destination"`
	CallerID string `json:"callerid"`
}

func handleEnd(payload []byte) {
	var msg ari.StasisEnd
	if err := json.Unmarshal(payload, &msg); err != nil {
		slog.Error("Failed to unmarshal message", slog.String("reason", err.Error()))
		return
	}

	call, ok := channelIndex[msg.Chan.ID]
	if !ok {
		return
	}

	_, ok = call.Channels[msg.Chan.ID]
	if !ok {
		delete(channelIndex, msg.Chan.ID)
		return
	}

	if len(call.Channels) <= 2 {
		teardownCall(call)
	} else {
		delete(call.Channels, msg.Chan.ID)
		delete(channelIndex, msg.Chan.ID)
	}
}

func teardownCall(call *Call) {
	for chid, channel := range call.Channels {
		if !channel.Ringing {
			slog.Info("Deleting channel", "chid", chid)
			_ = client.ChannelDelete(chid)
			delete(channelIndex, chid)
		} else {
			channel.Ringing = false
			go func() {
				_ = client.ChannelAnswer(chid)
				time.Sleep(1 * time.Second)
				slog.Info("Playing vouncer_timeout", "chid", chid)
				_ = client.ChannelPlay(chid, "sound:/sounds/vouncer_timeout")
			}()
		}
	}
	_ = client.BridgeDelete(call.Bridge)
}

func joinChannels(call *Call) {
	if call.Bridge == "" {
		brid, err := client.BridgeCreate()
		if err != nil {
			slog.Error("Failed to create bridge", slog.String("reason", err.Error()))
			return
		}
		call.Bridge = brid
		bridgeIndex[brid] = call
	}

	var err error
	for chid, channel := range call.Channels {
		if channel.Joined {
			continue
		}

		_ = client.ChannelRing(chid, false)
		_ = client.ChannelAnswer(chid)
		err = errors.Join(client.BridgeAddChannel(call.Bridge, chid))
	}

	if err != nil {
		slog.Error("Failed to join channels. Tearing down resources", "reason", err)
		teardownCall(call)
	}
}

func dialFarEnd(msg ari.StasisStart) error {
	var endpoint string
	if msg.Chan.AccountCode != "" {
		endpoint = msg.Chan.AccountCode
	} else {
		endpoint = msg.Chan.Caller.Number
	}

	body := BouncerRequest{
		Endpoint:  endpoint,
		Extension: msg.Chan.Plan.Extension,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	res, err := http.Post(serviceUrl, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed connection to service: %w", err)
	}

	result := &BouncerResponse{}
	if err = json.NewDecoder(res.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode bouncer response: %w", err)
	}

	if !result.Allow {
		return ari.ErrCallNotAllowed
	}

	params := url.Values{}
	params.Set("callerId", result.CallerID)
	chanVars := map[string]string{
		"CDR_PROP(disable)": "1",
	}

	dst, err := client.ChannelDial(result.Endpoint, "vouncer", params, chanVars)
	if err != nil {
		return fmt.Errorf("")
	}

	err = client.ChannelSetVar(msg.Chan.ID, "CDR(userfield)", result.Endpoint)
	if err != nil {
		slog.Warn("Unable to set CDR(userfield)", slog.String("chid", msg.Chan.ID), slog.String("reason", err.Error()))
	}

	err = client.ChannelRing(msg.Chan.ID, true)
	if err != nil {
		slog.Warn("Unable to set channel ring", slog.String("chid", msg.Chan.ID), slog.String("reason", err.Error()))
	}

	newCall := &Call{
		Channels: map[string]*Channel{
			dst:         {},
			msg.Chan.ID: {Ringing: true},
		},
	}
	channelIndex[dst] = newCall
	channelIndex[msg.Chan.ID] = newCall

	return nil
}

func handleStart(payload []byte) {
	var msg ari.StasisStart
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", slog.String("reason", err.Error()))
		return
	}

	if msg.Chan.State == "Down" {
		return
	}
	if msg.Chan.Plan.Context == "transfer" && msg.Chan.Plan.Extension == "_attended" {
		return
	}

	call, ok := channelIndex[msg.Chan.ID]
	if ok {
		joinChannels(call)
		return
	}

	err = dialFarEnd(msg)
	if err == nil {
		return
	} else if errors.Is(err, ari.ErrCallNotAllowed) {
		call := Call{
			Channels: map[string]*Channel{
				msg.Chan.ID: {},
			},
			Bridge: "",
		}
		channelIndex[msg.Chan.ID] = &call

		go func() {
			_ = client.ChannelRing(msg.Chan.ID, true)
			time.Sleep(2 * time.Second)
			_ = client.ChannelRing(msg.Chan.ID, false)
			_ = client.ChannelAnswer(msg.Chan.ID)
			time.Sleep(1 * time.Second)
			_ = client.ChannelPlay(msg.Chan.ID, "sound:/sounds/vouncer_reject")
		}()
		return
	}

	slog.Error("Unable to dial far end", slog.String("reason", err.Error()))
	err = client.ChannelDelete(msg.Chan.ID)
	if err != nil {
		slog.Error("Failed to delete channel", slog.String("chid", msg.Chan.ID), slog.String("reason", err.Error()))
	}
}

func handleBridgeDestroyed(payload []byte) {
	var msg ari.BridgeDestroyed
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshall message", "reason", err)
	}

	call, ok := bridgeIndex[msg.Bridge.ID]
	if !ok {
		return
	}

	teardownCall(call)
	delete(bridgeIndex, msg.Bridge.ID)
}

func handleBlindTransfer(payload []byte) {
	var msg ari.BridgeBlindTransfer
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", "reason", err)
		return
	}

	slog.Info("Call transfer initiated", "src", msg.Channel.ID, "dst", msg.ReplaceChannel.ID)
	call, ok := channelIndex[msg.Channel.ID]
	if !ok {
		return
	}

	delete(call.Channels, msg.Channel.ID)
	delete(channelIndex, msg.Channel.ID)

	call.Channels[msg.ReplaceChannel.ID] = &Channel{Joined: true}
	channelIndex[msg.ReplaceChannel.ID] = call
}

func handleChannelEnteredBridge(payload []byte) {
	var msg ari.ChannelMemberBridge
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", "reason", err)
		return
	}

	call, ok := bridgeIndex[msg.Bridge.ID]
	if !ok {
		return
	}

	call.Channels[msg.Channel.ID] = &Channel{Joined: true}
	channelIndex[msg.Channel.ID] = call
}

func handleChannelLeftBridge(payload []byte) {
	var msg ari.ChannelMemberBridge
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", "reason", err)
		return
	}

	bridge, ok := bridgeIndex[msg.Bridge.ID]
	if !ok {
		return
	}

	delete(channelIndex, msg.Channel.ID)
	delete(bridge.Channels, msg.Channel.ID)
}

func handlePlaybackFinished(payload []byte) {
	var msg ari.PlaybackFinished
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", "reason", err)
	}

	channelId := strings.TrimPrefix(msg.Playback.TargetURI, "channel:")
	call, ok := channelIndex[channelId]
	if !ok {
		slog.Warn("Couldn't find channel", "event", "PlaybackFinished", "chid", channelId)
		return
	}

	if msg.Playback.MediaURI == "sound:/sounds/vouncer_reject" || msg.Playback.MediaURI == "sound:/sounds/vouncer_timeout" {
		go func() {
			time.Sleep(1 * time.Second)
			teardownCall(call)
		}()
	}
}

func handleChannelDestroyed(payload []byte) {
	var msg ari.ChannelDestroyed
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", "reason", err)
	}

	call, ok := channelIndex[msg.Channel.ID]
	if !ok {
		return
	}

	delete(call.Channels, msg.Channel.ID)
	delete(channelIndex, msg.Channel.ID)

	if len(call.Channels) != 1 {
		return
	}

	teardownCall(call)
}
