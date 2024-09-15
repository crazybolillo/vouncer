package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"log/slog"
	"net/http"
	"net/url"
	"vouncer/pkg/ari"
)

type Config struct {
	AstHost     string `env:"AST_HOST"`
	ServiceHost string `env:"SERVICE_HOST"`
	AppName     string `env:"APP_NAME" envDefault:"vouncer"`
	Credentials string `env:"CREDENTIALS"`
}

type Call struct {
	From   string
	To     string
	Bridge string
}

var callStore = map[string]Call{}
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
		slog.Error("Websocket connection failed", slog.String("msg", err.Error()))
		return 1
	}
	slog.Info("Connected successfully")

	client = ari.New("http", cfg.AstHost, cfg.AppName, cfg.Credentials)

	serviceUrl, err = url.JoinPath(cfg.ServiceHost, "/bouncer")
	if err != nil {
		slog.Error("Failed to create service URL", slog.String("reason", err.Error()))
		return 2
	}

	return serve(ctx, c)
}

func serve(ctx context.Context, conn *websocket.Conn) int {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			slog.Error("Websocket read failed", slog.String("msg", err.Error()))
			return 1
		}

		evt := ari.Event{}
		if err := json.Unmarshal(payload, &evt); err != nil {
			slog.Error("Websocket message processing failed", slog.String("msg", err.Error()))
		}

		switch evt.Type {
		case "StasisStart":
			handleStart(payload)
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
		slog.Error("Failed to unmarshal message", slog.String("msg", err.Error()))
		return
	}

	call, ok := callStore[msg.Chan.ID]
	if !ok {
		return
	}
	teardownCall(call)
	delete(callStore, call.To)
	delete(callStore, call.From)
}

func teardownCall(call Call) {
	_ = client.ChannelDelete(call.To)
	_ = client.ChannelDelete(call.From)
	_ = client.BridgeDelete(call.Bridge)
}

func joinChannels(call Call) {
	slog.Info("Joining channels", slog.String("from", call.From), slog.String("to", call.To))

	brid, err := client.BridgeCreate()
	if err != nil {
		slog.Error("Failed to create bridge", slog.String("msg", err.Error()))
		return
	}
	client.ChannelRing(call.From, false)
	client.ChannelAnswer(call.From)

	call.Bridge = brid
	callStore[call.From] = call
	callStore[call.To] = call

	err = client.BridgeAddChannel(brid, call.From)
	err = errors.Join(err, client.BridgeAddChannel(brid, call.To))
	if err != nil {
		slog.Error("Failed to join channels. Tearing down resources", slog.String("msg", err.Error()))
		teardownCall(call)
	}
}

func handleStart(payload []byte) {
	var msg ari.StasisStart
	err := json.Unmarshal(payload, &msg)
	if err != nil {
		slog.Error("Failed to unmarshal message", slog.String("msg", err.Error()))
		return
	}

	call, ok := callStore[msg.Chan.ID]
	if ok {
		joinChannels(call)
		return
	}

	body := BouncerRequest{
		Endpoint:  msg.Chan.Caller.Number,
		Extension: msg.Chan.Plan.Extension,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		slog.Error("Failed to marshal body", slog.String("msg", err.Error()))
		return
	}

	res, err := http.Post(serviceUrl, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		slog.Error("Failed to post body", slog.String("msg", err.Error()))
		return
	}

	result := &BouncerResponse{}
	if err := json.NewDecoder(res.Body).Decode(result); err != nil {
		slog.Error("Failed to unmarshal body", slog.String("msg", err.Error()))
		return
	}

	params := url.Values{}
	params.Set("callerId", result.CallerID)
	chanVars := map[string]string{
		"CDR_PROP(disable)": "1",
	}
	dst, err := client.ChannelDial(result.Endpoint, "vouncer", params, chanVars)
	if err != nil {
		slog.Error("Failed to dial far end", slog.String("msg", err.Error()))
		return
	}
	client.ChannelSetVar(msg.Chan.ID, "CDR(userfield)", result.Endpoint)
	client.ChannelRing(msg.Chan.ID, true)

	newCall := Call{
		From: msg.Chan.ID,
		To:   dst,
	}
	callStore[dst] = newCall
	callStore[msg.Chan.ID] = newCall
}
